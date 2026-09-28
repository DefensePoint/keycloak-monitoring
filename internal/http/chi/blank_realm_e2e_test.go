// End-to-end coverage for a blank allowed_realm, against real alert rows.
//
// The suite in realm_policy_integration_test.go stubs the alerts service, so it
// proves which realm scope reaches the service but never what data comes back.
// This file wires the REAL alerts repository and service into the real router
// over real PostgreSQL rows, so a leak shows up as other realms' alerts in the
// HTTP response body rather than as an argument in a stub.
//
// Requires KMT_TEST_DATABASE_DSN; skipped when unset.
package chi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	rbacpostgres "github.com/DefensePoint/keycloak-monitoring/internal/rbac/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// blankRealmStack is the real stack: real RBAC over real policy rows, the real
// alerts repository over real alert rows, and the real router.
type blankRealmStack struct {
	router   http.Handler
	db       *gorm.DB
	tenantID string
}

func newBlankRealmStack(t *testing.T) *blankRealmStack {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping e2e test")
	}

	testdb.ShareDatabase(t, dsn)

	log := logger.NewNoop()
	client, err := database.NewClient(dsnConfig(t, dsn), log)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	run := time.Now().UnixNano()
	tenantID := fmt.Sprintf("blank-realm-tenant-%d", run)

	// Two active alerts, one per realm. A correctly scoped caller sees only
	// the realm it is entitled to; a leak shows up as a count of 2.
	for _, realm := range []string{policyRealmAllowed, policyRealmDenied} {
		alertID := fmt.Sprintf("blank-realm-%d-%s", run, realm)
		err := db.Exec(`
			INSERT INTO configuration_alerts
				(tenant_id, alert_id, source, type, severity, status, title,
				 resource_type, resource_id, resource_name, realm_name,
				 first_detected, last_seen, created_at, updated_at)
			VALUES (?, ?, 'configuration', 'TEST_ALERT', 'high', 'active', ?,
				'realm', ?, ?, ?, NOW(), NOW(), NOW(), NOW())`,
			tenantID, alertID, "alert in "+realm, realm, realm, realm).Error
		if err != nil {
			t.Fatalf("seed alert for %s: %v", realm, err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM configuration_alerts WHERE tenant_id = ?`, tenantID)
	})

	policyRepo := rbacpostgres.NewTenantPolicyRepository(client)
	rbacService := rbac.NewService(
		rbacpostgres.NewRoleRepository(client),
		rbacpostgres.NewPermissionRepository(client),
		rbacpostgres.NewUserRoleRepository(client),
		policyRepo,
		log,
	)

	// The real repository and the real service, not a stub.
	realAlerts := alerts.NewService(alertspostgres.NewRepository(db), log)

	checker := &policyRBACAdapter{service: rbacService}
	rbacMW := NewRBACMiddleware(checker, log)
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var id uint
			_, _ = fmt.Sscanf(r.Header.Get("X-Test-User"), "%d", &id)
			ctx := context.WithValue(r.Context(), UserInfoKey,
				&UserDetails{ID: id, Email: "blank-realm@example.invalid"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	handlers := NewAlertsHandlers(realAlerts, checker, &mockOperatorService{}, nil, log, authMW, rbacMW)
	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		handlers.RegisterTenantRoutes(r)
	})

	return &blankRealmStack{router: r, db: db, tenantID: tenantID}
}

// seedCaller creates a user whose policy carries exactly the given
// allowed_realms JSON, and returns its id. A nil literal means unrestricted.
func (s *blankRealmStack) seedCaller(t *testing.T, label, allowedRealmsJSON string) uint {
	t.Helper()

	run := time.Now().UnixNano()
	roleID := seedPolicyRole(t, s.db, run)
	userID := seedPolicyUser(t, s.db, run, label, roleID)

	sql := fmt.Sprintf(`
		INSERT INTO tenant_policies
			(user_id, tenant_id, allowed_realms, granted_by, granted_at, created_at, updated_at)
		VALUES (?, ?, %s, 'test', NOW(), NOW(), NOW())`, allowedRealmsJSON)
	if err := s.db.Exec(sql, userID, s.tenantID).Error; err != nil {
		t.Fatalf("seed policy for %s: %v", label, err)
	}
	t.Cleanup(func() {
		s.db.Unscoped().Where("user_id = ? AND tenant_id = ?", userID, s.tenantID).
			Delete(&database.TenantPolicy{})
	})

	return userID
}

// statsTotal drives GET /alerts/stats as the given caller and returns the
// total_active the API actually reported.
func (s *blankRealmStack) statsTotal(t *testing.T, userID uint) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet,
		"/api/tenants/"+s.tenantID+"/alerts/stats", nil)
	req.Header.Set("X-Test-User", fmt.Sprintf("%d", userID))
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	// The handler wraps the payload, so decode loosely and find total_active.
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode stats body %q: %v", rec.Body.String(), err)
	}
	total, ok := findTotalActive(body)
	if !ok {
		t.Fatalf("no total_active in stats body: %s", rec.Body.String())
	}
	return total
}

// findTotalActive walks the response for total_active, so the test does not
// depend on whether the handler wraps its payload in a data envelope.
func findTotalActive(v any) (int, bool) {
	switch typed := v.(type) {
	case map[string]any:
		if raw, ok := typed["total_active"]; ok {
			if f, ok := raw.(float64); ok {
				return int(f), true
			}
		}
		for _, nested := range typed {
			if total, ok := findTotalActive(nested); ok {
				return total, true
			}
		}
	}
	return 0, false
}

// The leak, end to end, against real rows. Three callers over the same two
// alerts discriminate the fix from both over- and under-blocking:
//
//	unrestricted  -> 2, the whole tenant
//	["realmB"]    -> 1, exactly its realm
//	[""]          -> 0, entitled to nothing
//
// Before the resolver dropped blanks, the last caller reported 2: the empty
// realm was stripped inside GetStatistics and the realm filter was then
// omitted entirely, so a caller entitled to no realm counted every alert in
// the tenant.
func TestBlankRealm_StatsLeakEndToEnd(t *testing.T) {
	stack := newBlankRealmStack(t)

	tests := []struct {
		name      string
		realmsSQL string
		want      int
	}{
		{name: "unrestricted caller reads the whole tenant", realmsSQL: "NULL", want: 2},
		{name: "caller scoped to one realm reads only it", realmsSQL: `'["` + policyRealmAllowed + `"]'::jsonb`, want: 1},
		{name: "caller with an empty realm list reads nothing", realmsSQL: `'[]'::jsonb`, want: 0},
		{name: "caller with a blank realm reads nothing", realmsSQL: `'[""]'::jsonb`, want: 0},
		{name: "caller with only blank realms reads nothing", realmsSQL: `'["", "  "]'::jsonb`, want: 0},
		{name: "caller with a blank beside a real realm reads only the real one", realmsSQL: `'["", "` + policyRealmAllowed + `"]'::jsonb`, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := stack.seedCaller(t, tt.name, tt.realmsSQL)
			got := stack.statsTotal(t, userID)
			if got != tt.want {
				t.Errorf("total_active = %d, want %d", got, tt.want)
				if got == 2 && tt.want == 0 {
					t.Errorf("LEAK: a caller entitled to no realm counted every alert in the tenant")
				}
			}
		})
	}
}
