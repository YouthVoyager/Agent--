package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/auth"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestUserLimiterLimitsPerIdentity(t *testing.T) {
	limiter := NewUserLimiter(config.UserRateLimitConfig{
		IdentityHeader:    "X-User-ID",
		RequestsPerSecond: 0.01,
		Burst:             1,
	})

	calls := 0
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	first := serveLimitedRequest(handler, "alice")
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNoContent)
	}

	second := serveLimitedRequest(handler, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("超限响应缺少 Retry-After")
	}

	third := serveLimitedRequest(handler, "bob")
	if third.Code != http.StatusNoContent {
		t.Fatalf("third status = %d, want %d", third.Code, http.StatusNoContent)
	}
	if calls != 2 {
		t.Fatalf("下游调用次数 = %d, want 2", calls)
	}
}

func TestUserLimiterRejectsMissingIdentity(t *testing.T) {
	limiter := NewUserLimiter(config.UserRateLimitConfig{
		IdentityHeader:    "X-User-ID",
		RequestsPerSecond: 1,
		Burst:             1,
	})

	calls := 0
	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := serveLimitedRequest(handler, "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(recorder.Body.String(), "缺少用户标识请求头") {
		t.Fatalf("响应体未说明缺失用户标识: %s", recorder.Body.String())
	}
	if calls != 0 {
		t.Fatalf("缺失用户标识时仍调用下游，calls = %d", calls)
	}
}

func TestUserLimiterUsesAuthenticatedIdentity(t *testing.T) {
	limiter := NewUserLimiter(config.UserRateLimitConfig{
		IdentityHeader:    "X-User-ID",
		RequestsPerSecond: 0.01,
		Burst:             1,
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	first := serveLimitedAuthenticatedRequest(handler, "alice")
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNoContent)
	}

	second := serveLimitedAuthenticatedRequest(handler, "alice")
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
}

func serveLimitedRequest(handler http.Handler, userID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if userID != "" {
		request.Header.Set("X-User-ID", userID)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func serveLimitedAuthenticatedRequest(handler http.Handler, userID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request = request.WithContext(auth.WithIdentity(request.Context(), auth.Identity{
		UserID: userID,
	}))
	handler.ServeHTTP(recorder, request)
	return recorder
}
