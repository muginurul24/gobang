package observability

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	httpRequestsTotal     *prometheus.CounterVec
	httpRequestDuration   *prometheus.HistogramVec
	upstreamDuration      *prometheus.HistogramVec
	cacheLookupsTotal     *prometheus.CounterVec
	callbackQueueDepth    prometheus.Gauge
	reconcileBacklogGauge *prometheus.GaugeVec
	websocketConnections  prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	metrics := &Metrics{
		registry: registry,
		httpRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "onixggr_http_requests_total",
			Help: "Total number of HTTP requests handled by the API.",
		}, []string{"method", "status"}),
		httpRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "onixggr_http_request_duration_seconds",
			Help:    "Latency distribution for HTTP requests handled by the API.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "status"}),
		upstreamDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "onixggr_upstream_request_duration_seconds",
			Help:    "Latency distribution for upstream provider requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider", "operation", "result"}),
		cacheLookupsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "onixggr_cache_lookups_total",
			Help: "Total number of cache lookups grouped by cache name and result.",
		}, []string{"cache", "result"}),
		callbackQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "onixggr_callback_queue_depth",
			Help: "Current depth of the outbound callback queue.",
		}),
		reconcileBacklogGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "onixggr_reconcile_backlog",
			Help: "Current backlog for reconciliation workers grouped by queue type.",
		}, []string{"queue"}),
		websocketConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "onixggr_websocket_connections",
			Help: "Current number of active websocket dashboard connections.",
		}),
	}

	registry.MustRegister(
		metrics.httpRequestsTotal,
		metrics.httpRequestDuration,
		metrics.upstreamDuration,
		metrics.cacheLookupsTotal,
		metrics.callbackQueueDepth,
		metrics.reconcileBacklogGauge,
		metrics.websocketConnections,
	)

	return metrics
}

func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}

	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveHTTPRequest(method string, status int, duration time.Duration) {
	if m == nil {
		return
	}

	statusLabel := strconv.Itoa(status)
	methodLabel := strings.ToUpper(strings.TrimSpace(method))
	m.httpRequestsTotal.WithLabelValues(methodLabel, statusLabel).Inc()
	m.httpRequestDuration.WithLabelValues(methodLabel, statusLabel).Observe(duration.Seconds())
}

func (m *Metrics) ObserveUpstream(provider string, operation string, result string, duration time.Duration) {
	if m == nil {
		return
	}

	providerLabel := strings.TrimSpace(provider)
	if providerLabel == "" {
		providerLabel = "unknown"
	}

	operationLabel := strings.TrimSpace(operation)
	if operationLabel == "" {
		operationLabel = "unknown"
	}

	resultLabel := strings.TrimSpace(result)
	if resultLabel == "" {
		resultLabel = "unknown"
	}

	m.upstreamDuration.WithLabelValues(providerLabel, operationLabel, resultLabel).Observe(duration.Seconds())
}

func (m *Metrics) ObserveCacheLookup(cache string, result string) {
	if m == nil {
		return
	}

	cacheLabel := strings.TrimSpace(cache)
	if cacheLabel == "" {
		cacheLabel = "unknown"
	}

	resultLabel := strings.TrimSpace(result)
	if resultLabel == "" {
		resultLabel = "unknown"
	}

	m.cacheLookupsTotal.WithLabelValues(cacheLabel, resultLabel).Inc()
}

func (m *Metrics) SetCallbackQueueDepth(depth int) {
	if m == nil {
		return
	}

	m.callbackQueueDepth.Set(float64(depth))
}

func (m *Metrics) SetReconcileBacklog(queue string, depth int) {
	if m == nil {
		return
	}

	queueLabel := strings.TrimSpace(queue)
	if queueLabel == "" {
		queueLabel = "unknown"
	}

	m.reconcileBacklogGauge.WithLabelValues(queueLabel).Set(float64(depth))
}

func (m *Metrics) SetWebsocketConnections(count int) {
	if m == nil {
		return
	}

	m.websocketConnections.Set(float64(count))
}
