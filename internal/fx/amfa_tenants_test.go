package fx

import (
	"context"
	"testing"
	"time"

	"go.uber.org/fx/fxtest"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/tenant"
)

// fakeAmfaTenantService is a minimal tenant.Service for exercising
// registerAmfaTenantSync's Fx wiring specifically: whether its OnStart hook
// actually loads and registers database tenants, independent of any other
// module's invoke/hook ordering.
type fakeAmfaTenantService struct {
	tenants        []*domain.KeycloakTenant
	loadTenantsErr error
	loadCalls      int
	callback       tenant.ChangeCallback
}

func (f *fakeAmfaTenantService) LoadTenants(ctx context.Context) error {
	f.loadCalls++
	return f.loadTenantsErr
}
func (f *fakeAmfaTenantService) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return f.tenants, nil
}
func (f *fakeAmfaTenantService) ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return f.tenants, nil
}
func (f *fakeAmfaTenantService) GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) CreateTenant(ctx context.Context, req *tenant.CreateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) UpdateTenant(ctx context.Context, tenantID string, req *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) DeleteTenant(ctx context.Context, tenantID string) error {
	return nil
}
func (f *fakeAmfaTenantService) SyncFromConfig(ctx context.Context, tenantID string, createReq *tenant.CreateRequest, updateReq *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) UnmarkConfigDefined(ctx context.Context, tenantID string) error {
	return nil
}
func (f *fakeAmfaTenantService) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	return nil
}
func (f *fakeAmfaTenantService) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	return nil
}
func (f *fakeAmfaTenantService) GetHealth(ctx context.Context, tenantID string) (*tenant.HealthStatus, error) {
	return nil, nil
}
func (f *fakeAmfaTenantService) RegisterCallback(callback tenant.ChangeCallback) {
	f.callback = callback
}
func (f *fakeAmfaTenantService) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func dbTenant(id string, amfaOn bool, baseURL string) *domain.KeycloakTenant {
	t := &domain.KeycloakTenant{
		TenantID:     id,
		Name:         id,
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "monitoring",
		ClientSecret: "secret",
		Enabled:      true,
	}
	if amfaOn || baseURL != "" {
		t.Amfa = &domain.TenantAmfa{Enabled: amfaOn, APIBaseURL: baseURL}
	}
	return t
}

func newSync() (*amfaTenantSync, *amfa.Registry) {
	reg := amfa.NewRegistry()
	return &amfaTenantSync{registry: reg, cfg: &config.AppConfig{}}, reg
}

func TestApplyRegistersAPITenantWithAmfa(t *testing.T) {
	s, reg := newSync()
	if !s.apply(dbTenant("t1", true, "https://amfa.internal")) {
		t.Fatal("tenant with AMFA should register")
	}
	if _, err := reg.RepositoryFor("t1"); err != nil {
		t.Errorf("RepositoryFor: %v", err)
	}
}

func TestApplySkipsConfigDefinedTenant(t *testing.T) {
	// config.yaml is authoritative for these; re-registering from a database
	// copy could override the file with stale values.
	s, reg := newSync()
	tn := dbTenant("t1", true, "https://amfa.internal")
	tn.IsConfigDefined = true

	if s.apply(tn) {
		t.Error("config-defined tenant should not register from the database")
	}
	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("expected no repository for a config-defined tenant")
	}
}

func TestApplyUnregistersWhenAmfaDisabled(t *testing.T) {
	s, reg := newSync()
	s.apply(dbTenant("t1", true, "https://amfa.internal"))

	if s.apply(dbTenant("t1", false, "https://amfa.internal")) {
		t.Error("disabled AMFA should not stay registered")
	}
	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("repository should be gone once AMFA is disabled")
	}
}

func TestApplyUnregistersWhenTenantDisabled(t *testing.T) {
	// A disabled tenant must stop being polled even with AMFA still configured.
	s, reg := newSync()
	s.apply(dbTenant("t1", true, "https://amfa.internal"))

	tn := dbTenant("t1", true, "https://amfa.internal")
	tn.Enabled = false
	s.apply(tn)

	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("repository should be gone once the tenant is disabled")
	}
}

func TestApplyIgnoresTenantWithoutAmfa(t *testing.T) {
	s, reg := newSync()
	if s.apply(dbTenant("t1", false, "")) {
		t.Error("tenant without AMFA should not register")
	}
	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("expected no repository")
	}
}

func TestApplyRejectsUnusableBaseURL(t *testing.T) {
	s, reg := newSync()
	if s.apply(dbTenant("t1", true, "not-a-url")) {
		t.Error("invalid base_url should not register")
	}
	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("expected no repository for an invalid URL")
	}
}

func TestRemovalUnregisters(t *testing.T) {
	// The endpoint a tenant pointed at must stop being queried the moment the
	// tenant is gone.
	s, reg := newSync()
	s.apply(dbTenant("t1", true, "https://amfa.internal"))

	s.handleTenantChange(tenant.EventRemoved, dbTenant("t1", true, "https://amfa.internal"))
	if _, err := reg.RepositoryFor("t1"); err == nil {
		t.Error("repository should be gone after removal")
	}
}

func TestUpdateReRegistersWithTheNewEndpoint(t *testing.T) {
	s, reg := newSync()
	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://old.internal"))
	s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", true, "https://new.internal"))

	if _, err := reg.RepositoryFor("t1"); err != nil {
		t.Errorf("tenant should still be registered after an update: %v", err)
	}
}

func TestNilTenantIsIgnored(t *testing.T) {
	s, _ := newSync()
	s.handleTenantChange(tenant.EventAdded, nil)
	if s.apply(nil) {
		t.Error("nil tenant should not register")
	}
}

func TestTenantConfigConversionCarriesCredentialsAndTimeout(t *testing.T) {
	tn := dbTenant("t1", true, "https://amfa.internal")
	tn.Amfa.EventsLookbackDays = 14
	tn.Amfa.APITimeoutSeconds = 45

	got := amfaTenantConfigFrom(tn)
	if got.ClientID != "monitoring" || got.ClientSecret != "secret" {
		t.Error("Keycloak credentials must carry through: tokens come from the tenant's own Keycloak")
	}
	if got.Amfa.API.Timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", got.Amfa.API.Timeout)
	}
	if got.Amfa.EventsLookbackDays != 14 {
		t.Errorf("lookback = %d, want 14", got.Amfa.EventsLookbackDays)
	}
}

func TestZeroTimeoutStaysUnsetSoDefaultsApply(t *testing.T) {
	tn := dbTenant("t1", true, "https://amfa.internal")
	if got := amfaTenantConfigFrom(tn); got.Amfa.API.Timeout != 0 {
		t.Errorf("timeout = %v, want 0 so Validate applies the default", got.Amfa.API.Timeout)
	}
}

// TestRegisterAmfaTenantSyncLoadsDatabaseTenantsOnStart exercises the actual
// Fx wiring, not just amfaTenantSync's methods directly: a database tenant
// with AMFA configured must be registered once the app's OnStart hooks run,
// without depending on any other module having loaded the tenant cache
// first.
func TestRegisterAmfaTenantSyncLoadsDatabaseTenantsOnStart(t *testing.T) {
	svc := &fakeAmfaTenantService{
		tenants: []*domain.KeycloakTenant{dbTenant("t1", true, "https://amfa.internal")},
	}
	registry := amfa.NewRegistry()
	lc := fxtest.NewLifecycle(t)

	registerAmfaTenantSync(lc, registry, svc, &config.AppConfig{}, logger.NewNoop())

	if _, err := registry.RepositoryFor("t1"); err == nil {
		t.Fatal("tenant should not be registered before OnStart runs")
	}
	if svc.callback == nil {
		t.Fatal("callback should be registered immediately, not deferred to OnStart")
	}

	lc.RequireStart()
	defer lc.RequireStop()

	if svc.loadCalls == 0 {
		t.Error("OnStart should have loaded the tenant cache itself")
	}
	if _, err := registry.RepositoryFor("t1"); err != nil {
		t.Errorf("tenant should be registered once OnStart has run: %v", err)
	}
}

func TestRegisterAmfaTenantSyncSkipsWhenLoadTenantsFails(t *testing.T) {
	svc := &fakeAmfaTenantService{
		tenants:        []*domain.KeycloakTenant{dbTenant("t1", true, "https://amfa.internal")},
		loadTenantsErr: context.DeadlineExceeded,
	}
	registry := amfa.NewRegistry()
	lc := fxtest.NewLifecycle(t)

	registerAmfaTenantSync(lc, registry, svc, &config.AppConfig{}, logger.NewNoop())
	lc.RequireStart()
	defer lc.RequireStop()

	if _, err := registry.RepositoryFor("t1"); err == nil {
		t.Error("a failed cache load must not be treated as an empty tenant list")
	}
}
