package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	a.ready.Store(true)
	a.logger.Info("网关启动", "address", a.cfg.Server.Address)

	errCh := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		a.ready.Store(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.Server.ShutdownTimeout.Duration)
		defer cancel()

		a.logger.Info("网关开始优雅退出")
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("停止 HTTP 服务: %w", err)
		}

		if err := <-errCh; err != nil {
			return fmt.Errorf("HTTP 服务退出: %w", err)
		}
		return nil

	case err := <-errCh:
		a.ready.Store(false)
		if err != nil {
			return fmt.Errorf("启动 HTTP 服务: %w", err)
		}
		return nil
	}
}
