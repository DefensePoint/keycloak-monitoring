package fx

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/eventscheck"
)

type ecFakeAlertStore struct{}

func (ecFakeAlertStore) GetAlertByID(_ context.Context, _, _ string) (*domain.Alert, error) {
	return nil, nil
}
func (ecFakeAlertStore) SaveAlert(_ context.Context, _ *domain.Alert) error { return nil }
func (ecFakeAlertStore) ListOpenAlertsByCheck(context.Context, string, string) ([]*domain.Alert, error) {
	return nil, nil
}
func (ecFakeAlertStore) ResolveAlert(context.Context, string, string) error { return nil }

type ecFakeNotifier struct{}

func (ecFakeNotifier) NotifyAlert(_ context.Context, _ *domain.Alert) error { return nil }
func (ecFakeNotifier) NotifyAlertResolution(_ context.Context, _ *domain.Alert) error {
	return nil
}

type ecRealmRepoStub struct{}

func (ecRealmRepoStub) ListRealms(_ context.Context, _ string) ([]*domain.KeycloakRealmInfo, error) {
	return []*domain.KeycloakRealmInfo{{RealmName: "master"}}, nil
}

type ecEventReaderStub struct{}

func (ecEventReaderStub) CountEventsByType(_ context.Context, _, _, _ string, _, _ time.Time) (int64, error) {
	return 0, nil
}

var (
	_ eventscheck.AlertStore          = ecFakeAlertStore{}
	_ eventscheck.NotificationService = ecFakeNotifier{}
	_ eventscheck.KeycloakEventReader = ecEventReaderStub{}
	_ realmLister                     = ecRealmRepoStub{}
)

func TestBuildEventsCheckServices_OnePerEnabledTenant(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.EventChecker.Enabled = true
	cfg.Keycloak.Global.EventChecker.Checks.LoginError.Enabled = true
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Enabled: true},
		"t2": {Enabled: false},
		"t3": {Enabled: true},
	}

	svcs := buildEventsCheckServices(cfg, ecEventReaderStub{}, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	if len(svcs) != 2 {
		t.Errorf("expected 2 services (t1, t3), got %d", len(svcs))
	}
}

func TestBuildEventsCheckServices_MasterSwitchOff(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.EventChecker.Checks.LoginError.Enabled = true
	cfg.Keycloak.Global.EventChecker.Checks.IdpError.Enabled = true
	cfg.Keycloak.Global.EventChecker.Checks.CodeToToken.Enabled = true
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Enabled: true},
	}

	svcs := buildEventsCheckServices(cfg, ecEventReaderStub{}, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	if len(svcs) != 0 {
		t.Errorf("expected 0 services when the master switch is off, got %d", len(svcs))
	}
}

func TestBuildEventsCheckServices_SkipsWhenNoChecksEnabled(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.EventChecker.Enabled = true
	cfg.Keycloak.Tenants = map[string]config.TenantConfig{
		"t1": {Enabled: true},
	}

	svcs := buildEventsCheckServices(cfg, ecEventReaderStub{}, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	if len(svcs) != 0 {
		t.Errorf("expected 0 services when no checks enabled, got %d", len(svcs))
	}
}

func TestBuildEventsChecks_EnablesAllFiveFromConfig(t *testing.T) {
	var checkerCfg config.EventCheckerConfig
	checkerCfg.Checks.LoginError.Enabled = true
	checkerCfg.Checks.ClientError.Enabled = true
	checkerCfg.Checks.CodeToToken.Enabled = true
	checkerCfg.Checks.IdpError.Enabled = true
	checkerCfg.Checks.CommonErrors.Enabled = true
	checkerCfg.ApplyDefaults()

	realmsFn := eventscheck.RealmsFunc(func(context.Context) ([]string, error) { return []string{"master"}, nil })
	checks := buildEventsChecks("tenant-1", realmsFn, ecEventReaderStub{}, checkerCfg, noopEventscheckLogger{})

	want := map[string]bool{
		"login_error_events":         false,
		"client_login_error_events":  false,
		"code_to_token_error_events": false,
		"idp_error_events":           false,
		"common_error_events":        false,
	}
	for _, c := range checks {
		if _, ok := want[c.GetCheckType()]; ok {
			want[c.GetCheckType()] = true
		}
	}
	if len(checks) != 5 {
		t.Errorf("expected 5 checks, got %d", len(checks))
	}
	for checkType, found := range want {
		if !found {
			t.Errorf("expected check %q to be present when its config flag is enabled", checkType)
		}
	}
}

func TestBuildEventsChecks_AbsentWhenDisabled(t *testing.T) {
	var checkerCfg config.EventCheckerConfig
	checkerCfg.ApplyDefaults()

	realmsFn := eventscheck.RealmsFunc(func(context.Context) ([]string, error) { return []string{"master"}, nil })
	checks := buildEventsChecks("tenant-1", realmsFn, ecEventReaderStub{}, checkerCfg, noopEventscheckLogger{})
	if len(checks) != 0 {
		t.Errorf("expected 0 checks when all disabled, got %d", len(checks))
	}
}
