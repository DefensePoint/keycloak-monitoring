package alerts

import (
	"reflect"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

func unsanitizedAlert() *domain.Alert {
	ruleID := "rule-42"
	eventID := "event-99"
	return &domain.Alert{
		ID:             3,
		TenantID:       "tenant-a",
		AlertID:        "alert-1",
		Source:         domain.AlertSourceEvent,
		Type:           "brute_force",
		Severity:       domain.AlertSeverityCritical,
		Status:         domain.AlertStatusActive,
		Title:          "Brute force detected",
		Description:    "Repeated login failures",
		ResourceType:   "realm",
		ResourceID:     "res-1",
		ResourceName:   "prod",
		RealmName:      "prod",
		CheckType:      "internal-check",
		RuleID:         &ruleID,
		EventID:        &eventID,
		Recommendation: "Lock the account",
		Metadata:       `{"internal":"detail"}`,
		FirstDetected:  time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		LastSeen:       time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
	}
}

func TestSanitizeAlertStripsExactlyInternalFields(t *testing.T) {
	original := unsanitizedAlert()

	got := SanitizeAlert(original)

	want := unsanitizedAlert()
	want.Metadata = ""
	want.CheckType = ""
	want.RuleID = nil
	want.EventID = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SanitizeAlert = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(original, unsanitizedAlert()) {
		t.Fatalf("SanitizeAlert mutated its input: %+v", original)
	}
}

func TestSanitizeAlertNil(t *testing.T) {
	if got := SanitizeAlert(nil); got != nil {
		t.Fatalf("SanitizeAlert(nil) = %+v, want nil", got)
	}
}

func TestSanitizeAlertsStripsEveryAlert(t *testing.T) {
	list := []*domain.Alert{unsanitizedAlert(), unsanitizedAlert()}

	got := SanitizeAlerts(list)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	for i, alert := range got {
		if alert.Metadata != "" || alert.CheckType != "" || alert.RuleID != nil || alert.EventID != nil {
			t.Errorf("alert %d not sanitized: %+v", i, alert)
		}
	}
}

func TestSanitizeAlertsNilYieldsEmptySlice(t *testing.T) {
	got := SanitizeAlerts(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("SanitizeAlerts(nil) = %v, want empty non-nil slice", got)
	}
}
