package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/observability"
	dto "github.com/prometheus/client_model/go"
)

func TestOpenAICompatibleStreamProxyRecordsFirstContentTokenMetric(t *testing.T) {
	metrics := observability.NewMetrics("gateway", nil)
	handler := newProxyTestHandlerWithMetrics(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("Content-Type", "text/event-stream")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body: io.NopCloser(strings.NewReader(
				`data: {"choices":[{"delta":{"role":"assistant"},"index":0}]}` + "\n\n" +
					`data: {"choices":[{"delta":{"content":"proxy stream"},"index":0}]}` + "\n\n" +
					`data: {"choices":[{"delta":{"content":"ignored for first token"},"index":0}]}` + "\n\n" +
					"data: [DONE]\n\n",
			)),
			Request: r,
		}, nil
	}), metrics)

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

	count, _ := firstTokenHistogramSample(t, metrics, "real-a")
	if count != 1 {
		t.Fatalf("首 token 统计次数 = %d, want 1", count)
	}
}

func TestOpenAICompatibleProxyRecordsRequestDurationAndSuccessRate(t *testing.T) {
	metrics := observability.NewMetrics("gateway", nil)
	statusCodes := []int{http.StatusOK, http.StatusInternalServerError}
	callIndex := 0
	handler := newProxyTestHandlerWithMetrics(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		statusCode := statusCodes[callIndex]
		callIndex++
		time.Sleep(time.Millisecond)

		header := make(http.Header)
		header.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: statusCode,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl-upstream"}`)),
			Request:    r,
		}, nil
	}), metrics)

	for range statusCodes {
		recorder := performChatCompletion(t, handler, "real-a", false)
		if recorder.Code != statusCodes[callIndex-1] {
			t.Fatalf("status = %d, want %d", recorder.Code, statusCodes[callIndex-1])
		}
	}

	successLabels := map[string]string{"backend": "real-a", "result": requestResultSuccess}
	failureLabels := map[string]string{"backend": "real-a", "result": requestResultFailure}

	if got := counterValue(t, metrics, "gateway_requests_total", successLabels); got != 1 {
		t.Fatalf("成功请求数 = %f, want 1", got)
	}
	if got := counterValue(t, metrics, "gateway_requests_total", failureLabels); got != 1 {
		t.Fatalf("失败请求数 = %f, want 1", got)
	}

	successDurationCount, successDurationSum := histogramSample(t, metrics, "gateway_request_duration_seconds", successLabels)
	if successDurationCount != 1 {
		t.Fatalf("成功请求总延迟统计次数 = %d, want 1", successDurationCount)
	}
	if successDurationSum <= 0 {
		t.Fatalf("成功请求总延迟样本和 = %f, want > 0", successDurationSum)
	}

	failureDurationCount, _ := histogramSample(t, metrics, "gateway_request_duration_seconds", failureLabels)
	if failureDurationCount != 1 {
		t.Fatalf("失败请求总延迟统计次数 = %d, want 1", failureDurationCount)
	}

	if got := gaugeValue(t, metrics, "gateway_request_success_rate", map[string]string{"backend": "real-a"}); got != 0.5 {
		t.Fatalf("请求成功率 = %f, want 0.5", got)
	}
}

func TestMockBackendRecordsRequestMetrics(t *testing.T) {
	metrics := observability.NewMetrics("gateway", nil)
	handler := newProxyTestHandlerWithMetrics(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("mock 后端不应请求真实上游")
		return nil, nil
	}), metrics)

	recorder := performChatCompletion(t, handler, "mock-b", false)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	labels := map[string]string{"backend": "mock-b", "result": requestResultSuccess}
	if got := counterValue(t, metrics, "gateway_requests_total", labels); got != 1 {
		t.Fatalf("mock 成功请求数 = %f, want 1", got)
	}
	durationCount, _ := histogramSample(t, metrics, "gateway_request_duration_seconds", labels)
	if durationCount != 1 {
		t.Fatalf("mock 总延迟统计次数 = %d, want 1", durationCount)
	}
	if got := gaugeValue(t, metrics, "gateway_request_success_rate", map[string]string{"backend": "mock-b"}); got != 1 {
		t.Fatalf("mock 请求成功率 = %f, want 1", got)
	}
}

func performChatCompletion(t *testing.T, handler *Handler, model string, stream bool) *httptest.ResponseRecorder {
	t.Helper()

	streamField := ""
	if stream {
		streamField = `"stream": true,`
	}
	body := strings.NewReader(`{
  "model": "` + model + `",
  ` + streamField + `
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	handler.ServeHTTP(recorder, request)
	return recorder
}

func counterValue(t *testing.T, metrics *observability.Metrics, name string, labels map[string]string) float64 {
	t.Helper()

	metric := findMetric(t, metrics, name, labels)
	if metric.GetCounter() == nil {
		t.Fatalf("%s 不是 counter", name)
	}
	return metric.GetCounter().GetValue()
}

func gaugeValue(t *testing.T, metrics *observability.Metrics, name string, labels map[string]string) float64 {
	t.Helper()

	metric := findMetric(t, metrics, name, labels)
	if metric.GetGauge() == nil {
		t.Fatalf("%s 不是 gauge", name)
	}
	return metric.GetGauge().GetValue()
}

func histogramSample(t *testing.T, metrics *observability.Metrics, name string, labels map[string]string) (uint64, float64) {
	t.Helper()

	metric := findMetric(t, metrics, name, labels)
	if metric.GetHistogram() == nil {
		t.Fatalf("%s 不是 histogram", name)
	}
	histogram := metric.GetHistogram()
	return histogram.GetSampleCount(), histogram.GetSampleSum()
}

func findMetric(t *testing.T, metrics *observability.Metrics, name string, labels map[string]string) *dto.Metric {
	t.Helper()

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather metrics error = %v", err)
	}

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricMatchesLabels(metric, labels) {
				return metric
			}
		}
	}

	t.Fatalf("未找到指标 %s labels=%v", name, labels)
	return nil
}

func metricMatchesLabels(metric *dto.Metric, labels map[string]string) bool {
	for name, want := range labels {
		if metricLabelValue(metric, name) != want {
			return false
		}
	}
	return true
}
