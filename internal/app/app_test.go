package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestAdminPage(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "Agent 网关管理") {
		t.Fatalf("响应体未包含管理页面标题: %s", recorder.Body.String())
	}
}

func TestTracingAddsResponseHeaders(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	traceID := recorder.Header().Get("X-Trace-ID")
	if len(traceID) != 32 {
		t.Fatalf("X-Trace-ID = %q, want 32 位 trace id", traceID)
	}
	if got := recorder.Header().Get("X-Request-ID"); got != traceID {
		t.Fatalf("X-Request-ID = %q, want %q", got, traceID)
	}
	if got := recorder.Header().Get("Traceparent"); !strings.Contains(got, traceID) {
		t.Fatalf("Traceparent = %q, want 包含 %q", got, traceID)
	}
}

func TestTracingContinuesIncomingTraceParent(t *testing.T) {
	gateway := newTestApp(t)
	incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Traceparent", incoming)
	gateway.Handler().ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Trace-ID"); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-ID = %q", got)
	}
	if got := recorder.Header().Get("Traceparent"); got == incoming {
		t.Fatalf("响应 Traceparent 应使用网关当前 span，实际仍为入站值 %q", got)
	}
}

func TestTracingPropagatesToUpstream(t *testing.T) {
	var upstreamTraceParent string
	var upstreamTraceID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamTraceParent = r.Header.Get("Traceparent")
		upstreamTraceID = r.Header.Get("X-Trace-ID")

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-upstream","object":"chat.completion","created":1,"model":"real-a","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`)
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	cfg.AI.Backends = []config.ModelBackendConfig{
		{
			Name:    "real-a",
			Type:    "openai",
			BaseURL: upstream.URL,
			Models:  []string{"real-a"},
		},
		{
			Name:   "mock-b",
			Type:   "mock",
			Models: []string{"mock-b"},
		},
	}
	gateway := newTestAppWithConfig(t, cfg)

	body := strings.NewReader(`{
  "model": "real-a",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	request.Header.Set("Traceparent", incoming)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if upstreamTraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("上游 X-Trace-ID = %q", upstreamTraceID)
	}
	if !strings.Contains(upstreamTraceParent, upstreamTraceID) {
		t.Fatalf("上游 Traceparent = %q, want 包含 %q", upstreamTraceParent, upstreamTraceID)
	}
	if upstreamTraceParent == incoming {
		t.Fatalf("上游 Traceparent 应使用新的子 span，实际仍为入站值 %q", upstreamTraceParent)
	}
}

func TestTracingCanBeDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	cfg.Observability.Tracing.Enabled = false
	gateway := newTestAppWithConfig(t, cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Trace-ID"); got != "" {
		t.Fatalf("关闭追踪后 X-Trace-ID = %q, want empty", got)
	}
	if got := recorder.Header().Get("Traceparent"); got != "" {
		t.Fatalf("关闭追踪后 Traceparent = %q, want empty", got)
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

func TestUserRateLimitAppliesToChatCompletion(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	cfg.RateLimit.User.Enabled = true
	cfg.RateLimit.User.IdentityHeader = "X-User-ID"
	cfg.RateLimit.User.RequestsPerSecond = 0.01
	cfg.RateLimit.User.Burst = 1
	gateway := newTestAppWithConfig(t, cfg)

	first := serveChatCompletion(t, gateway, "alice")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveChatCompletion(t, gateway, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("聊天补全超限响应缺少 Retry-After")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestConcurrencyLimitAppliesToChatCompletion(t *testing.T) {
	upstreamStarted := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() {
			close(upstreamStarted)
		})
		<-releaseUpstream

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"blocked-model","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`)
	}))
	defer upstream.Close()
	defer releaseOnce.Do(func() {
		close(releaseUpstream)
	})

	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	cfg.RateLimit.Concurrency.Enabled = true
	cfg.RateLimit.Concurrency.MaxInFlight = 1
	cfg.AI.Backends = []config.ModelBackendConfig{
		{
			Name:    "blocked",
			Type:    "openai",
			BaseURL: upstream.URL,
			Models:  []string{"blocked-model"},
		},
		{
			Name:   "mock-b",
			Type:   "mock",
			Models: []string{"mock-b"},
		},
	}
	gateway := newTestAppWithConfig(t, cfg)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- serveChatCompletionWithModel(gateway, "blocked-model", "")
	}()

	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		t.Fatal("等待阻塞上游请求超时")
	}

	second := serveChatCompletionWithModel(gateway, "blocked-model", "")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("聊天补全并发超限响应缺少 Retry-After")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d", recorder.Code, http.StatusOK)
	}

	releaseOnce.Do(func() {
		close(releaseUpstream)
	})
	select {
	case first := <-firstDone:
		if first.Code != http.StatusOK {
			t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("等待第一个聊天请求完成超时")
	}
}

func TestTokenUsageBudgetAppliesToChatCompletion(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	cfg.TokenUsage.Enabled = true
	cfg.TokenUsage.IdentityHeader = "X-User-ID"
	cfg.TokenUsage.Window = config.Duration{Duration: time.Hour}
	cfg.TokenUsage.DefaultBudgetTokens = 10
	cfg.TokenUsage.DefaultMaxCompletionTokens = 1
	gateway := newTestAppWithConfig(t, cfg)

	first := serveChatCompletion(t, gateway, "alice")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveChatCompletion(t, gateway, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("token 预算超限响应缺少 Retry-After")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestTokenUsageBudgetUsesAPIKeyIdentity(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	enableTestAPIKeyAuth(&cfg)
	cfg.TokenUsage.Enabled = true
	cfg.TokenUsage.IdentityHeader = "X-User-ID"
	cfg.TokenUsage.Window = config.Duration{Duration: time.Hour}
	cfg.TokenUsage.DefaultBudgetTokens = 10
	cfg.TokenUsage.DefaultMaxCompletionTokens = 1
	gateway := newTestAppWithConfig(t, cfg)

	first := serveAuthenticatedChatCompletion(t, gateway)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveAuthenticatedChatCompletion(t, gateway)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
}

func TestAPIKeyAuthProtectsV1Routes(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	enableTestAPIKeyAuth(&cfg)
	gateway := newTestAppWithConfig(t, cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("models status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAPIKeyAuthAllowsV1Routes(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	enableTestAPIKeyAuth(&cfg)
	gateway := newTestAppWithConfig(t, cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set("Authorization", "Bearer test-key")
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	recorder = serveAuthenticatedChatCompletion(t, gateway)
	if recorder.Code != http.StatusOK {
		t.Fatalf("chat status = %d, want %d, body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestUserRateLimitUsesAPIKeyIdentity(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	enableTestAPIKeyAuth(&cfg)
	cfg.RateLimit.User.Enabled = true
	cfg.RateLimit.User.IdentityHeader = "X-User-ID"
	cfg.RateLimit.User.RequestsPerSecond = 0.01
	cfg.RateLimit.User.Burst = 1
	gateway := newTestAppWithConfig(t, cfg)

	first := serveAuthenticatedChatCompletion(t, gateway)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveAuthenticatedChatCompletion(t, gateway)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
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

func serveChatCompletion(t *testing.T, gateway *App, userID string) *httptest.ResponseRecorder {
	t.Helper()

	return serveChatCompletionWithModel(gateway, "mock-a", userID)
}

func serveChatCompletionWithModel(gateway *App, model string, userID string) *httptest.ResponseRecorder {
	body := strings.NewReader(`{
  "model": "` + model + `",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	gateway.Handler().ServeHTTP(recorder, request)
	return recorder
}

func serveAuthenticatedChatCompletion(t *testing.T, gateway *App) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{
  "model": "mock-a",
  "messages": [
    {"role": "user", "content": "ping"}
  ]
}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	request.Header.Set("Authorization", "Bearer test-key")
	gateway.Handler().ServeHTTP(recorder, request)
	return recorder
}

func enableTestAPIKeyAuth(cfg *config.Config) {
	cfg.Auth.APIKey.Enabled = true
	cfg.Auth.APIKey.Keys = []config.APIKeyCredentialConfig{
		{
			ID:       "test-key",
			KeyHash:  "sha256:62af8704764faf8ea82fc61ce9c4c3908b6cb97d463a634e9e587d7c885db0ef",
			UserID:   "alice",
			TenantID: "tenant-a",
			Scopes:   []string{"chat:completions", "models:read"},
		},
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
