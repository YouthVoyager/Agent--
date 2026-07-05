package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
)

// NewLogger 创建同时写入本地输出和 OpenTelemetry Logs 的 slog.Logger。
func NewLogger(output io.Writer, level slog.Leveler, runtime *Runtime) *slog.Logger {
	if output == nil {
		output = io.Discard
	}
	if level == nil {
		level = slog.LevelInfo
	}

	handlers := []slog.Handler{
		slog.NewTextHandler(output, &slog.HandlerOptions{
			Level: level,
		}),
	}
	if runtime != nil && runtime.LogsEnabled() {
		handlers = append(handlers, otelslog.NewHandler(
			instrumentationName,
			otelslog.WithLoggerProvider(runtime.LoggerProvider()),
		))
	}

	return slog.New(newFanoutHandler(handlers...))
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func newFanoutHandler(handlers ...slog.Handler) slog.Handler {
	copied := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			copied = append(copied, handler)
		}
	}
	return fanoutHandler{handlers: copied}
}

func (h fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return fanoutHandler{handlers: handlers}
}

func (h fanoutHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return fanoutHandler{handlers: handlers}
}
