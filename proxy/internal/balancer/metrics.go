package balancer

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	requestCount        *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	backendUpGauge      *prometheus.GaugeVec
	activeConnections   *prometheus.GaugeVec
	backendResponseTime *prometheus.HistogramVec
	backendErrors       *prometheus.CounterVec
}

func NewMetrics(namespace string) *Metrics {
	m := &Metrics{
		requestCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "request_count_total",
				Help:      "Total number of requests handled by the load balancer",
			},
			[]string{"backend", "status", "method"},
		),
		requestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "request_duration_seconds",
				Help:      "Request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"backend"},
		),
		backendUpGauge: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "backend_up",
				Help:      "Whether the backend is up (1) or down (0)",
			},
			[]string{"backend"},
		),
		activeConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "backend_connections_active",
				Help:      "Number of active connections per backend",
			},
			[]string{"backend"},
		),
		backendResponseTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "backend_response_seconds",
				Help:      "Backend response time in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"backend"},
		),
		backendErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "backend_errors_total",
				Help:      "Total number of backend errors",
			},
			[]string{"backend", "error_type"},
		),
	}

	// Register metrics
	prometheus.MustRegister(m.requestCount)
	prometheus.MustRegister(m.requestDuration)
	prometheus.MustRegister(m.backendUpGauge)
	prometheus.MustRegister(m.activeConnections)
	prometheus.MustRegister(m.backendResponseTime)
	prometheus.MustRegister(m.backendErrors)

	return m
}

// metricsResponseWriter wraps http.ResponseWriter to capture the status code
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader intercepts the status code
func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
