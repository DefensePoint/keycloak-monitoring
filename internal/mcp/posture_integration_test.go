// End-to-end coverage for the MCP server's security posture. The assembly
// internal/fx/mcp.go builds at runtime is stood up here against a real
// PostgreSQL schema and driven over HTTP by the real MCP SDK client: real
// migrations, the real startup RBAC seed, real repositories, real
// rbac.Service, real API tokens minted through apitoken.Service and validated
// through it again. Nothing in this file is mocked.
//
// It lives beside the package rather than in tests/ so it can assert on the
// tool DTOs and sentinel errors directly, the way the unit suites next to it
// already do.
//
// Requires a real PostgreSQL instance. The connection DSN is taken from the
// KMT_TEST_DATABASE_DSN environment variable; the test is skipped when
// unset. Like the other DSN-gated suites it shares one database, so run it
// with -p 1 to keep packages from migrating and cleaning up underneath each
// other:
//
//	KMT_TEST_DATABASE_DSN="host=localhost port=5432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
//	    go test ./internal/mcp/... -run TestMCPSecurityPosture -p 1 -count=1
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	alertspostgres "github.com/DefensePoint/keycloak-monitoring/internal/alerts/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/amfa"
	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	apitokenpostgres "github.com/DefensePoint/keycloak-monitoring/internal/apitoken/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/events"
	eventspostgres "github.com/DefensePoint/keycloak-monitoring/internal/events/postgres"
	kcpostgres "github.com/DefensePoint/keycloak-monitoring/internal/keycloak/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	rbacpostgres "github.com/DefensePoint/keycloak-monitoring/internal/rbac/postgres"
	tenantpostgres "github.com/DefensePoint/keycloak-monitoring/internal/tenant/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
	userspostgres "github.com/DefensePoint/keycloak-monitoring/internal/users/postgres"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
)

const (
	postureRealmProd    = "prod"
	postureRealmStaging = "staging"
	postureRealmBOnly   = "b-only"

	// posturePageCap is the server's absolute page-size cap for this suite,
	// small enough that clamping shows up in the returned page length.
	posturePageCap = 3
	// posturePageDefault is the page size applied when a call requests none.
	posturePageDefault = 2

	postureGrantedBy = "mcp-posture-integration"
)

// tokenServiceLogger adapts *logger.Logger to apitoken.Logger. The assembly's
// own adapter lives in package fx, which this package cannot import.
type tokenServiceLogger struct{ log *logger.Logger }

func (l tokenServiceLogger) Info(msg string, _ ...any)  { l.log.Info(msg) }
func (l tokenServiceLogger) Error(msg string, _ ...any) { l.log.Error(msg) }
func (l tokenServiceLogger) Warn(msg string, _ ...any)  { l.log.Warn(msg) }
func (l tokenServiceLogger) Debug(msg string, _ ...any) { l.log.Debug(msg) }

// writeRecorder captures every mutation the MCP assembly's database client
// attempts. The test DSN user is a superuser, so without this a stray write
// would succeed silently instead of failing the way the deployed read-only
// database role makes it fail.
type writeRecorder struct {
	mu       sync.Mutex
	attempts []string
}

func (w *writeRecorder) record(op, target string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attempts = append(w.attempts, op+" "+target)
}

func (w *writeRecorder) seen() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.attempts)
}

// guardReadOnly installs the mutation guard on a GORM client. Create, Update
// and Delete cover the model paths; Raw covers Exec. Reads issued as
// Raw(...).Scan(...) run through the Row processor and are left alone.
func guardReadOnly(t *testing.T, db *gorm.DB) *writeRecorder {
	t.Helper()
	rec := &writeRecorder{}
	hooks := []struct {
		op       string
		register func(string, func(*gorm.DB)) error
	}{
		{"CREATE", db.Callback().Create().Before("gorm:create").Register},
		{"UPDATE", db.Callback().Update().Before("gorm:update").Register},
		{"DELETE", db.Callback().Delete().Before("gorm:delete").Register},
		{"RAW", db.Callback().Raw().Before("gorm:raw").Register},
	}
	for _, hook := range hooks {
		op := hook.op
		err := hook.register("mcp_readonly_guard_"+op, func(tx *gorm.DB) {
			target := tx.Statement.Table
			if target == "" {
				target = tx.Statement.SQL.String()
			}
			rec.record(op, target)
		})
		if err != nil {
			t.Fatalf("register the %s guard: %v", op, err)
		}
	}
	return rec
}

// parseTestDSN turns the libpq key/value DSN the suites are configured with
// into the DatabaseConfig database.NewClient takes. The DSN carries a
// password, so no failure here echoes it back.
func parseTestDSN(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()
	cfg := &config.DatabaseConfig{SSLMode: "disable", MaxConns: 10, MinConns: 2, Timeout: 30 * time.Second}
	for _, field := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "host":
			cfg.Host = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal("KMT_TEST_DATABASE_DSN: port is not a number")
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
	if cfg.Host == "" || cfg.Database == "" {
		t.Fatal("KMT_TEST_DATABASE_DSN must set at least host and dbname")
	}
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	return cfg
}

type toolCall struct {
	name string
	args map[string]any
}

// postureEnv is the stood-up stack plus the identifiers of everything seeded
// into it.
type postureEnv struct {
	ts     *httptest.Server
	writes *writeRecorder

	writeDB     *gorm.DB
	writeTokens apitoken.Service
	writeRBAC   rbac.Service
	writeUsers  users.Repository

	tenantA, tenantB, tenantC string

	// tokenA is allowlisted to tenant A and its user holds no tenant policy.
	// tokenAAlt is a second token for the same user and allowlist, used to
	// prove cursors bind to one token rather than to one user.
	tokenA, tokenAAlt, tokenB, tokenC string
	// tokenStray belongs to a user who also holds a stray global role;
	// tokenStrayMulti is the same user with a two-tenant allowlist, which is
	// the only shape where the tenant argument still exists.
	tokenStray, tokenStrayMulti string
	// tokenRealmX is restricted by policy to the prod realm of tenant A;
	// tokenNoRealms belongs to a user whose policy allows no realm at all.
	tokenRealmX, tokenNoRealms string
	// tokenNullRealms belongs to a user whose allowed_realms column is a SQL
	// NULL, which is unrestricted rather than a denial.
	tokenNullRealms string
	tokenRevoked    string
	revokedTokenID  uint
	tokenExpired    string
	tokenBlocked    string
	blockedSubject  string
	strayUserID     uint

	aProdEvents    []string
	aStagingEvent  string
	bProdEvent     string
	aProdAlerts    []string
	aStagingAlert  string
	bProdAlert     string
	missingEventID string
	missingAlertID string

	windowFrom, windowTo string

	baseline map[string]int64
	counts   func() map[string]int64
}

// tenantScopedCalls is every tool taking a tenant argument, so one verdict can
// be asserted across the whole surface at once. An empty tenantID leaves the
// argument off, which is how a token allowlisted to one tenant calls.
func tenantScopedCalls(tenantID, realm, alertID, eventID string) []toolCall {
	calls := []toolCall{
		{"get_tenant_health", map[string]any{}},
		{"list_realms", map[string]any{}},
		{"list_alerts", map[string]any{}},
		{"get_alert", map[string]any{"alert_id": alertID}},
		{"get_alert_stats", map[string]any{}},
		{"get_amfa_stats", map[string]any{"realm": realm}},
		{"list_events", map[string]any{}},
		{"get_event", map[string]any{"event_id": eventID}},
		{"get_event_stats", map[string]any{}},
	}
	if tenantID != "" {
		for _, call := range calls {
			call.args["tenant"] = tenantID
		}
	}
	return calls
}

func callToolOKAs(t *testing.T, ts *httptest.Server, token, name string, args map[string]any, out any) {
	t.Helper()
	res := callToolResultAs(t, ts, token, name, args)
	if res.IsError {
		t.Fatalf("%s returned tool error: %+v", name, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal %s structured content: %v", name, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("unmarshal %s output: %v", name, err)
	}
}

func callToolErrTextAs(t *testing.T, ts *httptest.Server, token, name string, args map[string]any) string {
	t.Helper()
	return errTextOf(t, callToolResultAs(t, ts, token, name, args), name)
}

// postInitialize sends a bare initialize request and returns the HTTP status,
// which is how a token rejected by the auth middleware is observed: the
// middleware answers before the MCP transport ever sees the body.
func postInitialize(t *testing.T, ts *httptest.Server, token string) int {
	t.Helper()
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
		`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"posture","version":"0"}}}`)
	req, err := http.NewRequest(http.MethodPost, ts.URL+Endpoint, body)
	if err != nil {
		t.Fatalf("build initialize request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("initialize request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	return res.StatusCode
}

func eventIDsOf(out listEventsOutput) []string {
	ids := make([]string, len(out.Events))
	for i, e := range out.Events {
		ids[i] = e.EventID
	}
	return ids
}

func newPostureEnv(t *testing.T) *postureEnv {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	testdb.ShareDatabase(t, dsn)
	dbCfg := parseTestDSN(t, dsn)
	log := logger.NewNoop()
	ctx := context.Background()

	// The write side stands in for the web application: the same AutoMigrate,
	// the same startup RBAC seed, and the same repositories that create every
	// row in production.
	writeClient, err := database.NewClient(dbCfg, log)
	if err != nil {
		t.Fatalf("migrate the test database: %v", err)
	}
	t.Cleanup(func() { _ = writeClient.Close() })
	writeDB := writeClient.DB()

	writeRBAC := rbac.NewService(
		rbacpostgres.NewRoleRepository(writeClient),
		rbacpostgres.NewPermissionRepository(writeClient),
		rbacpostgres.NewUserRoleRepository(writeClient),
		rbacpostgres.NewTenantPolicyRepository(writeClient),
		log,
	)
	if err := rbac.NewSeeder(writeRBAC, log).EnsureSystemRolesAndPermissions(ctx); err != nil {
		t.Fatalf("seed roles and permissions: %v", err)
	}
	viewer, err := writeRBAC.GetRoleByName(ctx, rbac.RoleViewer)
	if err != nil {
		t.Fatalf("look up the viewer role: %v", err)
	}
	if viewer == nil {
		t.Fatal("the viewer role is missing after seeding")
	}

	stamp := time.Now().UnixNano()
	tenantA := fmt.Sprintf("mcp-posture-a-%d", stamp)
	tenantB := fmt.Sprintf("mcp-posture-b-%d", stamp)
	tenantC := fmt.Sprintf("mcp-posture-c-%d", stamp)
	tenantIDs := []string{tenantA, tenantB, tenantC}
	subjectPattern := fmt.Sprintf("mcp-posture-%d-%%", stamp)

	t.Cleanup(func() {
		const bySubject = `user_id IN (SELECT id FROM users WHERE subject LIKE ?)`
		writeDB.Exec(`DELETE FROM api_tokens WHERE `+bySubject, subjectPattern)
		writeDB.Exec(`DELETE FROM tenant_policies WHERE `+bySubject, subjectPattern)
		writeDB.Exec(`DELETE FROM user_roles WHERE `+bySubject, subjectPattern)
		writeDB.Exec(`DELETE FROM users WHERE subject LIKE ?`, subjectPattern)
		writeDB.Exec(`DELETE FROM events WHERE tenant_id IN ?`, tenantIDs)
		writeDB.Exec(`DELETE FROM configuration_alerts WHERE tenant_id IN ?`, tenantIDs)
		writeDB.Exec(`DELETE FROM keycloak_realms WHERE tenant_id IN ?`, tenantIDs)
		writeDB.Exec(`DELETE FROM keycloak_tenants WHERE tenant_id IN ?`, tenantIDs)
	})

	tenantRepo := tenantpostgres.NewRepository(writeClient, nil, log)
	created := make(map[string]*domain.KeycloakTenant, len(tenantIDs))
	for _, id := range tenantIDs {
		row := &domain.KeycloakTenant{
			TenantID:     id,
			Name:         id,
			ServerURL:    "https://keycloak.invalid",
			AdminRealm:   "master",
			ClientID:     "admin-cli",
			ClientSecret: "not-a-real-secret",
			Enabled:      true,
			HealthStatus: "healthy",
		}
		if err := tenantRepo.Create(ctx, row); err != nil {
			t.Fatalf("create tenant %s: %v", id, err)
		}
		created[id] = row
	}

	// Disabled through Update, the path the tenant service takes: keycloak_tenants.enabled
	// carries a database default of true, and Create omits the column when the
	// field is false, so a tenant created disabled comes back enabled.
	created[tenantC].Enabled = false
	if err := tenantRepo.Update(ctx, created[tenantC]); err != nil {
		t.Fatalf("disable tenant %s: %v", tenantC, err)
	}
	if row, err := tenantRepo.GetByTenantID(ctx, tenantC); err != nil || row.Enabled {
		t.Fatalf("tenant %s is still enabled after the update (err %v); the disabled-tenant case would test nothing", tenantC, err)
	}

	realmRepo := kcpostgres.NewRealmRepository(writeDB)
	for _, spec := range []struct{ tenantID, realm string }{
		{tenantA, postureRealmProd},
		{tenantA, postureRealmStaging},
		{tenantB, postureRealmProd},
		{tenantB, postureRealmBOnly},
		{tenantC, postureRealmProd},
	} {
		row := &domain.KeycloakRealmInfo{
			TenantID:  spec.tenantID,
			RealmID:   spec.realm,
			RealmName: spec.realm,
			Enabled:   true,
		}
		if err := realmRepo.SaveRealm(ctx, row); err != nil {
			t.Fatalf("create realm %s/%s: %v", spec.tenantID, spec.realm, err)
		}
	}

	// Two hours back so every row sits comfortably inside the default 24 hour
	// window as well as the explicit one the pagination cases pass.
	base := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Hour)
	eventRepo := eventspostgres.NewRepository(writeDB, log)
	saveEvent := func(tenantID, eventID, realm string, at time.Time) {
		t.Helper()
		row := &domain.Event{
			TenantID:     tenantID,
			EventID:      eventID,
			Type:         "LOGIN",
			Category:     "authentication",
			Severity:     "info",
			Description:  "user logged in",
			Source:       events.SourceForKeycloakRealm(realm),
			SourceSystem: events.SourceSystemKeycloak,
			Username:     "alice",
			Status:       "processed",
			Timestamp:    at,
		}
		if err := eventRepo.Save(ctx, row); err != nil {
			t.Fatalf("save event %s: %v", eventID, err)
		}
	}

	aProdEvents := make([]string, 5)
	for i := range aProdEvents {
		aProdEvents[i] = fmt.Sprintf("ev-a-prod-%d-%d", stamp, i+1)
		saveEvent(tenantA, aProdEvents[i], postureRealmProd, base.Add(time.Duration(i+1)*time.Second))
	}
	aStagingEvent := fmt.Sprintf("ev-a-staging-%d", stamp)
	saveEvent(tenantA, aStagingEvent, postureRealmStaging, base.Add(10*time.Second))
	saveEvent(tenantA, fmt.Sprintf("ev-a-staging2-%d", stamp), postureRealmStaging, base.Add(11*time.Second))
	// Tenant B monitors a realm called "prod" too: the realm name alone can
	// never separate these rows from tenant A's, only tenant_id can.
	bProdEvent := fmt.Sprintf("ev-b-prod-%d", stamp)
	saveEvent(tenantB, bProdEvent, postureRealmProd, base.Add(20*time.Second))
	saveEvent(tenantB, fmt.Sprintf("ev-b-only-%d", stamp), postureRealmBOnly, base.Add(21*time.Second))

	alertRepo := alertspostgres.NewRepository(writeDB)
	saveAlert := func(tenantID, alertID, realm string) {
		t.Helper()
		row := &domain.Alert{
			TenantID:       tenantID,
			AlertID:        alertID,
			Source:         domain.AlertSourceConfiguration,
			Type:           domain.AlertTypeSecurity,
			Severity:       domain.AlertSeverityWarning,
			Status:         domain.AlertStatusActive,
			Title:          "brute force protection disabled",
			Description:    "the realm accepts unlimited failed logins",
			Recommendation: "enable brute force detection",
			ResourceType:   "realm",
			ResourceID:     realm,
			ResourceName:   realm,
			RealmName:      realm,
			CheckType:      "realm_security",
			Metadata:       `{"internal":"must-not-leak"}`,
			FirstDetected:  base,
			LastSeen:       base,
		}
		if err := alertRepo.Save(ctx, row); err != nil {
			t.Fatalf("save alert %s: %v", alertID, err)
		}
	}

	aProdAlerts := make([]string, 4)
	for i := range aProdAlerts {
		aProdAlerts[i] = fmt.Sprintf("al-a-prod-%d-%d", stamp, i+1)
		saveAlert(tenantA, aProdAlerts[i], postureRealmProd)
	}
	aStagingAlert := fmt.Sprintf("al-a-staging-%d", stamp)
	saveAlert(tenantA, aStagingAlert, postureRealmStaging)
	bProdAlert := fmt.Sprintf("al-b-prod-%d", stamp)
	saveAlert(tenantB, bProdAlert, postureRealmProd)

	userRepo := userspostgres.NewRepository(writeDB)
	newUser := func(name string) *domain.User {
		t.Helper()
		row := &domain.User{
			Subject:  fmt.Sprintf("mcp-posture-%d-%s", stamp, name),
			Email:    fmt.Sprintf("%s-%d@example.invalid", name, stamp),
			Name:     name,
			IsActive: true,
		}
		created, err := userRepo.FindOrCreateBySubject(ctx, row)
		if err != nil {
			t.Fatalf("create user %s: %v", name, err)
		}
		return created
	}
	grant := func(userID uint, tenantID *string) {
		t.Helper()
		if err := writeRBAC.AssignRoleToUser(ctx, userID, viewer.ID, tenantID, postureGrantedBy, nil); err != nil {
			t.Fatalf("assign the viewer role to user %d: %v", userID, err)
		}
	}

	customer := newUser("customer")
	grant(customer.ID, &tenantA)
	grant(customer.ID, &tenantB)
	grant(customer.ID, &tenantC)

	stray := newUser("stray")
	grant(stray.ID, &tenantA)
	// The stray grant: a global role that RBAC alone would read as access to
	// every tenant. Only the token allowlist narrows it back down.
	grant(stray.ID, nil)

	realmX := newUser("realmx")
	grant(realmX.ID, &tenantA)
	if err := writeRBAC.CreateTenantPolicy(ctx, realmX.ID, tenantA, []string{postureRealmProd}, postureGrantedBy); err != nil {
		t.Fatalf("create the realm-scoped policy: %v", err)
	}

	noRealms := newUser("norealms")
	grant(noRealms.ID, &tenantA)
	if err := writeRBAC.CreateTenantPolicy(ctx, noRealms.ID, tenantA, []string{}, postureGrantedBy); err != nil {
		t.Fatalf("create the empty-realm policy: %v", err)
	}

	// A nil realm list stores a SQL NULL allowed_realms column, which is an
	// absent restriction rather than an empty one. This distinguishes NULL
	// from the stored '[]' above: they differ by one character on disk and
	// mean opposite things, and this resolver used to deny both.
	nullRealms := newUser("nullrealms")
	grant(nullRealms.ID, &tenantA)
	if err := writeRBAC.CreateTenantPolicy(ctx, nullRealms.ID, tenantA, nil, postureGrantedBy); err != nil {
		t.Fatalf("create the null-realm policy: %v", err)
	}

	blocked := newUser("blocked")
	grant(blocked.ID, &tenantA)

	writeTokens := apitoken.NewService(
		apitokenpostgres.NewRepository(writeDB),
		userRepo,
		tokenServiceLogger{log: log},
	)
	mint := func(userID uint, name string, tenantIDs []string) *apitoken.CreatedToken {
		t.Helper()
		created, err := writeTokens.Create(ctx, userID, name, tenantIDs, nil)
		if err != nil {
			t.Fatalf("mint the %s token: %v", name, err)
		}
		return created
	}

	tokenA := mint(customer.ID, "customer-a", []string{tenantA})
	tokenAAlt := mint(customer.ID, "customer-a-alt", []string{tenantA})
	tokenB := mint(customer.ID, "customer-b", []string{tenantB})
	tokenC := mint(customer.ID, "customer-c", []string{tenantC})
	tokenStray := mint(stray.ID, "stray-a", []string{tenantA})
	tokenStrayMulti := mint(stray.ID, "stray-a-c", []string{tenantA, tenantC})
	tokenRealmX := mint(realmX.ID, "realmx-a", []string{tenantA})
	tokenNoRealms := mint(noRealms.ID, "norealms-a", []string{tenantA})
	tokenNullRealms := mint(nullRealms.ID, "nullrealms-a", []string{tenantA})
	tokenRevoked := mint(customer.ID, "customer-revoked", []string{tenantA})
	tokenBlocked := mint(blocked.ID, "blocked-a", []string{tenantA})

	// Create clamps a requested expiry that is already in the past, so an
	// expired token has to be aged after the fact.
	tokenExpired := mint(customer.ID, "customer-expired", []string{tenantA})
	if err := writeDB.Exec(`UPDATE api_tokens SET expires_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-time.Hour), tokenExpired.ID).Error; err != nil {
		t.Fatalf("age the expired token: %v", err)
	}

	// From here down everything is the read-only half of the assembly: one
	// client that never migrates, shared by every repository and service the
	// MCP server is given, exactly as internal/fx/mcp.go wires it.
	readClient, err := database.NewReadOnlyClient(dbCfg, log)
	if err != nil {
		t.Fatalf("open the read-only client: %v", err)
	}
	t.Cleanup(func() { _ = readClient.Close() })
	readDB := readClient.DB()
	writes := guardReadOnly(t, readDB)

	cfg := &config.AppConfig{MCP: config.MCPConfig{
		Port:            0,
		DefaultPageSize: posturePageDefault,
		MaxPageSize:     posturePageCap,
		CursorHMACKey:   strings.Repeat("posture-cursor-key", 2),
	}}
	server := New(
		cfg,
		log,
		apitoken.NewService(apitokenpostgres.NewRepository(readDB), userspostgres.NewRepository(readDB), tokenServiceLogger{log: log}),
		rbac.NewService(
			rbacpostgres.NewRoleRepository(readClient),
			rbacpostgres.NewPermissionRepository(readClient),
			rbacpostgres.NewUserRoleRepository(readClient),
			rbacpostgres.NewTenantPolicyRepository(readClient),
			log,
		),
		tenantpostgres.NewRepository(readClient, nil, log),
		kcpostgres.NewRealmRepository(readDB),
		alerts.NewService(alertspostgres.NewRepository(readDB), log),
		// No tenant here has AMFA configured, so the registry is empty and
		// every allowed realm resolves to the generic not-available error.
		amfa.NewService(amfa.NewRegistry(), nil),
		eventspostgres.NewRepository(readDB, log),
		metrics.New(),
	)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	counts := func() map[string]int64 {
		t.Helper()
		const bySubject = `user_id IN (SELECT id FROM users WHERE subject LIKE ?)`
		queries := []struct {
			table string
			query string
			args  []any
		}{
			{"events", `SELECT count(*) FROM events WHERE tenant_id IN ?`, []any{tenantIDs}},
			{"configuration_alerts", `SELECT count(*) FROM configuration_alerts WHERE tenant_id IN ?`, []any{tenantIDs}},
			{"keycloak_realms", `SELECT count(*) FROM keycloak_realms WHERE tenant_id IN ?`, []any{tenantIDs}},
			{"keycloak_tenants", `SELECT count(*) FROM keycloak_tenants WHERE tenant_id IN ?`, []any{tenantIDs}},
			{"users", `SELECT count(*) FROM users WHERE subject LIKE ?`, []any{subjectPattern}},
			{"user_roles", `SELECT count(*) FROM user_roles WHERE ` + bySubject, []any{subjectPattern}},
			{"tenant_policies", `SELECT count(*) FROM tenant_policies WHERE ` + bySubject, []any{subjectPattern}},
			{"api_tokens", `SELECT count(*) FROM api_tokens WHERE ` + bySubject, []any{subjectPattern}},
			{"roles", `SELECT count(*) FROM roles`, nil},
			{"permissions", `SELECT count(*) FROM permissions`, nil},
			{"role_permissions", `SELECT count(*) FROM role_permissions`, nil},
		}
		out := make(map[string]int64, len(queries))
		for _, q := range queries {
			var n int64
			if err := writeDB.Raw(q.query, q.args...).Scan(&n).Error; err != nil {
				t.Fatalf("count %s: %v", q.table, err)
			}
			out[q.table] = n
		}
		return out
	}

	return &postureEnv{
		ts:              ts,
		writes:          writes,
		writeDB:         writeDB,
		writeTokens:     writeTokens,
		writeRBAC:       writeRBAC,
		writeUsers:      userRepo,
		tenantA:         tenantA,
		tenantB:         tenantB,
		tenantC:         tenantC,
		tokenA:          tokenA.Plaintext,
		tokenAAlt:       tokenAAlt.Plaintext,
		tokenB:          tokenB.Plaintext,
		tokenC:          tokenC.Plaintext,
		tokenStray:      tokenStray.Plaintext,
		tokenStrayMulti: tokenStrayMulti.Plaintext,
		tokenRealmX:     tokenRealmX.Plaintext,
		tokenNoRealms:   tokenNoRealms.Plaintext,
		tokenNullRealms: tokenNullRealms.Plaintext,
		tokenRevoked:    tokenRevoked.Plaintext,
		revokedTokenID:  tokenRevoked.ID,
		tokenExpired:    tokenExpired.Plaintext,
		tokenBlocked:    tokenBlocked.Plaintext,
		blockedSubject:  blocked.Subject,
		strayUserID:     stray.ID,
		aProdEvents:     aProdEvents,
		aStagingEvent:   aStagingEvent,
		bProdEvent:      bProdEvent,
		aProdAlerts:     aProdAlerts,
		aStagingAlert:   aStagingAlert,
		bProdAlert:      bProdAlert,
		missingEventID:  fmt.Sprintf("ev-nowhere-%d", stamp),
		missingAlertID:  fmt.Sprintf("al-nowhere-%d", stamp),
		windowFrom:      base.Add(-time.Hour).Format(time.RFC3339),
		windowTo:        base.Add(time.Hour).Format(time.RFC3339),
		baseline:        counts(),
		counts:          counts,
	}
}

func TestMCPSecurityPosture(t *testing.T) {
	env := newPostureEnv(t)
	ctx := context.Background()

	t.Run("tenant isolation on every tool", func(t *testing.T) {
		var who whoamiOutput
		callToolOKAs(t, env.ts, env.tokenA, "whoami", nil, &who)
		if !slices.Equal(who.TenantAllowlist, []string{env.tenantA}) {
			t.Errorf("tenant_allowlist = %v, want [%s]", who.TenantAllowlist, env.tenantA)
		}
		if who.Admin {
			t.Error("a tenant-scoped viewer must not report as a platform administrator")
		}

		var tenants listTenantsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_tenants", nil, &tenants)
		if got := tenantIDsOf(tenants); !slices.Equal(got, []string{env.tenantA}) {
			t.Errorf("list_tenants = %v, want [%s]", got, env.tenantA)
		}

		// The allowlist holds one tenant, so no tool lets the caller name a
		// tenant at all: naming tenant B is refused before any lookup runs.
		for _, call := range tenantScopedCalls(env.tenantB, postureRealmProd, env.bProdAlert, env.bProdEvent) {
			got := callToolErrTextAs(t, env.ts, env.tokenA, call.name, call.args)
			if !strings.Contains(got, "tenant must be omitted") {
				t.Errorf("%s naming tenant B = %q, want the tenant-must-be-omitted refusal", call.name, got)
			}
		}

		// Asking under the caller's own tenant for another tenant's row must
		// be word-for-word what a row that never existed returns.
		missingEvent := callToolErrTextAs(t, env.ts, env.tokenA, "get_event",
			map[string]any{"event_id": env.missingEventID})
		crossedEvent := callToolErrTextAs(t, env.ts, env.tokenA, "get_event",
			map[string]any{"event_id": env.bProdEvent})
		if crossedEvent != missingEvent || crossedEvent != ErrEventNotFound.Error() {
			t.Errorf("get_event for tenant B's event = %q, missing event = %q, want both %q",
				crossedEvent, missingEvent, ErrEventNotFound.Error())
		}

		missingAlert := callToolErrTextAs(t, env.ts, env.tokenA, "get_alert",
			map[string]any{"alert_id": env.missingAlertID})
		crossedAlert := callToolErrTextAs(t, env.ts, env.tokenA, "get_alert",
			map[string]any{"alert_id": env.bProdAlert})
		if crossedAlert != missingAlert || crossedAlert != ErrAlertNotFound.Error() {
			t.Errorf("get_alert for tenant B's alert = %q, missing alert = %q, want both %q",
				crossedAlert, missingAlert, ErrAlertNotFound.Error())
		}

		// Both tenants own a realm called "prod", so the source values are
		// identical strings and only the counts can show tenant_id scoping.
		var stats eventStatsOutput
		callToolOKAs(t, env.ts, env.tokenA, "get_event_stats", map[string]any{
			"from": env.windowFrom, "to": env.windowTo,
		}, &stats)
		want := map[string]int64{
			events.SourceForKeycloakRealm(postureRealmProd):    5,
			events.SourceForKeycloakRealm(postureRealmStaging): 2,
		}
		if !maps.Equal(stats.BySource, want) {
			t.Errorf("by_source = %v, want %v", stats.BySource, want)
		}

		var alertStats alertStatsOutput
		callToolOKAs(t, env.ts, env.tokenA, "get_alert_stats", nil, &alertStats)
		if alertStats.TotalActive != 5 {
			t.Errorf("total_active = %d, want 5 (tenant B's prod alert must not count)", alertStats.TotalActive)
		}
	})

	t.Run("token allowlist intersects rbac", func(t *testing.T) {
		// The stray global role really does grant tenant B through RBAC; the
		// allowlist is the only thing standing between the caller and it.
		allowed, err := env.writeRBAC.HasPermission(ctx, env.strayUserID, rbac.PermissionTenantsRead, &env.tenantB)
		if err != nil {
			t.Fatalf("check the stray global grant: %v", err)
		}
		if !allowed {
			t.Fatal("the stray global role should grant tenant B through RBAC; the fixture is wrong")
		}

		var tenants listTenantsOutput
		callToolOKAs(t, env.ts, env.tokenStray, "list_tenants", nil, &tenants)
		if got := tenantIDsOf(tenants); !slices.Equal(got, []string{env.tenantA}) {
			t.Errorf("list_tenants = %v, want [%s]: the allowlist must narrow a globally-granted caller",
				got, env.tenantA)
		}

		// The two-tenant token is the one shape that still names a tenant, so
		// it is where the allowlist itself has to do the denying.
		for _, call := range tenantScopedCalls(env.tenantB, postureRealmProd, env.bProdAlert, env.bProdEvent) {
			got := callToolErrTextAs(t, env.ts, env.tokenStrayMulti, call.name, call.args)
			if got != ErrForbidden.Error() {
				t.Errorf("%s for tenant B = %q, want %q", call.name, got, ErrForbidden.Error())
			}
		}

		var strayRealms listRealmsOutput
		callToolOKAs(t, env.ts, env.tokenStrayMulti, "list_realms",
			map[string]any{"tenant": env.tenantA}, &strayRealms)
		if len(strayRealms.Realms) == 0 {
			t.Error("a tenant named inside the allowlist must still be reachable")
		}
	})

	t.Run("realm policy", func(t *testing.T) {
		var realms listRealmsOutput
		callToolOKAs(t, env.ts, env.tokenRealmX, "list_realms", nil, &realms)
		if !slices.Equal(realms.Realms, []string{postureRealmProd}) {
			t.Errorf("list_realms = %v, want [%s]", realms.Realms, postureRealmProd)
		}

		var page listEventsOutput
		callToolOKAs(t, env.ts, env.tokenRealmX, "list_events", map[string]any{
			"from": env.windowFrom, "to": env.windowTo, "limit": posturePageCap,
		}, &page)
		if len(page.Events) == 0 {
			t.Fatal("a realm-restricted caller saw no events at all")
		}
		for _, e := range page.Events {
			if e.Realm != postureRealmProd {
				t.Errorf("realm-omitted list_events returned a %s event; the policy allows only %s",
					e.Realm, postureRealmProd)
			}
		}

		var stats eventStatsOutput
		callToolOKAs(t, env.ts, env.tokenRealmX, "get_event_stats", map[string]any{
			"from": env.windowFrom, "to": env.windowTo,
		}, &stats)
		want := map[string]int64{events.SourceForKeycloakRealm(postureRealmProd): 5}
		if !maps.Equal(stats.BySource, want) {
			t.Errorf("by_source = %v, want %v", stats.BySource, want)
		}

		if got := callToolErrTextAs(t, env.ts, env.tokenRealmX, "get_event",
			map[string]any{"event_id": env.aStagingEvent}); got != ErrEventNotFound.Error() {
			t.Errorf("get_event for a staging event = %q, want %q", got, ErrEventNotFound.Error())
		}
		if got := callToolErrTextAs(t, env.ts, env.tokenRealmX, "get_alert",
			map[string]any{"alert_id": env.aStagingAlert}); got != ErrAlertNotFound.Error() {
			t.Errorf("get_alert for a staging alert = %q, want %q", got, ErrAlertNotFound.Error())
		}

		// The forbidden path must be reached before the AMFA lookup, so a
		// denied realm never reveals whether the tenant even has AMFA.
		denied := callToolErrTextAs(t, env.ts, env.tokenRealmX, "get_amfa_stats",
			map[string]any{"realm": postureRealmStaging})
		if denied != ErrForbidden.Error() {
			t.Errorf("get_amfa_stats for a denied realm = %q, want %q", denied, ErrForbidden.Error())
		}
		unavailable := callToolErrTextAs(t, env.ts, env.tokenRealmX, "get_amfa_stats",
			map[string]any{"realm": postureRealmProd})
		if unavailable != ErrAmfaNotAvailable.Error() {
			t.Errorf("get_amfa_stats for an allowed realm = %q, want %q", unavailable, ErrAmfaNotAvailable.Error())
		}
		if denied == unavailable {
			t.Error("the denied and the not-available paths returned the same message")
		}
	})

	t.Run("policy with no allowed realms denies everything realm-scoped", func(t *testing.T) {
		calls := []toolCall{
			{"list_realms", nil},
			{"list_events", nil},
			{"get_event_stats", nil},
			{"get_event", map[string]any{"event_id": env.aProdEvents[0]}},
			{"list_alerts", nil},
			{"get_alert", map[string]any{"alert_id": env.aProdAlerts[0]}},
			{"get_alert_stats", nil},
			{"get_amfa_stats", map[string]any{"realm": postureRealmProd}},
		}
		for _, call := range calls {
			got := callToolErrTextAs(t, env.ts, env.tokenNoRealms, call.name, call.args)
			if got != ErrForbidden.Error() {
				t.Errorf("%s under an empty allowed_realms policy = %q, want %q",
					call.name, got, ErrForbidden.Error())
			}
		}
	})

	t.Run("no policy row leaves the tenant unrestricted", func(t *testing.T) {
		var realms listRealmsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_realms", nil, &realms)
		if !slices.Equal(realms.Realms, []string{postureRealmProd, postureRealmStaging}) {
			t.Errorf("list_realms = %v, want [%s %s]", realms.Realms, postureRealmProd, postureRealmStaging)
		}

		var alert alertSummary
		callToolOKAs(t, env.ts, env.tokenA, "get_alert",
			map[string]any{"alert_id": env.aStagingAlert}, &alert)
		if alert.RealmName != postureRealmStaging {
			t.Errorf("get_alert realm = %q, want %q", alert.RealmName, postureRealmStaging)
		}
	})

	// A SQL NULL allowed_realms column must behave exactly like the absence of
	// a policy row: tenant_policies is an opt-in restriction, so a row naming
	// no list is not a restriction. This resolver used to deny it, disagreeing
	// with the web for the same row, and nothing here covered the shape.
	//
	// It runs every realm-scoped tool because the fix makes all = true
	// reachable through a policy row for the first time, and a caller that
	// read the realm set without checking all would now return nothing
	// instead of everything.
	t.Run("a null allowed_realms column leaves the tenant unrestricted", func(t *testing.T) {
		var realms listRealmsOutput
		callToolOKAs(t, env.ts, env.tokenNullRealms, "list_realms", nil, &realms)
		if !slices.Equal(realms.Realms, []string{postureRealmProd, postureRealmStaging}) {
			t.Errorf("list_realms = %v, want [%s %s]", realms.Realms, postureRealmProd, postureRealmStaging)
		}

		// Reachable only if the realm scope is unrestricted: this alert is in
		// staging, which no restricted policy in this suite allows.
		var alert alertSummary
		callToolOKAs(t, env.ts, env.tokenNullRealms, "get_alert",
			map[string]any{"alert_id": env.aStagingAlert}, &alert)
		if alert.RealmName != postureRealmStaging {
			t.Errorf("get_alert realm = %q, want %q", alert.RealmName, postureRealmStaging)
		}

		// Statistics must count the whole tenant rather than nothing, which is
		// what a scope-filtering bug would produce.
		var stats alertStatsOutput
		callToolOKAs(t, env.ts, env.tokenNullRealms, "get_alert_stats", nil, &stats)
		if stats.TotalActive == 0 {
			t.Error("get_alert_stats counted nothing for an unrestricted caller")
		}

		var listed listAlertsOutput
		callToolOKAs(t, env.ts, env.tokenNullRealms, "list_alerts", nil, &listed)
		if len(listed.Alerts) == 0 {
			t.Error("list_alerts returned nothing for an unrestricted caller")
		}
	})

	t.Run("cursor", func(t *testing.T) {
		args := map[string]any{
			"realm": postureRealmProd,
			"from":  env.windowFrom,
			"to":    env.windowTo,
			"limit": 2,
		}
		var page1 listEventsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_events", args, &page1)
		newest := []string{env.aProdEvents[4], env.aProdEvents[3]}
		if got := eventIDsOf(page1); !slices.Equal(got, newest) {
			t.Fatalf("page 1 = %v, want %v", got, newest)
		}
		if page1.NextCursor == "" {
			t.Fatal("a full page minted no cursor")
		}

		args["cursor"] = page1.NextCursor
		var page2 listEventsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_events", args, &page2)
		next := []string{env.aProdEvents[2], env.aProdEvents[1]}
		if got := eventIDsOf(page2); !slices.Equal(got, next) {
			t.Fatalf("page 2 = %v, want %v", got, next)
		}

		// Same user, same allowlist, identical filters: only the token id
		// differs, and that alone must invalidate the cursor.
		if got := callToolErrTextAs(t, env.ts, env.tokenAAlt, "list_events", args); got != ErrInvalidInput.Error() {
			t.Errorf("tenant A cursor under a second token = %q, want %q", got, ErrInvalidInput.Error())
		}

		// Identical arguments under a different token, so the filter digest
		// matches and only the token binding can reject the cursor.
		replay := map[string]any{
			"realm":  postureRealmProd,
			"from":   env.windowFrom,
			"to":     env.windowTo,
			"limit":  2,
			"cursor": page1.NextCursor,
		}
		if got := callToolErrTextAs(t, env.ts, env.tokenB, "list_events", replay); got != ErrInvalidInput.Error() {
			t.Errorf("tenant A cursor replayed under a tenant B token = %q, want %q", got, ErrInvalidInput.Error())
		}

		raw, err := base64.RawURLEncoding.DecodeString(page1.NextCursor)
		if err != nil {
			t.Fatalf("decode cursor: %v", err)
		}
		var payload cursorPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal cursor: %v", err)
		}
		payload.ID++
		tampered, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal the tampered cursor: %v", err)
		}
		args["cursor"] = base64.RawURLEncoding.EncodeToString(tampered)
		if got := callToolErrTextAs(t, env.ts, env.tokenA, "list_events", args); got != ErrInvalidInput.Error() {
			t.Errorf("tampered cursor = %q, want %q", got, ErrInvalidInput.Error())
		}
	})

	t.Run("token lifecycle", func(t *testing.T) {
		session := connectClient(t, env.ts, env.tokenRevoked)
		if _, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "whoami"}); err != nil {
			t.Fatalf("whoami before revocation: %v", err)
		}
		if err := env.writeTokens.Revoke(ctx, env.revokedTokenID); err != nil {
			t.Fatalf("revoke the token: %v", err)
		}
		if _, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "whoami"}); err == nil {
			t.Error("a token revoked mid-session was still accepted on the next call")
		}

		if code := postInitialize(t, env.ts, env.tokenExpired); code != http.StatusUnauthorized {
			t.Errorf("expired token: status %d, want %d", code, http.StatusUnauthorized)
		}

		if err := env.writeUsers.BlockUser(ctx, env.blockedSubject, "posture integration test"); err != nil {
			t.Fatalf("block the user: %v", err)
		}
		if code := postInitialize(t, env.ts, env.tokenBlocked); code != http.StatusUnauthorized {
			t.Errorf("blocked user: status %d, want %d", code, http.StatusUnauthorized)
		}
	})

	t.Run("disabled tenant", func(t *testing.T) {
		for _, call := range tenantScopedCalls("", postureRealmProd, env.missingAlertID, env.missingEventID) {
			got := callToolErrTextAs(t, env.ts, env.tokenC, call.name, call.args)
			if got != ErrTenantNotAvailable.Error() {
				t.Errorf("%s for a disabled tenant = %q, want %q", call.name, got, ErrTenantNotAvailable.Error())
			}
		}
	})

	t.Run("caps", func(t *testing.T) {
		var alertPage listAlertsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_alerts",
			map[string]any{"limit": 1000}, &alertPage)
		if alertPage.Limit != posturePageCap {
			t.Errorf("list_alerts limit = %d, want %d", alertPage.Limit, posturePageCap)
		}
		if len(alertPage.Alerts) != posturePageCap {
			t.Errorf("list_alerts returned %d alerts, want %d", len(alertPage.Alerts), posturePageCap)
		}
		if alertPage.Total != 5 {
			t.Errorf("list_alerts total = %d, want 5", alertPage.Total)
		}

		var eventPage listEventsOutput
		callToolOKAs(t, env.ts, env.tokenA, "list_events", map[string]any{
			"from": env.windowFrom, "to": env.windowTo, "limit": 1000,
		}, &eventPage)
		if len(eventPage.Events) != posturePageCap {
			t.Errorf("list_events returned %d events, want %d", len(eventPage.Events), posturePageCap)
		}

		now := time.Now().UTC()
		wide := callToolErrTextAs(t, env.ts, env.tokenA, "list_events", map[string]any{
			"from": now.Add(-100 * 24 * time.Hour).Format(time.RFC3339),
			"to":   now.Format(time.RFC3339),
		})
		if !strings.Contains(wide, "90 days") {
			t.Errorf("a 100 day window = %q, want the 90 day cap message", wide)
		}

		for name, args := range map[string]map[string]any{
			"from without an offset": {"from": "2026-01-02 15:04:05"},
			"to that is not a time":  {"to": "yesterday"},
		} {
			got := callToolErrTextAs(t, env.ts, env.tokenA, "list_events", args)
			if !strings.HasPrefix(got, "invalid input:") || !strings.Contains(got, "RFC3339") {
				t.Errorf("%s = %q, want an RFC3339 validation message", name, got)
			}
		}
	})

	// Declared last so it observes every call the subtests above made.
	t.Run("read-only", func(t *testing.T) {
		if attempts := env.writes.seen(); len(attempts) > 0 {
			t.Errorf("the MCP path issued %d write(s) against its database client: %v",
				len(attempts), attempts)
		}

		after := env.counts()
		for table, before := range env.baseline {
			if after[table] != before {
				t.Errorf("%s row count moved from %d to %d across the MCP calls",
					table, before, after[table])
			}
		}
	})
}
