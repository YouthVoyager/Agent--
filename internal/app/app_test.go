package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

func TestHealthz(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ok"`) {
		t.Fatalf("响应体未包含健康状态: %s", recorder.Body.String())
	}
}

func TestReadyzReflectsState(t *testing.T) {
	gateway := newTestApp(t)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("未启动时 readyz status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	gateway.ready.Store(true)
	recorder = httptest.NewRecorder()
	gateway.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready 后 readyz status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestMetrics(t *testing.T) {
	gateway := newTestApp(t)
	gateway.ready.Store(true)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	gateway.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "gateway_up 1") {
		t.Fatalf("metrics 未包含 gateway_up: %s", body)
	}
	if !strings.Contains(body, "gateway_ready 1") {
		t.Fatalf("metrics 未包含 gateway_ready: %s", body)
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()

	cfg := config.Default()
	cfg.Server.Address = "127.0.0.1:18080"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	gateway, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return gateway
}
