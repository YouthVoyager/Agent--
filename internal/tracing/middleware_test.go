package tracing

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareGeneratesTraceHeaders(t *testing.T) {
	handler := Middleware(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := TraceIDFromContext(r.Context()); got == "" {
			t.Fatal("context 缺少 trace id")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(recorder, request)

	traceID := recorder.Header().Get(HeaderTraceID)
	if len(traceID) != 32 {
		t.Fatalf("X-Trace-ID = %q, want 32 位 trace id", traceID)
	}
	if got := recorder.Header().Get(HeaderRequestID); got != traceID {
		t.Fatalf("X-Request-ID = %q, want %q", got, traceID)
	}
	if got := recorder.Header().Get(HeaderTraceParent); !strings.Contains(got, traceID) {
		t.Fatalf("Traceparent = %q, want 包含 trace id %q", got, traceID)
	}
}

func TestMiddlewareContinuesIncomingTraceParent(t *testing.T) {
	incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	handler := Middleware(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("context 缺少追踪信息")
		}
		if info.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("trace id = %q", info.TraceID)
		}
		if info.ParentSpanID != "00f067aa0ba902b7" {
			t.Fatalf("parent span id = %q", info.ParentSpanID)
		}
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	request.Header.Set(HeaderTraceParent, incoming)
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(HeaderTraceID); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("X-Trace-ID = %q", got)
	}
	if got := recorder.Header().Get(HeaderTraceParent); got == incoming {
		t.Fatalf("响应 Traceparent 应使用网关当前 span，实际仍为入站值 %q", got)
	}
}

func TestInjectPropagatesChildTraceParent(t *testing.T) {
	info := Info{
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:     "00f067aa0ba902b7",
		TraceFlags: "01",
		TraceState: "vendor=value",
	}
	request := httptest.NewRequest(http.MethodPost, "http://upstream.test/v1/chat/completions", nil)

	Inject(request, WithInfo(request.Context(), info))

	if got := request.Header.Get(HeaderTraceID); got != info.TraceID {
		t.Fatalf("X-Trace-ID = %q", got)
	}
	traceParent := request.Header.Get(HeaderTraceParent)
	if !strings.Contains(traceParent, info.TraceID) {
		t.Fatalf("Traceparent = %q, want 包含 trace id %q", traceParent, info.TraceID)
	}
	if strings.Contains(traceParent, info.SpanID) {
		t.Fatalf("出站 Traceparent 应使用子 span，实际仍包含父 span: %q", traceParent)
	}
	if got := request.Header.Get(HeaderTraceState); got != info.TraceState {
		t.Fatalf("Tracestate = %q", got)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
