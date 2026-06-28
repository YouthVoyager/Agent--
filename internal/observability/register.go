package observability

import "net/http"

func Register(mux *http.ServeMux, state State, metrics *Metrics) {
	state = normalizeState(state)
	if metrics == nil {
		metrics = NewMetrics(state.MetricsNamespace, state.Ready)
	}

	mux.HandleFunc("/healthz", healthHandler(state))
	mux.HandleFunc("/readyz", readyHandler(state))
	registerMetrics(mux, metrics)
	registerPprof(mux)
}
