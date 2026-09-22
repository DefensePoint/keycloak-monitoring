package fx

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/apitoken"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/events/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	mcpserver "github.com/DefensePoint/keycloak-monitoring/internal/mcp"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/keycloak"
	kcpostgres "github.com/DefensePoint/keycloak-monitoring/keycloak/postgres"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"github.com/DefensePoint/keycloak-monitoring/pkg/secretcrypto"
	"github.com/DefensePoint/keycloak-monitoring/rbac"
	"github.com/DefensePoint/keycloak-monitoring/tenant"
	tenantpostgres "github.com/DefensePoint/keycloak-monitoring/tenant/postgres"
)

// MCPModule provides the MCP server over a read-only database client.
var MCPModule = fx.Module("mcp",
	fx.Provide(
		provideReadOnlyDatabase,
		provideGormDB,
		provideUserRepository,
		provideMetrics,
		provideMCPTenantReader,
		provideMCPRealmReader,
		provideMCPAlertReader,
		provideMCPAmfaStatsReader,
		provideMCPEventReader,
		provideMCPServer,
	),
	fx.Invoke(registerMCPServerHooks),
)

// provideReadOnlyDatabase opens the database without migrations, under the
// role the required mcp.database block names. Falling back to the main
// database block would run the whole MCP assembly under the read-write
// application role, so an absent block fails startup instead.
func provideReadOnlyDatabase(lc fx.Lifecycle, cfg *config.AppConfig, log *logger.Logger) (*database.Client, error) {
	if cfg.MCP.Database == nil {
		return nil, fmt.Errorf("SECURITY ERROR: mcp.database must name a read-only database role; " +
			"create it with deployments/postgres/mcp-readonly-role.sql")
	}

	dbCfg := *cfg.MCP.Database
	if dbCfg.MaxConns <= 0 {
		dbCfg.MaxConns = 10
	}
	if dbCfg.MinConns < 0 {
		dbCfg.MinConns = 0
	}
	if dbCfg.Timeout < 0 {
		dbCfg.Timeout = 0
	}

	client, err := database.NewReadOnlyClient(&dbCfg, log)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			log.Info("Closing database connection")
			return client.Close()
		},
	})

	return client, nil
}

// provideMCPTenantReader creates a read-only tenant repository. The encryptor
// is built when the key is configured because converting a tenant row fails
// on an encrypted client_secret otherwise, even though the MCP server never
// uses the secret.
func provideMCPTenantReader(db *database.Client, cfg *config.AppConfig, log *logger.Logger) (tenant.Reader, error) {
	var encryptor *secretcrypto.Encryptor
	if cfg.Security.EncryptionKey != "" {
		e, err := secretcrypto.New(cfg.Security.EncryptionKey)
		if err != nil {
			return nil, err
		}
		encryptor = e
	}
	return tenantpostgres.NewRepository(db, encryptor, log), nil
}

// provideMCPRealmReader creates a read-only realm repository: the same rows
// keycloak.Service.ListRealms serves to the web handlers.
func provideMCPRealmReader(db *gorm.DB) keycloak.RealmReader {
	return kcpostgres.NewRealmRepository(db)
}

// provideMCPAlertReader creates a read-only alerts service over the same
// read-only database client the rest of the MCP assembly runs on.
func provideMCPAlertReader(db *gorm.DB, log *logger.Logger) mcpserver.AlertReader {
	return alerts.NewService(alertspostgres.NewRepository(db), log)
}

// provideMCPEventReader creates a read-only events repository over the same
// read-only database client the rest of the MCP assembly runs on.
func provideMCPEventReader(db *gorm.DB, log *logger.Logger) mcpserver.EventReader {
	return eventspostgres.NewRepository(db, log)
}

// provideMCPAmfaStatsReader builds the per-tenant AMFA registry the same way
// the web assembly does and wraps it in a stats-only service. Enrichment is
// nil: GetStats never touches Keycloak user enrichment. Config tenants come
// from config.yaml: one with amfa.api uses the outbound read-only HTTP client,
// and one without it gets a direct GORM connection pool to the tenant's own
// AMFA database, opened in a read-only Postgres session, using the same
// credentials the web assembly uses. Database-defined tenants are then loaded
// through the tenant reader and registered via the same per-tenant path the
// web tenant sync uses. The registry is startup-static: this process registers no
// tenant-change callbacks, so a tenant configured after boot appears on the
// next restart.
func provideMCPAmfaStatsReader(cfg *config.AppConfig, tenants tenant.Reader, log *logger.Logger) mcpserver.AmfaStatsReader {
	registry := buildAmfaRegistry(cfg, cfg.Keycloak.Tenants, log)
	registerDatabaseAmfaTenants(registry, tenants, cfg, log)
	return amfa.NewService(registry, nil)
}

// registerDatabaseAmfaTenants registers every enabled database tenant that has
// AMFA configured, mirroring amfaTenantSync.loadExisting for an assembly that
// reads tenants directly instead of through the tenant cache.
func registerDatabaseAmfaTenants(registry *amfa.Registry, tenants tenant.Reader, cfg *config.AppConfig, log *logger.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), startupLoadTimeout)
	defer cancel()

	rows, err := tenants.ListEnabled(ctx)
	if err != nil {
		logWarn(log, "Could not list tenants for AMFA; database-defined tenants will not expose AMFA stats until the next restart",
			logger.Err(err))
		return
	}

	loaded := 0
	for _, t := range rows {
		if applyAmfaTenant(registry, t, cfg, log) {
			loaded++
		}
	}
	if loaded > 0 {
		logInfo(log, "AMFA registered for database tenants", logger.Int("count", loaded))
	}
}

func provideMCPServer(
	cfg *config.AppConfig,
	log *logger.Logger,
	tokens apitoken.Service,
	rbacService rbac.Service,
	tenants tenant.Reader,
	realms keycloak.RealmReader,
	alertReader mcpserver.AlertReader,
	amfaStats mcpserver.AmfaStatsReader,
	eventReader mcpserver.EventReader,
	m *metrics.Registry,
) (*mcpserver.Server, error) {
	if err := cfg.MCP.RemovedKeyError(); err != nil {
		return nil, err
	}
	if len(cfg.MCP.CursorHMACKey) < mcpserver.MinCursorKeyLen {
		return nil, fmt.Errorf("SECURITY ERROR: mcp.cursor_hmac_key must be at least %d bytes; "+
			"generate one with: openssl rand -base64 32", mcpserver.MinCursorKeyLen)
	}
	// /metrics shares the MCP port and is served outside the auth middleware,
	// so an enabled endpoint without a token is world-readable to anything
	// that can reach the port.
	if cfg.MCP.Metrics.Enabled && cfg.MCP.Metrics.AuthToken == "" {
		return nil, fmt.Errorf("SECURITY ERROR: mcp.metrics.auth_token must be set when mcp.metrics.enabled is true; " +
			"the endpoint is served without authentication otherwise")
	}
	return mcpserver.New(cfg, log, tokens, rbacService, tenants, realms, alertReader, amfaStats, eventReader, m), nil
}

func registerMCPServerHooks(lc fx.Lifecycle, server *mcpserver.Server, log *logger.Logger, sd fx.Shutdowner) {
	// ReadTimeout and WriteTimeout stay 0: they would cut off long-lived
	// streaming responses.
	httpServer := &http.Server{
		Addr:              server.Addr(),
		Handler:           chihttp.RecoveryMiddleware(log)(server.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info("Starting MCP HTTP server", logger.Str("addr", httpServer.Addr))
			go serveOrShutdown("MCP HTTP server", httpServer, log, sd)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Stopping MCP HTTP server")
			if err := httpServer.Shutdown(ctx); err != nil {
				log.Warn("MCP HTTP server graceful shutdown failed, closing held connections",
					logger.Err(err))
				return httpServer.Close()
			}
			return nil
		},
	})
}
