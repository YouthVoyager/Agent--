package concurrency

import (
	"net/http"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

// Limiter 限制同时处理中的请求数量。
type Limiter struct {
	slots chan struct{}
}

// NewLimiter 创建并发限制器。
func NewLimiter(cfg config.ConcurrencyLimitConfig) *Limiter {
	return &Limiter{
		slots: make(chan struct{}, cfg.MaxInFlight),
	}
}

// Middleware 返回全局并发限制 HTTP 中间件。
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.acquire() {
			w.Header().Set("Retry-After", "1")
			writeConcurrencyError(
				w,
				http.StatusTooManyRequests,
				"当前并发请求过多，请稍后重试",
				"rate_limit_error",
			)
			return
		}
		defer l.release()

		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) acquire() bool {
	select {
	case l.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *Limiter) release() {
	<-l.slots
}
