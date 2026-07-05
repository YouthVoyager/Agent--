package llm

import (
	"context"
	"sync"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

const (
	circuitStateClosed = iota
	circuitStateHalfOpen
	circuitStateOpen
)

type circuitBreaker struct {
	mu      sync.Mutex
	cfg     config.CircuitBreakerConfig
	metrics *observability.Metrics
	now     func() time.Time
	states  map[string]*backendCircuitState
}

type backendCircuitState struct {
	state               int
	consecutiveFailures int
	openedAt            time.Time
	halfOpenInFlight    int
}

type circuitBreakerPermit struct {
	breaker *circuitBreaker
	backend string
	allowed bool
	done    bool
}

func newCircuitBreaker(cfg config.CircuitBreakerConfig, backends []string, metrics *observability.Metrics) *circuitBreaker {
	breaker := &circuitBreaker{
		cfg:     cfg,
		metrics: metrics,
		now:     time.Now,
		states:  make(map[string]*backendCircuitState, len(backends)),
	}
	for _, backend := range backends {
		breaker.states[backend] = &backendCircuitState{state: circuitStateClosed}
		breaker.setStateMetricLocked(backend, circuitStateClosed)
	}
	return breaker
}

func (b *circuitBreaker) Allow(backend string) (circuitBreakerPermit, bool) {
	if b == nil || !b.cfg.Enabled {
		return circuitBreakerPermit{}, true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	state := b.stateLocked(backend)
	now := b.now()
	//如果当前启动了熔断,并且熔断时间已过,就半开尝试是否恢复
	if state.state == circuitStateOpen && now.Sub(state.openedAt) >= b.cfg.OpenTimeout.Duration {
		b.transitionLocked(backend, state, circuitStateHalfOpen)
	}

	switch state.state {
	case circuitStateOpen:
		return circuitBreakerPermit{}, false
	case circuitStateHalfOpen:
		//只放有限个请求试探
		if state.halfOpenInFlight >= b.cfg.HalfOpenMaxRequests {
			return circuitBreakerPermit{}, false
		}
		state.halfOpenInFlight++
		return circuitBreakerPermit{breaker: b, backend: backend, allowed: true}, true
	default:
		return circuitBreakerPermit{breaker: b, backend: backend, allowed: true}, true
	}
}

func (p *circuitBreakerPermit) Succeed() {
	p.finish(true, true)
}

func (p *circuitBreakerPermit) Fail() {
	p.finish(false, true)
}

func (p *circuitBreakerPermit) Ignore() {
	p.finish(false, false)
}

func (p *circuitBreakerPermit) finish(success bool, count bool) {
	if p == nil || p.done || !p.allowed || p.breaker == nil || !p.breaker.cfg.Enabled {
		return
	}
	p.done = true

	p.breaker.mu.Lock()
	defer p.breaker.mu.Unlock()

	state := p.breaker.stateLocked(p.backend)
	if state.state == circuitStateHalfOpen && state.halfOpenInFlight > 0 {
		state.halfOpenInFlight--
	}

	if !count {
		return
	}

	if success {
		state.consecutiveFailures = 0
		p.breaker.transitionLocked(p.backend, state, circuitStateClosed)
		return
	}

	switch state.state {
	case circuitStateHalfOpen:
		p.breaker.openLocked(p.backend, state)
	case circuitStateOpen:
		return
	default:
		state.consecutiveFailures++
		if state.consecutiveFailures >= p.breaker.cfg.FailureThreshold {
			p.breaker.openLocked(p.backend, state)
		}
	}
}

func (b *circuitBreaker) stateLocked(backend string) *backendCircuitState {
	state, ok := b.states[backend]
	if ok {
		return state
	}
	state = &backendCircuitState{state: circuitStateClosed}
	b.states[backend] = state
	b.setStateMetricLocked(backend, circuitStateClosed)
	return state
}

func (b *circuitBreaker) openLocked(backend string, state *backendCircuitState) {
	state.openedAt = b.now()
	state.halfOpenInFlight = 0
	b.transitionLocked(backend, state, circuitStateOpen)
}

func (b *circuitBreaker) transitionLocked(backend string, state *backendCircuitState, next int) {
	if state.state == next {
		b.setStateMetricLocked(backend, next)
		return
	}
	state.state = next
	if next == circuitStateClosed {
		state.consecutiveFailures = 0
		state.halfOpenInFlight = 0
	}
	b.setStateMetricLocked(backend, next)
}

func (b *circuitBreaker) setStateMetricLocked(backend string, state int) {
	if b.metrics == nil {
		return
	}
	b.metrics.SetCircuitBreakerState(context.Background(), backend, float64(state))
}
