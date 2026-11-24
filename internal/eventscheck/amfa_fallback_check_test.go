package eventscheck

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
)

type fakeFallbackCounter struct {
	byRealm map[string]keycloak.AMFAFallbackCounts
	errFor  map[string]error
	window  time.Duration
}

func (f *fakeFallbackCounter) CountAMFAFallbackLogins(_ context.Context, _, realm string, from, to time.Time) (keycloak.AMFAFallbackCounts, error) {
	f.window = to.Sub(from)
	if err := f.errFor[realm]; err != nil {
		return keycloak.AMFAFallbackCounts{}, err
	}
	return f.byRealm[realm], nil
}

func newFallbackCheck(counter *fakeFallbackCounter, threshold int, realms ...string) *AMFAFallbackCheck {
	return NewAMFAFallbackCheck(counter, discardLogger{}, "tenant-1", staticRealms(realms...), threshold, 5*time.Minute)
}

// One login without the engine's decision is enough: with the fallback level
// at 1, it went through without any step-up.
func TestAMFAFallbackCheck_AlertsOnTheFirstLogin(t *testing.T) {
	counter := &fakeFallbackCounter{byRealm: map[string]keycloak.AMFAFallbackCounts{"prod": {Fallback: 1}}}
	alerts, err := newFallbackCheck(counter, 0, "prod").Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1 with the default threshold", len(alerts))
	}
	a := alerts[0]
	if a.CheckType != "amfa_fallback_events" || a.RealmName != "prod" || a.TenantID != "tenant-1" {
		t.Errorf("alert = %s/%s/%s, want amfa_fallback_events/prod/tenant-1", a.CheckType, a.RealmName, a.TenantID)
	}
	if a.Severity != domain.AlertSeverityError {
		t.Errorf("severity = %q, want error", a.Severity)
	}
	if !strings.Contains(a.Description, "1 login ran at the fallback risk level and 0 logins were refused") {
		t.Errorf("description = %q", a.Description)
	}
	if counter.window != 5*time.Minute {
		t.Errorf("counted over %s, want the 5m window", counter.window)
	}
}

// Refused logins count towards the threshold too, and the alert says how many
// of each there were.
func TestAMFAFallbackCheck_CountsFallbackAndRefusedLogins(t *testing.T) {
	counter := &fakeFallbackCounter{byRealm: map[string]keycloak.AMFAFallbackCounts{"prod": {Fallback: 2, Refused: 3}}}
	alerts, err := newFallbackCheck(counter, 5, "prod").Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want 1: 2 + 3 reaches the threshold of 5", len(alerts))
	}
	a := alerts[0]
	for _, want := range []string{"2 logins ran at the fallback risk level", "3 logins were refused"} {
		if !strings.Contains(a.Description, want) {
			t.Errorf("description %q lacks %q", a.Description, want)
		}
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(a.Metadata), &meta); err != nil {
		t.Fatalf("metadata: %v", err)
	}
	if meta["fallback_logins"] != float64(2) || meta["refused_logins"] != float64(3) {
		t.Errorf("metadata = %v, want fallback_logins 2 and refused_logins 3", meta)
	}
}

func TestAMFAFallbackCheck_NoAlertBelowThresholdOrWithout(t *testing.T) {
	counter := &fakeFallbackCounter{byRealm: map[string]keycloak.AMFAFallbackCounts{"prod": {Fallback: 1, Refused: 1}}}
	alerts, err := newFallbackCheck(counter, 3, "prod", "staging").Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("alerts = %d, want 0: 2 logins is below 3 and staging had none", len(alerts))
	}
}

// Each realm gets its own alert, with an id that stays the same between runs
// so a repeat updates it instead of raising another.
func TestAMFAFallbackCheck_OneStableAlertPerRealm(t *testing.T) {
	counter := &fakeFallbackCounter{byRealm: map[string]keycloak.AMFAFallbackCounts{
		"prod": {Fallback: 1}, "staging": {Refused: 1},
	}}
	c := newFallbackCheck(counter, 1, "prod", "staging")
	first, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	second, err := c.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("alerts = %d, %d, want 2 each", len(first), len(second))
	}
	if first[0].AlertID == first[1].AlertID {
		t.Error("two realms share an alert id")
	}
	if first[0].AlertID != second[0].AlertID || first[1].AlertID != second[1].AlertID {
		t.Error("alert ids changed between runs")
	}
}

// A realm whose count failed was not evaluated, so its open alert must not
// resolve; the other realms still are.
func TestAMFAFallbackCheck_FailedCountIsNotEvaluated(t *testing.T) {
	counter := &fakeFallbackCounter{
		byRealm: map[string]keycloak.AMFAFallbackCounts{"staging": {}},
		errFor:  map[string]error{"prod": errors.New("db down")},
	}
	c := newFallbackCheck(counter, 1, "prod", "staging")
	ev, err := c.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if ev.Covers(&domain.Alert{AlertID: c.generateAlertID("prod"), RealmName: "prod"}) {
		t.Error("prod was marked evaluated though its count failed")
	}
	if !ev.Covers(&domain.Alert{AlertID: c.generateAlertID("staging"), RealmName: "staging"}) {
		t.Error("staging was not marked evaluated")
	}
}
