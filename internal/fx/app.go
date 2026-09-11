package fx

import (
	"go.uber.org/fx"
)

// NewApp creates a new Fx application with all modules
func NewApp(configPath string, opts ...fx.Option) *fx.App {
	baseOpts := []fx.Option{
		// Provide config path
		fx.Supply(ConfigPath(configPath)),

		// Core modules
		ConfigModule,
		LoggerModule,
		DatabaseModule,

		// Repository modules (provides all repositories from pkg/database)
		RepositoriesModule,

		// Domain modules (provides services and domain-specific adapters)
		KeycloakModule,
		TenantModule,
		AlertsModule,
		AmfaModule,
		OperatorModule,
		RBACModule,
		APITokenModule,
		NotificationsModule,
		ReportsModule,
		ConfigCheckModule,
		EventsCheckModule,
		AmfaCheckModule,
		AmfaMirrorModule,
		AuthModule,

		// Bootstrap module (seeds RBAC, creates default admin, syncs tenants from config)
		// This MUST run before the server starts to ensure initial data exists
		BootstrapModule,

		// Handler modules (provides all HTTP handlers)
		HandlersModule,

		// Metrics module (Prometheus registry + /metrics endpoint)
		MetricsModule,

		// HTTP Server module
		ServerModule,

		// Monitoring module (MonitorPoolManager - starts goroutines for each tenant)
		// This is the CORE of the application that collects data from Keycloak instances
		MonitoringModule,
	}

	// Append any additional options
	allOpts := append(baseOpts, opts...)

	return fx.New(allOpts...)
}

// NewStandaloneApp creates a standalone app (current mode - server with all features)
func NewStandaloneApp(configPath string, opts ...fx.Option) *fx.App {
	return NewApp(configPath, opts...)
}

// NewMCPApp creates the MCP server app: a read-only assembly with no
// monitoring, no bootstrap and no schema migrations. It wires only the
// dependencies the MCP server needs (apitoken, rbac, tenant reader, users).
func NewMCPApp(configPath string, opts ...fx.Option) *fx.App {
	baseOpts := []fx.Option{
		fx.Supply(ConfigPath(configPath)),

		ConfigModule,
		LoggerModule,

		APITokenModule,
		RBACModule,

		// MCPModule provides the read-only database client this whole
		// assembly runs on, in place of DatabaseModule.
		MCPModule,
	}

	allOpts := append(baseOpts, opts...)

	return fx.New(allOpts...)
}

// NewAgentApp creates an agent app (future - only monitoring, sends to control plane)
// func NewAgentApp(configPath string, opts ...fx.Option) *fx.App {
//     return NewApp(configPath, append(opts,
//         IngestersModule, // Uses HTTPIngester instead of LocalIngester
//     )...)
// }

// NewControlPlaneApp creates a control plane app (future - receives data from agents)
// func NewControlPlaneApp(configPath string, opts ...fx.Option) *fx.App {
//     return NewApp(configPath, append(opts,
//         IngestHandlersModule, // Additional endpoints to receive agent data
//     )...)
// }
