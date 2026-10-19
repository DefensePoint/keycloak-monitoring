package eventscheck

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// What a re-detected alert looks like when the lookup of the stored alert
// fails for a moment, through the real alerts repository: a read error says
// nothing about whether the alert exists, so the stored alert must come out of
// that poll unchanged and no one must be notified about it again.

// flakyLookupStore fails the next lookup when told to, the way a dropped
// connection or a statement timeout would, and otherwise uses the repository.
type flakyLookupStore struct {
	repoStore
	failNext atomic.Bool
}

func (s *flakyLookupStore) GetAlertByID(ctx context.Context, tenantID, id string) (*domain.Alert, error) {
	if s.failNext.CompareAndSwap(true, false) {
		return nil, errors.New("failed to get alert: driver: bad connection")
	}
	return s.repoStore.GetAlertByID(ctx, tenantID, id)
}

func newLookupErrorFixture(t *testing.T) (*renotifyFixture, *flakyLookupStore) {
	t.Helper()
	f := newRenotifyFixture(t)
	store := &flakyLookupStore{repoStore: repoStore{f.repo}}
	f.svc = NewService([]Check{f.check}, store, f.notifier, discardLogger{}, time.Hour)
	return f, store
}

// storedAlert reads the tenant's one alert straight from the table, whatever
// its status.
func (f *renotifyFixture) storedAlert(t *testing.T) database.ConfigurationAlert {
	t.Helper()
	var rows []database.ConfigurationAlert
	if err := f.db.Where("tenant_id = ?", f.tenantID).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("stored alerts: %v (%d)", err, len(rows))
	}
	return rows[0]
}

func (f *renotifyFixture) setStatus(t *testing.T, status domain.AlertStatus, by string) {
	t.Helper()
	row := f.storedAlert(t)
	if err := f.repo.UpdateStatus(context.Background(), f.tenantID, row.AlertID, status, by); err != nil {
		t.Fatalf("UpdateStatus(%s): %v", status, err)
	}
}

func TestLookupError_AnAcknowledgedAlertStaysAcknowledged(t *testing.T) {
	f, store := newLookupErrorFixture(t)
	f.counts.set("master", 1)
	f.poll(t)
	f.setStatus(t, domain.AlertStatusAcknowled, "operator@example.com")
	before := f.storedAlert(t)

	store.failNext.Store(true)
	f.poll(t)

	after := f.storedAlert(t)
	if after.Status != string(domain.AlertStatusAcknowled) {
		t.Errorf("status after a failed lookup = %q, want acknowledged", after.Status)
	}
	if after.AcknowledgedBy != "operator@example.com" || after.AcknowledgedAt == nil {
		t.Errorf("acknowledgment after a failed lookup = %q at %v, want it kept", after.AcknowledgedBy, after.AcknowledgedAt)
	}
	if !after.FirstDetected.Equal(before.FirstDetected) {
		t.Errorf("first detected moved from %v to %v", before.FirstDetected, after.FirstDetected)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: only the first detection, none for the failed lookup", raised)
	}

	// The next poll reads the alert again and treats it as the acknowledged
	// alert it is.
	f.poll(t)
	if got := f.storedAlert(t).Status; got != string(domain.AlertStatusAcknowled) {
		t.Errorf("status after the next poll = %q, want acknowledged", got)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications after the next poll = %d, want 1", raised)
	}
}

func TestLookupError_AnIgnoredAlertStaysIgnored(t *testing.T) {
	f, store := newLookupErrorFixture(t)
	f.counts.set("master", 1)
	f.poll(t)
	f.setStatus(t, domain.AlertStatusIgnored, "operator@example.com")

	store.failNext.Store(true)
	f.poll(t)

	if got := f.storedAlert(t).Status; got != string(domain.AlertStatusIgnored) {
		t.Errorf("status after a failed lookup = %q, want ignored", got)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: none for the failed lookup", raised)
	}
}

func TestLookupError_AnActiveAlertIsNotAnnouncedAsNew(t *testing.T) {
	f, store := newLookupErrorFixture(t)
	f.counts.set("master", 1)
	f.poll(t)
	before := f.storedAlert(t)

	store.failNext.Store(true)
	f.poll(t)
	f.poll(t)

	after := f.storedAlert(t)
	if !after.FirstDetected.Equal(before.FirstDetected) {
		t.Errorf("first detected moved from %v to %v", before.FirstDetected, after.FirstDetected)
	}
	if after.Status != string(domain.AlertStatusActive) {
		t.Errorf("status = %q, want active", after.Status)
	}
	if raised, _ := f.notifier.counts(); raised != 1 {
		t.Errorf("notifications = %d, want 1: the failed lookup must not send a second \"new alert\"", raised)
	}
}
