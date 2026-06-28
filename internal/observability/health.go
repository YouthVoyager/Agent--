package observability

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
	mux.HandleFunc("/metrics", metricsHandler(state))
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

func metricsHandler(state State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		ready := 0
		if isReady(state) {
			ready = 1
		}

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "# HELP %s_up Whether the gateway process is running.\n", state.MetricsNamespace)
		fmt.Fprintf(w, "# TYPE %s_up gauge\n", state.MetricsNamespace)
		fmt.Fprintf(w, "%s_up 1\n", state.MetricsNamespace)
		fmt.Fprintf(w, "# HELP %s_ready Whether the gateway is ready to receive traffic.\n", state.MetricsNamespace)
		fmt.Fprintf(w, "# TYPE %s_ready gauge\n", state.MetricsNamespace)
		fmt.Fprintf(w, "%s_ready %d\n", state.MetricsNamespace, ready)
		fmt.Fprintf(w, "# HELP %s_start_time_seconds Gateway process start time.\n", state.MetricsNamespace)
		fmt.Fprintf(w, "# TYPE %s_start_time_seconds gauge\n", state.MetricsNamespace)
		fmt.Fprintf(w, "%s_start_time_seconds %d\n", state.MetricsNamespace, state.StartTime.Unix())
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
