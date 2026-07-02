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

// App 表示遥测网关应用，负责持有配置、HTTP 服务与运行状态。
type App struct {
	//主程序属性,包含配置,状态,所需依赖等
	cfg       config.Config
	logger    *slog.Logger
	server    *http.Server
	handler   http.Handler
	ready     atomic.Bool
	startedAt time.Time
}

// New 创建并初始化一个遥测网关应用实例。
func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	//加载默认配置
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

	//创建服务器
	mux := http.NewServeMux()
	//创建观测指标
	metrics := observability.NewMetrics(cfg.Observability.MetricsNamespace, func() bool {
		return gateway.ready.Load()
	})
	//新建ai聊天接口句柄
	llmHandler, err := llm.NewHandler(cfg.AI, logger, metrics)
	if err != nil {
		return nil, err
	}
	//注册api路由
	llm.Register(mux, llmHandler)
	//注册观测器
	observability.Register(mux, observability.State{
		ServiceName:      "telemetry-gateway",
		StartTime:        gateway.startedAt,
		MetricsNamespace: cfg.Observability.MetricsNamespace,
		Ready: func() bool {
			return gateway.ready.Load()
		},
	}, metrics)

	gateway.handler = mux
	gateway.server = &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           mux,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.Duration,
	}

	return gateway, nil
}

// Handler 返回应用使用的 HTTP 处理器。
func (a *App) Handler() http.Handler {
	return a.handler
}
