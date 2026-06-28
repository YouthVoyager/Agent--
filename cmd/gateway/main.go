package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/agent-gateway/telemetry-gateway/internal/app"
	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

const exitFailure = 1

func main() {
	os.Exit(run())
}

func run() int {
	var configPath string
	flag.StringVar(&configPath, "config", "", "配置文件路径，默认读取 GATEWAY_CONFIG")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("加载配置失败", "error", err)
		return exitFailure
	}

	ctx, stop := app.SignalContext(context.Background())
	defer stop()

	gateway, err := app.New(cfg, logger)
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
