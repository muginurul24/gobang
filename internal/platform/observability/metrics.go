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
	webhookEventsTotal    *prometheus.CounterVec
	cacheLookupsTotal     *prometheus.CounterVec
	recentFailuresGauge   *prometheus.GaugeVec
	dependencyUpGauge     *prometheus.GaugeVec
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
		webhookEventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "onixggr_webhook_events_total",
			Help: "Total number of inbound webhook events grouped by provider, kind, and result.",
		}, []string{"provider", "kind", "result"}),
		cacheLookupsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "onixggr_cache_lookups_total",
			Help: "Total number of cache lookups grouped by cache name and result.",
		}, []string{"cache", "result"}),
		recentFailuresGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "onixggr_recent_failures",
			Help: "Rolling recent failure counts grouped by operational signal.",
		}, []string{"signal"}),
		dependencyUpGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "onixggr_dependency_up",
			Help: "Dependency health status where 1 means healthy and 0 means unavailable or degraded.",
		}, []string{"dependency"}),
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
		metrics.webhookEventsTotal,
		metrics.cacheLookupsTotal,
		metrics.recentFailuresGauge,
		metrics.dependencyUpGauge,
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

	providerLabel := sanitizeLabel(provider)
	operationLabel := sanitizeLabel(operation)
	resultLabel := sanitizeLabel(result)
	m.upstreamDuration.WithLabelValues(providerLabel, operationLabel, resultLabel).Observe(duration.Seconds())
}

func (m *Metrics) ObserveWebhook(provider string, kind string, result string) {
	if m == nil {
		return
	}

	providerLabel := sanitizeLabel(provider)
	kindLabel := sanitizeLabel(kind)
	resultLabel := sanitizeLabel(result)
	m.webhookEventsTotal.WithLabelValues(providerLabel, kindLabel, resultLabel).Inc()
}

func (m *Metrics) ObserveCacheLookup(cache string, result string) {
	if m == nil {
		return
	}

	cacheLabel := sanitizeLabel(cache)
	resultLabel := sanitizeLabel(result)
	m.cacheLookupsTotal.WithLabelValues(cacheLabel, resultLabel).Inc()
}

func (m *Metrics) SetRecentFailures(signal string, count int) {
	if m == nil {
		return
	}

	m.recentFailuresGauge.WithLabelValues(sanitizeLabel(signal)).Set(float64(count))
}

func (m *Metrics) SetDependencyUp(name string, up bool) {
	if m == nil {
		return
	}

	value := 0.0
	if up {
		value = 1
	}

	m.dependencyUpGauge.WithLabelValues(sanitizeLabel(name)).Set(value)
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

	queueLabel := sanitizeLabel(queue)
	m.reconcileBacklogGauge.WithLabelValues(queueLabel).Set(float64(depth))
}

func (m *Metrics) SetWebsocketConnections(count int) {
	if m == nil {
		return
	}

	m.websocketConnections.Set(float64(count))
}

func sanitizeLabel(value string) string {
	label := strings.TrimSpace(value)
	if label == "" {
		return "unknown"
	}

	return label
}
