package llm

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

func TestModelFallbackAfterNetworkError(t *testing.T) {
	metrics := observability.NewMetrics("gateway", nil)
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := requestBodyString(t, r)
		if strings.Contains(body, `"primary-model"`) {
			return nil, errors.New("dial failed")
		}
		if !strings.Contains(body, `"fallback-model"`) {
			t.Fatalf("降级请求未改写 model: %s", body)
		}
		return jsonResponse(http.StatusOK, `{"id":"chatcmpl-fallback","choices":[{"message":{"content":"fallback ok"}}]}`, r), nil
	}), metrics)

	recorder := performChatCompletion(t, handler, "primary-model", false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "fallback ok") {
		t.Fatalf("未返回备用模型响应: %s", recorder.Body.String())
	}

	if got := counterValue(t, metrics, "gateway_fallbacks_total", map[string]string{
		"from_model": "primary-model",
		"to_model":   "fallback-model",
		"reason":     failureReasonNetwork,
	}); got != 1 {
		t.Fatalf("fallback 计数 = %f, want 1", got)
	}
	if got := counterValue(t, metrics, "gateway_upstream_errors_total", map[string]string{
		"backend": "primary",
		"reason":  failureReasonNetwork,
	}); got != 1 {
		t.Fatalf("上游错误计数 = %f, want 1", got)
	}
}

func TestModelFallbackAfterRetryableStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "5xx", status: http.StatusInternalServerError},
		{name: "429", status: http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				body := requestBodyString(t, r)
				if strings.Contains(body, `"primary-model"`) {
					return jsonResponse(tt.status, `{"error":{"message":"upstream failed"}}`, r), nil
				}
				return jsonResponse(http.StatusOK, `{"id":"chatcmpl-fallback","choices":[{"message":{"content":"fallback ok"}}]}`, r), nil
			}), nil)

			recorder := performChatCompletion(t, handler, "primary-model", false)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "fallback ok") {
				t.Fatalf("未返回备用模型响应: %s", recorder.Body.String())
			}
		})
	}
}

func TestModelFallbackDoesNotHandleClientError(t *testing.T) {
	callCount := 0
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		return jsonResponse(http.StatusBadRequest, `{"error":{"message":"bad request"}}`, r), nil
	}), nil)

	recorder := performChatCompletion(t, handler, "primary-model", false)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if callCount != 1 {
		t.Fatalf("上游调用次数 = %d, want 1", callCount)
	}
}

func TestModelFallbackReturnsUnavailableWhenAllCandidatesFail(t *testing.T) {
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusInternalServerError, `{"error":{"message":"upstream failed"}}`, r), nil
	}), nil)

	recorder := performChatCompletion(t, handler, "primary-model", false)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"error"`) {
		t.Fatalf("响应体未包含 OpenAI 风格错误: %s", recorder.Body.String())
	}
}

func TestCircuitOpenSkipsPrimaryBackend(t *testing.T) {
	primaryCalls := 0
	fallbackCalls := 0
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := requestBodyString(t, r)
		if strings.Contains(body, `"primary-model"`) {
			primaryCalls++
			return jsonResponse(http.StatusInternalServerError, `{"error":{"message":"upstream failed"}}`, r), nil
		}
		fallbackCalls++
		return jsonResponse(http.StatusOK, `{"id":"chatcmpl-fallback","choices":[{"message":{"content":"fallback ok"}}]}`, r), nil
	}), nil)
	handler.circuitBreaker.cfg.FailureThreshold = 1

	first := performChatCompletion(t, handler, "primary-model", false)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}
	second := performChatCompletion(t, handler, "primary-model", false)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusOK, second.Body.String())
	}

	if primaryCalls != 1 {
		t.Fatalf("主后端调用次数 = %d, want 1", primaryCalls)
	}
	if fallbackCalls != 2 {
		t.Fatalf("备用后端调用次数 = %d, want 2", fallbackCalls)
	}
}

func TestCircuitBreakerAllowsHalfOpenAfterOpenTimeout(t *testing.T) {
	now := time.Unix(100, 0)
	breaker := newCircuitBreaker(config.CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		OpenTimeout:         config.Duration{Duration: time.Second},
		HalfOpenMaxRequests: 1,
	}, []string{"primary"}, nil)
	breaker.now = func() time.Time {
		return now
	}

	permit, ok := breaker.Allow("primary")
	if !ok {
		t.Fatal("首次请求应允许通过")
	}
	permit.Fail()

	if _, ok := breaker.Allow("primary"); ok {
		t.Fatal("熔断打开后不应立即允许请求")
	}

	now = now.Add(time.Second)
	halfOpenPermit, ok := breaker.Allow("primary")
	if !ok {
		t.Fatal("open_timeout 后应允许半开探测")
	}
	if _, ok := breaker.Allow("primary"); ok {
		t.Fatal("半开探测达到上限后不应允许更多请求")
	}
	halfOpenPermit.Succeed()

	if _, ok := breaker.Allow("primary"); !ok {
		t.Fatal("半开探测成功后应关闭熔断")
	}
}

func TestStreamFallbackBeforeFirstTokenTimeout(t *testing.T) {
	callCount := 0
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		body := requestBodyString(t, r)
		if strings.Contains(body, `"primary-model"`) {
			return streamResponse(http.StatusOK, newBlockingReadCloser(), r), nil
		}
		return streamResponse(http.StatusOK, io.NopCloser(strings.NewReader(
			`data: {"choices":[{"delta":{"content":"fallback stream"}}]}`+"\n\n"+
				"data: [DONE]\n\n",
		)), r), nil
	}), nil)
	handler.firstTokenTimeout = 10 * time.Millisecond

	recorder := performChatCompletion(t, handler, "primary-model", true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "fallback stream") {
		t.Fatalf("未返回备用模型流式响应: %s", recorder.Body.String())
	}
	if callCount != 2 {
		t.Fatalf("上游调用次数 = %d, want 2", callCount)
	}
}

func TestStreamDoesNotFallbackAfterResponseStarted(t *testing.T) {
	callCount := 0
	handler := newFallbackTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		callCount++
		body := requestBodyString(t, r)
		if strings.Contains(body, `"primary-model"`) {
			return streamResponse(http.StatusOK, &errorAfterDataReadCloser{
				data: []byte(`data: {"choices":[{"delta":{"content":"primary stream"}}]}` + "\n\n"),
			}, r), nil
		}
		return streamResponse(http.StatusOK, io.NopCloser(strings.NewReader(
			`data: {"choices":[{"delta":{"content":"fallback stream"}}]}`+"\n\n"+
				"data: [DONE]\n\n",
		)), r), nil
	}), nil)

	recorder := performChatCompletion(t, handler, "primary-model", true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "primary stream") {
		t.Fatalf("未返回主模型已写出的流式响应: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "fallback stream") {
		t.Fatalf("响应开始后不应降级: %s", recorder.Body.String())
	}
	if callCount != 1 {
		t.Fatalf("上游调用次数 = %d, want 1", callCount)
	}
}

func newFallbackTestHandler(t *testing.T, transport http.RoundTripper, metrics *observability.Metrics) *Handler {
	t.Helper()

	handler, err := NewHandler(config.AIConfig{
		RequestTimeout:    config.Duration{Duration: time.Second},
		FirstTokenTimeout: config.Duration{Duration: time.Second},
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:             true,
			FailureThreshold:    3,
			OpenTimeout:         config.Duration{Duration: time.Second},
			HalfOpenMaxRequests: 1,
		},
		Fallbacks: map[string][]string{
			"primary-model": {"fallback-model"},
		},
		Backends: []config.ModelBackendConfig{
			{
				Name:    "primary",
				Type:    "openai",
				BaseURL: "http://primary.test/v1",
				Models:  []string{"primary-model"},
			},
			{
				Name:    "fallback",
				Type:    "openai",
				BaseURL: "http://fallback.test/v1",
				Models:  []string{"fallback-model"},
			},
		},
	}, nil, metrics)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	handler.client = &http.Client{Transport: transport}
	return handler
}

func requestBodyString(t *testing.T, r *http.Request) string {
	t.Helper()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("读取请求体失败: %v", err)
	}
	return string(body)
}

func jsonResponse(status int, body string, request *http.Request) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func streamResponse(status int, body io.ReadCloser, request *http.Request) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "text/event-stream")
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       body,
		Request:    request,
	}
}

type blockingReadCloser struct {
	done chan struct{}
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{done: make(chan struct{})}
}

func (r *blockingReadCloser) Read(_ []byte) (int, error) {
	<-r.done
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	return nil
}

type errorAfterDataReadCloser struct {
	data   []byte
	offset int
}

func (r *errorAfterDataReadCloser) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func (r *errorAfterDataReadCloser) Close() error {
	return nil
}
