package llm

import (
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

func (h *Handler) observeBackendRequest(backendName, result string, duration time.Duration) {
	if h.metrics == nil {
		return
	}
	h.metrics.ObserveModelBackendRequest(backendName, result, duration)
}
