package fx

import (
	"context"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/eventscheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"

	"go.uber.org/fx/fxtest"
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

// tenantEventReader reports, once per tenant, that a check read its events.
type tenantEventReader struct{ asked chan string }

func (r tenantEventReader) CountEventsByType(_ context.Context, tenantID, _, _ string, _, _ time.Time) (int64, error) {
	select {
	case r.asked <- tenantID:
	default:
	}
	return 0, nil
}

func newEventsWorkersUnderTest(t *testing.T, checkerCfg config.EventCheckerConfig, events eventscheck.KeycloakEventReader, tenants *fakeAmfaTenantService) {
	t.Helper()
	checkerCfg.Enabled = true
	checkerCfg.PollInterval = time.Hour
	w := newEventsCheckWorkers(checkerCfg, events, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	lc := fxtest.NewLifecycle(t)
	w.hook(lc, tenants)
	lc.RequireStart()
	t.Cleanup(func() { lc.RequireStop() })
}

func wantChecked(t *testing.T, asked <-chan string, tenantID string) {
	t.Helper()
	select {
	case got := <-asked:
		if got != tenantID {
			t.Fatalf("checked tenant %q, want %q", got, tenantID)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("tenant %q was never checked", tenantID)
	}
}

func TestRegisterEventsCheckHooks_MasterSwitchOffRegistersNothing(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.EventChecker.Checks.LoginError.Enabled = true

	registerEventsCheckHooks(EventsCheckParams{Lifecycle: lc, Config: cfg})

	// Tenants is nil, so a registered hook would panic on start.
	lc.RequireStart().RequireStop()
}

// The bug: the event checks were built from config.yaml, so a tenant created
// in the interface was never checked, not even after a restart.
func TestEventsCheckWorkers_ChecksATenantFromTheDatabaseAtStartup(t *testing.T) {
	var checkerCfg config.EventCheckerConfig
	checkerCfg.Checks.LoginError.Enabled = true
	asked := make(chan string, 1)

	newEventsWorkersUnderTest(t, checkerCfg, tenantEventReader{asked: asked},
		&fakeAmfaTenantService{tenants: []*domain.KeycloakTenant{enabledTenant("from-ui")}})

	wantChecked(t, asked, "from-ui")
}

func TestEventsCheckWorkers_TenantAddedAtRuntimeIsChecked(t *testing.T) {
	var checkerCfg config.EventCheckerConfig
	checkerCfg.Checks.LoginError.Enabled = true
	asked := make(chan string, 1)
	tenants := &fakeAmfaTenantService{}
	newEventsWorkersUnderTest(t, checkerCfg, tenantEventReader{asked: asked}, tenants)

	tenants.callback(tenant.EventAdded, enabledTenant("new"))

	wantChecked(t, asked, "new")
}

func TestEventsCheckWorkers_NoCheckEnabledStartsNothing(t *testing.T) {
	asked := make(chan string, 1)
	newEventsWorkersUnderTest(t, config.EventCheckerConfig{}, tenantEventReader{asked: asked},
		&fakeAmfaTenantService{tenants: []*domain.KeycloakTenant{enabledTenant("a")}})

	select {
	case got := <-asked:
		t.Fatalf("tenant %q was checked with every check off", got)
	case <-time.After(200 * time.Millisecond):
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

// The event checker starts before the monitoring module loads the tenant
// cache, and before the HTTP server serves, so it loads the cache itself.
func TestEventsCheckWorkers_LoadTheTenantCache(t *testing.T) {
	w := newEventsCheckWorkers(config.EventCheckerConfig{}, ecEventReaderStub{}, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	if !w.loadTenants {
		t.Fatal("event check workers do not load the tenant cache")
	}
}
