package fx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// cwAlerts is the alert store and the notifier as the workers see them, with
// a journal of what happened in which order.
type cwAlerts struct {
	mu       sync.Mutex
	open     map[string][]*domain.Alert // tenant -> open alerts
	journal  []string
	notified []*domain.Alert
}

func (a *cwAlerts) record(s string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.journal = append(a.journal, s)
}

func (a *cwAlerts) ListOpenAlertsByCheck(_ context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
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

func (a *cwAlerts) ResolveAlert(_ context.Context, tenantID, alertID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.open[tenantID][:0]
	for _, al := range a.open[tenantID] {
		if al.AlertID != alertID {
			kept = append(kept, al)
		}
	}
	a.open[tenantID] = kept
	a.journal = append(a.journal, "resolve "+tenantID+"/"+alertID)
	return nil
}

func (a *cwAlerts) NotifyAlertResolution(_ context.Context, alert *domain.Alert) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notified = append(a.notified, alert)
	return nil
}

func (a *cwAlerts) openIDs(tenantID string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ids []string
	for _, al := range a.open[tenantID] {
		ids = append(ids, al.AlertID)
	}
	return ids
}

// journalingService records its stop in the alerts journal, so a test can see
// whether the checks were stopped before their alerts were resolved.
type journalingService struct {
	cwService
	alerts *cwAlerts
}

func (s *journalingService) Stop() error {
	s.alerts.record("stop " + s.tenantID)
	return s.cwService.Stop()
}

func newAlertsHarness(t *testing.T) (*tenantCheckWorkers[*journalingService], *cwAlerts) {
	t.Helper()
	al := &cwAlerts{open: map[string][]*domain.Alert{
		"a": {
			{AlertID: "a-realm", TenantID: "a", CheckType: "realm_security", Status: domain.AlertStatusActive},
			{AlertID: "a-client", TenantID: "a", CheckType: "client_security", Status: domain.AlertStatusAcknowled},
			{AlertID: "a-amfa", TenantID: "a", CheckType: "amfa_risk_rejected", Status: domain.AlertStatusActive},
		},
		"b": {
			{AlertID: "b-realm", TenantID: "b", CheckType: "realm_security", Status: domain.AlertStatusActive},
		},
	}}
	w := &tenantCheckWorkers[*journalingService]{
		component: "test",
		build: func(tenantID string) (*journalingService, bool) {
			return &journalingService{cwService: cwService{tenantID: tenantID}, alerts: al}, true
		},
		alertTypes:  []string{"realm_security", "client_security"},
		alertStore:  al,
		resolutions: al,
	}
	lister := &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("a"), enabledTenant("b")}}
	if err := w.start(context.Background(), lister); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = w.stop() })
	return w, al
}

// A tenant that is no longer checked keeps no open alerts from these checks:
// nothing would ever resolve them. The monitors do the same for health alerts.
func TestTenantCheckWorkers_DisablingATenantResolvesItsOpenAlerts(t *testing.T) {
	cases := map[string]struct {
		event tenant.Event
		t     *domain.KeycloakTenant
		how   string
	}{
		"disabled":             {tenant.EventDisabled, disabledTenant("a"), "disabled"},
		"deleted":              {tenant.EventRemoved, enabledTenant("a"), "removed"},
		"saved while disabled": {tenant.EventUpdated, disabledTenant("a"), "disabled"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w, al := newAlertsHarness(t)

			w.tenantChanged(tc.event, tc.t)

			if got := al.openIDs("a"); len(got) != 1 || got[0] != "a-amfa" {
				t.Errorf("tenant a open alerts = %v, want only a-amfa (another checker's)", got)
			}
			if got := al.openIDs("b"); len(got) != 1 {
				t.Errorf("tenant b open alerts = %v, want b-realm left alone", got)
			}
			if len(al.journal) < 3 || al.journal[0] != "stop a" {
				t.Errorf("journal = %v, want the checks stopped before any alert is resolved", al.journal)
			}
			if len(al.notified) != 2 {
				t.Fatalf("resolution notices = %d, want 2", len(al.notified))
			}
			for _, n := range al.notified {
				if n.Status != domain.AlertStatusResolved || n.ResolvedAt == nil ||
					!strings.Contains(n.ResolutionNote, "Tenant "+tc.how+": it is no longer monitored") {
					t.Errorf("notice for %s: status %s, resolved_at %v, note %q", n.AlertID, n.Status, n.ResolvedAt, n.ResolutionNote)
				}
			}
		})
	}
}

func TestTenantCheckWorkers_EnablingOrEditingResolvesNothing(t *testing.T) {
	w, al := newAlertsHarness(t)

	w.tenantChanged(tenant.EventUpdated, enabledTenant("a"))
	w.tenantChanged(tenant.EventEnabled, enabledTenant("b"))
	w.tenantChanged(tenant.EventAdded, enabledTenant("c"))

	if len(al.openIDs("a")) != 3 || len(al.openIDs("b")) != 1 || len(al.notified) != 0 {
		t.Errorf("open a=%v b=%v, notices %d: want nothing resolved", al.openIDs("a"), al.openIDs("b"), len(al.notified))
	}
}

// Each checker resolves the alerts of the checks its configuration runs, and
// only those: the two checkers and the AMFA checker share one alerts table.
func TestCheckWorkers_ResolveTheirOwnChecksAlerts(t *testing.T) {
	var cc config.ConfigCheckerConfig
	cc.Checks.RealmSecurity.Enabled = true
	cc.Checks.ClientSecurity.Enabled = true
	cw := newConfigCheckWorkers(cc, func(string) configcheck.KeycloakClient { return nil }, saferequest.Policy{},
		ecFakeAlertStore{}, ecFakeNotifier{}, nil, logger.NewNoop())
	if got := strings.Join(cw.alertTypes, ","); got != "realm_security,client_security" {
		t.Errorf("configuration checker alert types = %q", got)
	}
	if cw.alertStore == nil || cw.resolutions == nil {
		t.Error("configuration checker cannot resolve: no alert store or notifier")
	}

	var ec config.EventCheckerConfig
	ec.Checks.LoginError.Enabled = true
	ec.Checks.IdpError.Enabled = true
	ec.Checks.CodeToToken.Enabled = true
	ec.Checks.ClientError.Enabled = true
	ec.Checks.CommonErrors.Enabled = true
	ew := newEventsCheckWorkers(ec, ecEventReaderStub{}, ecRealmRepoStub{}, ecFakeAlertStore{}, ecFakeNotifier{}, nil, nil)
	for _, typ := range ew.alertTypes {
		if !strings.HasSuffix(typ, "_events") {
			t.Errorf("event checker alert type %q is not an event check's", typ)
		}
	}
	if len(ew.alertTypes) != 5 || ew.alertStore == nil || ew.resolutions == nil {
		t.Errorf("event checker: types %v, store set %v, notifier set %v; want all five checks' types and both set",
			ew.alertTypes, ew.alertStore != nil, ew.resolutions != nil)
	}
}

// A tenant already disabled when the server starts (disabled before an
// upgrade, while the old version left its alerts open) gets the same: its
// open alerts are resolved at startup.
func TestTenantCheckWorkers_ResolvesTheAlertsOfTenantsDisabledAtStartup(t *testing.T) {
	al := &cwAlerts{open: map[string][]*domain.Alert{
		"off": {{AlertID: "off-realm", TenantID: "off", CheckType: "realm_security", Status: domain.AlertStatusActive}},
		"on":  {{AlertID: "on-realm", TenantID: "on", CheckType: "realm_security", Status: domain.AlertStatusActive}},
	}}
	w := &tenantCheckWorkers[*journalingService]{
		component: "test",
		build: func(tenantID string) (*journalingService, bool) {
			return &journalingService{cwService: cwService{tenantID: tenantID}, alerts: al}, true
		},
		alertTypes:  []string{"realm_security"},
		alertStore:  al,
		resolutions: al,
	}
	lister := &cwLister{tenants: []*domain.KeycloakTenant{enabledTenant("on"), disabledTenant("off")}}
	if err := w.start(context.Background(), lister); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = w.stop() })

	resolved := func() bool { return len(al.openIDs("off")) == 0 }
	for i := 0; i < 100 && !resolved(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !resolved() {
		t.Errorf("disabled tenant's open alerts = %v, want them resolved at startup", al.openIDs("off"))
	}
	if got := al.openIDs("on"); len(got) != 1 {
		t.Errorf("enabled tenant's open alerts = %v, want them left alone", got)
	}
	al.mu.Lock()
	n := len(al.notified)
	al.mu.Unlock()
	if n != 1 {
		t.Errorf("resolution notices = %d, want 1", n)
	}
}
