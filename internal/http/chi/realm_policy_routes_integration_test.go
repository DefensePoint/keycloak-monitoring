// Realm-policy coverage for the routes realm_policy_integration_test.go did not
// reach: a single event by ID, and the Keycloak event, statistics, dashboard
// and realm-list routes. Same stack, same column shapes, same rule: a caller
// restricted to policyRealmAllowed must never read policyRealmDenied, and an
// unrestricted caller must still read both.
//
// Requires KMT_TEST_DATABASE_DSN, like the rest of the realm-policy suite.
package chi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
)

// policyRealmLookalikes are realms whose names a careless filter would let
// through to a caller allowed policyRealmAllowed: one extends the name, one
// differs only in case.
var policyRealmLookalikes = []string{policyRealmAllowed + "x", strings.ToLower(policyRealmAllowed)}

// policyKeycloakService records the realm every realm-scoped Keycloak read ran
// for, and answers each one with data stamped with that realm, so a test can
// see both what was asked for and what came back.
type policyKeycloakService struct {
	*stubKeycloakService
	realms *mockRealmLister
	reads  []string // "<what>:<realm>" for every realm-scoped read
}

func (s *policyKeycloakService) reset() { s.reads = nil }

func (s *policyKeycloakService) readRealms(what string) []string {
	var out []string
	for _, r := range s.reads {
		if realm, ok := strings.CutPrefix(r, what+":"); ok {
			out = append(out, realm)
		}
	}
	return out
}

func (s *policyKeycloakService) ListEventsByRealm(_ context.Context, tenantID, realm string, _, _ time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.reads = append(s.reads, "events:"+realm)
	return []*domain.KeycloakEvent{{TenantID: tenantID, EventID: "kev-" + realm, RealmName: realm}}, nil
}

func (s *policyKeycloakService) ListEventsByType(_ context.Context, tenantID, realm, eventType string, _, _ time.Time, _, _ int) ([]*domain.KeycloakEvent, error) {
	s.reads = append(s.reads, "events:"+realm)
	return []*domain.KeycloakEvent{{TenantID: tenantID, EventID: "kev-" + realm, RealmName: realm, EventType: eventType}}, nil
}

func (s *policyKeycloakService) GetEventStats(_ context.Context, _, realm string, _, _ time.Time) (*keycloak.EventStats, error) {
	s.reads = append(s.reads, "stats:"+realm)
	return &keycloak.EventStats{Realm: realm, LoginCount: 1, TotalEvents: 1}, nil
}

// GetDashboard answers as the real service does: for one realm, that realm's
// view; for none, a summary of every realm the tenant holds.
func (s *policyKeycloakService) GetDashboard(_ context.Context, _, realm string) (*keycloak.Dashboard, error) {
	s.reads = append(s.reads, "dashboard:"+realm)
	if realm != "" {
		return &keycloak.Dashboard{Realm: realm}, nil
	}
	d := &keycloak.Dashboard{}
	for _, info := range s.realms.realms {
		d.Realms = append(d.Realms, &keycloak.RealmSummary{RealmName: info.RealmName})
	}
	d.TotalRealms = len(d.Realms)
	return d, nil
}

func (s *policyKeycloakService) ListRealms(context.Context, string) ([]*domain.KeycloakRealmInfo, error) {
	return s.realms.realms, nil
}

// addLookalikeRealms makes the tenant also hold the look-alike realms for the
// rest of the test.
func (s *policyStack) addLookalikeRealms(t *testing.T) {
	t.Helper()
	before := s.realms.realms
	s.realms.realms = append(slices.Clone(before), realms(policyRealmLookalikes...)...)
	t.Cleanup(func() { s.realms.realms = before })
}

// restrictedTo reports the realms this column shape may read, nil meaning all.
func (c policyColumn) restrictedTo() []string {
	switch {
	case c.unrestrict:
		return nil
	case c.allowsAllowedRealm():
		return []string{policyRealmAllowed}
	default:
		return []string{}
	}
}

// GET /events/{eventID}: an event in another realm must be indistinguishable
// from one that does not exist, for every column shape.
func TestRealmPolicy_EventByIDRefusesAnEventOutsideTheCallersRealms(t *testing.T) {
	stack := newPolicyStack(t)
	stack.addLookalikeRealms(t)

	// One event per realm and source system; the event ID names its realm.
	var all []*domain.Event
	for _, realm := range append([]string{policyRealmAllowed, policyRealmDenied}, policyRealmLookalikes...) {
		for _, system := range []string{"keycloak", "amfa"} {
			all = append(all, &domain.Event{TenantID: stack.tenantID, EventID: system + "-in-" + realm, Source: system + ":" + realm})
		}
	}
	stack.events.events = all

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()
			missing := stack.do(t, http.MethodGet, "/events/no-such-event", column.name)
			if missing.Code != http.StatusNotFound {
				t.Fatalf("a missing event answered %d, want 404; body = %s", missing.Code, missing.Body.String())
			}

			for _, event := range all {
				realm := strings.SplitN(event.Source, ":", 2)[1]
				allowed := column.unrestrict || (column.allowsAllowedRealm() && realm == policyRealmAllowed)

				rec := stack.do(t, http.MethodGet, "/events/"+event.EventID, column.name)
				switch {
				case allowed && rec.Code != http.StatusOK:
					t.Errorf("%s: status = %d, want 200 for a realm the caller may read; body = %s", event.EventID, rec.Code, rec.Body.String())
				case !allowed && (rec.Code != missing.Code || rec.Body.String() != missing.Body.String()):
					t.Errorf("%s answers %d %s but a missing event answers %d %s: the event leaked, or the difference tells the caller it exists",
						event.EventID, rec.Code, rec.Body.String(), missing.Code, missing.Body.String())
				}
			}
		})
	}
}

// keycloakEventsResponse is the part of a Keycloak events reply these tests read.
type keycloakEventsResponse struct {
	Events []*domain.KeycloakEvent `json:"events"`
}

// GET /keycloak/events, with and without a type filter, for every realm and
// every column shape.
func TestRealmPolicy_KeycloakEventsScope(t *testing.T) {
	stack := newPolicyStack(t)
	stack.addLookalikeRealms(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			for _, filter := range []string{"", "&type=LOGIN_ERROR"} {
				for _, realm := range append([]string{policyRealmAllowed, policyRealmDenied}, policyRealmLookalikes...) {
					stack.reset()
					allowed := column.unrestrict || (column.allowsAllowedRealm() && realm == policyRealmAllowed)

					rec := stack.do(t, http.MethodGet, "/keycloak/events?realm="+realm+filter, column.name)
					if !allowed {
						if rec.Code != http.StatusForbidden {
							t.Errorf("realm %s%s: status = %d, want 403; body = %s", realm, filter, rec.Code, rec.Body.String())
						}
						if reads := stack.keycloak.readRealms("events"); len(reads) != 0 {
							t.Errorf("realm %s%s: events were read for %v on behalf of a caller who may not read it", realm, filter, reads)
						}
						continue
					}
					if rec.Code != http.StatusOK {
						t.Errorf("realm %s%s: status = %d, want 200; body = %s", realm, filter, rec.Code, rec.Body.String())
						continue
					}
					var body keycloakEventsResponse
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
						t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
					}
					if reads := stack.keycloak.readRealms("events"); !slices.Equal(reads, []string{realm}) {
						t.Errorf("realm %s%s: events were read for %v, want only %s", realm, filter, reads, realm)
					}
					for _, e := range body.Events {
						if e.RealmName != realm {
							t.Errorf("realm %s%s: response carries an event from %s", realm, filter, e.RealmName)
						}
					}
				}
			}
		})
	}
}

// GET /keycloak/events/stats for a named realm, and the all-realms aggregate,
// which must add up only the realms the caller may read.
func TestRealmPolicy_KeycloakEventStatsScope(t *testing.T) {
	stack := newPolicyStack(t)
	stack.addLookalikeRealms(t)
	every := append([]string{policyRealmDenied, policyRealmAllowed}, policyRealmLookalikes...)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()
			rec := stack.do(t, http.MethodGet, "/keycloak/events/stats?realm="+policyRealmDenied, column.name)
			if column.unrestrict {
				if rec.Code != http.StatusOK {
					t.Errorf("named denied realm: status = %d, want 200 for an unrestricted caller; body = %s", rec.Code, rec.Body.String())
				}
			} else {
				if rec.Code != http.StatusForbidden {
					t.Errorf("named denied realm: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
				}
				if reads := stack.keycloak.readRealms("stats"); len(reads) != 0 {
					t.Errorf("named denied realm: statistics were read for %v", reads)
				}
			}

			stack.reset()
			rec = stack.do(t, http.MethodGet, "/keycloak/events/stats", column.name)
			reads := stack.keycloak.readRealms("stats")
			slices.Sort(reads)
			want := column.restrictedTo()
			switch {
			case want == nil:
				wantAll := slices.Clone(every)
				slices.Sort(wantAll)
				if rec.Code != http.StatusOK || !slices.Equal(reads, wantAll) {
					t.Errorf("aggregate: status %d over %v, want 200 over every realm %v", rec.Code, reads, wantAll)
				}
			case len(want) == 0:
				if rec.Code != http.StatusForbidden || len(reads) != 0 {
					t.Errorf("aggregate: status %d over %v, want 403 reading nothing for a caller allowed no realm", rec.Code, reads)
				}
			default:
				if rec.Code != http.StatusOK || !slices.Equal(reads, want) {
					t.Errorf("aggregate: status %d over %v, want 200 over only %v", rec.Code, reads, want)
				}
				var body keycloak.EventStats
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
				}
				if body.TotalEvents != int64(len(want)) {
					t.Errorf("aggregate total_events = %d, want %d: it counts realms the caller may not read", body.TotalEvents, len(want))
				}
			}
		})
	}
}

// GET /keycloak/dashboard: the all-realms view lists only the caller's realms,
// and a named realm the caller may not read is refused before any read.
func TestRealmPolicy_KeycloakDashboardScope(t *testing.T) {
	stack := newPolicyStack(t)
	stack.addLookalikeRealms(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()
			rec := stack.do(t, http.MethodGet, "/keycloak/dashboard", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("all realms: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			var body keycloak.Dashboard
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}
			var got []string
			for _, summary := range body.Realms {
				got = append(got, summary.RealmName)
			}
			if want := column.restrictedTo(); want != nil {
				if !slices.Equal(got, want) {
					t.Errorf("all realms: dashboard lists %v, want only %v", got, want)
				}
				if body.TotalRealms != len(want) {
					t.Errorf("all realms: total_realms = %d, want %d: the count includes realms the caller may not read", body.TotalRealms, len(want))
				}
			} else if len(got) != len(stack.realms.realms) {
				t.Errorf("all realms: an unrestricted caller sees %v, want every realm", got)
			}

			stack.reset()
			rec = stack.do(t, http.MethodGet, "/keycloak/dashboard?realm="+policyRealmDenied, column.name)
			if column.unrestrict {
				if rec.Code != http.StatusOK {
					t.Errorf("named denied realm: status = %d, want 200 for an unrestricted caller", rec.Code)
				}
				return
			}
			if rec.Code != http.StatusForbidden {
				t.Errorf("named denied realm: status = %d, want 403; body = %s", rec.Code, rec.Body.String())
			}
			if reads := stack.keycloak.readRealms("dashboard"); len(reads) != 0 {
				t.Errorf("named denied realm: the dashboard was read for %v", reads)
			}
		})
	}
}

// GET /keycloak/realms lists only the realms the caller may read.
func TestRealmPolicy_KeycloakRealmListScope(t *testing.T) {
	stack := newPolicyStack(t)
	stack.addLookalikeRealms(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			rec := stack.do(t, http.MethodGet, "/keycloak/realms", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Realms []*domain.KeycloakRealmInfo `json:"realms"`
				Count  int                         `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
			}
			var got []string
			for _, r := range body.Realms {
				got = append(got, r.RealmName)
			}
			want := column.restrictedTo()
			if want == nil {
				if len(got) != len(stack.realms.realms) {
					t.Errorf("an unrestricted caller sees %v, want every realm", got)
				}
				return
			}
			if !slices.Equal(got, want) {
				t.Errorf("realm list = %v, want only %v", got, want)
			}
			if body.Count != len(want) {
				t.Errorf("count = %d, want %d", body.Count, len(want))
			}
		})
	}
}
