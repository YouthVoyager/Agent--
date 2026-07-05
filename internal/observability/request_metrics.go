package observability

import (
	"context"
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

func (m *Metrics) ObserveModelBackendRequest(ctx context.Context, backend, result string, duration time.Duration) {
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
	successRate := 0.0
	if m.RequestSuccessRate != nil && m.requestStats != nil {
		successRate = m.requestStats.observe(backend, result)
		m.RequestSuccessRate.WithLabelValues(backend).Set(successRate)
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveModelBackendRequest(ctx, backend, result, duration, successRate)
	}
}

func (m *Metrics) ObserveFirstToken(ctx context.Context, model string, duration time.Duration) {
	if m == nil {
		return
	}
	if m.FirstTokenDuration != nil {
		m.FirstTokenDuration.WithLabelValues(model).Observe(duration.Seconds())
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveFirstToken(ctx, model, duration)
	}
}

func (m *Metrics) ObserveModelFallback(ctx context.Context, fromModel, toModel, reason string) {
	if m == nil {
		return
	}
	if m.FallbacksTotal != nil {
		m.FallbacksTotal.WithLabelValues(fromModel, toModel, reason).Inc()
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveModelFallback(ctx, fromModel, toModel, reason)
	}
}

func (m *Metrics) ObserveUpstreamError(ctx context.Context, backend, reason string) {
	if m == nil {
		return
	}
	if m.UpstreamErrorsTotal != nil {
		m.UpstreamErrorsTotal.WithLabelValues(backend, reason).Inc()
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveUpstreamError(ctx, backend, reason)
	}
}

func (m *Metrics) SetCircuitBreakerState(ctx context.Context, backend string, state float64) {
	if m == nil {
		return
	}
	if m.CircuitBreakerState != nil {
		m.CircuitBreakerState.WithLabelValues(backend).Set(state)
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.SetCircuitBreakerState(ctx, backend, int64(state))
	}
}

func (m *Metrics) ObserveTokenUsage(ctx context.Context, identity, model string, promptTokens, completionTokens, totalTokens int, estimated bool) {
	if m == nil {
		return
	}
	estimatedValue := strconv.FormatBool(estimated)
	if m.TokenUsageTotal != nil {
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
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveTokenUsage(
			ctx,
			identity,
			model,
			int64(promptTokens),
			int64(completionTokens),
			int64(totalTokens),
			estimated,
		)
	}
}

func (m *Metrics) SetTokenBudgetRemaining(ctx context.Context, identity string, remainingTokens int) {
	if m == nil {
		return
	}
	if remainingTokens < 0 {
		remainingTokens = 0
	}
	if m.TokenBudgetRemaining != nil {
		m.TokenBudgetRemaining.WithLabelValues(identity).Set(float64(remainingTokens))
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.SetTokenBudgetRemaining(ctx, identity, int64(remainingTokens))
	}
}

func (m *Metrics) ObserveTokenBudgetRejected(ctx context.Context, identity, model string) {
	if m == nil {
		return
	}
	if m.TokenBudgetRejectedTotal != nil {
		m.TokenBudgetRejectedTotal.WithLabelValues(identity, model).Inc()
	}
	if m.OpenTelemetry != nil {
		m.OpenTelemetry.ObserveTokenBudgetRejected(ctx, identity, model)
	}
}
