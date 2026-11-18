package fx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
)

// amfaAlertsFake is the AMFA checker's alert store and notifier, with a
// journal of stops and resolutions in order.
type amfaAlertsFake struct {
	mu       sync.Mutex
	open     map[string][]*domain.Alert
	journal  []string
	notified []*domain.Alert
}

func (a *amfaAlertsFake) record(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.journal = append(a.journal, s)
}
func (a *amfaAlertsFake) GetAlertByID(context.Context, string, string) (*domain.Alert, error) {
	return nil, nil
}
func (a *amfaAlertsFake) SaveAlert(context.Context, *domain.Alert) error { return nil }
func (a *amfaAlertsFake) AlertExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (a *amfaAlertsFake) ListOpenAlertsByCheck(_ context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []*domain.Alert
	for _, al := range a.open[tenantID] {
		if al.CheckType == checkType {
			cp := *al
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (a *amfaAlertsFake) ResolveAlert(_ context.Context, tenantID, alertID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.open[tenantID][:0]
	for _, al := range a.open[tenantID] {
		if al.AlertID != alertID {
			kept = append(kept, al)
		}
	}
	a.open[tenantID] = kept
	a.journal = append(a.journal, "resolve "+alertID)
	return nil
}
func (a *amfaAlertsFake) NotifyAlert(context.Context, *domain.Alert) error { return nil }
func (a *amfaAlertsFake) NotifyAlertResolution(_ context.Context, alert *domain.Alert) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notified = append(a.notified, alert)
	return nil
}
func (a *amfaAlertsFake) openIDs(tenantID string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []string
	for _, al := range a.open[tenantID] {
		ids = append(ids, al.AlertID)
	}
	return ids
}
func (a *amfaAlertsFake) notices() []*domain.Alert {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]*domain.Alert(nil), a.notified...)
}

// amfaJService records its stop in the journal.
type amfaJService struct {
	tenantID string
	alerts   *amfaAlertsFake
}

func (s *amfaJService) Start(context.Context) error { return nil }
func (s *amfaJService) Stop() error {
	s.alerts.record("stop " + s.tenantID)
	return nil
}

type amfaTenantList []*domain.KeycloakTenant

func (l amfaTenantList) ListTenants(context.Context) ([]*domain.KeycloakTenant, error) { return l, nil }

func amfaAlert(id, tenantID, checkType string) *domain.Alert {
	return &domain.Alert{AlertID: id, TenantID: tenantID, CheckType: checkType, Status: domain.AlertStatusActive}
}

func newAmfaCloseHarness(t *testing.T, configTenants map[string]config.TenantConfig, tenants amfaTenantList) (*amfaTenantSync, *amfaAlertsFake, *amfaTenantWorkers[*amfaJService]) {
	t.Helper()
	al := &amfaAlertsFake{open: map[string][]*domain.Alert{
		"t1": {amfaAlert("t1-rejected", "t1", "amfa-risk-rejected"), amfaAlert("t1-realm", "t1", "realm_security")},
		"t2": {amfaAlert("t2-rejected", "t2", "amfa-risk-rejected")},
		"cf": {amfaAlert("cf-rejected", "cf", "amfa-risk-rejected")},
	}}
	s, reg := newSync()
	w := &amfaTenantWorkers[*amfaJService]{
		component:     "amfacheck",
		configTenants: configTenants,
		build: func(tenantID string, _ amfa.Repository) *amfaJService {
			return &amfaJService{tenantID: tenantID, alerts: al}
		},
		closer: &amfaAlertCloser{types: []string{"amfa-risk-rejected"}, store: al, notifier: al, tenants: tenants},
	}
	w.follow(reg)
	return s, al, w
}

func startAmfaWorkers(t *testing.T, w *amfaTenantWorkers[*amfaJService]) {
	t.Helper()
	if err := w.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = w.stop() })
}

// When a tenant stops having AMFA checked (AMFA turned off, the tenant
// disabled or deleted), its checker stops and then its open AMFA alerts are
// resolved with a notice: nothing would ever resolve them otherwise.
func TestAmfaChecker_TenantNoLongerCheckedHasItsAlertsResolved(t *testing.T) {
	cases := map[string]func(s *amfaTenantSync){
		"AMFA turned off": func(s *amfaTenantSync) {
			s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", false, "https://amfa.internal"))
		},
		"tenant disabled": func(s *amfaTenantSync) {
			tn := dbTenant("t1", true, "https://amfa.internal")
			tn.Enabled = false
			s.handleTenantChange(tenant.EventDisabled, tn)
		},
		"tenant deleted": func(s *amfaTenantSync) {
			s.handleTenantChange(tenant.EventRemoved, dbTenant("t1", true, "https://amfa.internal"))
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, al, w := newAmfaCloseHarness(t, nil, nil)
			startAmfaWorkers(t, w)
			s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))

			change(s)

			if got := al.openIDs("t1"); len(got) != 1 || got[0] != "t1-realm" {
				t.Errorf("t1 open alerts = %v, want only t1-realm (not an AMFA check's)", got)
			}
			if len(al.openIDs("t2")) != 1 {
				t.Errorf("t2's alerts were touched: %v", al.openIDs("t2"))
			}
			if len(al.journal) != 2 || al.journal[0] != "stop t1" || al.journal[1] != "resolve t1-rejected" {
				t.Errorf("journal = %v, want the checker stopped, then its alert resolved", al.journal)
			}
			n := al.notices()
			if len(n) != 1 || n[0].Status != domain.AlertStatusResolved || n[0].ResolvedAt == nil ||
				!strings.Contains(n[0].ResolutionNote, "AMFA is no longer checked for this tenant") {
				t.Errorf("notices = %+v, want one resolution notice saying AMFA is no longer checked", n)
			}
		})
	}
}

func TestAmfaChecker_EndpointChangeResolvesNothing(t *testing.T) {
	s, al, w := newAmfaCloseHarness(t, nil, nil)
	startAmfaWorkers(t, w)
	s.handleTenantChange(tenant.EventAdded, dbTenant("t1", true, "https://amfa.internal"))

	s.handleTenantChange(tenant.EventUpdated, dbTenant("t1", true, "https://amfa-2.internal"))

	if len(al.openIDs("t1")) != 2 || len(al.notices()) != 0 {
		t.Errorf("open %v, notices %d: an endpoint change must resolve nothing", al.openIDs("t1"), len(al.notices()))
	}
}

// At startup, every tenant without an AMFA checker (AMFA off, the tenant
// disabled, or both, possibly from before the upgrade) has its open AMFA
// alerts resolved. Tenants with a checker, config.yaml ones included, keep
// theirs.
func TestAmfaChecker_StartupResolvesTheAlertsOfTenantsWithoutAChecker(t *testing.T) {
	cfgTenants := map[string]config.TenantConfig{"cf": {Enabled: true}}
	tenants := amfaTenantList{dbTenant("t1", false, "https://amfa.internal"), dbTenant("t2", true, "https://amfa.internal"),
		{TenantID: "cf", Enabled: true, IsConfigDefined: true}}
	s, al, w := newAmfaCloseHarness(t, cfgTenants, tenants)
	w.put("cf", &amfaJService{tenantID: "cf", alerts: al}) // built at startup from config.yaml
	s.handleTenantChange(tenant.EventAdded, dbTenant("t2", true, "https://amfa.internal"))
	startAmfaWorkers(t, w)

	done := func() bool { return len(al.openIDs("t1")) == 1 }
	for i := 0; i < 100 && !done(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if got := al.openIDs("t1"); len(got) != 1 || got[0] != "t1-realm" {
		t.Errorf("t1 (AMFA off) open alerts = %v, want its AMFA alert resolved", got)
	}
	if len(al.openIDs("t2")) != 1 || len(al.openIDs("cf")) != 1 {
		t.Errorf("t2 %v, cf %v: tenants with a checker must keep their alerts", al.openIDs("t2"), al.openIDs("cf"))
	}
	if len(al.notices()) != 1 {
		t.Errorf("notices = %d, want 1", len(al.notices()))
	}
}

// The checker resolves the alerts of the AMFA checks its configuration runs;
// the mirror raises no alerts and resolves none, and neither does a checker
// switched off globally.
func TestAmfaWorkers_OnlyTheEnabledCheckerResolvesItsChecksAlerts(t *testing.T) {
	cfg := checkerConfig(true)
	cfg.Keycloak.Global.AmfaChecker.Checks.VPNRisky.Enabled = true
	_, w := newAmfaCheckWorkers(cfg, amfa.NewRegistry(), acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil, amfaTenantList{}, nil)
	t.Cleanup(func() { _ = w.stop() })
	if w.closer == nil || strings.Join(w.closer.types, ",") != "amfa-risk-rejected,amfa-vpn-risky" ||
		w.closer.store == nil || w.closer.notifier == nil || w.closer.tenants == nil {
		t.Fatalf("checker closer = %+v, want the risk-rejected and VPN check types, a store, a notifier and the tenants", w.closer)
	}

	_, off := newAmfaCheckWorkers(checkerConfig(false), amfa.NewRegistry(), acRealmRepoStub{}, acFakeAlertStore{}, acFakeNotifier{}, nil, amfaTenantList{}, nil)
	t.Cleanup(func() { _ = off.stop() })
	if off.closer != nil {
		t.Error("a checker switched off globally must not resolve alerts")
	}

	m := newMirrorUnderTest(t, mirrorConfig(true), amfa.NewRegistry())
	if m.closer != nil {
		t.Error("the mirror must not resolve alerts")
	}
}
