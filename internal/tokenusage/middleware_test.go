package tokenusage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/auth"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestMiddlewareRejectsAfterActualUsageExhaustsBudget(t *testing.T) {
	controller := NewController(testTokenUsageConfig(8), nil)
	handler := controller.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("读取下游请求体失败: %v", err)
		}
		if !strings.Contains(string(body), `"model":"mock-a"`) {
			t.Fatalf("下游请求体未被恢复: %s", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mock-a","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`)
	}))

	first := serveTokenUsageRequest(handler, "alice")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveTokenUsageRequest(handler, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("预算超限响应缺少 Retry-After")
	}
	if !strings.Contains(second.Body.String(), "token 预算不足") {
		t.Fatalf("预算超限响应未说明原因: %s", second.Body.String())
	}
}

func TestMiddlewarePreventsConcurrentBudgetBreakthrough(t *testing.T) {
	controller := NewController(testTokenUsageConfig(5), nil)
	entered := make(chan struct{})
	release := make(chan struct{})

	handler := controller.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mock-a","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`)
	}))

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- serveTokenUsageRequest(handler, "alice")
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("等待第一个请求进入下游超时")
	}

	second := serveTokenUsageRequest(handler, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}

	close(release)
	select {
	case first := <-firstDone:
		if first.Code != http.StatusOK {
			t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("等待第一个请求完成超时")
	}
}

func TestMiddlewareReleasesReservationOnFailure(t *testing.T) {
	controller := NewController(testTokenUsageConfig(5), nil)
	calls := 0
	handler := controller.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			writeTokenUsageError(w, http.StatusInternalServerError, "上游失败", "server_error")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mock-a","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}`)
	}))

	first := serveTokenUsageRequest(handler, "alice")
	if first.Code != http.StatusInternalServerError {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusInternalServerError)
	}

	second := serveTokenUsageRequest(handler, "alice")
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusOK, second.Body.String())
	}
}

func TestMiddlewareUsesAuthenticatedIdentity(t *testing.T) {
	controller := NewController(testTokenUsageConfig(8), nil)
	handler := controller.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"mock-a","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`)
	}))

	first := serveAuthenticatedTokenUsageRequest(handler, "alice", "bob")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d, body = %s", first.Code, http.StatusOK, first.Body.String())
	}

	second := serveAuthenticatedTokenUsageRequest(handler, "alice", "")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d, body = %s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
}

func TestMiddlewareRequiresIdentity(t *testing.T) {
	controller := NewController(testTokenUsageConfig(10), nil)
	calls := 0
	handler := controller.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := serveTokenUsageRequest(handler, "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if calls != 0 {
		t.Fatalf("缺失身份时仍调用下游，calls = %d", calls)
	}
}

func testTokenUsageConfig(budget int) config.TokenUsageConfig {
	return config.TokenUsageConfig{
		Enabled:                    true,
		IdentityHeader:             "X-User-ID",
		Window:                     config.Duration{Duration: time.Hour},
		DefaultBudgetTokens:        budget,
		DefaultMaxCompletionTokens: 0,
		UserBudgets:                map[string]int{},
	}
}

func serveTokenUsageRequest(handler http.Handler, userID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tokenUsageRequestBody()))
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func serveAuthenticatedTokenUsageRequest(handler http.Handler, userID string, headerUserID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tokenUsageRequestBody()))
	if headerUserID != "" {
		request.Header.Set("X-User-ID", headerUserID)
	}
	request = request.WithContext(auth.WithIdentity(request.Context(), auth.Identity{
		UserID: userID,
	}))
	handler.ServeHTTP(recorder, request)
	return recorder
}

func tokenUsageRequestBody() string {
	return `{"model":"mock-a","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`
}
