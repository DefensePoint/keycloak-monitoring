package postgres

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// Operator metrics are measured per occurrence of an alert: response is an
// operator's first acknowledge, resolve or ignore of the occurrence, resolution
// is the time from the occurrence's start to a resolve, and work time is how
// long the operator had the occurrence in hand, from their first action on it
// to their resolve or ignore, counted once.

type workTimeFixture struct {
	t      *testing.T
	client *database.Client
	repo   *Repository
	tenant string
	now    time.Time
	// last is each alert's previous action, as the handler reads it.
	last map[uint]domain.OperatorAction
}

func newWorkTimeFixture(t *testing.T) *workTimeFixture {
	t.Helper()
	dsn := testdb.DSN(t, "KMT_TEST_DATABASE_DSN")
	testdb.Lock(t, dsn)
	client, err := database.NewClient(testdb.DatabaseConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	f := &workTimeFixture{t: t, client: client, repo: NewRepository(client).(*Repository),
		tenant: fmt.Sprintf("worktime-%d", time.Now().UnixNano()), now: time.Now().UTC().Truncate(time.Second),
		last: map[uint]domain.OperatorAction{}}
	t.Cleanup(func() {
		client.DB().Where("tenant_id = ?", f.tenant).Delete(&database.OperatorAction{})
		client.DB().Unscoped().Where("tenant_id = ?", f.tenant).Delete(&database.ConfigurationAlert{})
		client.DB().Unscoped().Where("email LIKE ?", f.tenant+"%").Delete(&database.User{})
	})
	return f
}

func (f *workTimeFixture) operator(name string) (string, uint) {
	f.t.Helper()
	email := f.tenant + "-" + name + "@example.com"
	user := &database.User{Email: email, Username: email}
	if err := f.client.DB().Create(user).Error; err != nil {
		f.t.Fatalf("create operator: %v", err)
	}
	return email, user.ID
}

// alert stores an alert whose current occurrence started at occurrence and
// which was first detected at firstDetected.
func (f *workTimeFixture) alert(firstDetected, occurrence time.Time) uint {
	f.t.Helper()
	a := &database.ConfigurationAlert{TenantID: f.tenant, AlertID: fmt.Sprintf("a-%d", time.Now().UnixNano()),
		Type: "configuration", Severity: "high", Status: "active", Title: "t", ResourceType: "realm",
		ResourceID: "r1", ResourceName: "r1", RealmName: "r1", FirstDetected: firstDetected, LastSeen: occurrence,
		OccurrenceStartedAt: &occurrence}
	if err := f.client.DB().Create(a).Error; err != nil {
		f.t.Fatalf("create alert: %v", err)
	}
	return a.ID
}

// act records an action as the alerts handler does: the legacy times are the
// seconds since the alert's first detection. A nil occurrence lets
// RecordAction look it up from the alert.
func (f *workTimeFixture) act(email string, operatorID, alertID uint, actionType string, firstDetected time.Time, occurrence *time.Time, at time.Time) {
	f.t.Helper()
	secs := int(at.Sub(firstDetected).Seconds())
	action := domain.OperatorAction{
		TenantID: f.tenant, AlertID: alertID, OperatorID: operatorID, OperatorEmail: email, ActionType: actionType,
		ActionTime: at, AlertSeverity: "high", AlertType: "configuration", RealmName: "r1",
		TimeFromDetectionSeconds: secs, ResponseTimeSeconds: secs, ResolutionTimeSeconds: secs,
		OccurrenceStartedAt: occurrence,
		PreviousStatus:      "active", TimeFromPreviousActionSeconds: secs,
	}
	if prev, ok := f.last[alertID]; ok {
		prevTime := prev.ActionTime
		action.PreviousActionTime = &prevTime
		action.TimeFromPreviousActionSeconds = int(at.Sub(prevTime).Seconds())
		switch prev.ActionType {
		case "acknowledged", "resolved", "ignored":
			action.PreviousStatus = prev.ActionType
		}
	}
	if err := f.repo.RecordAction(context.Background(), &action); err != nil {
		f.t.Fatalf("record %s: %v", actionType, err)
	}
	f.last[alertID] = action
}

func (f *workTimeFixture) summary(email string, start, end time.Time) *domain.OperatorMetricsSummary {
	f.t.Helper()
	s, err := f.repo.GetMetricsSummary(context.Background(), f.tenant, email, start, end, nil)
	if err != nil {
		f.t.Fatalf("GetMetricsSummary: %v", err)
	}
	return s
}

func assertMinutes(t *testing.T, what string, gotSeconds, wantMinutes float64) {
	t.Helper()
	if math.Abs(gotSeconds/60-wantMinutes) > 0.05 {
		t.Errorf("%s = %.2f min, want %.2f min", what, gotSeconds/60, wantMinutes)
	}
}

func TestWorkTime_OneAlertIsCountedOnce(t *testing.T) {
	f := newWorkTimeFixture(t)
	email, opID := f.operator("op")
	det := f.now.Add(-2 * time.Hour)
	alertID := f.alert(det, det)

	f.act(email, opID, alertID, "acknowledged", det, &det, det.Add(10*time.Minute))
	f.act(email, opID, alertID, "commented", det, &det, det.Add(30*time.Minute))
	f.act(email, opID, alertID, "resolved", det, &det, det.Add(60*time.Minute))

	s := f.summary(email, f.now.Add(-24*time.Hour), f.now)
	// In hand from the acknowledge at 10 to the resolve at 60.
	assertMinutes(t, "work time", float64(s.TotalWorkTimeSeconds), 50)
	assertMinutes(t, "work time hours", s.TotalWorkTimeHours*3600, 50)
	// The acknowledge is the only response; the comment and resolve are not.
	assertMinutes(t, "avg response", s.AvgResponseTime, 10)
	assertMinutes(t, "median response", s.MedianResponseTime, 10)
	assertMinutes(t, "min response", float64(s.MinResponseTime), 10)
	assertMinutes(t, "max response", float64(s.MaxResponseTime), 10)
	assertMinutes(t, "avg resolution", s.AvgResolutionTime, 60)
	assertMinutes(t, "avg time to acknowledge", s.AvgTimeToAcknowledge, 10)
	assertMinutes(t, "avg time to resolve", s.AvgTimeToResolve, 60)
}

func TestWorkTime_ARecurringAlertIsMeasuredFromItsCurrentOccurrence(t *testing.T) {
	f := newWorkTimeFixture(t)
	email, opID := f.operator("op")
	first := f.now.Add(-30 * 24 * time.Hour)
	// The earlier occurrence, resolved 29 days ago.
	f.act(email, opID, f.alert(first, first), "resolved", first, &first, first.Add(24*time.Hour))
	reopened := f.now.Add(-20 * time.Minute)
	alertID := f.alert(first, reopened)

	f.act(email, opID, alertID, "acknowledged", first, &reopened, reopened.Add(5*time.Minute))

	s := f.summary(email, f.now.Add(-time.Hour), f.now)
	assertMinutes(t, "avg response", s.AvgResponseTime, 5)
	assertMinutes(t, "avg time to acknowledge", s.AvgTimeToAcknowledge, 5)
	assertMinutes(t, "active to acknowledge", s.AvgActiveToAcknowledgeTime, 5)
	if s.TotalWorkTimeSeconds != 0 {
		t.Errorf("work time = %ds, want 0: nothing was closed in this period", s.TotalWorkTimeSeconds)
	}
}

func TestWorkTime_EachOperatorGetsOnlyTheirOwnTime(t *testing.T) {
	f := newWorkTimeFixture(t)
	acker, ackerID := f.operator("acker")
	resolver, resolverID := f.operator("resolver")
	det := f.now.Add(-2 * time.Hour)
	alertID := f.alert(det, det)

	f.act(acker, ackerID, alertID, "acknowledged", det, &det, det.Add(10*time.Minute))
	f.act(resolver, resolverID, alertID, "resolved", det, &det, det.Add(60*time.Minute))

	a := f.summary(acker, f.now.Add(-24*time.Hour), f.now)
	assertMinutes(t, "acker response", a.AvgResponseTime, 10)
	if a.TotalWorkTimeSeconds != 0 {
		t.Errorf("acker work time = %ds, want 0: they did not close it", a.TotalWorkTimeSeconds)
	}
	r := f.summary(resolver, f.now.Add(-24*time.Hour), f.now)
	assertMinutes(t, "resolver response", r.AvgResponseTime, 60)
	assertMinutes(t, "resolver resolution", r.AvgResolutionTime, 60)
	assertMinutes(t, "resolver acknowledge to resolve", r.AvgAcknowledgeToResolveTime, 50)
	if r.TotalWorkTimeSeconds != 0 {
		t.Errorf("resolver work time = %ds, want 0: their first action was the resolve", r.TotalWorkTimeSeconds)
	}
}

func TestWorkTime_HandlingThatStartedBeforeThePeriodCountsWhereItEnds(t *testing.T) {
	f := newWorkTimeFixture(t)
	email, opID := f.operator("op")
	det := f.now.Add(-10 * time.Hour)
	alertID := f.alert(det, det)
	periodStart := f.now.Add(-5 * time.Hour)

	f.act(email, opID, alertID, "acknowledged", det, &det, periodStart.Add(-time.Hour))
	f.act(email, opID, alertID, "resolved", det, &det, periodStart.Add(time.Hour))

	s := f.summary(email, periodStart, f.now)
	assertMinutes(t, "work time", float64(s.TotalWorkTimeSeconds), 120)
	if s.AvgResponseTime != 0 {
		t.Errorf("avg response = %.0fs, want none in this period: the acknowledge was before it", s.AvgResponseTime)
	}
}

// Actions recorded before occurrences were stored keep the figures they
// always produced.
func TestWorkTime_OldActionsKeepTheirFigures(t *testing.T) {
	f := newWorkTimeFixture(t)
	email, opID := f.operator("op")
	det := f.now.Add(-2 * time.Hour)
	alertID := f.alert(det, det)
	for _, a := range []struct {
		typ string
		at  time.Duration
	}{{"acknowledged", 10 * time.Minute}, {"commented", 30 * time.Minute}, {"resolved", 60 * time.Minute}} {
		at := det.Add(a.at)
		secs := int(a.at.Seconds())
		old := &database.OperatorAction{TenantID: f.tenant, AlertID: alertID, OperatorID: opID, OperatorEmail: email,
			ActionType: a.typ, ActionTime: at, AlertSeverity: "high", AlertType: "configuration", RealmName: "r1",
			TimeFromDetectionSeconds: secs, ResponseTimeSeconds: secs, ResolutionTimeSeconds: secs}
		if err := f.client.DB().Create(old).Error; err != nil {
			t.Fatal(err)
		}
	}

	s := f.summary(email, f.now.Add(-24*time.Hour), f.now)
	assertMinutes(t, "work time", float64(s.TotalWorkTimeSeconds), 100)
	assertMinutes(t, "avg response", s.AvgResponseTime, (10.0+30+60)/3)
}

func TestRecordAction_TakesTheOccurrenceFromTheAlert(t *testing.T) {
	f := newWorkTimeFixture(t)
	email, opID := f.operator("op")
	first := f.now.Add(-30 * 24 * time.Hour)
	reopened := f.now.Add(-20 * time.Minute)
	alertID := f.alert(first, reopened)

	f.act(email, opID, alertID, "acknowledged", first, nil, reopened.Add(5*time.Minute))

	var stored database.OperatorAction
	if err := f.client.DB().Where("tenant_id = ?", f.tenant).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.OccurrenceStartedAt == nil || !stored.OccurrenceStartedAt.Equal(reopened) {
		t.Fatalf("stored occurrence = %v, want the alert's %v", stored.OccurrenceStartedAt, reopened)
	}
	assertMinutes(t, "avg response", f.summary(email, f.now.Add(-time.Hour), f.now).AvgResponseTime, 5)
}
