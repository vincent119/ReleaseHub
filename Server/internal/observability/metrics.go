package observability

import (
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetrics stores low-cardinality HTTP request metrics.
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewHTTPMetrics registers ReleaseHub HTTP metrics with the given registry.
func NewHTTPMetrics(registry prometheus.Registerer) (*HTTPMetrics, error) {
	metrics := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "releasehub",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of ReleaseHub HTTP requests.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "releasehub",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "ReleaseHub HTTP request latency in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	if err := registry.Register(metrics.requests); err != nil {
		return nil, fmt.Errorf("register HTTP request counter: %w", err)
	}
	if err := registry.Register(metrics.duration); err != nil {
		return nil, fmt.Errorf("register HTTP request duration: %w", err)
	}
	return metrics, nil
}

// Observe records one HTTP request; route must be a template without user IDs.
func (m *HTTPMetrics) Observe(method, route string, status int, elapsed time.Duration) {
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}
