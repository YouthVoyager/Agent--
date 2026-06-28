package observability

import "time"

type State struct {
	ServiceName      string
	StartTime        time.Time
	MetricsNamespace string
	Ready            func() bool
}

func normalizeState(state State) State {
	if state.ServiceName == "" {
		state.ServiceName = "telemetry-gateway"
	}
	if state.StartTime.IsZero() {
		state.StartTime = time.Now()
	}
	if state.MetricsNamespace == "" {
		state.MetricsNamespace = "gateway"
	}
	return state
}
