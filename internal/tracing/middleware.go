package tracing

import (
	"log/slog"
	"net/http"
	"time"
)

// Middleware 为每个入口 HTTP 请求创建或延续 trace，并输出访问日志。
func Middleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := NewInfoFromHeaders(r.Header)
			setResponseHeaders(w.Header(), info)

			start := time.Now()
			trackedWriter := newResponseWriter(w)
			next.ServeHTTP(trackedWriter, r.WithContext(WithInfo(r.Context(), info)))

			logger.Info(
				"HTTP 请求完成",
				"trace_id", info.TraceID,
				"span_id", info.SpanID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", trackedWriter.Status(),
				"bytes", trackedWriter.BytesWritten(),
				"duration", time.Since(start),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}
