package observability

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type State struct {
	ServiceName      string
	StartTime        time.Time
	MetricsNamespace string
	Ready            func() bool
}

func Register(mux *http.ServeMux, state State) {
	if state.ServiceName == "" {
		state.ServiceName = "telemetry-gateway"
	}
	if state.StartTime.IsZero() {
		state.StartTime = time.Now()
	}
	if state.MetricsNamespace == "" {
		state.MetricsNamespace = "gateway"
	}
	


	mux.HandleFunc("/healthz", healthHandler(state))
	mux.HandleFunc("/readyz", readyHandler(state))
	mux.Handle("/metrics", promhttp.Handler())
	registerPprof(mux)
}

func healthHandler(state State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"service":        state.ServiceName,
			"status":         "ok",
			"uptime_seconds": int64(time.Since(state.StartTime).Seconds()),
		})
	}
}

func readyHandler(state State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if !isReady(state) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"service": state.ServiceName,
				"status":  "not_ready",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"service": state.ServiceName,
			"status":  "ready",
		})
	}
}



func isReady(state State) bool {
	return state.Ready != nil && state.Ready()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
