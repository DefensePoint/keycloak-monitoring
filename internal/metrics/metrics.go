// Package metrics provides the Prometheus registry and collectors exposed on
// the /metrics endpoint.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Namespace prefixes every metric exported by this application.
const Namespace = "pmp"

// Registry holds the application's collectors and exposes them to Prometheus.
type Registry struct {
	registry *prometheus.Registry

	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge

	TenantMonitorUp      *prometheus.GaugeVec
	TenantMonitorsActive prometheus.Gauge

	PollCycleDuration *prometheus.HistogramVec
	PollCycleErrors   *prometheus.CounterVec

	EventsCollected   *prometheus.CounterVec
	AlertsTotal       *prometheus.CounterVec
	NotificationsSent *prometheus.CounterVec

	MCPRequests         *prometheus.CounterVec
	MCPToolCalls        *prometheus.CounterVec
	MCPToolCallDuration *prometheus.HistogramVec

	buildInfo *prometheus.GaugeVec
}

// New creates a Registry with all application collectors registered.
func New() *Registry {
	reg := prometheus.NewRegistry()

	m := &Registry{
		registry: reg,

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests by route, method and status.",
		}, []string{"route", "method", "status"}),

		HTTPRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency by route and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),

		HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		}),

		TenantMonitorUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "tenant_monitor_up",
			Help:      "Whether a tenant's Keycloak monitor is running (1) or not (0).",
		}, []string{"tenant_id", "tenant_name"}),

		TenantMonitorsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "tenant_monitors_active",
			Help:      "Number of active tenant monitors.",
		}),

		PollCycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "poll_cycle_duration_seconds",
			Help:      "Duration of a monitor poll cycle by tenant and poll type.",
			Buckets:   []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"tenant_id", "poll_type"}),

		PollCycleErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "poll_cycle_errors_total",
			Help:      "Total number of failed monitor poll cycles.",
		}, []string{"tenant_id", "poll_type"}),

		EventsCollected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "events_collected_total",
			Help:      "Total number of Keycloak events collected.",
		}, []string{"tenant_id", "realm"}),

		AlertsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "alerts_total",
			Help:      "Total number of alerts raised by severity and check type.",
		}, []string{"severity", "check_type"}),

		NotificationsSent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "notifications_sent_total",
			Help:      "Total notification delivery attempts by channel and result.",
		}, []string{"channel", "result"}),

		MCPRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "mcp_requests_total",
			Help:      "Total inbound MCP JSON-RPC requests by method; methods outside the protocol set are labeled other.",
		}, []string{"method"}),

		MCPToolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "mcp_tool_calls_total",
			Help:      "Total MCP tool calls by tool and outcome (allowed, denied, invalid, not_found, error or rate_limited).",
		}, []string{"tool", "outcome"}),

		MCPToolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "mcp_tool_call_duration_seconds",
			Help:      "MCP tool call latency by tool, observed for allowed calls only.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tool"}),

		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "build_info",
			Help:      "Build information for the running binary.",
		}, []string{"version", "commit"}),
	}

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPRequestsInFlight,
		m.TenantMonitorUp,
		m.TenantMonitorsActive,
		m.PollCycleDuration,
		m.PollCycleErrors,
		m.EventsCollected,
		m.AlertsTotal,
		m.NotificationsSent,
		m.MCPRequests,
		m.MCPToolCalls,
		m.MCPToolCallDuration,
		m.buildInfo,
	)

	return m
}

// SetBuildInfo records the running binary's version and commit.
func (m *Registry) SetBuildInfo(version, commit string) {
	m.buildInfo.WithLabelValues(version, commit).Set(1)
}

// Gatherer exposes the underlying registry for the HTTP handler.
func (m *Registry) Gatherer() prometheus.Gatherer {
	return m.registry
}
