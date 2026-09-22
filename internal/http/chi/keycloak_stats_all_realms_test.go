package chi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
)

// The dashboard KPI strip runs in All Realms mode by default, because
// default_realm ships empty. Refusing that query left the strip rendering zero
// failed and zero successful logins while failures existed, which is the one
// number a SOC operator must be able to trust. Aggregate instead, and only over
// the realms the caller is allowed to read.

type statsStubService struct {
	*stubKeycloakService
	statsByRealm map[string]*keycloak.EventStats
	calls        []string
	windows      [][2]time.Time
}

func (s *statsStubService) GetEventStats(_ context.Context, _, realmName string, from, to time.Time) (*keycloak.EventStats, error) {
	s.calls = append(s.calls, realmName)
	s.windows = append(s.windows, [2]time.Time{from, to})
	if st, ok := s.statsByRealm[realmName]; ok {
		return st, nil
	}
	return &keycloak.EventStats{Realm: realmName}, nil
}

func newStatsStub() *statsStubService {
	return &statsStubService{
		stubKeycloakService: &stubKeycloakService{
			realms: []*domain.KeycloakRealmInfo{
				{TenantID: "tenant-a", RealmName: "realmA"},
				{TenantID: "tenant-a", RealmName: "realmB"},
			},
		},
		statsByRealm: map[string]*keycloak.EventStats{
			"realmA": {Realm: "realmA", LoginCount: 3, LoginErrorCount: 2, TotalEvents: 5},
			"realmB": {Realm: "realmB", LoginCount: 7, LoginErrorCount: 1, TotalEvents: 8},
		},
	}
}

func statsResponse(t *testing.T, rec *httptest.ResponseRecorder) keycloak.EventStats {
	t.Helper()
	var got keycloak.EventStats
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	return got
}

func TestEventStats_AllRealmsAggregatesRatherThanRefusing(t *testing.T) {
	svc := newStatsStub()
	rbacSvc := &mockRBACChecker{isAdmin: true}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/events/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("All Realms must return stats, got %d; body = %s", rec.Code, rec.Body.String())
	}

	got := statsResponse(t, rec)
	if got.LoginCount != 10 {
		t.Errorf("login_count = %d, want 10 (3 + 7)", got.LoginCount)
	}
	if got.LoginErrorCount != 3 {
		t.Errorf("login_error_count = %d, want 3 (2 + 1)", got.LoginErrorCount)
	}
	if got.TotalEvents != 13 {
		t.Errorf("total_events = %d, want 13 (5 + 8)", got.TotalEvents)
	}
}

func TestEventStats_AllRealmsSumsOnlyRealmsTheCallerMayRead(t *testing.T) {
	svc := newStatsStub()
	rbacSvc := &mockRBACChecker{policies: policyFor("tenant-a", "realmB")}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/events/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	sort.Strings(svc.calls)
	if len(svc.calls) != 1 || svc.calls[0] != "realmB" {
		t.Fatalf("stats were read for %v; a caller scoped to realmB must not reach realmA", svc.calls)
	}

	got := statsResponse(t, rec)
	if got.LoginCount != 7 || got.LoginErrorCount != 1 {
		t.Errorf("counts = %d/%d, want realmB's 7/1 only", got.LoginCount, got.LoginErrorCount)
	}
}

func TestEventStats_AllRealmsRefusesACallerWithNoRealms(t *testing.T) {
	svc := newStatsStub()
	rbacSvc := &mockRBACChecker{policies: policyFor("tenant-a")}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/events/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a caller with no permitted realms must not get stats, got 200; body = %s", rec.Body.String())
	}
	if len(svc.calls) != 0 {
		t.Errorf("no realm may be queried for a caller with none permitted, queried %v", svc.calls)
	}
}

func TestEventStats_HonoursStartAndEndRatherThanTheHoursDefault(t *testing.T) {
	svc := newStatsStub()
	rbacSvc := &mockRBACChecker{
		policies:           policyFor("tenant-a", "realmA"),
		hasAccessToRealmFn: func(_ context.Context, _ uint, _, realm string) (bool, error) { return realm == "realmA", nil },
	}
	router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc))

	start := time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Second)
	end := time.Now().UTC().Truncate(time.Second)
	url := "/api/tenants/tenant-a/keycloak/events/stats?realm=realmA" +
		"&start=" + start.Format(time.RFC3339) + "&end=" + end.Format(time.RFC3339)

	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	if len(svc.windows) != 1 {
		t.Fatalf("expected one stats read, got %d", len(svc.windows))
	}

	gotFrom, gotTo := svc.windows[0][0].UTC(), svc.windows[0][1].UTC()
	if !gotFrom.Equal(start) {
		t.Errorf("from = %s, want the requested %s; the hours default is overriding start", gotFrom, start)
	}
	if !gotTo.Equal(end) {
		t.Errorf("to = %s, want the requested %s", gotTo, end)
	}
}
