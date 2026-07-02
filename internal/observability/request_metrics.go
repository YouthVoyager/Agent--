package observability

import (
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
