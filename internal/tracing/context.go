package tracing

import "context"

type contextKey struct{}

// Info 保存一次请求链路中的追踪标识。
type Info struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	TraceFlags   string
	TraceState   string
}

// WithInfo 将追踪信息写入 context。
func WithInfo(ctx context.Context, info Info) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, info)
}

// FromContext 从 context 读取追踪信息。
func FromContext(ctx context.Context) (Info, bool) {
	if ctx == nil {
		return Info{}, false
	}
	info, ok := ctx.Value(contextKey{}).(Info)
	if !ok || info.TraceID == "" || info.SpanID == "" {
		return Info{}, false
	}
	return info, true
}

// TraceIDFromContext 从 context 读取 trace id。
func TraceIDFromContext(ctx context.Context) string {
	info, ok := FromContext(ctx)
	if !ok {
		return ""
	}
	return info.TraceID
}
