package amfacheck

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspg "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	"gorm.io/gorm"
)

// What a re-detected AMFA alert looks like when the lookup of the stored alert
// fails for a moment, through the real alerts repository: a read error says
// nothing about whether the alert exists, so the stored alert must come out of
// that run unchanged and no notification must be requested for it.

// flakyRepoStore adapts alerts.Repository to AlertStore, as the application
// does, and fails the next lookup when told to, the way a dropped connection or
// a statement timeout would.
type flakyRepoStore struct {
	repo     alerts.Repository
	failNext atomic.Bool
}

func (s *flakyRepoStore) GetAlertByID(ctx context.Context, tenantID, id string) (*domain.Alert, error) {
	if s.failNext.CompareAndSwap(true, false) {
		return nil, errors.New("failed to get alert: driver: bad connection")
	}
	return s.repo.GetByAlertID(ctx, tenantID, id)
}
func (s *flakyRepoStore) SaveAlert(ctx context.Context, a *domain.Alert) error {
	return s.repo.Save(ctx, a)
}
func (s *flakyRepoStore) ListOpenAlertsByCheck(ctx context.Context, tenantID, checkType string) ([]*domain.Alert, error) {
	return s.repo.ListOpenByCheck(ctx, tenantID, checkType)
}
func (s *flakyRepoStore) ResolveAlert(ctx context.Context, tenantID, id string) error {
	return s.repo.Resolve(ctx, tenantID, id)
}
func (s *flakyRepoStore) AlertExists(ctx context.Context, tenantID, id string) (bool, error) {
	if s.failNext.CompareAndSwap(true, false) {
		return false, errors.New("failed to check alert: driver: bad connection")
	}
	return s.repo.ExistsIncludingDeleted(ctx, tenantID, id)
}

// freshAlertCheck reports the same condition on every run with a new alert
// value, as the real checks do.
type freshAlertCheck struct{ tenantID string }

func (c freshAlertCheck) GetCheckType() string   { return "amfa_lookup_test" }
func (c freshAlertCheck) GetDescription() string { return "lookup error test check" }
func (c freshAlertCheck) Execute(context.Context) ([]*domain.Alert, error) {
	a := newAlert("amfa-lookup-test")
	a.TenantID = c.tenantID
	a.CheckType = c.GetCheckType()
	return []*domain.Alert{a}, nil
}

type lookupErrorFixture struct {
	db       *gorm.DB
	repo     alerts.Repository
	store    *flakyRepoStore
	notifier *fakeNotifier
	svc      Service
	tenantID string
}

func newLookupErrorFixture(t *testing.T) *lookupErrorFixture {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	f := &lookupErrorFixture{
		db:       client.DB(),
		repo:     alertspg.NewRepository(client.DB()),
		notifier: newFakeNotifier(),
		tenantID: fmt.Sprintf("amfa-lookup-%d", time.Now().UnixNano()),
	}
	t.Cleanup(func() {
		f.db.Unscoped().Where("tenant_id = ?", f.tenantID).Delete(&database.ConfigurationAlert{})
	})
	f.store = &flakyRepoStore{repo: f.repo}
	f.svc = NewService(f.tenantID, []Check{freshAlertCheck{tenantID: f.tenantID}}, f.store, f.notifier, testLogger{}, time.Hour)
	return f
}

func (f *lookupErrorFixture) run(t *testing.T) {
	t.Helper()
	if err := f.svc.RunCheckNow(context.Background()); err != nil {
		t.Fatalf("RunCheckNow: %v", err)
	}
}

func (f *lookupErrorFixture) stored(t *testing.T) database.ConfigurationAlert {
	t.Helper()
	var rows []database.ConfigurationAlert
	if err := f.db.Where("tenant_id = ?", f.tenantID).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("stored alerts: %v (%d)", err, len(rows))
	}
	return rows[0]
}

func (f *lookupErrorFixture) setStatus(t *testing.T, status domain.AlertStatus, by string) {
	t.Helper()
	if err := f.repo.UpdateStatus(context.Background(), f.tenantID, "amfa-lookup-test", status, by); err != nil {
		t.Fatalf("UpdateStatus(%s): %v", status, err)
	}
}

func TestLookupError_ANewAlertIsStillRaised(t *testing.T) {
	f := newLookupErrorFixture(t)
	f.run(t)

	if got := f.stored(t).Status; got != string(domain.AlertStatusActive) {
		t.Errorf("status = %q, want active", got)
	}
	if n := f.notifier.notifs("amfa-lookup-test"); n != 1 {
		t.Errorf("notifications = %d, want 1 for a genuinely new alert", n)
	}
}

func TestLookupError_AnAcknowledgedAlertStaysAcknowledged(t *testing.T) {
	f := newLookupErrorFixture(t)
	f.run(t)
	f.setStatus(t, domain.AlertStatusAcknowled, "operator@example.com")
	before := f.stored(t)

	f.store.failNext.Store(true)
	f.run(t)

	after := f.stored(t)
	if after.Status != string(domain.AlertStatusAcknowled) {
		t.Errorf("status after a failed lookup = %q, want acknowledged", after.Status)
	}
	if after.AcknowledgedBy != "operator@example.com" || after.AcknowledgedAt == nil {
		t.Errorf("acknowledgment after a failed lookup = %q at %v, want it kept", after.AcknowledgedBy, after.AcknowledgedAt)
	}
	if !after.FirstDetected.Equal(before.FirstDetected) {
		t.Errorf("first detected moved from %v to %v", before.FirstDetected, after.FirstDetected)
	}
	if n := f.notifier.notifs("amfa-lookup-test"); n != 1 {
		t.Errorf("notifications = %d, want 1: only the first detection, none for the failed lookup", n)
	}

	// The next run reads the alert again and treats it as the acknowledged
	// alert it is.
	f.run(t)
	if got := f.stored(t).Status; got != string(domain.AlertStatusAcknowled) {
		t.Errorf("status after the next run = %q, want acknowledged", got)
	}
	if n := f.notifier.notifs("amfa-lookup-test"); n != 1 {
		t.Errorf("notifications after the next run = %d, want 1", n)
	}
}

func TestLookupError_AnIgnoredAlertStaysIgnored(t *testing.T) {
	f := newLookupErrorFixture(t)
	f.run(t)
	f.setStatus(t, domain.AlertStatusIgnored, "operator@example.com")

	f.store.failNext.Store(true)
	f.run(t)

	if got := f.stored(t).Status; got != string(domain.AlertStatusIgnored) {
		t.Errorf("status after a failed lookup = %q, want ignored", got)
	}
	if n := f.notifier.notifs("amfa-lookup-test"); n != 1 {
		t.Errorf("notifications = %d, want 1: none for the failed lookup", n)
	}
}

func TestLookupError_AnActiveAlertIsNotAnnouncedAsNew(t *testing.T) {
	f := newLookupErrorFixture(t)
	f.run(t)
	before := f.stored(t)

	f.store.failNext.Store(true)
	f.run(t)
	f.run(t)

	after := f.stored(t)
	if !after.FirstDetected.Equal(before.FirstDetected) {
		t.Errorf("first detected moved from %v to %v", before.FirstDetected, after.FirstDetected)
	}
	if after.Status != string(domain.AlertStatusActive) {
		t.Errorf("status = %q, want active", after.Status)
	}
	if n := f.notifier.notifs("amfa-lookup-test"); n != 1 {
		t.Errorf("notifications = %d, want 1: the failed lookup must not request a second \"new alert\"", n)
	}
}
