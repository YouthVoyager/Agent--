package app

import (
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/llm"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
)

type App struct {
	cfg       config.Config
	logger    *slog.Logger
	server    *http.Server
	handler   http.Handler
	ready     atomic.Bool
	startedAt time.Time
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}

	gateway := &App{
		cfg:       cfg,
		logger:    logger,
		startedAt: time.Now(),
	}

	mux := http.NewServeMux()
	metrics := observability.NewMetrics()

	llmHandler, err := llm.NewHandler(cfg.AI, logger,metrics)
	if err != nil {
		return nil, err
	}
	llm.Register(mux, llmHandler)

	observability.Register(mux, observability.State{
		ServiceName:      "telemetry-gateway",
		StartTime:        gateway.startedAt,
		MetricsNamespace: cfg.Observability.MetricsNamespace,
		Ready: func() bool {
			return gateway.ready.Load()
		},
	})

	gateway.handler = mux
	gateway.server = &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           mux,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.Duration,
	}

	return gateway, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}
