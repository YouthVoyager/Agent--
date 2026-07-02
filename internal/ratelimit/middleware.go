package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/agent-gateway/telemetry-gateway/internal/auth"
)

// Middleware 返回按用户标识限流的 HTTP 中间件。
func (l *UserLimiter) Middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity := identityFromRequest(r, l.identityHeader)
		if identity == "" {
			writeRateLimitError(
				w,
				http.StatusUnauthorized,
				"缺少用户标识请求头 "+l.identityHeader,
				"invalid_request_error",
			)
			return
		}

		limiter := l.bucket(identity)
		if limiter.Allow() {
			next.ServeHTTP(w, r)
			return
		}

		retryAfter := 1
		reservation := limiter.Reserve()
		if reservation.OK() {
			delay := reservation.Delay()
			reservation.Cancel()
			retryAfter = int(math.Ceil(delay.Seconds()))
		}
		if retryAfter < 1 {
			retryAfter = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeRateLimitError(
			w,
			http.StatusTooManyRequests,
			"用户请求过于频繁，请稍后重试",
			"rate_limit_error",
		)
	})
}

func identityFromRequest(r *http.Request, fallbackHeader string) string {
	if identity, ok := auth.IdentityFromContext(r.Context()); ok {
		if userID := strings.TrimSpace(identity.UserID); userID != "" {
			return userID
		}
	}
	return strings.TrimSpace(r.Header.Get(fallbackHeader))
}
