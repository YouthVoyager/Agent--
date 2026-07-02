package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestOpenAICompatibleProxy(t *testing.T) {
	handler := newProxyTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "http://upstream.test/v1/chat/completions" {
			t.Errorf("上游 URL = %q, want http://upstream.test/v1/chat/completions", r.URL.String())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}

		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl-upstream","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"upstream ok"},"finish_reason":"stop"}]}`)),
			Request:    r,
		}, nil
	}))

	body := strings.NewReader(`{
  "model": "real-a",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "upstream ok") {
		t.Fatalf("响应体未透传上游响应: %s", recorder.Body.String())
	}
}

func TestOpenAICompatibleStreamProxy(t *testing.T) {
	handler := newProxyTestHandler(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "text/event-stream")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body: io.NopCloser(strings.NewReader(
				`data: {"object":"chat.completion.chunk","choices":[{"delta":{"content":"proxy stream"},"index":0}]}` + "\n\n" +
					"data: [DONE]\n\n",
			)),
			Request: r,
		}, nil
	}))

	body := strings.NewReader(`{
  "model": "real-a",
  "stream": true,
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	bodyText := recorder.Body.String()
	if !strings.Contains(bodyText, "proxy stream") || !strings.Contains(bodyText, "data: [DONE]") {
		t.Fatalf("SSE 响应未透传上游事件: %s", bodyText)
	}
}

func newProxyTestHandler(t *testing.T, transport http.RoundTripper) *Handler {
	t.Helper()

	return newProxyTestHandlerWithMetrics(t, transport, nil)
}

func newProxyTestHandlerWithMetrics(t *testing.T, transport http.RoundTripper, metrics *observability.Metrics) *Handler {
	t.Helper()

	handler, err := NewHandler(config.AIConfig{
		RequestTimeout: config.Duration{Duration: 1},
		Backends: []config.ModelBackendConfig{
			{
				Name:    "real-a",
				Type:    "openai",
				BaseURL: "http://upstream.test/v1",
				APIKey:  "test-key",
				Models:  []string{"real-a"},
			},
			{
				Name:   "mock-b",
				Type:   "mock",
				Models: []string{"mock-b"},
			},
		},
	}, nil, metrics)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	handler.client = &http.Client{Transport: transport}
	return handler
}
