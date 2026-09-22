// Behavioural coverage for tenant_policies.allowed_realms, driven through the
// real chi router against real rows written by the real repository.
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the tests are skipped when
// unset. Run with:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=5432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./internal/http/chi/... -run TestRealmPolicy
package chi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	rbacpostgres "github.com/DefensePoint/keycloak-monitoring/internal/rbac/postgres"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

// The two realms every caller in this file is scoped against. A restricted
// caller may read realmB and must never read realmA.
const (
	policyRealmAllowed = "realmB"
	policyRealmDenied  = "realmA"
)

// policyColumn is one shape the allowed_realms column can hold. writeSQL is the
// literal the seeder writes; an empty writeSQL means no policy row at all.
type policyColumn struct {
	name        string
	writeSQL    string
	unrestrict  bool
	viaAPI      []string
	useAPIWrite bool
	// grantsNothing marks a shape whose realms match no realm at all, so the
	// caller is entitled to read nothing. Set explicitly because the shape is
	// not detectable by sniffing writeSQL for '[]'.
	grantsNothing bool
}

// policyColumns covers every shape a tenant_policies row can carry.
func policyColumns() []policyColumn {
	return []policyColumn{
		{name: "no policy row at all", unrestrict: true},
		{name: "SQL NULL column", writeSQL: "NULL", unrestrict: true},
		{name: "stored null literal", writeSQL: `'null'::jsonb`, unrestrict: true},
		{name: "stored empty list", writeSQL: `'[]'::jsonb`},
		{name: "stored realm list", writeSQL: `'["` + policyRealmAllowed + `"]'::jsonb`},
		{name: "written through CreatePolicy with no realms", useAPIWrite: true, unrestrict: true},
		{name: "written through CreatePolicy with an empty list", useAPIWrite: true, viaAPI: []string{}},
		{name: "written through CreatePolicy with a realm", useAPIWrite: true, viaAPI: []string{policyRealmAllowed}},
		// A blank realm names no realm, so a policy of nothing but blanks
		// grants nothing, exactly like a stored '[]'. Left in the scope it
		// reads as "no filter" in alert statistics and as the all-realms
		// aggregate in the AMFA KPIs, turning no entitlement into the whole
		// tenant.
		{name: "stored list of one empty realm", writeSQL: `'[""]'::jsonb`, grantsNothing: true},
		{name: "stored list of only blank realms", writeSQL: `'["", "   "]'::jsonb`, grantsNothing: true},
		// A blank alongside a real realm keeps the real one and drops the
		// blank, so this caller reads exactly the realm it names.
		{name: "stored blank beside a real realm", writeSQL: `'["", "` + policyRealmAllowed + `"]'::jsonb`},
	}
}

// allowsAllowedRealm reports whether this caller may read policyRealmAllowed at
// all. Only the two empty-list shapes may not.
func (c policyColumn) allowsAllowedRealm() bool {
	if c.grantsNothing {
		return false
	}
	if c.unrestrict {
		return true
	}
	if c.useAPIWrite {
		return len(c.viaAPI) > 0
	}
	return !strings.Contains(c.writeSQL, `'[]'`)
}

// policyStack is a running slice of the API: real RBAC service over real rows,
// real chi routes and middleware, stubbed data services.
type policyStack struct {
	router   http.Handler
	db       *gorm.DB
	tenantID string
	users    map[string]uint

	events *stubEventRepo
	alerts *policyAlertsService
	amfa   *stubAmfaService
}

// dsnConfig turns a libpq key=value DSN into the config NewClient builds its own
// DSN from. Nothing here is echoed back to the log: the DSN carries a password.
func dsnConfig(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()

	cfg := &config.DatabaseConfig{MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second}
	for _, pair := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		switch key {
		case "host":
			cfg.Host = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("KMT_TEST_DATABASE_DSN has a non-numeric port")
			}
			cfg.Port = port
		case "dbname":
			cfg.Database = value
		case "user":
			cfg.User = value
		case "password":
			cfg.Password = value
		case "sslmode":
			cfg.SSLMode = value
		}
	}
	return cfg
}

// policyAlertsService records the realm scope every alerts query ran with, and
// serves one alert per realm so a by-ID read can be shown to cross realms or
// not.
type policyAlertsService struct {
	mockAlertsService

	listCalls  int
	lastOpts   *alerts.ListOptions
	statsCalls int
	lastStats  []string

	mutations []string
}

func (s *policyAlertsService) ListAlerts(_ context.Context, tenantID string, opts *alerts.ListOptions) ([]*domain.Alert, int64, error) {
	s.listCalls++
	s.lastOpts = opts
	return []*domain.Alert{{AlertID: "a-1", TenantID: tenantID, RealmName: policyRealmAllowed}}, 1, nil
}

func (s *policyAlertsService) GetStatistics(_ context.Context, _ string, realmNames ...string) (*alerts.Statistics, error) {
	s.statsCalls++
	s.lastStats = realmNames
	return &alerts.Statistics{TotalActive: 3, BySeverity: map[string]int{}, ByType: map[string]int{}}, nil
}

// GetAlert answers for both realms: "alert-in-<realm>" belongs to that realm.
func (s *policyAlertsService) GetAlert(_ context.Context, tenantID, alertID string) (*domain.Alert, error) {
	for _, realm := range []string{policyRealmAllowed, policyRealmDenied} {
		if alertID == "alert-in-"+realm {
			return &domain.Alert{AlertID: alertID, TenantID: tenantID, RealmName: realm, Status: domain.AlertStatusActive}, nil
		}
	}
	return nil, fmt.Errorf("alert %s not found", alertID)
}

func (s *policyAlertsService) UpdateStatus(ctx context.Context, tenantID, alertID string, status domain.AlertStatus, _ string) (*domain.Alert, error) {
	s.mutations = append(s.mutations, "update:"+alertID)
	alert, err := s.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}
	alert.Status = status
	return alert, nil
}

func (s *policyAlertsService) ResolveAlert(ctx context.Context, tenantID, alertID string) (*domain.Alert, error) {
	s.mutations = append(s.mutations, "resolve:"+alertID)
	alert, err := s.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}
	alert.Status = domain.AlertStatusResolved
	return alert, nil
}

func (s *policyAlertsService) DeleteAlert(_ context.Context, _, alertID string) error {
	s.mutations = append(s.mutations, "delete:"+alertID)
	return nil
}

// newPolicyStack wires the real RBAC service over a real database, seeds one
// caller per column shape, and mounts the real tenant routes.
func newPolicyStack(t *testing.T) *policyStack {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	log := logger.NewNoop()
	client, err := database.NewClient(dsnConfig(t, dsn), log)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	db := client.DB()

	run := time.Now().UnixNano()
	tenantID := fmt.Sprintf("realm-policy-tenant-%d", run)

	policyRepo := rbacpostgres.NewTenantPolicyRepository(client)
	rbacService := rbac.NewService(
		rbacpostgres.NewRoleRepository(client),
		rbacpostgres.NewPermissionRepository(client),
		rbacpostgres.NewUserRoleRepository(client),
		policyRepo,
		log,
	)

	roleID := seedPolicyRole(t, db, run)
	users := make(map[string]uint, len(policyColumns()))
	for _, column := range policyColumns() {
		userID := seedPolicyUser(t, db, run, column.name, roleID)
		users[column.name] = userID
		seedPolicyRow(t, db, policyRepo, tenantID, userID, column)
	}

	stack := &policyStack{
		db:       db,
		tenantID: tenantID,
		users:    users,
		events:   &stubEventRepo{events: []*domain.Event{{EventID: "e1"}}, total: 1},
		alerts:   &policyAlertsService{},
		amfa:     &stubAmfaService{},
	}
	stack.router = mountPolicyRoutes(stack, rbacService, log)
	return stack
}

// seedPolicyRole creates a role carrying every permission these routes gate on,
// reusing the permission rows the schema already seeds.
func seedPolicyRole(t *testing.T, db *gorm.DB, run int64) uint {
	t.Helper()

	role := &database.Role{
		Name:        fmt.Sprintf("realm-policy-role-%d", run),
		DisplayName: "realm policy test role",
		IsActive:    true,
	}
	if err := db.Create(role).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&database.Role{}, role.ID) })

	wanted := []string{
		rbac.PermissionTenantsRead,
		rbac.PermissionKeycloakRead,
		rbac.PermissionAmfaRead,
		rbac.PermissionAlertsRead,
		rbac.PermissionAlertsAcknowledge,
		rbac.PermissionAlertsResolve,
		rbac.PermissionAlertsDelete,
	}
	for _, name := range wanted {
		resource, action, _ := strings.Cut(name, ":")
		permission := &database.Permission{Name: name, DisplayName: name, Resource: resource, Action: action}
		if err := db.Where("name = ?", name).FirstOrCreate(permission, database.Permission{Name: name}).Error; err != nil {
			t.Fatalf("seed permission %s: %v", name, err)
		}
		link := &database.RolePermission{RoleID: role.ID, PermissionID: permission.ID}
		if err := db.Create(link).Error; err != nil {
			t.Fatalf("link permission %s: %v", name, err)
		}
		t.Cleanup(func() { db.Unscoped().Delete(&database.RolePermission{}, link.ID) })
	}

	return role.ID
}

// seedPolicyUser creates a user holding the test role globally, so tenant access
// comes from the tenants:read permission rather than from a policy row.
func seedPolicyUser(t *testing.T, db *gorm.DB, run int64, label string, roleID uint) uint {
	t.Helper()

	slug := strings.NewReplacer(" ", "-", "'", "").Replace(label)
	user := &database.User{
		Subject:  fmt.Sprintf("realm-policy-%d-%s", run, slug),
		Email:    fmt.Sprintf("realm-policy-%d-%s@example.invalid", run, slug),
		Username: fmt.Sprintf("realm-policy-%d-%s", run, slug),
		IsActive: true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&database.User{}, user.ID) })

	userRole := &database.UserRole{UserID: user.ID, RoleID: roleID, AssignedAt: time.Now().UTC()}
	if err := db.Create(userRole).Error; err != nil {
		t.Fatalf("assign role to %s: %v", label, err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&database.UserRole{}, userRole.ID) })

	return user.ID
}

// seedPolicyRow writes the caller's tenant_policies row in the shape under test.
// The useAPIWrite shapes go through the repository.
func seedPolicyRow(t *testing.T, db *gorm.DB, repo rbac.TenantPolicyRepository, tenantID string, userID uint, column policyColumn) {
	t.Helper()

	t.Cleanup(func() {
		db.Unscoped().Where("user_id = ? AND tenant_id = ?", userID, tenantID).Delete(&database.TenantPolicy{})
	})

	if column.useAPIWrite {
		err := repo.CreatePolicy(context.Background(), &domain.TenantPolicy{
			UserID: userID, TenantID: tenantID, AllowedRealms: column.viaAPI,
			GrantedBy: "test", GrantedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("CreatePolicy for %s: %v", column.name, err)
		}
		return
	}
	if column.writeSQL == "" {
		return
	}

	sql := fmt.Sprintf(
		`INSERT INTO tenant_policies (user_id, tenant_id, allowed_realms, granted_by, granted_at, created_at, updated_at)
		 VALUES (?, ?, %s, 'test', NOW(), NOW(), NOW())`, column.writeSQL)
	if err := db.Exec(sql, userID, tenantID).Error; err != nil {
		t.Fatalf("seed policy row for %s: %v", column.name, err)
	}
}

// mountPolicyRoutes mounts the system, alerts and AMFA tenant routes as the
// application router does. Only authentication is a double, reading the caller
// from the X-Test-User header; every authorization layer below it is real.
func mountPolicyRoutes(stack *policyStack, rbacService rbac.Service, log *logger.Logger) http.Handler {
	checker := &policyRBACAdapter{service: rbacService}
	rbacMW := NewRBACMiddleware(checker, log)
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := strconv.ParseUint(r.Header.Get("X-Test-User"), 10, 64)
			ctx := context.WithValue(r.Context(), UserInfoKey, &UserDetails{ID: uint(id), Email: "policy@example.invalid"})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	system := NewSystemHandlers(stack.events, nil, &mockRealmLister{
		realms: realms(policyRealmDenied, policyRealmAllowed),
	}, checker, log, authMW, rbacMW)
	alertsHandlers := NewAlertsHandlers(stack.alerts, checker, &mockOperatorService{}, nil, log, authMW, rbacMW)
	amfaHandlers := NewAmfaHandlers(stack.amfa, nil, checker, log, authMW, rbacMW, nil)

	r := chi.NewRouter()
	r.Route("/api/tenants/{tenantID}", func(r chi.Router) {
		r.Use(TenantIDMiddleware)
		system.RegisterTenantRoutes(r)
		alertsHandlers.RegisterTenantRoutes(r)
		amfaHandlers.RegisterTenantRoutes(r)
	})
	return r
}

// policyRBACAdapter narrows the RBAC service to the checker the HTTP layer
// depends on, mirroring the adapter the fx wiring installs in production.
type policyRBACAdapter struct{ service rbac.Service }

func (a *policyRBACAdapter) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	return a.service.HasPermission(ctx, userID, permission, tenantID)
}

func (a *policyRBACAdapter) GetUserPermissions(ctx context.Context, userID uint, tenantID *string) ([]string, error) {
	return a.service.GetUserPermissions(ctx, userID, tenantID)
}

func (a *policyRBACAdapter) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	return a.service.IsAdmin(ctx, userID)
}

func (a *policyRBACAdapter) GetUserRoles(ctx context.Context, userID uint) ([]*UserRoleInfo, error) {
	roles, err := a.service.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*UserRoleInfo, 0, len(roles))
	for _, role := range roles {
		info := &UserRoleInfo{RoleID: role.RoleID, TenantID: role.TenantID}
		if role.Role != nil {
			info.RoleName = role.Role.Name
		}
		out = append(out, info)
	}
	return out, nil
}

func (a *policyRBACAdapter) HasAccessToTenant(ctx context.Context, userID uint, tenantID string) (bool, error) {
	return a.service.HasAccessToTenant(ctx, userID, tenantID)
}

func (a *policyRBACAdapter) HasAccessToRealm(ctx context.Context, userID uint, tenantID, realmName string) (bool, error) {
	return a.service.HasAccessToRealm(ctx, userID, tenantID, realmName)
}

func (a *policyRBACAdapter) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	return a.service.GetUserPolicies(ctx, userID)
}

// do drives one request as the caller seeded for this column shape.
func (s *policyStack) do(t *testing.T, method, path, column string) *httptest.ResponseRecorder {
	t.Helper()
	return s.doBody(t, method, path, column, "")
}

// doBody drives one request carrying a JSON body, for the routes that take one.
func (s *policyStack) doBody(t *testing.T, method, path, column, body string) *httptest.ResponseRecorder {
	t.Helper()

	userID, ok := s.users[column]
	if !ok {
		t.Fatalf("no seeded caller for column shape %q", column)
	}
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/tenants/"+s.tenantID+path, reader)
	req.Header.Set("X-Test-User", strconv.FormatUint(uint64(userID), 10))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// storedColumn reads allowed_realms back as text so a test can assert the shape
// on disk rather than the shape the domain model reports.
func (s *policyStack) storedColumn(t *testing.T, column string) (string, bool) {
	t.Helper()

	var rows []struct{ AllowedRealms *string }
	err := s.db.Raw(
		`SELECT allowed_realms::text AS allowed_realms FROM tenant_policies WHERE user_id = ? AND tenant_id = ?`,
		s.users[column], s.tenantID,
	).Scan(&rows).Error
	if err != nil {
		t.Fatalf("read back allowed_realms: %v", err)
	}
	if len(rows) == 0 {
		return "", false
	}
	if rows[0].AllowedRealms == nil {
		return "", true
	}
	return *rows[0].AllowedRealms, true
}

// reset clears the stub call records so one stack can serve every column shape.
func (s *policyStack) reset() {
	s.events.calls = 0
	s.events.lastOpts = nil
	s.alerts.listCalls = 0
	s.alerts.lastOpts = nil
	s.alerts.statsCalls = 0
	s.alerts.lastStats = nil
	s.alerts.mutations = nil
	s.amfa.calls = 0
	s.amfa.lastStatsOpts = amfa.StatsOptions{}
	s.amfa.lastListOpts = amfa.ListEventsOptions{}
	s.amfa.lastGeoOpts = amfa.GeoOptions{}
}

// CreatePolicy with no realms must leave a column every reader treats as
// unrestricted.
func TestRealmPolicy_NoRealmsWritesAnUnrestrictedColumn(t *testing.T) {
	stack := newPolicyStack(t)

	stored, exists := stack.storedColumn(t, "written through CreatePolicy with no realms")
	if !exists {
		t.Fatal("CreatePolicy wrote no row")
	}
	if stored != "" {
		t.Errorf("allowed_realms = %q, want SQL NULL", stored)
	}

	stored, exists = stack.storedColumn(t, "written through CreatePolicy with an empty list")
	if !exists {
		t.Fatal("CreatePolicy wrote no row")
	}
	if stored != "[]" {
		t.Errorf("allowed_realms = %q, want [] so an explicit empty list stays fail-closed", stored)
	}
}

// GET /events and GET /stats against every column shape.
func TestRealmPolicy_EventsAndStatsScope(t *testing.T) {
	stack := newPolicyStack(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()

			rec := stack.do(t, http.MethodGet, "/events", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}

			gotSources := []string(nil)
			if stack.events.lastOpts != nil {
				gotSources = stack.events.lastOpts.Sources
			}
			switch {
			case column.unrestrict:
				for _, want := range []string{"keycloak:" + policyRealmDenied, "keycloak:" + policyRealmAllowed} {
					if !slices.Contains(gotSources, want) {
						t.Errorf("sources = %v, missing %q; an unrestricted caller must see every realm", gotSources, want)
					}
				}
			case column.allowsAllowedRealm():
				if slices.Contains(gotSources, "keycloak:"+policyRealmDenied) {
					t.Errorf("sources = %v, leaks %s to a caller restricted to %s", gotSources, policyRealmDenied, policyRealmAllowed)
				}
				if !slices.Contains(gotSources, "keycloak:"+policyRealmAllowed) {
					t.Errorf("sources = %v, missing the caller's own realm", gotSources)
				}
			default:
				if stack.events.calls != 0 {
					t.Errorf("events queried %d times for a caller allowed no realm, want 0", stack.events.calls)
				}
			}

			rec = stack.do(t, http.MethodGet, "/stats", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("stats status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// GET /alerts and GET /alerts/stats against every column shape.
func TestRealmPolicy_AlertsScope(t *testing.T) {
	stack := newPolicyStack(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()

			rec := stack.do(t, http.MethodGet, "/alerts", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}

			switch {
			case column.unrestrict:
				if stack.alerts.lastOpts == nil {
					t.Fatal("alerts were never listed for an unrestricted caller")
				}
				if len(stack.alerts.lastOpts.RealmNames) != 0 {
					t.Errorf("RealmNames = %v, want none: an unrestricted caller reads the whole tenant",
						stack.alerts.lastOpts.RealmNames)
				}
			case column.allowsAllowedRealm():
				if stack.alerts.lastOpts == nil {
					t.Fatal("alerts were never listed for a restricted caller with a realm")
				}
				if !slices.Equal(stack.alerts.lastOpts.RealmNames, []string{policyRealmAllowed}) {
					t.Errorf("RealmNames = %v, want [%s]", stack.alerts.lastOpts.RealmNames, policyRealmAllowed)
				}
			default:
				if stack.alerts.listCalls != 0 {
					t.Errorf("alerts listed %d times for a caller allowed no realm, want 0", stack.alerts.listCalls)
				}
			}

			rec = stack.do(t, http.MethodGet, "/alerts/stats", column.name)
			if rec.Code != http.StatusOK {
				t.Fatalf("stats status = %d, want 200; body = %s", rec.Code, rec.Body.String())
			}
			switch {
			case column.unrestrict:
				if len(stack.alerts.lastStats) != 0 {
					t.Errorf("statistics realms = %v, want none", stack.alerts.lastStats)
				}
			case column.allowsAllowedRealm():
				if !slices.Equal(stack.alerts.lastStats, []string{policyRealmAllowed}) {
					t.Errorf("statistics realms = %v, want [%s]", stack.alerts.lastStats, policyRealmAllowed)
				}
			default:
				if stack.alerts.statsCalls != 0 {
					t.Errorf("statistics queried %d times for a caller allowed no realm, want 0", stack.alerts.statsCalls)
				}
			}
		})
	}
}

// GET /alerts/by-realm naming the realm the caller may not read.
func TestRealmPolicy_ByRealmRefusesTheDeniedRealm(t *testing.T) {
	stack := newPolicyStack(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()

			rec := stack.do(t, http.MethodGet, "/alerts/by-realm?realm="+policyRealmDenied, column.name)
			want := http.StatusForbidden
			if column.unrestrict {
				want = http.StatusOK
			}
			if rec.Code != want {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, want, rec.Body.String())
			}
		})
	}
}

// GET /amfa/geo for the denied realm, and GET /amfa/stats with no realm at all.
func TestRealmPolicy_AmfaScope(t *testing.T) {
	stack := newPolicyStack(t)

	for _, column := range policyColumns() {
		t.Run(column.name, func(t *testing.T) {
			stack.reset()

			rec := stack.do(t, http.MethodGet, "/amfa/geo?realm_id="+policyRealmDenied, column.name)
			wantGeo := http.StatusForbidden
			if column.unrestrict {
				wantGeo = http.StatusOK
			}
			if rec.Code != wantGeo {
				t.Errorf("geo status = %d, want %d; body = %s", rec.Code, wantGeo, rec.Body.String())
			}

			rec = stack.do(t, http.MethodGet, "/amfa/stats", column.name)
			var body struct {
				AppliedRealmID string `json:"applied_realm_id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &body)

			switch {
			case column.unrestrict:
				if rec.Code != http.StatusOK {
					t.Errorf("stats status = %d, want 200; body = %s", rec.Code, rec.Body.String())
				}
				if body.AppliedRealmID != "" {
					t.Errorf("applied_realm_id = %q, want empty for an unrestricted caller", body.AppliedRealmID)
				}
			case column.allowsAllowedRealm():
				if rec.Code != http.StatusOK {
					t.Fatalf("stats status = %d, want 200; body = %s", rec.Code, rec.Body.String())
				}
				if body.AppliedRealmID != policyRealmAllowed {
					t.Errorf("applied_realm_id = %q, want %q", body.AppliedRealmID, policyRealmAllowed)
				}
				if stack.amfa.lastStatsOpts.RealmID != policyRealmAllowed {
					t.Errorf("stats ran for realm %q, want %q", stack.amfa.lastStatsOpts.RealmID, policyRealmAllowed)
				}
			default:
				if rec.Code != http.StatusForbidden {
					t.Errorf("stats status = %d, want 403 for a caller allowed no realm; body = %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

// byIDRoute is one way to reach a single alert; four of them mutate.
type byIDRoute struct {
	name    string
	method  string
	path    func(alertID string) string
	mutates bool
}

func byIDRoutes() []byIDRoute {
	return []byIDRoute{
		{name: "GET /alerts/get", method: http.MethodGet, path: func(id string) string { return "/alerts/get?id=" + id }},
		{name: "GET /alerts/{alertID}", method: http.MethodGet, path: func(id string) string { return "/alerts/" + id }},
		{name: "PUT /alerts/{alertID}/status", method: http.MethodPut, path: func(id string) string { return "/alerts/" + id + "/status" }, mutates: true},
		{name: "POST /alerts/update-status", method: http.MethodPost, path: func(id string) string { return "/alerts/update-status?id=" + id }, mutates: true},
		{name: "POST /alerts/{alertID}/resolve", method: http.MethodPost, path: func(id string) string { return "/alerts/" + id + "/resolve" }, mutates: true},
		{name: "POST /alerts/resolve", method: http.MethodPost, path: func(id string) string { return "/alerts/resolve?id=" + id }, mutates: true},
		{name: "DELETE /alerts/delete", method: http.MethodDelete, path: func(id string) string { return "/alerts/delete?id=" + id }, mutates: true},
	}
}

func TestRealmPolicy_ByIDRoutesRefuseAnAlertOutsideTheCallersRealms(t *testing.T) {
	stack := newPolicyStack(t)

	const (
		restricted   = "stored realm list"
		unrestricted = "SQL NULL column"
	)

	for _, route := range byIDRoutes() {
		t.Run(route.name, func(t *testing.T) {
			t.Run("the caller's own realm is served", func(t *testing.T) {
				stack.reset()

				rec := stack.doBody(t, route.method, route.path("alert-in-"+policyRealmAllowed), restricted, `{"status":"acknowledged"}`)
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
				}
				if route.mutates && len(stack.alerts.mutations) == 0 {
					t.Error("the alert in the caller's own realm was not mutated; the control proves nothing")
				}
			})

			t.Run("another realm is indistinguishable from missing", func(t *testing.T) {
				stack.reset()

				denied := stack.doBody(t, route.method, route.path("alert-in-"+policyRealmDenied), restricted, `{"status":"acknowledged"}`)
				if denied.Code != http.StatusNotFound {
					t.Errorf("status = %d, want 404; body = %s", denied.Code, denied.Body.String())
				}
				if len(stack.alerts.mutations) != 0 {
					t.Errorf("alert in %s was mutated by a caller restricted to %s: %v",
						policyRealmDenied, policyRealmAllowed, stack.alerts.mutations)
				}

				stack.reset()
				missing := stack.doBody(t, route.method, route.path("alert-that-does-not-exist"), restricted, `{"status":"acknowledged"}`)
				if missing.Code != denied.Code || missing.Body.String() != denied.Body.String() {
					t.Errorf("an alert in another realm answers %d %s but a missing one answers %d %s; the difference is an oracle for which alert IDs exist",
						denied.Code, denied.Body.String(), missing.Code, missing.Body.String())
				}
			})

			t.Run("an unrestricted caller still reaches both realms", func(t *testing.T) {
				for _, realm := range []string{policyRealmAllowed, policyRealmDenied} {
					stack.reset()

					rec := stack.doBody(t, route.method, route.path("alert-in-"+realm), unrestricted, `{"status":"acknowledged"}`)
					if rec.Code != http.StatusOK {
						t.Errorf("realm %s: status = %d, want 200; body = %s", realm, rec.Code, rec.Body.String())
					}
					if route.mutates && len(stack.alerts.mutations) == 0 {
						t.Errorf("realm %s: an unrestricted caller's mutation never reached the service", realm)
					}
				}
			})
		})
	}
}

func TestRealmPolicy_MultiRealmCallerGetsThePermissionRefusal(t *testing.T) {
	stack := newPolicyStack(t)

	const column = "stored realm list"
	seedPolicyRow(t, stack.db, nil, stack.tenantID, stack.users[column], policyColumn{
		writeSQL: `'["` + policyRealmDenied + `"]'::jsonb`,
	})

	rec := stack.do(t, http.MethodGet, "/amfa/stats", column)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body = %s", err, rec.Body.String())
	}
	if body.Error != "amfa_realm_scope_requires_realm" {
		t.Errorf("error = %q, want amfa_realm_scope_requires_realm", body.Error)
	}
	if body.Error == "amfa_all_realms_unsupported" {
		t.Error("a permission refusal is reported as an unsupported AMFA integration")
	}
	if !strings.Contains(body.Message, "limited to specific realms") {
		t.Errorf("message = %q, want it to name the caller's realm scope as the reason", body.Message)
	}
	if stack.amfa.calls != 0 {
		t.Errorf("AMFA service called %d times for a refused request, want 0", stack.amfa.calls)
	}
}

var _ amfa.Service = (*stubAmfaService)(nil)
