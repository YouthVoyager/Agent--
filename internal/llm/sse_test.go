package llm

import (
	"bufio"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-gateway/telemetry-gateway/internal/observability"
	dto "github.com/prometheus/client_model/go"
)

func TestReadSSEEventParsesDataLines(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(": heartbeat\nid: 1\ndata: hello\ndata: world\n\n"))

	event, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("readSSEEvent() error = %v", err)
	}
	if event.Data != "hello\nworld" {
		t.Fatalf("event.Data = %q, want %q", event.Data, "hello\nworld")
	}
}

func TestReadSSEEventReturnsLastEventAtEOF(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("data: tail"))

	event, err := readSSEEvent(reader)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("readSSEEvent() error = %v, want EOF", err)
	}
	if event.Data != "tail" {
		t.Fatalf("event.Data = %q, want tail", event.Data)
	}
}

func TestHasContentToken(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "done",
			data: "[DONE]",
			want: false,
		},
		{
			name: "role only",
			data: `{"choices":[{"delta":{"role":"assistant"},"index":0}]}`,
			want: false,
		},
		{
			name: "empty content",
			data: `{"choices":[{"delta":{"content":""},"index":0}]}`,
			want: false,
		},
		{
			name: "content",
			data: `{"choices":[{"delta":{"content":"你好"},"index":0}]}`,
			want: true,
		},
		{
			name: "later choice content",
			data: `{"choices":[{"delta":{},"index":0},{"delta":{"content":"ok"},"index":1}]}`,
			want: true,
		},
		{
			name: "invalid json",
			data: `not json`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasContentToken(tt.data); got != tt.want {
				t.Fatalf("hasContentToken() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStreamCopyObservesFirstContentTokenOnce(t *testing.T) {
	metrics := observability.NewMetrics("gateway", nil)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(
		": heartbeat\n\n" +
			`data: {"choices":[{"delta":{"role":"assistant"},"index":0}]}` + "\n\n" +
			`data: {"choices":[{"delta":{"content":"first"},"index":0}]}` + "\n\n" +
			`data: {"choices":[{"delta":{"content":"second"},"index":0}]}` + "\n\n" +
			"data: [DONE]\n\n",
	)

	streamCopy(recorder, body, metrics, "real-a", time.Now().Add(-time.Second))

	bodyText := recorder.Body.String()
	if !strings.Contains(bodyText, `"role":"assistant"`) {
		t.Fatalf("SSE 响应未透传 role chunk: %s", bodyText)
	}
	if !strings.Contains(bodyText, `"content":"first"`) || !strings.Contains(bodyText, `"content":"second"`) {
		t.Fatalf("SSE 响应未透传 content chunk: %s", bodyText)
	}
	if !strings.Contains(bodyText, "data: [DONE]") {
		t.Fatalf("SSE 响应未透传 DONE: %s", bodyText)
	}

	count, sum := firstTokenHistogramSample(t, metrics, "real-a")
	if count != 1 {
		t.Fatalf("首 token 统计次数 = %d, want 1", count)
	}
	if sum <= 0 {
		t.Fatalf("首 token 延迟样本和 = %f, want > 0", sum)
	}
}

func firstTokenHistogramSample(t *testing.T, metrics *observability.Metrics, model string) (uint64, float64) {
	t.Helper()

	families, err := metrics.Registry.Gather()
	if err != nil {
		t.Fatalf("Gather metrics error = %v", err)
	}

	for _, family := range families {
		if family.GetName() != "aegis_first_token_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabelValue(metric, "model") != model || metric.GetHistogram() == nil {
				continue
			}
			histogram := metric.GetHistogram()
			return histogram.GetSampleCount(), histogram.GetSampleSum()
		}
	}

	return 0, 0
}

func metricLabelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
