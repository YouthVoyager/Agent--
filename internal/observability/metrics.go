package observability

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry      *prometheus.Registry
	RequestsTotal *prometheus.CounterVec
}

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
	}

	registry.MustRegister(
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

func registerMetrics(mux *http.ServeMux, metrics *Metrics) {
	if metrics == nil || metrics.Registry == nil {
		metrics = NewMetrics("gateway", nil)
	}
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
}
