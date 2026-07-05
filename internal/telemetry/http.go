package telemetry

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// WrapHandler 为 HTTP 入站请求生成 OpenTelemetry server span 和 HTTP 指标。
func (r *Runtime) WrapHandler(handler http.Handler, operation string) http.Handler {
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	if r == nil || (!r.TracesEnabled() && !r.MetricsEnabled()) {
		return handler
	}

	options := []otelhttp.Option{
		otelhttp.WithPropagators(r.Propagator()),
		otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
			if req == nil || req.URL == nil {
				return operation
			}
			return req.Method + " " + req.URL.Path
		}),
	}
	if r.TracesEnabled() {
		options = append(options, otelhttp.WithTracerProvider(r.TracerProvider()))
	}
	if r.MetricsEnabled() {
		options = append(options, otelhttp.WithMeterProvider(r.MeterProvider()))
	}

	return otelhttp.NewHandler(handler, operation, options...)
}

// WrapTransport 为 HTTP 出站请求生成 OpenTelemetry client span 和 HTTP 指标。
func (r *Runtime) WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if r == nil || (!r.TracesEnabled() && !r.MetricsEnabled()) {
		return base
	}

	options := []otelhttp.Option{
		otelhttp.WithPropagators(r.Propagator()),
	}
	if r.TracesEnabled() {
		options = append(options, otelhttp.WithTracerProvider(r.TracerProvider()))
	}
	if r.MetricsEnabled() {
		options = append(options, otelhttp.WithMeterProvider(r.MeterProvider()))
	}

	return otelhttp.NewTransport(base, options...)
}
