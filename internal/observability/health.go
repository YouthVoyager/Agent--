package observability

import (
	"net/http"
	"time"
)

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
