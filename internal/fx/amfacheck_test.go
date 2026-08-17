package fx

import (
	"context"
	"testing"
	"time"

	"repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
	"repos.defensepoint.com/iam/oss-pmt/kmt/amfacheck"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
	"repos.defensepoint.com/iam/oss-pmt/kmt/internal/domain"
)

// acFakeAlertStore satisfies amfacheck.AlertStore (the already-adapted store
// that buildAmfaCheckServices consumes).
type acFakeAlertStore struct{}

func (acFakeAlertStore) GetAlertByID(_ context.Context, _, _ string) (*domain.Alert, error) {
	return nil, nil
}
func (acFakeAlertStore) SaveAlert(_ context.Context, _ *domain.Alert) error { return nil }

// acFakeNotifier satisfies amfacheck.NotificationService.
type acFakeNotifier struct{}

func (acFakeNotifier) NotifyAlert(_ context.Context, _ *domain.Alert) error { return nil }

// acRealmRepoStub satisfies realmLister.
type acRealmRepoStub struct{}

func (acRealmRepoStub) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return []*domain.KeycloakRealmInfo{{RealmName: "master"}}, nil
}

// acRepoStub is a no-op amfa.Repository for registry wiring tests.
type acRepoStub struct{}

func (acRepoStub) ListEvents(_ context.Context, _ amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
	return &amfa.ListEventsResult{}, nil
}
func (acRepoStub) GetStats(_ context.Context, _ amfa.StatsOptions) (amfa.Stats, error) {
	return amfa.Stats{}, nil
}
func (acRepoStub) GetGeoBuckets(_ context.Context, _ amfa.GeoOptions) ([]amfa.GeoBucket, error) {
	return nil, nil
}
func (acRepoStub) ListRejectedEventsSince(_ context.Context, _ string, _ time.Time) ([]amfa.EventRow, error) {
	return nil, nil
}
func (acRepoStub) ListVPNRiskyEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return nil, nil
}
func (acRepoStub) CountRepeatedRiskyByUser(_ context.Context, _ string, _ time.Time, _, _ int) ([]amfa.UserRiskyCount, error) {
	return nil, nil
}
func (acRepoStub) CountByEventTypeInWindow(_ context.Context, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}

func (acRepoStub) ListEventsSince(_ context.Context, _ string, _ time.Time, _ int) ([]amfa.EventRow, error) {
	return nil, nil
}

func acRegistry(tenantIDs ...string) *amfa.Registry {
	reg := amfa.NewRegistry()
	for _, id := range tenantIDs {
		reg.Register(id, acRepoStub{})
	}
	return reg
}

// compile-time: the adapters satisfy the amfacheck interfaces.
var (
	_ amfacheck.AlertStore          = acFakeAlertStore{}
	_ amfacheck.NotificationService = acFakeNotifier{}
	_ realmLister                   = acRealmRepoStub{}
)

func TestBuildAmfaCheckServices_SkipsWhenGloballyDisabled(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaChecker.Enabled = false
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
	}

	svcs := buildAmfaCheckServices(cfg, acRegistry("t1"), acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil)
	if len(svcs) != 0 {
		t.Errorf("expected 0 services when globally disabled, got %d", len(svcs))
	}
}

func TestBuildAmfaCheckServices_OnePerEnabledTenant(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.AmfaChecker.Enabled = true
	cfg.Keycloak.Global.AmfaChecker.Checks.RiskRejected.Enabled = true
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Amfa: &config.AmfaTenantConfig{Enabled: true}},
		"t2": {Amfa: &config.AmfaTenantConfig{Enabled: false}},
		"t3": {Amfa: nil},
	}

	svcs := buildAmfaCheckServices(cfg, acRegistry("t1", "t2", "t3"), acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil)
	if len(svcs) != 1 {
		t.Errorf("expected 1 service (only t1), got %d", len(svcs))
	}
}
