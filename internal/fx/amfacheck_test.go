package fx

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/amfa"
	"github.com/DefensePoint/keycloak-monitoring/amfacheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
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
func (acRepoStub) CountByEventTypeAndClientInWindow(_ context.Context, _, _ string, _, _ time.Time, _ int) ([]amfa.ClientEventCount, error) {
	return nil, nil
}
func (acRepoStub) CountDistinctRejectedUsersSince(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}
func (acRepoStub) CountByEventTypeByUser(_ context.Context, _, _ string, _ time.Time, _ int) ([]amfa.UserRiskyCount, error) {
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

func TestBuildAmfaChecks_EnablesNewRulesFromConfig(t *testing.T) {
	var amfaCfg config.AmfaCheckerConfig
	amfaCfg.Checks.LoginErrorByClient.Enabled = true
	amfaCfg.Checks.RealmRejectBurst.Enabled = true
	amfaCfg.Checks.LoginErrorByAccount.Enabled = true
	amfaCfg.ApplyDefaults()

	checks := buildAmfaChecks("tenant-1", amfacheck.RealmsFunc(func(context.Context) ([]string, error) { return nil, nil }),
		acRepoStub{}, amfaCfg, noopAmfacheckLogger{})

	want := map[string]bool{
		"amfa-login-error-by-client":  false,
		"amfa-realm-reject-burst":     false,
		"amfa-login-error-by-account": false,
	}
	for _, c := range checks {
		if _, ok := want[c.GetCheckType()]; ok {
			want[c.GetCheckType()] = true
		}
	}
	for checkType, found := range want {
		if !found {
			t.Errorf("expected check %q to be present when its config flag is enabled", checkType)
		}
	}
}

func TestBuildAmfaChecks_NewRulesAbsentWhenDisabled(t *testing.T) {
	var amfaCfg config.AmfaCheckerConfig
	amfaCfg.ApplyDefaults() // all three new checks default to Enabled: false

	checks := buildAmfaChecks("tenant-1", amfacheck.RealmsFunc(func(context.Context) ([]string, error) { return nil, nil }),
		acRepoStub{}, amfaCfg, noopAmfacheckLogger{})

	disallowed := map[string]bool{
		"amfa-login-error-by-client":  true,
		"amfa-realm-reject-burst":     true,
		"amfa-login-error-by-account": true,
	}
	for _, c := range checks {
		if disallowed[c.GetCheckType()] {
			t.Errorf("check %q present despite being disabled by default", c.GetCheckType())
		}
	}
}
