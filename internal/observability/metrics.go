package observability

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics 聚合网关暴露给 Prometheus 的观测指标。
type Metrics struct {
	// Registry 存放网关使用的 Prometheus 指标注册表。
	Registry *prometheus.Registry
	// RequestsTotal 记录模型后端请求总数。
	RequestsTotal *prometheus.CounterVec
	// RequestDuration 记录模型后端请求总耗时分布。
	RequestDuration *prometheus.HistogramVec
	// RequestSuccessRate 记录模型后端请求的累计成功率。
	RequestSuccessRate *prometheus.GaugeVec
	// FirstTokenDuration 记录首个内容 token 返回前的耗时分布。
	FirstTokenDuration *prometheus.HistogramVec

	requestStats *requestMetricStats
}

// NewMetrics 创建并注册网关观测指标集合。
func NewMetrics(namespace string, ready func() bool) *Metrics {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "gateway"
	}

	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		Registry: registry,
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "requests_total",
				Help:      "Total number of model backend requests.",
			},
			[]string{"backend", "result"},
		),

		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Total duration of model backend requests.",
				Buckets: []float64{
					0.05, 0.1, 0.2, 0.3,
					0.5, 0.8, 1,
					1.5, 2, 3, 5, 10, 20, 30, 60, 120,
				},
			},
			[]string{"backend", "result"},
		),

		RequestSuccessRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "request_success_rate",
				Help:      "Cumulative success rate of model backend requests since process start.",
			},
			[]string{"backend"},
		),

		FirstTokenDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "aegis",
				Name:      "first_token_duration_seconds",
				Help:      "Duration before first content token is flushed.",
				Buckets: []float64{
					0.05, 0.1, 0.2, 0.3,
					0.5, 0.8, 1,
					1.5, 2, 3, 5, 10, 20, 30,
				},
			},
			[]string{
				"model",
			},
		),

		requestStats: newRequestMetricStats(),
	}

	registry.MustRegister(
		metrics.FirstTokenDuration,
		metrics.RequestDuration,
		metrics.RequestSuccessRate,
		metrics.RequestsTotal,
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "up",
			Help:      "Whether the gateway process is up.",
		}, func() float64 {
			return 1
		}),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "ready",
			Help:      "Whether the gateway is ready to serve traffic.",
		}, func() float64 {
			if ready != nil && ready() {
				return 1
			}
			return 0
		}),
	)

	return metrics
}

// registerMetrics 将 Prometheus 指标处理器注册到 HTTP 多路复用器。
func registerMetrics(mux *http.ServeMux, metrics *Metrics) {
	if metrics == nil || metrics.Registry == nil {
		metrics = NewMetrics("gateway", nil)
	}
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
}
