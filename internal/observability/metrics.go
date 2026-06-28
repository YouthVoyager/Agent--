package observability

import "github.com/prometheus/client_golang/prometheus"
type Metrics struct {
	RequestsTotal  *prometheus.CounterVec
}
func NewMetrics() *Metrics {
	m := &Metrics{
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "aegisllm",
				Name:      "requests_total",
				Help:      "Total number of requests.",
			},
			[]string{"method", "result"},
		),
	}

	prometheus.MustRegister(
		m.RequestsTotal,
	)

	return m
}