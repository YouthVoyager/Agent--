package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

const observabilityCheckTimeout = 1500 * time.Millisecond

type observabilityResponse struct {
	Enabled    bool                         `json:"enabled"`
	Services   []observabilityServiceStatus `json:"services"`
	Dashboards []observabilityDashboard     `json:"dashboards"`
}

type observabilityServiceStatus struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url,omitempty"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type observabilityDashboard struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	EmbedURL string `json:"embed_url,omitempty"`
}

// ObservabilityHandler 返回观测栈状态 API。
func ObservabilityHandler(stack config.ObservabilityStack) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "仅支持 GET 方法", http.StatusMethodNotAllowed)
			return
		}

		writeJSON(w, http.StatusOK, buildObservabilityResponse(r.Context(), stack))
	}
}

func buildObservabilityResponse(ctx context.Context, stack config.ObservabilityStack) observabilityResponse {
	response := observabilityResponse{
		Enabled:    stack.Enabled,
		Services:   make([]observabilityServiceStatus, 0, len(stack.Services)),
		Dashboards: make([]observabilityDashboard, 0, len(stack.Dashboards)),
	}
	for _, dashboard := range stack.Dashboards {
		response.Dashboards = append(response.Dashboards, observabilityDashboard{
			Name:     strings.TrimSpace(dashboard.Name),
			URL:      strings.TrimSpace(dashboard.URL),
			EmbedURL: strings.TrimSpace(dashboard.EmbedURL),
		})
	}
	if !stack.Enabled {
		return response
	}

	response.Services = checkObservabilityServices(ctx, stack.Services)
	return response
}

func checkObservabilityServices(ctx context.Context, services []config.ObservabilityServiceConfig) []observabilityServiceStatus {
	statuses := make([]observabilityServiceStatus, len(services))
	var waitGroup sync.WaitGroup
	for index, service := range services {
		waitGroup.Add(1)
		go func(index int, service config.ObservabilityServiceConfig) {
			defer waitGroup.Done()
			statuses[index] = checkObservabilityService(ctx, service)
		}(index, service)
	}
	waitGroup.Wait()
	return statuses
}

func checkObservabilityService(ctx context.Context, service config.ObservabilityServiceConfig) observabilityServiceStatus {
	status := observabilityServiceStatus{
		Name:   strings.TrimSpace(service.Name),
		Kind:   strings.TrimSpace(service.Kind),
		URL:    strings.TrimSpace(service.PublicURL),
		Status: "unknown",
	}
	healthURL := strings.TrimSpace(service.HealthURL)
	if healthURL == "" {
		status.Message = "未配置健康检查地址"
		return status
	}

	checkCtx, cancel := context.WithTimeout(ctx, observabilityCheckTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		status.Status = "offline"
		status.Message = err.Error()
		return status
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		status.Status = "offline"
		status.Message = err.Error()
		return status
	}
	defer response.Body.Close()

	status.StatusCode = response.StatusCode
	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		status.Status = "online"
	case response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusInternalServerError:
		status.Status = "degraded"
		status.Message = fmt.Sprintf("HTTP %d", response.StatusCode)
	default:
		status.Status = "offline"
		status.Message = fmt.Sprintf("HTTP %d", response.StatusCode)
	}
	return status
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
