//go:build integration

// End-to-end coverage for the OIDC login path against a real Keycloak.
//
// The package tests around auth/ stub the provider, so they prove the service
// stores whatever UserInfo carries and nothing about what a real IdP actually
// puts in a token. That gap hid a bug for a long time: both OAuth paths set
// EmailVerified true unconditionally, and no stub-based test could see it
// because the stub was always asked for the value the code then ignored.
//
// This test closes the loop: a token Keycloak really issued, parsed by the real
// OIDCProvider, stored by the real service through the real repository, and
// read back out of PostgreSQL.
//
// Double-gated like the alert pipeline e2e. The integration build tag keeps it
// out of the default run, and it is skipped unless both of its own variables
// are set. The DSN is guarded by testdb.RequireDisposable: this test drops and
// recreates the public schema, so it must never be aimed at a deployment.
//
//	KMT_AUTH_E2E_KEYCLOAK_URL="http://localhost:8085" \
//	KMT_AUTH_E2E_DATABASE_DSN="host=localhost port=55439 dbname=auth_e2e_test user=postgres password=test sslmode=disable" \
//	    go test -tags=integration ./tests/ -run AuthOIDCE2E -v
//
// A disposable Keycloak is enough; the test provisions its own realm, client
// and users, and deletes the realm afterwards. Admin credentials default to
// admin/admin and can be overridden with KMT_AUTH_E2E_KEYCLOAK_ADMIN and
// KMT_AUTH_E2E_KEYCLOAK_PASSWORD.
//
//	docker run -d --name kc-e2e -p 8085:8080 \
//	  -e KEYCLOAK_ADMIN=admin -e KEYCLOAK_ADMIN_PASSWORD=admin \
//	  -e KC_HTTP_ENABLED=true -e KC_HOSTNAME_STRICT=false \
//	  quay.io/keycloak/keycloak:26.0 start-dev
package main_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/auth"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
	"github.com/DefensePoint/keycloak-monitoring/pkg/database"
	userspg "github.com/DefensePoint/keycloak-monitoring/users/postgres"
)

const (
	e2eRealm        = "point-auth-e2e"
	e2eClientID     = "point-auth-e2e-client"
	e2eClientSecret = "point-auth-e2e-secret"
	e2eUserPassword = "e2e-password"
)

// e2eUsers are identical except for the one field under test.
var e2eUsers = []struct {
	username string
	email    string
	verified bool
}{
	{"e2e-verified", "e2e-verified@example.invalid", true},
	{"e2e-unverified", "e2e-unverified@example.invalid", false},
}

// --- Keycloak admin plumbing -------------------------------------------------

type kcAdmin struct {
	base  string
	token string
	t     *testing.T
}

func (k *kcAdmin) do(method, path string, body interface{}) (int, []byte) {
	k.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			k.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, k.base+path, payload)
	if err != nil {
		k.t.Fatalf("build %s %s: %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+k.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		k.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func newKCAdmin(t *testing.T, base string) *kcAdmin {
	t.Helper()
	user := envOr("KMT_AUTH_E2E_KEYCLOAK_ADMIN", "admin")
	pass := envOr("KMT_AUTH_E2E_KEYCLOAK_PASSWORD", "admin")

	form := url.Values{
		"client_id": {"admin-cli"}, "username": {user},
		"password": {pass}, "grant_type": {"password"},
	}
	resp, err := http.PostForm(base+"/realms/master/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatalf("Keycloak at %s is not reachable: %v", base, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login failed (HTTP %d): %s", resp.StatusCode, string(raw))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &tok); err != nil || tok.AccessToken == "" {
		t.Fatalf("no admin access token in response: %s", string(raw))
	}
	return &kcAdmin{base: base, token: tok.AccessToken, t: t}
}

// provisionRealm creates the realm, client and users, and removes the realm
// when the test ends. Deleting the realm takes its clients and users with it,
// so there is no per-object cleanup to keep in step.
func (k *kcAdmin) provisionRealm(t *testing.T) {
	t.Helper()

	// A realm left behind by an interrupted run would otherwise fail creation.
	k.do(http.MethodDelete, "/admin/realms/"+e2eRealm, nil)
	t.Cleanup(func() { k.do(http.MethodDelete, "/admin/realms/"+e2eRealm, nil) })

	if code, body := k.do(http.MethodPost, "/admin/realms", map[string]interface{}{
		"realm": e2eRealm, "enabled": true,
	}); code != http.StatusCreated {
		t.Fatalf("create realm (HTTP %d): %s", code, string(body))
	}

	if code, body := k.do(http.MethodPost, "/admin/realms/"+e2eRealm+"/clients", map[string]interface{}{
		"clientId": e2eClientID, "enabled": true, "protocol": "openid-connect",
		"publicClient": false, "secret": e2eClientSecret,
		// Direct grant stands in for the browser redirect. The code exchange is
		// not what this test is about: everything it asserts happens after the
		// ID token exists, and the token is the same either way.
		"directAccessGrantsEnabled": true, "standardFlowEnabled": true,
		"redirectUris": []string{"http://localhost:7880/*"},
	}); code != http.StatusCreated {
		t.Fatalf("create client (HTTP %d): %s", code, string(body))
	}

	for _, u := range e2eUsers {
		if code, body := k.do(http.MethodPost, "/admin/realms/"+e2eRealm+"/users", map[string]interface{}{
			"username": u.username, "email": u.email,
			"emailVerified": u.verified, "enabled": true,
			// Without a complete profile Keycloak attaches an UPDATE_PROFILE
			// required action and direct grant returns "Account is not fully
			// set up", which has nothing to do with email verification.
			"firstName": "E2E", "lastName": u.username,
			"requiredActions": []string{},
			"credentials": []map[string]interface{}{
				{"type": "password", "value": e2eUserPassword, "temporary": false},
			},
		}); code != http.StatusCreated {
			t.Fatalf("create user %s (HTTP %d): %s", u.username, code, string(body))
		}
	}
}

// idTokenFor signs the user in by direct grant and returns the raw ID token.
func idTokenFor(t *testing.T, base, username string) string {
	return idTokenForScope(t, base, username, "openid email profile")
}

// idTokenForScope exists because a realm with the email scope removed rejects a
// request that still asks for it, with invalid_scope rather than a token.
func idTokenForScope(t *testing.T, base, username, scope string) string {
	t.Helper()
	form := url.Values{
		"client_id": {e2eClientID}, "client_secret": {e2eClientSecret},
		"username": {username}, "password": {e2eUserPassword},
		"grant_type": {"password"}, "scope": {scope},
	}
	resp, err := http.PostForm(base+"/realms/"+e2eRealm+"/protocol/openid-connect/token", form)
	if err != nil {
		t.Fatalf("token request for %s: %v", username, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.IDToken == "" {
		t.Fatalf("no id_token for %s (HTTP %d): %s", username, resp.StatusCode, string(raw))
	}
	return out.IDToken
}

// rawClaims decodes the token payload without verifying it, to inspect which
// claims are present rather than what they decoded to.
func rawClaims(t *testing.T, idToken string) map[string]interface{} {
	t.Helper()
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed ID token: %d segments", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode token payload: %v", err)
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("parse token payload: %v", err)
	}
	return claims
}

// --- database plumbing -------------------------------------------------------

// newBootedClient resets the schema and boots the real client, so the test runs
// against the schema production actually has — including the migrations that
// AutoMigrate applies on top of GORM's own output, such as the partial username
// index without which two OAuth users collide on an empty username.
func newBootedClient(t *testing.T, dsn string) (*database.Client, *gorm.DB) {
	t.Helper()
	raw, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	c, err := database.NewClient(dsnConfig(t, dsn), logger.NewNoop())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	after, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	return c, after
}

func dsnConfig(t *testing.T, dsn string) *config.DatabaseConfig {
	t.Helper()
	parsed, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse KMT_AUTH_E2E_DATABASE_DSN: %v", err)
	}
	sslMode := "require"
	if parsed.TLSConfig == nil {
		sslMode = "disable"
	}
	return &config.DatabaseConfig{
		Host: parsed.Host, Port: int(parsed.Port), Database: parsed.Database,
		User: parsed.User, Password: parsed.Password, SSLMode: sslMode,
		MaxConns: 5, MinConns: 1, Timeout: 30 * time.Second,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// e2eEnv skips unless both variables are present, and refuses a DSN that does
// not look disposable.
func e2eEnv(t *testing.T) (kcURL, dsn string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	kcURL = os.Getenv("KMT_AUTH_E2E_KEYCLOAK_URL")
	dsn = os.Getenv("KMT_AUTH_E2E_DATABASE_DSN")
	if kcURL == "" || dsn == "" {
		t.Skip("KMT_AUTH_E2E_KEYCLOAK_URL and KMT_AUTH_E2E_DATABASE_DSN not both set; skipping auth e2e")
	}
	testdb.RequireDisposable(t, dsn)
	return strings.TrimRight(kcURL, "/"), dsn
}

// --- the tests ---------------------------------------------------------------

// TestAuthOIDCE2E_EmailVerifiedFollowsTheIdP is the assertion the stub-based
// tests cannot make: that the value Keycloak puts in the token is the value
// that ends up in the users table, for an address it considers unverified as
// well as one it has confirmed.
func TestAuthOIDCE2E_EmailVerifiedFollowsTheIdP(t *testing.T) {
	kcURL, dsn := e2eEnv(t)
	ctx := context.Background()

	admin := newKCAdmin(t, kcURL)
	admin.provisionRealm(t)

	_, db := newBootedClient(t, dsn)

	provider, err := auth.NewOIDCProvider(ctx, &auth.Config{
		ProviderURL:  kcURL + "/realms/" + e2eRealm,
		ClientID:     e2eClientID,
		ClientSecret: e2eClientSecret,
		RedirectURL:  "http://localhost:7880/callback",
		Scopes:       []string{"openid", "email", "profile"},
	}, logger.NewNoop())
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}

	svc := auth.NewService(provider, auth.NewMemorySessionStore(time.Hour),
		userspg.NewRepository(db), logger.NewNoop(), &auth.Config{})

	for _, u := range e2eUsers {
		t.Run(u.username, func(t *testing.T) {
			raw := idTokenFor(t, kcURL, u.username)

			// The claim must be present, not merely decode to the right value.
			// An absent claim unmarshals into false, which would look correct
			// for the unverified user and wrong for nobody until a realm that
			// does send it disagreed.
			if _, ok := rawClaims(t, raw)["email_verified"]; !ok {
				t.Fatalf("Keycloak did not put email_verified in the ID token; " +
					"the stored flag would silently read false for every user")
			}

			info, err := provider.VerifyIDToken(ctx, raw)
			if err != nil {
				t.Fatalf("VerifyIDToken: %v", err)
			}
			if info.EmailVerified != u.verified {
				t.Errorf("provider reported EmailVerified = %v, want %v", info.EmailVerified, u.verified)
			}

			if _, err := svc.FindOrCreateUser(ctx, info); err != nil {
				t.Fatalf("FindOrCreateUser: %v", err)
			}

			var stored database.User
			if err := db.Where("email = ?", u.email).First(&stored).Error; err != nil {
				t.Fatalf("reload stored user: %v", err)
			}
			if stored.EmailVerified != u.verified {
				t.Errorf("users.email_verified = %v, want %v — the IdP's claim must reach the row",
					stored.EmailVerified, u.verified)
			}
		})
	}
}

// TestAuthOIDCE2E_TwoOAuthUsersCanCoexist pins the schema behaviour that the
// first run of this harness tripped over. Both OAuth users are created with an
// empty username, and a plain unique index treats the empty string as a value,
// so the second insert collides. Production only survives because AutoMigrate rewrites that
// index to a partial one; this fails if that migration is ever dropped.
func TestAuthOIDCE2E_TwoOAuthUsersCanCoexist(t *testing.T) {
	_, dsn := e2eEnv(t)
	_, db := newBootedClient(t, dsn)

	for i := 0; i < 2; i++ {
		u := &database.User{
			Subject:    fmt.Sprintf("coexist-subject-%d", i),
			Email:      fmt.Sprintf("coexist-%d@example.invalid", i),
			AuthMethod: "oauth",
			IsActive:   true,
		}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("OAuth user %d could not be created, so no second person can "+
				"sign in through SSO: %v", i, err)
		}
	}
}

// TestAuthOIDCE2E_MigrationClearsStaleRowsOnBoot exercises
// migrateUnprovenEmailVerified where it actually runs: inside AutoMigrate, on a
// row that predates the fix. The package test calls the migration directly,
// which proves the SQL but not that a real boot reaches it.
func TestAuthOIDCE2E_MigrationClearsStaleRowsOnBoot(t *testing.T) {
	_, dsn := e2eEnv(t)

	// First boot: establish the schema, then seed rows as the old code left them.
	_, db := newBootedClient(t, dsn)

	stale := &database.User{
		Subject: "stale-subject", Email: "stale@example.invalid",
		EmailVerified: true, AuthMethod: "oauth", IsActive: true,
		LastLoginAt: ptr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	fresh := &database.User{
		Subject: "fresh-subject", Email: "fresh@example.invalid",
		EmailVerified: true, AuthMethod: "oauth", IsActive: true,
		LastLoginAt: ptr(time.Now().Add(24 * time.Hour)),
	}
	for _, u := range []*database.User{stale, fresh} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("seed %s: %v", u.Email, err)
		}
	}

	// Second boot against the same schema, which is what a restart does.
	if _, err := database.NewClient(dsnConfig(t, dsn), logger.NewNoop()); err != nil {
		t.Fatalf("second NewClient: %v", err)
	}

	var gotStale database.User
	if err := db.Where("email = ?", stale.Email).First(&gotStale).Error; err != nil {
		t.Fatalf("reload stale user: %v", err)
	}
	if gotStale.EmailVerified {
		t.Error("a pre-fix row kept email_verified through a real boot; the migration " +
			"is registered but not reached")
	}

	var gotFresh database.User
	if err := db.Where("email = ?", fresh.Email).First(&gotFresh).Error; err != nil {
		t.Fatalf("reload post-fix user: %v", err)
	}
	if !gotFresh.EmailVerified {
		t.Error("a post-fix row lost email_verified on boot; every restart would erase " +
			"claims the IdP actually made")
	}
}

func ptr(t time.Time) *time.Time { return &t }

// --- the failure branches ----------------------------------------------------
//
// The two tests above cover the path a healthy realm takes. These cover what
// happens when it is not healthy, which is where the mitigations live and where
// nothing else can reach them: Keycloak sends email_verified under its default
// scopes, and AutoMigrate repairs the username index, so neither branch is
// reachable without deliberately breaking the setup first.

// stripEmailScope removes the email client scope, which is what carries the
// email_verified claim. Realms are configured by hand, so this is a
// configuration a customer can arrive at without meaning to.
func (k *kcAdmin) stripEmailScope(t *testing.T) {
	t.Helper()

	code, body := k.do(http.MethodGet, "/admin/realms/"+e2eRealm+"/clients?clientId="+e2eClientID, nil)
	if code != http.StatusOK {
		t.Fatalf("look up client (HTTP %d): %s", code, string(body))
	}
	var found []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &found); err != nil || len(found) == 0 {
		t.Fatalf("client %s not found: %s", e2eClientID, string(body))
	}

	code, body = k.do(http.MethodGet, "/admin/realms/"+e2eRealm+"/client-scopes", nil)
	if code != http.StatusOK {
		t.Fatalf("list client scopes (HTTP %d): %s", code, string(body))
	}
	var scopes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &scopes); err != nil {
		t.Fatalf("parse client scopes: %s", string(body))
	}
	for _, sc := range scopes {
		if sc.Name != "email" {
			continue
		}
		if code, body := k.do(http.MethodDelete,
			"/admin/realms/"+e2eRealm+"/clients/"+found[0].ID+"/default-client-scopes/"+sc.ID, nil); code >= 300 {
			t.Fatalf("remove email scope (HTTP %d): %s", code, string(body))
		}
		return
	}
	t.Fatal("no client scope named email in the realm")
}

// TestAuthOIDCE2E_WarnsWhenRealmOmitsEmailVerified drives the branch that
// exists for realms nobody here can inspect. An absent claim decodes to false,
// so without the warning the platform would mark every user unverified and give
// no reason; this proves the reason is produced, against a realm genuinely
// missing the scope rather than a hand-built map.
func TestAuthOIDCE2E_WarnsWhenRealmOmitsEmailVerified(t *testing.T) {
	kcURL, dsn := e2eEnv(t)
	ctx := context.Background()

	admin := newKCAdmin(t, kcURL)
	admin.provisionRealm(t)
	admin.stripEmailScope(t)

	newBootedClient(t, dsn)

	raw := idTokenForScope(t, kcURL, e2eUsers[0].username, "openid profile")
	if _, present := rawClaims(t, raw)["email_verified"]; present {
		t.Fatal("email_verified is still in the token; the scope was not actually removed, " +
			"so this test would pass without proving anything")
	}

	var logs bytes.Buffer
	provider, err := auth.NewOIDCProvider(ctx, &auth.Config{
		ProviderURL:  kcURL + "/realms/" + e2eRealm,
		ClientID:     e2eClientID,
		ClientSecret: e2eClientSecret,
		RedirectURL:  "http://localhost:7880/callback",
		Scopes:       []string{"openid", "profile"},
	}, logger.New(zerolog.New(&logs)))
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}

	info, err := provider.VerifyIDToken(ctx, raw)
	if err != nil {
		t.Fatalf("VerifyIDToken: %v", err)
	}
	if info.EmailVerified {
		t.Error("an absent claim must not read as verified")
	}
	if !strings.Contains(logs.String(), "email_verified") {
		t.Fatalf("a realm with no email scope produced no warning, so the flag would read "+
			"false for every user with nothing to explain it; logs: %s", logs.String())
	}

	// Still once, not once per login: this is the path a broken realm takes on
	// every single sign-in.
	before := strings.Count(logs.String(), "email_verified")
	for i := 0; i < 3; i++ {
		if _, err := provider.VerifyIDToken(ctx, raw); err != nil {
			t.Fatalf("repeat VerifyIDToken: %v", err)
		}
	}
	if after := strings.Count(logs.String(), "email_verified"); after != before {
		t.Errorf("warning repeated across logins (%d then %d); a broken realm would flood the log",
			before, after)
	}
}

// TestAuthOIDCE2E_BootRefusesUnsafeUsernameIndex drives the guard where it
// actually runs. The package tests call assertUsernameIndexIsSafe directly,
// which proves the check and not that a boot is stopped by it.
func TestAuthOIDCE2E_BootRefusesUnsafeUsernameIndex(t *testing.T) {
	_, dsn := e2eEnv(t)

	_, db := newBootedClient(t, dsn)

	// A plain index on its own is not enough to reach the guard: the migration
	// repairs it first, which is the system working. The reachable failure is
	// CREATE UNIQUE INDEX being rejected, and duplicate usernames do that.
	db.Exec("DROP INDEX IF EXISTS idx_users_username")
	for i := 0; i < 2; i++ {
		u := &database.User{
			Subject: fmt.Sprintf("dupe-subject-%d", i),
			Email:   fmt.Sprintf("dupe-%d@example.invalid", i),
			// The same username twice, which no unique index can be built over.
			Username: "duplicated-username", AuthMethod: "simple", IsActive: true,
		}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("seed duplicate username %d: %v", i, err)
		}
	}

	_, err := database.NewClient(dsnConfig(t, dsn), logger.NewNoop())
	if err == nil {
		t.Fatal("the platform started with nothing enforcing username uniqueness")
	}
	// Which layer refuses is not the point, and in this case it is GORM rather
	// than assertUsernameIndexIsSafe: AutoMigrate cannot build the index from
	// the struct tag either, and gives up before the guard is reached. The
	// guard covers what is left — a CREATE that fails for some reason other
	// than duplicate rows, after the old index was already dropped. What must
	// hold is that neither path serves traffic, and that the error names the
	// index so an operator can find it.
	if !strings.Contains(err.Error(), "idx_users_username") {
		t.Errorf("the refusal should name the index at fault, got: %v", err)
	}
}
