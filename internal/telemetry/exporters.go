package telemetry

import (
	"context"
	"os"
	"strings"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	traceSignalPath = "/v1/traces"
	logSignalPath   = "/v1/logs"
)

func newTracerProvider(ctx context.Context, cfg config.OpenTelemetryConfig, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}

	if shouldExport(cfg, cfg.Traces, "TRACES") {
		exporter, err := otlptracehttp.New(ctx, traceExporterOptions(cfg)...)
		if err != nil {
			return nil, err
		}
		options = append(options, sdktrace.WithBatcher(
			exporter,
			sdktrace.WithExportTimeout(cfg.ExportTimeout.Duration),
		))
	}

	return sdktrace.NewTracerProvider(options...), nil
}

func newLoggerProvider(ctx context.Context, cfg config.OpenTelemetryConfig, res *resource.Resource) (*sdklog.LoggerProvider, error) {
	options := []sdklog.LoggerProviderOption{
		sdklog.WithResource(res),
	}

	if shouldExport(cfg, cfg.Logs, "LOGS") {
		exporter, err := otlploghttp.New(ctx, logExporterOptions(cfg)...)
		if err != nil {
			return nil, err
		}
		options = append(options, sdklog.WithProcessor(sdklog.NewBatchProcessor(
			exporter,
			sdklog.WithExportTimeout(cfg.ExportTimeout.Duration),
		)))
	}

	return sdklog.NewLoggerProvider(options...), nil
}

func traceExporterOptions(cfg config.OpenTelemetryConfig) []otlptracehttp.Option {
	options := []otlptracehttp.Option{
		otlptracehttp.WithTimeout(cfg.ExportTimeout.Duration),
	}
	if endpoint := signalEndpoint(cfg.Endpoint, cfg.Traces.Endpoint, traceSignalPath); endpoint != "" {
		options = append(options, otlptracehttp.WithEndpointURL(endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if headers := cleanHeaders(cfg.Headers); len(headers) > 0 {
		options = append(options, otlptracehttp.WithHeaders(headers))
	}
	return options
}

func logExporterOptions(cfg config.OpenTelemetryConfig) []otlploghttp.Option {
	options := []otlploghttp.Option{
		otlploghttp.WithTimeout(cfg.ExportTimeout.Duration),
	}
	if endpoint := signalEndpoint(cfg.Endpoint, cfg.Logs.Endpoint, logSignalPath); endpoint != "" {
		options = append(options, otlploghttp.WithEndpointURL(endpoint))
	}
	if cfg.Insecure {
		options = append(options, otlploghttp.WithInsecure())
	}
	if headers := cleanHeaders(cfg.Headers); len(headers) > 0 {
		options = append(options, otlploghttp.WithHeaders(headers))
	}
	return options
}

func shouldExport(cfg config.OpenTelemetryConfig, signal config.OpenTelemetrySignalConfig, signalEnv string) bool {
	if !cfg.Enabled || !signal.Enabled {
		return false
	}
	if strings.TrimSpace(cfg.Endpoint) != "" || strings.TrimSpace(signal.Endpoint) != "" {
		return true
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_"+signalEnv+"_ENDPOINT") != ""
}

func signalEndpoint(commonEndpoint string, signalEndpoint string, signalPath string) string {
	if endpoint := strings.TrimSpace(signalEndpoint); endpoint != "" {
		return endpoint
	}
	endpoint := strings.TrimRight(strings.TrimSpace(commonEndpoint), "/")
	if endpoint == "" {
		return ""
	}
	return endpoint + signalPath
}

func cleanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	result := make(map[string]string, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		result[key] = strings.TrimSpace(value)
	}
	return result
}
