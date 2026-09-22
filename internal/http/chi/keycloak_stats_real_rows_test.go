// The login KPI tile end to end: real rows, real repositories, real service,
// through the real HTTP endpoint the dashboard calls.
//
// keycloak_stats_all_realms_test.go covers the same endpoint with the service
// stubbed, which proves the aggregate sums what it is handed. That is not the
// same as the number being right. The bug this endpoint was rewritten for was
// precisely a tile showing zero while the events sat in the table, and a
// stubbed service cannot tell those two states apart: it returns whatever the
// stub was told to, whether or not any query works.
//
// Requires a real PostgreSQL instance via KMT_TEST_DATABASE_DSN; skipped when
// unset.
package chi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	kcpg "github.com/DefensePoint/keycloak-monitoring/internal/keycloak/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// statsRealRowsStack is the endpoint over real storage: nothing between the
// HTTP request and the rows except the code that runs in production.
type statsRealRowsStack struct {
	router   http.Handler
	db       *gorm.DB
	tenantID string
}

func newStatsRealRowsStack(t *testing.T, rbacSvc RBACChecker) *statsRealRowsStack {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	testdb.ShareDatabase(t, dsn)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.AutoMigrate(&database.KeycloakEvent{}, &database.KeycloakRealmInfo{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	tenantID := fmt.Sprintf("kpi-e2e-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		db.Where("tenant_id = ?", tenantID).Delete(&database.KeycloakEvent{})
		db.Unscoped().Where("tenant_id = ?", tenantID).Delete(&database.KeycloakRealmInfo{})
	})

	// The real service over the real repositories. Only the event and realm
	// repositories are reached by this endpoint; the rest stay nil so an
	// unexpected call panics loudly rather than passing quietly.
	svc := keycloak.NewService(
		kcpg.NewEventRepository(db),
		nil,
		nil,
		kcpg.NewRealmRepository(db),
		nil,
		nil,
	)

	return &statsRealRowsStack{
		router:   setupTestKeycloakRouter(newTestKeycloakHandlers(svc, rbacSvc)),
		db:       db,
		tenantID: tenantID,
	}
}

func (s *statsRealRowsStack) seedRealm(t *testing.T, realm string) {
	t.Helper()
	row := &database.KeycloakRealmInfo{TenantID: s.tenantID, RealmID: realm, RealmName: realm}
	if err := s.db.Create(row).Error; err != nil {
		t.Fatalf("seed realm %s: %v", realm, err)
	}
}

func (s *statsRealRowsStack) seedEvents(t *testing.T, realm, eventType string, at time.Time, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		row := &database.KeycloakEvent{
			TenantID:  s.tenantID,
			Time:      at.Add(time.Duration(i) * time.Millisecond),
			EventID:   fmt.Sprintf("%s-%s-%s-%d", s.tenantID, realm, eventType, i),
			RealmID:   realm,
			RealmName: realm,
			EventType: eventType,
			Success:   true,
		}
		if err := s.db.Create(row).Error; err != nil {
			t.Fatalf("seed %s in %s: %v", eventType, realm, err)
		}
	}
}

// get calls the endpoint the dashboard calls and returns the decoded tile.
func (s *statsRealRowsStack) get(t *testing.T, query string) (int, keycloak.EventStats) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet,
		"/api/tenants/"+s.tenantID+"/keycloak/events/stats"+query, nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return rec.Code, keycloak.EventStats{}
	}

	// Decode loosely so the test does not depend on whether the handler wraps
	// its payload in a data envelope.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	var stats keycloak.EventStats
	if raw, ok := envelope["data"]; ok {
		if err := json.Unmarshal(raw, &stats); err != nil {
			t.Fatalf("decode data %q: %v", string(raw), err)
		}
		return rec.Code, stats
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return rec.Code, stats
}

// The All Realms tile, which is the view the dashboard opens on, against rows
// somebody can count by hand. This is the assertion the stubbed suite cannot
// make: the aggregate is computed from the events, not from a stub.
func TestEventStatsRealRows_AllRealmsCountsEveryRealm(t *testing.T) {
	stack := newStatsRealRowsStack(t, &mockRBACChecker{isAdmin: true})

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	stack.seedRealm(t, "prod")
	stack.seedRealm(t, "staging")
	stack.seedEvents(t, "prod", "LOGIN", base, 5)
	stack.seedEvents(t, "prod", "LOGIN_ERROR", base, 2)
	stack.seedEvents(t, "staging", "LOGIN", base, 3)

	code, all := stack.get(t, "?hours=24")
	if code != http.StatusOK {
		t.Fatalf("All Realms status = %d, want 200", code)
	}
	if all.LoginCount != 8 {
		t.Errorf("All Realms login_count = %d, want 8 (prod 5 + staging 3)", all.LoginCount)
	}
	if all.LoginErrorCount != 2 {
		t.Errorf("All Realms login_error_count = %d, want 2", all.LoginErrorCount)
	}

	// And the per-realm views must add up to it, or an operator switching
	// between them sees two numbers that disagree.
	_, prod := stack.get(t, "?hours=24&realm=prod")
	_, staging := stack.get(t, "?hours=24&realm=staging")
	if prod.LoginCount != 5 || staging.LoginCount != 3 {
		t.Errorf("per-realm logins = prod %d, staging %d; want 5 and 3",
			prod.LoginCount, staging.LoginCount)
	}
	if prod.LoginCount+staging.LoginCount != all.LoginCount {
		t.Errorf("per-realm logins sum to %d but All Realms reports %d",
			prod.LoginCount+staging.LoginCount, all.LoginCount)
	}
}

// The date-range picker, end to end. parseTimeWindow is unit-tested for parsing
// and the repository for filtering; this is the only place that proves the two
// are wired together, which is what a regression would break.
func TestEventStatsRealRows_RangePickerNarrowsTheTile(t *testing.T) {
	stack := newStatsRealRowsStack(t, &mockRBACChecker{isAdmin: true})

	now := time.Now().UTC().Truncate(time.Second)
	stack.seedRealm(t, "prod")
	stack.seedEvents(t, "prod", "LOGIN", now.Add(-48*time.Hour), 4) // older
	stack.seedEvents(t, "prod", "LOGIN", now.Add(-1*time.Hour), 6)  // recent

	_, wide := stack.get(t, fmt.Sprintf("?start=%s&end=%s",
		now.Add(-72*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339)))
	if wide.LoginCount != 10 {
		t.Errorf("a 72h range reports %d logins, want 10", wide.LoginCount)
	}

	_, narrow := stack.get(t, fmt.Sprintf("?start=%s&end=%s",
		now.Add(-24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339)))
	if narrow.LoginCount != 6 {
		t.Errorf("a 24h range reports %d logins, want 6; the picker is not reaching the query",
			narrow.LoginCount)
	}

	if wide.LoginCount == narrow.LoginCount {
		t.Error("both ranges returned the same number, so changing the range does nothing")
	}
}

// A realm-scoped caller's All Realms tile must cover only their realms. This is
// the confidentiality edge of the KPI: a total that includes realms the caller
// cannot open leaks those realms' activity through a single number.
func TestEventStatsRealRows_ScopedCallerSeesOnlyTheirRealm(t *testing.T) {
	stack := newStatsRealRowsStack(t, &mockRBACChecker{policies: policyFor("PLACEHOLDER", "prod")})

	// The policy has to name the generated tenant, so rebuild the checker now
	// that the id exists.
	stack.router = setupTestKeycloakRouter(newTestKeycloakHandlers(
		keycloak.NewService(kcpg.NewEventRepository(stack.db), nil, nil,
			kcpg.NewRealmRepository(stack.db), nil, nil),
		&mockRBACChecker{policies: policyFor(stack.tenantID, "prod")},
	))

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	stack.seedRealm(t, "prod")
	stack.seedRealm(t, "secret")
	stack.seedEvents(t, "prod", "LOGIN", base, 4)
	stack.seedEvents(t, "secret", "LOGIN", base, 9)

	code, got := stack.get(t, "?hours=24")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if got.LoginCount != 4 {
		t.Errorf("scoped caller's All Realms login_count = %d, want 4; "+
			"secret's 9 must not be summed into a realm they cannot read", got.LoginCount)
	}
}
