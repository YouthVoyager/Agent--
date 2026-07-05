package tracing

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// HeaderTraceID 是网关对外暴露的简单追踪标识请求头。
	HeaderTraceID = "X-Trace-ID"
	// HeaderRequestID 兼容常见请求 ID 请求头，取值与 trace id 保持一致。
	HeaderRequestID = "X-Request-ID"
	// HeaderTraceParent 是 W3C Trace Context 主请求头。
	HeaderTraceParent = "Traceparent"
	// HeaderTraceState 是 W3C Trace Context 供应商扩展请求头。
	HeaderTraceState = "Tracestate"
)

const defaultTraceFlags = "01"

// NewInfoFromHeaders 从请求头解析追踪信息；不存在或无效时生成新的 trace。
func NewInfoFromHeaders(headers http.Header) Info {
	if info, ok := parseTraceParent(headers.Get(HeaderTraceParent)); ok {
		info.SpanID = newSpanID()
		info.TraceState = traceStateFromHeaders(headers)
		return info
	}

	for _, header := range []string{HeaderTraceID, HeaderRequestID} {
		traceID := normalizeTraceID(headers.Get(header))
		if traceID != "" {
			return Info{
				TraceID:    traceID,
				SpanID:     newSpanID(),
				TraceFlags: defaultTraceFlags,
				TraceState: traceStateFromHeaders(headers),
			}
		}
	}

	return Info{
		TraceID:    newTraceID(),
		SpanID:     newSpanID(),
		TraceFlags: defaultTraceFlags,
		TraceState: traceStateFromHeaders(headers),
	}
}

// Inject 将 context 中的追踪信息注入到出站 HTTP 请求。
func Inject(req *http.Request, ctx context.Context) {
	if req == nil {
		return
	}
	if spanContext := oteltrace.SpanContextFromContext(ctx); spanContext.IsValid() {
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
		traceID := spanContext.TraceID().String()
		req.Header.Set(HeaderTraceID, traceID)
		req.Header.Set(HeaderRequestID, traceID)
		return
	}
	info, ok := FromContext(ctx)
	if !ok {
		return
	}

	child := info
	child.ParentSpanID = info.SpanID
	child.SpanID = newSpanID()

	req.Header.Set(HeaderTraceParent, traceParentValue(child))
	req.Header.Set(HeaderTraceID, info.TraceID)
	req.Header.Set(HeaderRequestID, info.TraceID)
	if info.TraceState != "" {
		req.Header.Set(HeaderTraceState, info.TraceState)
	}
}

func setResponseHeaders(headers http.Header, info Info) {
	headers.Set(HeaderTraceParent, traceParentValue(info))
	headers.Set(HeaderTraceID, info.TraceID)
	headers.Set(HeaderRequestID, info.TraceID)
	if info.TraceState != "" {
		headers.Set(HeaderTraceState, info.TraceState)
	}
}

func traceParentValue(info Info) string {
	flags := strings.ToLower(strings.TrimSpace(info.TraceFlags))
	if !isValidTraceFlags(flags) {
		flags = defaultTraceFlags
	}
	return "00-" + info.TraceID + "-" + info.SpanID + "-" + flags
}

func parseTraceParent(value string) (Info, bool) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 {
		return Info{}, false
	}
	version := strings.ToLower(parts[0])
	traceID := normalizeTraceID(parts[1])
	parentSpanID := normalizeSpanID(parts[2])
	flags := strings.ToLower(parts[3])
	if version == "ff" || len(version) != 2 || !isHex(version) {
		return Info{}, false
	}
	if traceID == "" || parentSpanID == "" || !isValidTraceFlags(flags) {
		return Info{}, false
	}
	return Info{
		TraceID:      traceID,
		ParentSpanID: parentSpanID,
		TraceFlags:   flags,
	}, true
}

func traceStateFromHeaders(headers http.Header) string {
	value := strings.TrimSpace(headers.Get(HeaderTraceState))
	if len(value) > 512 {
		return ""
	}
	return value
}
