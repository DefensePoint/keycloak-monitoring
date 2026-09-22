package fx

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// stubTenantService records the calls syncTenant makes so the config-sync
// branches can be asserted on. Only the methods syncTenant touches carry
// behaviour; the rest satisfy tenant.Service. allTenants backs
// ListConfigDefinedTenants/UnmarkConfigDefined for the reconciliation-pass
// tests, independent of the single-tenant live/create/update fields above.
type stubTenantService struct {
	live       *domain.KeycloakTenant
	updateErr  error
	createErr  error
	deleteErr  error
	calls      []string
	allTenants []*domain.KeycloakTenant
	listErr    error
	unmarkErr  error
}

func (s *stubTenantService) record(name string) { s.calls = append(s.calls, name) }

func (s *stubTenantService) GetTenant(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	s.record("GetTenant")
	if s.live == nil {
		return nil, errors.New("tenant not found")
	}
	return s.live, nil
}

func (s *stubTenantService) UpdateTenant(ctx context.Context, tenantID string, req *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	s.record("UpdateTenant")
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return s.live, nil
}

func (s *stubTenantService) CreateTenant(ctx context.Context, req *tenant.CreateRequest) (*domain.KeycloakTenant, error) {
	s.record("CreateTenant")
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &domain.KeycloakTenant{TenantID: req.TenantID}, nil
}

func (s *stubTenantService) DeleteTenant(ctx context.Context, tenantID string) error {
	s.record("DeleteTenant")
	return s.deleteErr
}

func (s *stubTenantService) SyncFromConfig(ctx context.Context, tenantID string, createReq *tenant.CreateRequest, updateReq *tenant.UpdateRequest) (*domain.KeycloakTenant, error) {
	s.record("SyncFromConfig")
	if s.live != nil {
		if s.updateErr != nil {
			return nil, s.updateErr
		}
		s.live.IsConfigDefined = true
		return s.live, nil
	}
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &domain.KeycloakTenant{TenantID: createReq.TenantID, IsConfigDefined: true}, nil
}

func (s *stubTenantService) ListConfigDefinedTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	s.record("ListConfigDefinedTenants")
	if s.listErr != nil {
		return nil, s.listErr
	}
	var configDefined []*domain.KeycloakTenant
	for _, t := range s.allTenants {
		if t.IsConfigDefined {
			configDefined = append(configDefined, t)
		}
	}
	return configDefined, nil
}

func (s *stubTenantService) UnmarkConfigDefined(ctx context.Context, tenantID string) error {
	s.record("UnmarkConfigDefined:" + tenantID)
	if s.unmarkErr != nil {
		return s.unmarkErr
	}
	for _, t := range s.allTenants {
		if t.TenantID == tenantID {
			t.IsConfigDefined = false
		}
	}
	return nil
}

func (s *stubTenantService) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return s.allTenants, nil
}
func (s *stubTenantService) ListEnabledTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
}
func (s *stubTenantService) UpdateHealth(ctx context.Context, tenantID, status, message string) error {
	return nil
}
func (s *stubTenantService) LoadTenants(ctx context.Context) error { return nil }
func (s *stubTenantService) UpdateError(ctx context.Context, tenantID, errorMsg string) error {
	return nil
}
func (s *stubTenantService) GetHealth(ctx context.Context, tenantID string) (*tenant.HealthStatus, error) {
	return nil, nil
}
func (s *stubTenantService) RegisterCallback(callback tenant.ChangeCallback) {}
func (s *stubTenantService) GetDefault(ctx context.Context) (*domain.KeycloakTenant, error) {
	return nil, nil
}

func testCfgTenant() config.TenantConfig {
	return config.TenantConfig{
		Name:         "From Config",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "config-secret",
		Enabled:      true,
	}
}

func params(svc tenant.Service) BootstrapParams {
	return BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: svc,
	}
}

func contains(calls []string, want string) bool {
	for _, c := range calls {
		if c == want {
			return true
		}
	}
	return false
}

// A tenant_id that was never seen before is created normally. This is also
// what now happens to a previously deleted tenant: deletion is a hard delete,
// so there is no row left to restore, and the stub has no way to represent
// "previously deleted" as distinct from "never existed" — both look like an
// empty stub to syncTenant, and both are created.
func TestSyncTenant_CreatesWhenNeverExisted(t *testing.T) {
	svc := &stubTenantService{} // no live row, no deleted row

	if err := syncTenant(context.Background(), params(svc), "t1", testCfgTenant()); err != nil {
		t.Fatalf("syncTenant() unexpected error = %v", err)
	}
	if !contains(svc.calls, "SyncFromConfig") {
		t.Errorf("expected SyncFromConfig, calls = %v", svc.calls)
	}
}

// A live tenant takes the update path and is marked config-defined.
func TestSyncTenant_UpdatesLiveTenant(t *testing.T) {
	live := &domain.KeycloakTenant{TenantID: "t1", Name: "Live"}
	svc := &stubTenantService{live: live}

	if err := syncTenant(context.Background(), params(svc), "t1", testCfgTenant()); err != nil {
		t.Fatalf("syncTenant() unexpected error = %v", err)
	}
	if !contains(svc.calls, "SyncFromConfig") {
		t.Errorf("expected SyncFromConfig, calls = %v", svc.calls)
	}
	if !live.IsConfigDefined {
		t.Errorf("expected tenant to be marked IsConfigDefined after sync")
	}
}

// TestUnlockOrphanedConfigDefinedTenants_UnlocksMissingFromConfig covers
// point 1: a tenant marked IsConfigDefined from a prior sync, but whose
// tenant_id is no longer a key in config.yaml, must have the flag cleared so
// it becomes an ordinary editable/deletable tenant again. Its data must be
// left untouched — only the flag changes.
func TestUnlockOrphanedConfigDefinedTenants_UnlocksMissingFromConfig(t *testing.T) {
	orphan := &domain.KeycloakTenant{TenantID: "removed-tenant", Name: "Removed", IsConfigDefined: true}
	svc := &stubTenantService{allTenants: []*domain.KeycloakTenant{orphan}}
	p := BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: svc,
		Config:        &config.AppConfig{Keycloak: config.KeycloakConfig{Tenants: map[string]config.TenantConfig{}}},
	}

	unlockOrphanedConfigDefinedTenants(context.Background(), p)

	if orphan.IsConfigDefined {
		t.Error("expected orphaned tenant to be unmarked IsConfigDefined")
	}
	if orphan.Name != "Removed" {
		t.Errorf("expected tenant data to be untouched, Name = %q", orphan.Name)
	}
	if !contains(svc.calls, "UnmarkConfigDefined:removed-tenant") {
		t.Errorf("expected UnmarkConfigDefined call for removed-tenant, calls = %v", svc.calls)
	}
}

// A config-defined tenant whose tenant_id is still present in config.yaml
// must be left alone by the reconciliation pass.
func TestUnlockOrphanedConfigDefinedTenants_LeavesActiveTenantsAlone(t *testing.T) {
	active := &domain.KeycloakTenant{TenantID: "still-here", IsConfigDefined: true}
	svc := &stubTenantService{allTenants: []*domain.KeycloakTenant{active}}
	p := BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: svc,
		Config: &config.AppConfig{Keycloak: config.KeycloakConfig{Tenants: map[string]config.TenantConfig{
			"still-here": testCfgTenant(),
		}}},
	}

	unlockOrphanedConfigDefinedTenants(context.Background(), p)

	if !active.IsConfigDefined {
		t.Error("expected a tenant still present in config.yaml to remain IsConfigDefined")
	}
	if contains(svc.calls, "UnmarkConfigDefined:still-here") {
		t.Error("did not expect UnmarkConfigDefined to be called for a tenant still in config.yaml")
	}
}

// syncTenantsFromConfig must run the reconciliation pass even when
// config.yaml defines zero tenants — otherwise removing every tenant from
// config would leave them all orphaned instead of unlocked.
func TestSyncTenantsFromConfig_ReconcilesEvenWithNoConfiguredTenants(t *testing.T) {
	orphan := &domain.KeycloakTenant{TenantID: "removed-tenant", IsConfigDefined: true}
	svc := &stubTenantService{allTenants: []*domain.KeycloakTenant{orphan}}
	p := BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: svc,
		Config:        &config.AppConfig{Keycloak: config.KeycloakConfig{Tenants: map[string]config.TenantConfig{}}},
	}

	if err := syncTenantsFromConfig(context.Background(), p); err != nil {
		t.Fatalf("syncTenantsFromConfig() unexpected error = %v", err)
	}
	if orphan.IsConfigDefined {
		t.Error("expected reconciliation to run and unlock the orphaned tenant even with an empty config.yaml tenants map")
	}
}

// This covers only the nil-DB guard: a nil DB must not panic, and
// drainPurgeQueue should skip the drain and continue. It does not verify
// ordering against syncTenantsFromConfig — the call-site places the drain
// before config sync as defence-in-depth, but that ordering is not a
// correctness requirement, since the drain's time < requested_at bound
// already protects a recreated tenant's fresh telemetry regardless of when
// the drain runs.
func TestDrainPurgeQueueToleratesNilDB(t *testing.T) {
	p := BootstrapParams{
		Logger:        logger.NewNoop(),
		TenantService: &stubTenantService{},
		DB:            nil,
	}
	// drainPurgeQueue reports nothing to its caller; the assertion is that it
	// returns without panicking.
	drainPurgeQueue(context.Background(), p)
}
