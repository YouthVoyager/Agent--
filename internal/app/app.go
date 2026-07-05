package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/admin"
	"github.com/agent-gateway/telemetry-gateway/internal/auth"
	"github.com/agent-gateway/telemetry-gateway/internal/concurrency"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/llm"
	"github.com/agent-gateway/telemetry-gateway/internal/observability"
	"github.com/agent-gateway/telemetry-gateway/internal/ratelimit"
	"github.com/agent-gateway/telemetry-gateway/internal/telemetry"
	"github.com/agent-gateway/telemetry-gateway/internal/tokenusage"
	"github.com/agent-gateway/telemetry-gateway/internal/tracing"
)

// App 表示遥测网关应用，负责持有配置、HTTP 服务与运行状态。
type App struct {
	//主程序属性,包含配置,状态,所需依赖等
	cfg              config.Config
	logger           *slog.Logger
	server           *http.Server
	handler          http.Handler
	telemetryRuntime *telemetry.Runtime
	ready            atomic.Bool
	startedAt        time.Time
}

// New 创建并初始化一个遥测网关应用实例。
func New(cfg config.Config, logger *slog.Logger, telemetryRuntimes ...*telemetry.Runtime) (*App, error) {
	//加载默认配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	var telemetryRuntime *telemetry.Runtime
	if len(telemetryRuntimes) > 0 {
		telemetryRuntime = telemetryRuntimes[0]
	}
	if telemetryRuntime == nil {
		var err error
		telemetryRuntime, err = telemetry.New(context.Background(), cfg.Observability.OpenTelemetry)
		if err != nil {
			return nil, err
		}
	}

	gateway := &App{
		cfg:              cfg,
		logger:           logger,
		telemetryRuntime: telemetryRuntime,
		startedAt:        time.Now(),
	}

	//创建服务器
	mux := http.NewServeMux()
	//创建观测指标
	metrics := observability.NewMetrics(cfg.Observability.MetricsNamespace, func() bool {
		return gateway.ready.Load()
	})
	//新建ai聊天接口句柄
	llmHandler, err := llm.NewHandler(cfg.AI, logger, metrics, telemetryRuntime)
	if err != nil {
		return nil, err
	}

	var chatMiddlewares []func(http.Handler) http.Handler
	if cfg.RateLimit.User.Enabled {
		userLimiter := ratelimit.NewUserLimiter(cfg.RateLimit.User)
		chatMiddlewares = append(chatMiddlewares, userLimiter.Middleware)
	}
	if cfg.TokenUsage.Enabled {
		tokenUsageController := tokenusage.NewController(cfg.TokenUsage, metrics)
		chatMiddlewares = append(chatMiddlewares, tokenUsageController.Middleware)
	}
	if cfg.RateLimit.Concurrency.Enabled {
		concurrencyLimiter := concurrency.NewLimiter(cfg.RateLimit.Concurrency)
		chatMiddlewares = append(chatMiddlewares, concurrencyLimiter.Middleware)
	}
	//注册api路由
	if cfg.Auth.APIKey.Enabled {
		authenticator, err := auth.NewAPIKeyAuthenticator(cfg.Auth.APIKey)
		if err != nil {
			return nil, err
		}
		llmMux := http.NewServeMux()
		llm.Register(llmMux, llmHandler, chatMiddlewares...)
		mux.Handle("/v1/", authenticator.Middleware(llmMux))
	} else {
		llm.Register(mux, llmHandler, chatMiddlewares...)
	}
	//注册观测器
	observability.Register(mux, observability.State{
		ServiceName:      "telemetry-gateway",
		StartTime:        gateway.startedAt,
		MetricsNamespace: cfg.Observability.MetricsNamespace,
		Ready: func() bool {
			return gateway.ready.Load()
		},
	}, metrics)
	//注册管理页面
	admin.Register(mux, cfg.Observability.Stack)

	var rootHandler http.Handler = mux
	if cfg.Observability.Tracing.Enabled {
		rootHandler = tracing.Middleware(logger)(rootHandler)
	}
	rootHandler = telemetryRuntime.WrapHandler(rootHandler, "gateway.http")

	gateway.handler = rootHandler
	gateway.server = &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           rootHandler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.Duration,
	}

	return gateway, nil
}

// Handler 返回应用使用的 HTTP 处理器。
func (a *App) Handler() http.Handler {
	return a.handler
}
