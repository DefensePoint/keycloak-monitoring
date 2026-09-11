package fx

import (
	"go.uber.org/fx"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/version"
	"github.com/DefensePoint/keycloak-monitoring/notifications"
	"github.com/DefensePoint/keycloak-monitoring/tenant"
)

// MetricsModule provides the Prometheus registry and wires the /metrics endpoint.
var MetricsModule = fx.Module("metrics",
	fx.Provide(
		provideMetrics,
	),
	fx.Invoke(
		attachMetricsRecorders,
	),
)

func provideMetrics() *metrics.Registry {
	m := metrics.New()
	m.SetBuildInfo(version.Version, version.GitCommit)
	return m
}

// metricsConsumer is implemented by domain services that accept a recorder.
type metricsConsumer interface {
	SetMetrics(metrics.Recorder)
}

// attachMetricsRecorders hands the registry to every domain service that
// records metrics. Services are attached after construction so the domain
// packages stay free of Fx dependencies.
func attachMetricsRecorders(
	m *metrics.Registry,
	pool *tenant.MonitorPoolManager,
	alertsSvc alerts.Service,
	notifySvc notifications.Service,
) {
	pool.SetMetrics(m)

	for _, svc := range []any{alertsSvc, notifySvc} {
		if c, ok := svc.(metricsConsumer); ok {
			c.SetMetrics(m)
		}
	}
}
