package telemetry

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// BusinessMetrics 保存网关业务层 OpenTelemetry 指标 instruments。
type BusinessMetrics struct {
	requestsTotal            metric.Int64Counter
	requestDuration          metric.Float64Histogram
	requestSuccessRate       metric.Float64Gauge
	firstTokenDuration       metric.Float64Histogram
	fallbacksTotal           metric.Int64Counter
	upstreamErrorsTotal      metric.Int64Counter
	circuitBreakerState      metric.Int64Gauge
	tokenUsageTotal          metric.Int64Counter
	tokenBudgetRemaining     metric.Int64Gauge
	tokenBudgetRejectedTotal metric.Int64Counter
}

// NewBusinessMetrics 创建网关业务指标 instruments。
func NewBusinessMetrics(provider metric.MeterProvider, namespace string) (*BusinessMetrics, error) {
	if provider == nil {
		provider = otel.GetMeterProvider()
	}
	meter := provider.Meter(instrumentationName)
	prefix := metricPrefix(namespace)

	requestsTotal, err := meter.Int64Counter(
		prefix+"requests.total",
		metric.WithDescription("模型后端请求总数。"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}
	requestDuration, err := meter.Float64Histogram(
		prefix+"request.duration",
		metric.WithDescription("模型后端请求总耗时。"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.2, 0.3, 0.5, 0.8, 1, 1.5, 2, 3, 5, 10, 20, 30, 60, 120),
	)
	if err != nil {
		return nil, err
	}
	requestSuccessRate, err := meter.Float64Gauge(
		prefix+"request.success_rate",
		metric.WithDescription("进程启动以来模型后端累计请求成功率。"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	firstTokenDuration, err := meter.Float64Histogram(
		prefix+"first_token.duration",
		metric.WithDescription("流式响应首个内容 token 返回前的耗时。"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.05, 0.1, 0.2, 0.3, 0.5, 0.8, 1, 1.5, 2, 3, 5, 10, 20, 30),
	)
	if err != nil {
		return nil, err
	}
	fallbacksTotal, err := meter.Int64Counter(
		prefix+"fallbacks.total",
		metric.WithDescription("模型降级次数。"),
		metric.WithUnit("{fallback}"),
	)
	if err != nil {
		return nil, err
	}
	upstreamErrorsTotal, err := meter.Int64Counter(
		prefix+"upstream_errors.total",
		metric.WithDescription("可触发容错的上游错误次数。"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, err
	}
	circuitBreakerState, err := meter.Int64Gauge(
		prefix+"circuit_breaker.state",
		metric.WithDescription("后端熔断状态：0 关闭，1 半开，2 打开。"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}
	tokenUsageTotal, err := meter.Int64Counter(
		prefix+"token_usage.total",
		metric.WithDescription("按身份、模型和 token 类型统计的 token 用量。"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}
	tokenBudgetRemaining, err := meter.Int64Gauge(
		prefix+"token_budget.remaining",
		metric.WithDescription("当前预算窗口内的剩余 token。"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		return nil, err
	}
	tokenBudgetRejectedTotal, err := meter.Int64Counter(
		prefix+"token_budget.rejected_total",
		metric.WithDescription("因 token 预算不足被拒绝的请求数。"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	return &BusinessMetrics{
		requestsTotal:            requestsTotal,
		requestDuration:          requestDuration,
		requestSuccessRate:       requestSuccessRate,
		firstTokenDuration:       firstTokenDuration,
		fallbacksTotal:           fallbacksTotal,
		upstreamErrorsTotal:      upstreamErrorsTotal,
		circuitBreakerState:      circuitBreakerState,
		tokenUsageTotal:          tokenUsageTotal,
		tokenBudgetRemaining:     tokenBudgetRemaining,
		tokenBudgetRejectedTotal: tokenBudgetRejectedTotal,
	}, nil
}

func (m *BusinessMetrics) ObserveModelBackendRequest(ctx context.Context, backend, result string, duration time.Duration, successRate float64) {
	if m == nil {
		return
	}
	ctx = contextOrBackground(ctx)
	attrs := metric.WithAttributes(
		attribute.String("backend", backend),
		attribute.String("result", result),
	)
	m.requestsTotal.Add(ctx, 1, attrs)
	m.requestDuration.Record(ctx, duration.Seconds(), attrs)
	m.requestSuccessRate.Record(ctx, successRate, metric.WithAttributes(attribute.String("backend", backend)))
}

func (m *BusinessMetrics) ObserveFirstToken(ctx context.Context, model string, duration time.Duration) {
	if m == nil {
		return
	}
	m.firstTokenDuration.Record(
		contextOrBackground(ctx),
		duration.Seconds(),
		metric.WithAttributes(attribute.String("model", model)),
	)
}

func (m *BusinessMetrics) ObserveModelFallback(ctx context.Context, fromModel, toModel, reason string) {
	if m == nil {
		return
	}
	m.fallbacksTotal.Add(contextOrBackground(ctx), 1, metric.WithAttributes(
		attribute.String("from_model", fromModel),
		attribute.String("to_model", toModel),
		attribute.String("reason", reason),
	))
}

func (m *BusinessMetrics) ObserveUpstreamError(ctx context.Context, backend, reason string) {
	if m == nil {
		return
	}
	m.upstreamErrorsTotal.Add(contextOrBackground(ctx), 1, metric.WithAttributes(
		attribute.String("backend", backend),
		attribute.String("reason", reason),
	))
}

func (m *BusinessMetrics) SetCircuitBreakerState(ctx context.Context, backend string, state int64) {
	if m == nil {
		return
	}
	m.circuitBreakerState.Record(contextOrBackground(ctx), state, metric.WithAttributes(attribute.String("backend", backend)))
}

func (m *BusinessMetrics) ObserveTokenUsage(ctx context.Context, identity, model string, promptTokens, completionTokens, totalTokens int64, estimated bool) {
	if m == nil {
		return
	}
	ctx = contextOrBackground(ctx)
	estimatedAttr := attribute.Bool("estimated", estimated)
	if promptTokens > 0 {
		m.tokenUsageTotal.Add(ctx, promptTokens, tokenUsageAttrs(identity, model, "prompt", estimatedAttr))
	}
	if completionTokens > 0 {
		m.tokenUsageTotal.Add(ctx, completionTokens, tokenUsageAttrs(identity, model, "completion", estimatedAttr))
	}
	if totalTokens > 0 {
		m.tokenUsageTotal.Add(ctx, totalTokens, tokenUsageAttrs(identity, model, "total", estimatedAttr))
	}
}

func (m *BusinessMetrics) SetTokenBudgetRemaining(ctx context.Context, identity string, remainingTokens int64) {
	if m == nil {
		return
	}
	if remainingTokens < 0 {
		remainingTokens = 0
	}
	m.tokenBudgetRemaining.Record(contextOrBackground(ctx), remainingTokens, metric.WithAttributes(attribute.String("identity", identity)))
}

func (m *BusinessMetrics) ObserveTokenBudgetRejected(ctx context.Context, identity, model string) {
	if m == nil {
		return
	}
	m.tokenBudgetRejectedTotal.Add(contextOrBackground(ctx), 1, metric.WithAttributes(
		attribute.String("identity", identity),
		attribute.String("model", model),
	))
}

func metricPrefix(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "gateway"
	}
	return namespace + "."
}

func tokenUsageAttrs(identity, model, tokenType string, estimated attribute.KeyValue) metric.AddOption {
	return metric.WithAttributes(
		attribute.String("identity", identity),
		attribute.String("model", model),
		attribute.String("type", tokenType),
		estimated,
	)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
