package fx

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/notifications"
)

// A deleted tenant's alerts are purged with it. Each unresolved one must be
// closed on its notification channels, saying why, once the deletion is done.

type unresolvedRepo struct {
	alerts.Repository
	alerts []*domain.Alert
	err    error
}

func (r *unresolvedRepo) ListUnresolved(context.Context, string) ([]*domain.Alert, error) {
	return r.alerts, r.err
}

type closingNotifier struct {
	notifications.Service
	mu       sync.Mutex
	resolved []*domain.Alert
	failFor  string
	// cancelled counts resolutions sent with a context already cancelled,
	// which a real channel would refuse.
	cancelled int
	done      chan struct{}
	want      int
}

func (n *closingNotifier) NotifyAlertResolution(ctx context.Context, a *domain.Alert) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ctx.Err() != nil {
		n.cancelled++
	}
	copied := *a
	n.resolved = append(n.resolved, &copied)
	if len(n.resolved) == n.want && n.done != nil {
		close(n.done)
	}
	if a.AlertID == n.failFor {
		return errors.New("gitlab unreachable")
	}
	return nil
}

var deletedTenant = &domain.KeycloakTenant{TenantID: "t-del", Name: "Gone Tenant"}

func openAlerts() []*domain.Alert {
	return []*domain.Alert{
		{TenantID: "t-del", AlertID: "tenant-health-down-t-del", Status: domain.AlertStatusActive},
		{TenantID: "t-del", AlertID: "evt-1", Status: domain.AlertStatusAcknowled},
		{TenantID: "t-del", AlertID: "evt-2", Status: domain.AlertStatusIgnored},
	}
}

func TestTenantAlertCloser_ClosesEveryUnresolvedAlertAfterTheDeletion(t *testing.T) {
	notifier := &closingNotifier{done: make(chan struct{}), want: 3, failFor: "evt-1"}
	c := newTenantAlertCloser(&unresolvedRepo{alerts: openAlerts()}, notifier, logger.NewNoop())

	after := c.PrepareDeletion(context.Background(), deletedTenant)
	if after == nil {
		t.Fatal("PrepareDeletion returned nothing to run for a tenant with open alerts")
	}
	notifier.mu.Lock()
	early := len(notifier.resolved)
	notifier.mu.Unlock()
	if early != 0 {
		t.Fatalf("%d resolutions sent before the deletion committed", early)
	}

	ctx, cancel := context.WithCancel(context.Background())
	after(ctx)
	cancel() // the request ends; closing must carry on regardless
	select {
	case <-notifier.done:
	case <-time.After(5 * time.Second):
		t.Fatal("not every alert was closed")
	}

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if notifier.cancelled != 0 {
		t.Errorf("%d resolutions were sent with the ended request's context, which a real channel refuses", notifier.cancelled)
	}
	for _, a := range notifier.resolved {
		if a.Status != domain.AlertStatusResolved || a.ResolvedAt == nil {
			t.Errorf("%s sent as %s resolved at %v, want resolved", a.AlertID, a.Status, a.ResolvedAt)
		}
		if !strings.Contains(a.ResolutionNote, "'Gone Tenant' was deleted") {
			t.Errorf("%s note = %q, want it to say the tenant was deleted", a.AlertID, a.ResolutionNote)
		}
	}
}

func TestTenantAlertCloser_NothingToCloseOrUnreadable(t *testing.T) {
	notifier := &closingNotifier{}
	for name, repo := range map[string]*unresolvedRepo{
		"no open alerts":        {},
		"alerts cannot be read": {err: errors.New("db down")},
	} {
		if after := newTenantAlertCloser(repo, notifier, logger.NewNoop()).PrepareDeletion(context.Background(), deletedTenant); after != nil {
			t.Errorf("%s: PrepareDeletion returned a follow-up, want nil", name)
		}
	}
}

// slowNotifier holds each resolution until released, like a slow channel.
type slowNotifier struct {
	notifications.Service
	release chan struct{}
	mu      sync.Mutex
	sent    int
}

func (n *slowNotifier) NotifyAlertResolution(context.Context, *domain.Alert) error {
	<-n.release
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent++
	return nil
}

// A restart right after a deletion used to drop the closing under way: most
// of the tenant's issues stayed open. Shutdown must wait for it.
func TestTenantAlertCloser_ShutdownWaitsForClosingUnderWay(t *testing.T) {
	notifier := &slowNotifier{release: make(chan struct{})}
	c := newTenantAlertCloser(&unresolvedRepo{alerts: openAlerts()}, notifier, logger.NewNoop())
	c.PrepareDeletion(context.Background(), deletedTenant)(context.Background())

	drained := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.drain(ctx)
		close(drained)
	}()

	select {
	case <-drained:
		t.Fatal("shutdown returned while alerts were still being closed")
	case <-time.After(100 * time.Millisecond):
	}
	close(notifier.release)
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not return once the closing finished")
	}
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if notifier.sent != 3 {
		t.Errorf("resolutions sent before shutdown returned = %d, want 3", notifier.sent)
	}
}
