package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestHealthz(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("响应体未包含健康状态: %s", recorder.Body.String())
	}
}

func TestReadyzReflectsState(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("未启动时 readyz status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	gateway.ready.Store(true)
	recorder = httptest.NewRecorder()
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready 后 readyz status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestMetrics(t *testing.T) {
	gateway := newTestApp(t)
	gateway.ready.Store(true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "gateway_up 1") {
		t.Fatalf("metrics 未包含 gateway_up: %s", body)
	}
	if !strings.Contains(body, "gateway_ready 1") {
		t.Fatalf("metrics 未包含 gateway_ready: %s", body)
	}
}

func TestChatCompletionMockNonStream(t *testing.T) {
	gateway := newTestApp(t)

	body := strings.NewReader(`{
  "model": "mock-a",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	if !strings.Contains(responseBody, `"object":"chat.completion"`) {
		t.Fatalf("响应体未包含 chat.completion: %s", responseBody)
	}
	if !strings.Contains(responseBody, `mock-a 已收到请求：ping`) {
		t.Fatalf("响应体未包含 mock-a 回复: %s", responseBody)
	}
}

func TestChatCompletionMockStream(t *testing.T) {
	gateway := newTestApp(t)

	body := strings.NewReader(`{
  "model": "mock-b",
  "stream": true,
  "messages": [
    {"role": "user", "content": "stream ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}
	responseBody := recorder.Body.String()
	if !strings.Contains(responseBody, "data: ") {
		t.Fatalf("SSE 响应未包含 data 行: %s", responseBody)
	}
	if !strings.Contains(responseBody, `"object":"chat.completion.chunk"`) {
		t.Fatalf("SSE 响应未包含 chunk: %s", responseBody)
	}
	if !strings.Contains(responseBody, "data: [DONE]") {
		t.Fatalf("SSE 响应未包含 DONE: %s", responseBody)
	}
}

func TestChatCompletionUnknownModel(t *testing.T) {
	gateway := newTestApp(t)

	body := strings.NewReader(`{
  "model": "missing-model",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if !strings.Contains(recorder.Body.String(), `"error"`) {
		t.Fatalf("响应体未包含 OpenAI 风格错误: %s", recorder.Body.String())
	}
}

func TestListModels(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	for _, model := range []string{"mock-a", "mock-b"} {
		if !strings.Contains(body, model) {
			t.Fatalf("模型列表未包含 %q: %s", model, body)
		}
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()

	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	return newTestAppWithConfig(t, cfg)
}

func newTestAppWithConfig(t *testing.T, cfg config.Config) *App {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gateway, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return gateway
}
