package concurrency

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestLimiterRejectsWhenFull(t *testing.T) {
	limiter := NewLimiter(config.ConcurrencyLimitConfig{
		MaxInFlight: 1,
	})
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, 1)
	var calls int32

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	go func() {
		done <- serveConcurrencyRequest(handler)
	}()
	<-started

	second := serveConcurrencyRequest(handler)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("并发超限响应缺少 Retry-After")
	}
	if !strings.Contains(second.Body.String(), "当前并发请求过多") {
		t.Fatalf("响应体未说明并发超限: %s", second.Body.String())
	}

	close(release)
	first := <-done
	if first.Code != http.StatusNoContent {
		t.Fatalf("first status = %d, want %d", first.Code, http.StatusNoContent)
	}

	third := serveConcurrencyRequest(handler)
	if third.Code != http.StatusNoContent {
		t.Fatalf("third status = %d, want %d", third.Code, http.StatusNoContent)
	}
}

func serveConcurrencyRequest(handler http.Handler) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	handler.ServeHTTP(recorder, request)
	return recorder
}
