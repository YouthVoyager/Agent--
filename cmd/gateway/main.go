package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/agent-gateway/telemetry-gateway/internal/app"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"github.com/agent-gateway/telemetry-gateway/internal/telemetry"
)

const exitFailure = 1

func main() {
	os.Exit(run())
}

func run() int {
	//读入配置文件
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件路径，默认读取 GATEWAY_CONFIG")
	flag.Parse()
	//加载启动阶段日志
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	//加载配置
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
		return exitFailure
	}
	//初始化 OpenTelemetry
	telemetryRuntime, err := telemetry.New(context.Background(), cfg.Observability.OpenTelemetry)
	if err != nil {
		logger.Error("初始化 OpenTelemetry 失败", "error", err)
		return exitFailure
	}
	logger = telemetry.NewLogger(os.Stdout, slog.LevelInfo, telemetryRuntime)
	slog.SetDefault(logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.Duration)
		defer cancel()
		if err := telemetryRuntime.Shutdown(shutdownCtx); err != nil {
			logger.Error("关闭 OpenTelemetry 失败", "error", err)
		}
	}()
	//配置进程上下文,实现优雅停机
	ctx, stop := app.SignalContext(context.Background())
	defer stop()
	//启动网关
	gateway, err := app.New(cfg, logger, telemetryRuntime)
	if err != nil {
		logger.Error("初始化网关失败", "error", err)
		return exitFailure
	}

	if err := gateway.Run(ctx); err != nil {
		logger.Error("网关退出异常", "error", err)
		return exitFailure
	}

	logger.Info("网关已停止")
	return 0
}
