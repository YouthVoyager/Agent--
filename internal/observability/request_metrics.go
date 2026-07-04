package observability

import (
	"strconv"
	"sync"
	"time"
)

const (
	RequestResultSuccess = "success"
	RequestResultFailure = "failure"
)

type requestMetricStats struct {
	mu     sync.Mutex
	counts map[string]requestMetricCount
}

type requestMetricCount struct {
	success uint64
	total   uint64
}

func newRequestMetricStats() *requestMetricStats {
	return &requestMetricStats{
		counts: make(map[string]requestMetricCount),
	}
}

func (s *requestMetricStats) observe(backend, result string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := s.counts[backend]
	count.total++
	if result == RequestResultSuccess {
		count.success++
	}
	s.counts[backend] = count

	if count.total == 0 {
		return 0
	}
	return float64(count.success) / float64(count.total)
}

func (m *Metrics) ObserveModelBackendRequest(backend, result string, duration time.Duration) {
	if m == nil {
		return
	}
	if result == "" {
		result = RequestResultFailure
	}
	if duration < 0 {
		duration = 0
	}

	if m.RequestsTotal != nil {
		m.RequestsTotal.WithLabelValues(backend, result).Inc()
	}
	if m.RequestDuration != nil {
		m.RequestDuration.WithLabelValues(backend, result).Observe(duration.Seconds())
	}
	if m.RequestSuccessRate != nil && m.requestStats != nil {
		rate := m.requestStats.observe(backend, result)
		m.RequestSuccessRate.WithLabelValues(backend).Set(rate)
	}
}

func (m *Metrics) ObserveModelFallback(fromModel, toModel, reason string) {
	if m == nil || m.FallbacksTotal == nil {
		return
	}
	m.FallbacksTotal.WithLabelValues(fromModel, toModel, reason).Inc()
}

func (m *Metrics) ObserveUpstreamError(backend, reason string) {
	if m == nil || m.UpstreamErrorsTotal == nil {
		return
	}
	m.UpstreamErrorsTotal.WithLabelValues(backend, reason).Inc()
}

func (m *Metrics) SetCircuitBreakerState(backend string, state float64) {
	if m == nil || m.CircuitBreakerState == nil {
		return
	}
	m.CircuitBreakerState.WithLabelValues(backend).Set(state)
}

func (m *Metrics) ObserveTokenUsage(identity, model string, promptTokens, completionTokens, totalTokens int, estimated bool) {
	if m == nil || m.TokenUsageTotal == nil {
		return
	}
	estimatedValue := strconv.FormatBool(estimated)
	if promptTokens > 0 {
		m.TokenUsageTotal.WithLabelValues(identity, model, "prompt", estimatedValue).Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		m.TokenUsageTotal.WithLabelValues(identity, model, "completion", estimatedValue).Add(float64(completionTokens))
	}
	if totalTokens > 0 {
		m.TokenUsageTotal.WithLabelValues(identity, model, "total", estimatedValue).Add(float64(totalTokens))
	}
}

func (m *Metrics) SetTokenBudgetRemaining(identity string, remainingTokens int) {
	if m == nil || m.TokenBudgetRemaining == nil {
		return
	}
	if remainingTokens < 0 {
		remainingTokens = 0
	}
	m.TokenBudgetRemaining.WithLabelValues(identity).Set(float64(remainingTokens))
}

func (m *Metrics) ObserveTokenBudgetRejected(identity, model string) {
	if m == nil || m.TokenBudgetRejectedTotal == nil {
		return
	}
	m.TokenBudgetRejectedTotal.WithLabelValues(identity, model).Inc()
}
