package fx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	mcpserver "github.com/DefensePoint/keycloak-monitoring/internal/mcp"
)

// fakeTenantReader is a minimal tenant.Reader for exercising the MCP
// assembly's startup registration of database-defined AMFA tenants.
type fakeTenantReader struct {
	tenants []*domain.KeycloakTenant
	listErr error
}

func (f *fakeTenantReader) GetByID(ctx context.Context, id uint) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func (f *fakeTenantReader) GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func (f *fakeTenantReader) List(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return f.tenants, f.listErr
}

func (f *fakeTenantReader) ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return f.tenants, f.listErr
}

func (f *fakeTenantReader) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func TestRegisterDatabaseAmfaTenantsRegistersAPITenant(t *testing.T) {
	registry := amfa.NewRegistry()
	reader := &fakeTenantReader{
		tenants: []*domain.KeycloakTenant{dbTenant("db-t1", true, "https://amfa.internal")},
	}

	registerDatabaseAmfaTenants(registry, reader, &config.AppConfig{}, nil)

	if _, err := registry.RepositoryFor("db-t1"); err != nil {
		t.Fatalf("RepositoryFor: %v", err)
	}
}

func TestRegisterDatabaseAmfaTenantsSkipsConfigDefined(t *testing.T) {
	registry := amfa.NewRegistry()
	configDefined := dbTenant("cfg-t1", true, "https://amfa.internal")
	configDefined.IsConfigDefined = true
	reader := &fakeTenantReader{tenants: []*domain.KeycloakTenant{configDefined}}

	registerDatabaseAmfaTenants(registry, reader, &config.AppConfig{}, nil)

	if _, err := registry.RepositoryFor("cfg-t1"); err == nil {
		t.Fatal("config-defined tenant must not register from the database")
	}
}

func TestRegisterDatabaseAmfaTenantsSkipsBrokenTenantKeepsRest(t *testing.T) {
	registry := amfa.NewRegistry()
	reader := &fakeTenantReader{tenants: []*domain.KeycloakTenant{
		dbTenant("bad", true, "not-a-url"),
		dbTenant("good", true, "https://amfa.internal"),
	}}

	registerDatabaseAmfaTenants(registry, reader, &config.AppConfig{}, nil)

	if _, err := registry.RepositoryFor("bad"); err == nil {
		t.Error("tenant with an unusable base URL must not register")
	}
	if _, err := registry.RepositoryFor("good"); err != nil {
		t.Errorf("valid tenant should register despite a broken sibling: %v", err)
	}
}

func TestRegisterDatabaseAmfaTenantsToleratesListError(t *testing.T) {
	registry := amfa.NewRegistry()
	reader := &fakeTenantReader{listErr: errors.New("database unreachable")}

	registerDatabaseAmfaTenants(registry, reader, &config.AppConfig{}, nil)

	if ids := registry.TenantIDs(); len(ids) != 0 {
		t.Fatalf("registry = %v, want empty when the tenant list fails", ids)
	}
}

func TestProvideMCPServerRequiresACursorKey(t *testing.T) {
	cfg := &config.AppConfig{MCP: config.MCPConfig{CursorHMACKey: "too-short"}}

	_, err := provideMCPServer(cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("a cursor key shorter than the minimum must fail startup")
	}
	if !strings.Contains(err.Error(), "mcp.cursor_hmac_key") {
		t.Fatalf("error = %v, want it to name mcp.cursor_hmac_key", err)
	}

	cfg.MCP.CursorHMACKey = strings.Repeat("k", mcpserver.MinCursorKeyLen)
	if _, err := provideMCPServer(cfg, logger.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("a long enough cursor key must start: %v", err)
	}
}

func TestProvideMCPServerRejectsARemovedPageSizeKey(t *testing.T) {
	cfg := &config.AppConfig{MCP: config.MCPConfig{
		CursorHMACKey: strings.Repeat("k", mcpserver.MinCursorKeyLen),
		RemovedKeys:   []string{"mcp.absolute_max_page_size"},
	}}

	_, err := provideMCPServer(cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("a config still carrying mcp.absolute_max_page_size must fail startup")
	}
	for _, want := range []string{"mcp.default_page_size", "mcp.max_page_size"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

func TestProvideMCPServerRequiresAMetricsToken(t *testing.T) {
	cfg := &config.AppConfig{MCP: config.MCPConfig{
		CursorHMACKey: strings.Repeat("k", mcpserver.MinCursorKeyLen),
		Metrics:       config.MetricsConfig{Enabled: true},
	}}

	_, err := provideMCPServer(cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("an enabled /metrics without a token must fail startup")
	}
	if !strings.Contains(err.Error(), "mcp.metrics.auth_token") {
		t.Fatalf("error = %v, want it to name mcp.metrics.auth_token", err)
	}

	cfg.MCP.Metrics.AuthToken = "scrape-token"
	if _, err := provideMCPServer(cfg, logger.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("a token-guarded /metrics must start: %v", err)
	}
}
