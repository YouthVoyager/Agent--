package telemetry

import (
	"context"
	"errors"
	"strings"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/agent-gateway/telemetry-gateway"

// Runtime 持有 OpenTelemetry SDK Provider 和跨进程传播器。
type Runtime struct {
	cfg            config.OpenTelemetryConfig
	resource       *resource.Resource
	propagator     propagation.TextMapPropagator
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
}

// New 初始化 OpenTelemetry Trace、Metric、Log SDK。
func New(ctx context.Context, cfg config.OpenTelemetryConfig) (*Runtime, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	runtime := &Runtime{
		cfg:        cfg,
		propagator: newPropagator(),
	}
	otel.SetTextMapPropagator(runtime.propagator)

	if !cfg.Enabled {
		return runtime, nil
	}

	res, err := newResource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	runtime.resource = res

	if cfg.Traces.Enabled {
		tracerProvider, err := newTracerProvider(ctx, cfg, res)
		if err != nil {
			return nil, err
		}
		runtime.tracerProvider = tracerProvider
		otel.SetTracerProvider(tracerProvider)
	}

	if cfg.Metrics.Enabled {
		meterProvider, err := newMeterProvider(ctx, cfg, res)
		if err != nil {
			return nil, err
		}
		runtime.meterProvider = meterProvider
		otel.SetMeterProvider(meterProvider)
	}

	if cfg.Logs.Enabled {
		loggerProvider, err := newLoggerProvider(ctx, cfg, res)
		if err != nil {
			return nil, err
		}
		runtime.loggerProvider = loggerProvider
	}

	return runtime, nil
}

// Shutdown 刷新并关闭所有 OpenTelemetry Provider。
func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var errs []error
	if r.loggerProvider != nil {
		if err := r.loggerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if r.meterProvider != nil {
		if err := r.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if r.tracerProvider != nil {
		if err := r.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) TracesEnabled() bool {
	return r != nil && r.cfg.Enabled && r.cfg.Traces.Enabled && r.tracerProvider != nil
}

func (r *Runtime) MetricsEnabled() bool {
	return r != nil && r.cfg.Enabled && r.cfg.Metrics.Enabled && r.meterProvider != nil
}

func (r *Runtime) LogsEnabled() bool {
	return r != nil && r.cfg.Enabled && r.cfg.Logs.Enabled && r.loggerProvider != nil
}

func (r *Runtime) TracerProvider() trace.TracerProvider {
	if r == nil || r.tracerProvider == nil {
		return otel.GetTracerProvider()
	}
	return r.tracerProvider
}

func (r *Runtime) MeterProvider() metric.MeterProvider {
	if r == nil || r.meterProvider == nil {
		return otel.GetMeterProvider()
	}
	return r.meterProvider
}

func (r *Runtime) LoggerProvider() otellog.LoggerProvider {
	if r == nil || r.loggerProvider == nil {
		return nil
	}
	return r.loggerProvider
}

func (r *Runtime) Propagator() propagation.TextMapPropagator {
	if r == nil || r.propagator == nil {
		return otel.GetTextMapPropagator()
	}
	return r.propagator
}

func newResource(ctx context.Context, cfg config.OpenTelemetryConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(strings.TrimSpace(cfg.ServiceName)),
	}
	if serviceVersion := strings.TrimSpace(cfg.ServiceVersion); serviceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(serviceVersion))
	}
	if environment := strings.TrimSpace(cfg.Environment); environment != "" {
		attrs = append(attrs, semconv.DeploymentEnvironmentName(environment))
	}

	return resource.New(
		ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
}
