package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestHandlerServesIndex(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "Agent 网关管理") {
		t.Fatalf("响应体未包含管理页面标题: %s", recorder.Body.String())
	}
}

func TestHandlerServesStaticFile(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/src/main.js", nil)

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "createRoot") {
		t.Fatalf("响应体未包含 React 入口: %s", recorder.Body.String())
	}
}

func TestHandlerRejectsUnsupportedMethod(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", nil)

	Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestObservabilityHandlerDisabled(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/api/observability", nil)

	ObservabilityHandler(config.ObservabilityStack{})(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response observabilityResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if response.Enabled {
		t.Fatal("enabled 应为 false")
	}
	if len(response.Services) != 0 {
		t.Fatalf("services 数量 = %d, want 0", len(response.Services))
	}
}

func TestObservabilityHandlerChecksServices(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if strings.Contains(r.URL.Host, "tempo") {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	})
	defer func() {
		http.DefaultTransport = originalTransport
	}()

	stack := config.ObservabilityStack{
		Enabled: true,
		Services: []config.ObservabilityServiceConfig{
			{
				Name:      "Grafana",
				Kind:      "dashboard",
				PublicURL: "http://grafana.test",
				HealthURL: "http://grafana.test/api/health",
			},
			{
				Name:      "Tempo",
				Kind:      "traces-store",
				PublicURL: "http://tempo.test",
				HealthURL: "http://tempo.test/ready",
			},
		},
		Dashboards: []config.ObservabilityDashboardConfig{
			{
				Name:     "Agent Gateway Overview",
				URL:      "http://grafana.test/d/gateway",
				EmbedURL: "http://grafana.test/d/gateway?kiosk",
			},
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/api/observability", nil)
	ObservabilityHandler(stack)(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response observabilityResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if !response.Enabled {
		t.Fatal("enabled 应为 true")
	}
	if len(response.Services) != 2 {
		t.Fatalf("services 数量 = %d, want 2", len(response.Services))
	}
	if response.Services[0].Status != "online" {
		t.Fatalf("Grafana status = %q", response.Services[0].Status)
	}
	if response.Services[1].Status != "offline" {
		t.Fatalf("Tempo status = %q", response.Services[1].Status)
	}
	if len(response.Dashboards) != 1 || response.Dashboards[0].EmbedURL == "" {
		t.Fatalf("dashboards = %+v", response.Dashboards)
	}
}

func TestRegisterServesObservabilityAPI(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, config.ObservabilityStack{
		Enabled: true,
		Services: []config.ObservabilityServiceConfig{
			{
				Name:      "Grafana",
				Kind:      "dashboard",
				PublicURL: "http://localhost:3000",
			},
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/api/observability", nil)
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "Grafana") {
		t.Fatalf("响应体未包含观测服务: %s", recorder.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
