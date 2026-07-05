package llm

import (
	"context"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

const (
	requestResultSuccess = observability.RequestResultSuccess
	requestResultFailure = observability.RequestResultFailure
)

func requestResultFromStatus(status int) string {
	if status >= 200 && status < 400 {
		return requestResultSuccess
	}
	return requestResultFailure
}

func (h *Handler) observeBackendRequest(ctx context.Context, backendName, result string, duration time.Duration) {
	if h.metrics == nil {
		return
	}
	h.metrics.ObserveModelBackendRequest(ctx, backendName, result, duration)
}

func (h *Handler) observeFallback(ctx context.Context, fromModel, toModel, reason string) {
	if h.metrics == nil {
		return
	}
	h.metrics.ObserveModelFallback(ctx, fromModel, toModel, reason)
}

func (h *Handler) observeUpstreamError(ctx context.Context, backendName, reason string) {
	if h.metrics == nil {
		return
	}
	h.metrics.ObserveUpstreamError(ctx, backendName, reason)
}
