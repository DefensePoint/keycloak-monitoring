// Counts of logins the AMFA plugin let through, or refused, without the
// engine's decision, checked against known rows.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// seedDetailEvent writes one event with the given details.
func seedDetailEvent(t *testing.T, db *gorm.DB, tenantID, realm, eventType, eventID string, at time.Time, details map[string]string) {
	t.Helper()

	row := &database.KeycloakEvent{
		TenantID:  tenantID,
		Time:      at,
		EventID:   eventID,
		RealmID:   realm,
		RealmName: realm,
		EventType: eventType,
		Success:   eventType == "LOGIN",
	}
	if details != nil {
		raw, err := json.Marshal(details)
		if err != nil {
			t.Fatalf("details: %v", err)
		}
		row.Details = raw
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed %s %s: %v", eventType, eventID, err)
	}
}

func TestCountAMFAFallbackLogins_MatchesKnownRows(t *testing.T) {
	db := newEventStatsDB(t)
	repo := NewEventRepository(db)
	ctx := context.Background()

	tenant := fmt.Sprintf("amfa-fallback-%d", time.Now().UnixNano())
	other := tenant + "-other"
	t.Cleanup(func() {
		db.Where("tenant_id IN ?", []string{tenant, other}).Delete(&database.KeycloakEvent{})
	})

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	from, to := base.Add(-time.Minute), base.Add(10*time.Minute)
	fallback := func(amfaEventID string) map[string]string {
		d := map[string]string{"decision_source": "fallback", "risk_level": "1"}
		if amfaEventID != "" {
			d["amfa_event_id"] = amfaEventID
		}
		return d
	}

	// Login A at the fallback level: a wrong code, then the login. One login.
	seedDetailEvent(t, db, tenant, "prod", "LOGIN_ERROR", "a1", base, fallback("amfa-a"))
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "a2", base.Add(time.Second), fallback("amfa-a"))
	// Login B at the fallback level, without an AMFA event id.
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "b1", base.Add(2*time.Second), fallback(""))
	// An SSO login: its post-broker event and its LOGIN both carry the details.
	seedDetailEvent(t, db, tenant, "prod", "IDENTITY_PROVIDER_POST_LOGIN", "s1", base.Add(3*time.Second), fallback("amfa-s"))
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "s2", base.Add(3*time.Second+time.Millisecond), fallback("amfa-s"))
	// A login mandatory mode refused because the engine could not be used.
	seedDetailEvent(t, db, tenant, "prod", "LOGIN_ERROR", "c1", base.Add(4*time.Second),
		map[string]string{"decision_source": "engine_unreachable_denied", "amfa_event_id": "amfa-c"})
	// Not counted: the engine decided, AMFA is off, no AMFA details at all.
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "d1", base.Add(5*time.Second),
		map[string]string{"decision_source": "engine", "amfa_event_id": "amfa-d"})
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "e1", base.Add(6*time.Second),
		map[string]string{"decision_source": "amfa_off"})
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "f1", base.Add(7*time.Second), nil)
	// Not counted: outside the window, another realm, another tenant.
	seedDetailEvent(t, db, tenant, "prod", "LOGIN", "g1", base.Add(-time.Hour), fallback("amfa-g"))
	seedDetailEvent(t, db, tenant, "staging", "LOGIN", "h1", base, fallback("amfa-h"))
	seedDetailEvent(t, db, other, "prod", "LOGIN", "i1", base, fallback("amfa-i"))

	got, err := repo.CountAMFAFallbackLogins(ctx, tenant, "prod", from, to)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got.Fallback != 3 || got.Refused != 1 {
		t.Errorf("prod = %+v, want 3 logins at the fallback level (A, B, the SSO login) and 1 refused", got)
	}

	got, err = repo.CountAMFAFallbackLogins(ctx, tenant, "staging", from, to)
	if err != nil {
		t.Fatalf("count staging: %v", err)
	}
	if got.Fallback != 1 || got.Refused != 0 {
		t.Errorf("staging = %+v, want 1 at the fallback level", got)
	}

	got, err = repo.CountAMFAFallbackLogins(ctx, tenant, "dev", from, to)
	if err != nil {
		t.Fatalf("count dev: %v", err)
	}
	if got.Fallback != 0 || got.Refused != 0 {
		t.Errorf("dev = %+v, want none", got)
	}
}
