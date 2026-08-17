package fx

import (
	"context"
	"errors"
	"testing"

	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
	"repos.defensepoint.com/iam/oss-pmt/kmt/tenant"
)

// stubTenantService records the calls syncTenant makes so the config-sync
// branches can be asserted on. Only the methods syncTenant touches carry
// behaviour; the rest satisfy tenant.Service.
type stubTenantService struct {
	live      *domain.KeycloakTenant
	updateErr error
	createErr error
	deleteErr error
	calls     []string
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

func (s *stubTenantService) ListTenants(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	return nil, nil
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
	if !contains(svc.calls, "CreateTenant") {
		t.Errorf("expected CreateTenant, calls = %v", svc.calls)
	}
}

// A live tenant takes the plain update path.
func TestSyncTenant_UpdatesLiveTenant(t *testing.T) {
	svc := &stubTenantService{
		live: &domain.KeycloakTenant{TenantID: "t1", Name: "Live"},
	}

	if err := syncTenant(context.Background(), params(svc), "t1", testCfgTenant()); err != nil {
		t.Fatalf("syncTenant() unexpected error = %v", err)
	}
	if !contains(svc.calls, "UpdateTenant") {
		t.Errorf("expected UpdateTenant, calls = %v", svc.calls)
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
