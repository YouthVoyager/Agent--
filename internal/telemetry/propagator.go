package telemetry

import (
	"context"
	"crypto/rand"

	"github.com/agent-gateway/telemetry-gateway/internal/tracing"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type legacyTraceIDPropagator struct{}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		legacyTraceIDPropagator{},
	)
}

func (legacyTraceIDPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return
	}
	traceID := spanContext.TraceID().String()
	carrier.Set(tracing.HeaderTraceID, traceID)
	carrier.Set(tracing.HeaderRequestID, traceID)
}

func (legacyTraceIDPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}

	traceIDValue := carrier.Get(tracing.HeaderTraceID)
	if traceIDValue == "" {
		traceIDValue = carrier.Get(tracing.HeaderRequestID)
	}
	traceID, err := trace.TraceIDFromHex(traceIDValue)
	if err != nil {
		return ctx
	}
	spanID, err := randomSpanID()
	if err != nil {
		return ctx
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(ctx, spanContext)
}

func (legacyTraceIDPropagator) Fields() []string {
	return []string{tracing.HeaderTraceID, tracing.HeaderRequestID}
}

func randomSpanID() (trace.SpanID, error) {
	var spanID trace.SpanID
	if _, err := rand.Read(spanID[:]); err != nil {
		return trace.SpanID{}, err
	}
	if !spanID.IsValid() {
		return randomSpanID()
	}
	return spanID, nil
}
