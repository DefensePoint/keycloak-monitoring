//go:build integration

// End-to-end coverage for renaming a simple-auth user and reusing the name it
// frees, through the platform's own HTTP API.
//
// A simple-auth user's subject is their username, and a rename rewrites it, so
// the old name can be given to a new account. A session cookie from before the
// rename then names the new account by subject. The session also carries the
// ID of the account it was issued for, and the auth middleware refuses it when
// the subject now names a different one. The package tests prove each half
// against doubles; this test boots the real server on a real database and
// does what an administrator and the users would do, over HTTP.
//
// The same server is signed in to through Keycloak, by the browser redirect
// and the platform's own callback, to show an SSO session carries the account
// ID and keeps working.
//
// It runs with the other auth e2e tests and needs the same two variables (see
// auth_oidc_e2e_test.go). It drops and recreates the public schema of the
// database it is given.
//
//	KMT_AUTH_E2E_KEYCLOAK_URL="http://localhost:8085" \
//	KMT_AUTH_E2E_DATABASE_DSN="host=localhost port=55439 dbname=auth_e2e_test user=postgres password=test sslmode=disable" \
//	    go test -tags=integration ./tests/ -run AuthOIDCE2E_Rename -v
package main_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/auth"
	appfx "github.com/DefensePoint/keycloak-monitoring/internal/fx"
	"github.com/DefensePoint/keycloak-monitoring/internal/testdb"
)

const (
	renameAdminUser     = "e2e-admin"
	renameAdminPass     = "Qx7#vRm2-Lpz9!Tw"
	renameSessionName   = "monitoring_session"
	renameSessionSecret = "e2e-rename-session-secret-0123456789abcdef"
)

// bootPlatform starts the real server on an empty schema and returns its base
// URL. It signs in to Keycloak's e2e realm, so the realm must exist first.
func bootPlatform(t *testing.T, kcURL, dsn string) string {
	t.Helper()

	// The config loader lets any MONITORING_* variable override the file. One
	// left in the shell could point the server at another database than the
	// disposable one reset below, so they are all blanked, which the loader
	// reads as unset, until the test ends.
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "MONITORING_") {
			t.Setenv(name, "")
		}
	}

	raw, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if db, err := raw.DB(); err == nil {
		db.Close()
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick a port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	base := fmt.Sprintf("http://localhost:%d", port)

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	db := testdb.DatabaseConfig(t, dsn)
	cfg := fmt.Sprintf(`security:
  encryption_key: %q
http:
  server:
    enabled: true
    host: "127.0.0.1"
    port: %d
web:
  server:
    enabled: false
keycloak:
  global: {}
  tenants: {}
database:
  host: %q
  port: %d
  database: %q
  user: %q
  password: %q
  ssl_mode: %q
  max_conns: 5
  min_conns: 1
  timeout: 30s
auth:
  simple:
    enabled: true
    default_user: %q
    default_pass: %q
    default_email: "e2e-admin@example.invalid"
  oauth2:
    enabled: true
    provider_url: %q
    client_id: %q
    client_secret: %q
    redirect_url: %q
    scopes: ["openid", "profile", "email"]
  session:
    secret: %q
    name: %q
    max_age: 24h
    secure: false
    same_site: "lax"
    state_max_age: 10m
logging:
  level: "warn"
  format: "json"
`, base64.StdEncoding.EncodeToString(key), port,
		db.Host, db.Port, db.Database, db.User, db.Password, db.SSLMode,
		renameAdminUser, renameAdminPass,
		kcURL+"/realms/"+e2eRealm, e2eClientID, e2eClientSecret, base+"/auth/callback",
		renameSessionSecret, renameSessionName)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	app := appfx.NewStandaloneApp(path)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("start the platform: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = app.Stop(ctx)
	})

	for i := 0; i < 60; i++ {
		if resp, err := http.Get(base + "/auth/config"); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return base
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("the platform did not answer on %s", base)
	return ""
}

// allowRedirect adds uri to the e2e client's redirect URIs, so the platform's
// callback on its test port is accepted.
func (k *kcAdmin) allowRedirect(t *testing.T, uri string) {
	t.Helper()
	code, body := k.do(http.MethodGet, "/admin/realms/"+e2eRealm+"/clients?clientId="+e2eClientID, nil)
	var found []map[string]interface{}
	if code != http.StatusOK || json.Unmarshal(body, &found) != nil || len(found) != 1 {
		t.Fatalf("find client (HTTP %d): %s", code, string(body))
	}
	client := found[0]
	client["redirectUris"] = []string{uri}
	if code, body := k.do(http.MethodPut, "/admin/realms/"+e2eRealm+"/clients/"+client["id"].(string), client); code >= 300 {
		t.Fatalf("set redirect URI (HTTP %d): %s", code, string(body))
	}
}

// browser is one person's cookie jar against the platform (and Keycloak). It
// never follows redirects, so each hop is seen.
type browser struct {
	t    *testing.T
	base string
	c    *http.Client
}

func newBrowser(t *testing.T, base string) *browser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &browser{t: t, base: base, c: &http.Client{
		Jar:     jar,
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

// send makes a request to an absolute URL and returns the response with its
// body read.
func (b *browser) send(req *http.Request) (*http.Response, []byte) {
	b.t.Helper()
	resp, err := b.c.Do(req)
	if err != nil {
		b.t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// call makes a request to the platform with a JSON body, if any.
func (b *browser) call(method, path string, body interface{}) (int, []byte) {
	b.t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			b.t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, b.base+path, payload)
	if err != nil {
		b.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, out := b.send(req)
	return resp.StatusCode, out
}

func (b *browser) signIn(username, password string) int {
	b.t.Helper()
	code, _ := b.call(http.MethodPost, "/auth/login/simple", map[string]string{"username": username, "password": password})
	return code
}

// whoami returns the status of /auth/userinfo and, when it answers, the ID of
// the account the session acts as.
func (b *browser) whoami() (int, uint) {
	b.t.Helper()
	code, body := b.call(http.MethodGet, "/auth/userinfo", nil)
	var info struct{ ID uint }
	if code == http.StatusOK {
		if err := json.Unmarshal(body, &info); err != nil {
			b.t.Fatalf("userinfo: %v: %s", err, string(body))
		}
	}
	return code, info.ID
}

// sessionAccountID reads the platform session cookie back with the server's
// own session store and returns the account ID it carries.
func (b *browser) sessionAccountID() (uint, bool) {
	b.t.Helper()
	u, _ := url.Parse(b.base)
	req := mustRequest(b.t, http.MethodGet, b.base+"/auth/userinfo", nil)
	for _, c := range b.c.Jar.Cookies(u) {
		req.AddCookie(c)
	}
	mgr := auth.NewCookieSessionManager(renameSessionSecret, renameSessionName, 86400, false, "lax")
	session, err := mgr.GetStore().Get(req, renameSessionName)
	if err != nil {
		b.t.Fatalf("read the session cookie: %v", err)
	}
	id, ok := session.Values["user_id"].(uint)
	return id, ok
}

// createUser creates a simple-auth account through the API and returns its ID.
func createUser(t *testing.T, admin *browser, username, email, password string) uint {
	t.Helper()
	code, body := admin.call(http.MethodPost, "/api/users/", map[string]string{
		"username": username, "email": email, "password": password, "name": username,
	})
	var created struct{ ID uint }
	if code != http.StatusCreated || json.Unmarshal(body, &created) != nil || created.ID == 0 {
		t.Fatalf("create %s: HTTP %d: %s", username, code, string(body))
	}
	return created.ID
}

func TestAuthOIDCE2E_RenameAndUsernameReuseThroughTheAPI(t *testing.T) {
	kcURL, dsn := e2eEnv(t)
	kc := newKCAdmin(t, kcURL)
	kc.provisionRealm(t)
	base := bootPlatform(t, kcURL, dsn)
	kc.allowRedirect(t, base+"/*")

	admin := newBrowser(t, base)
	if code := admin.signIn(renameAdminUser, renameAdminPass); code != http.StatusOK {
		t.Fatalf("admin sign-in: HTTP %d", code)
	}

	t.Run("rename, then a new account takes the old name", func(t *testing.T) {
		const alicePass, newAlicePass = "Hk4$wZp8-Nq3!Vb", "Rt6%yLm1-Gc5!Jd"
		aliceID := createUser(t, admin, "alice", "alice@example.invalid", alicePass)

		// The session alice holds before any of it; it must end up refused.
		oldAlice := newBrowser(t, base)
		if code := oldAlice.signIn("alice", alicePass); code != http.StatusOK {
			t.Fatalf("alice sign-in: HTTP %d", code)
		}
		if code, id := oldAlice.whoami(); code != http.StatusOK || id != aliceID {
			t.Fatalf("alice's session: HTTP %d as account %d, want 200 as %d", code, id, aliceID)
		}

		if code, body := admin.call(http.MethodPut, fmt.Sprintf("/api/users/%d/", aliceID), map[string]string{"username": renameAdminUser}); code != http.StatusConflict {
			t.Errorf("rename to a taken username: HTTP %d, want 409: %s", code, string(body))
		}
		if code, body := admin.call(http.MethodPut, fmt.Sprintf("/api/users/%d/", aliceID), map[string]string{"username": "alicia"}); code != http.StatusOK {
			t.Fatalf("rename to a free username: HTTP %d, want 200: %s", code, string(body))
		}

		if code, _ := oldAlice.whoami(); code != http.StatusUnauthorized {
			t.Errorf("alice's session after the rename: HTTP %d, want 401", code)
		}
		if code := newBrowser(t, base).signIn("alice", alicePass); code != http.StatusUnauthorized {
			t.Errorf("sign-in with the old name: HTTP %d, want 401", code)
		}
		alicia := newBrowser(t, base)
		if code := alicia.signIn("alicia", alicePass); code != http.StatusOK {
			t.Fatalf("sign-in with the new name: HTTP %d, want 200", code)
		}
		if code, id := alicia.whoami(); code != http.StatusOK || id != aliceID {
			t.Errorf("alicia's session: HTTP %d as account %d, want 200 as %d", code, id, aliceID)
		}

		newAliceID := createUser(t, admin, "alice", "alice2@example.invalid", newAlicePass)
		if newAliceID == aliceID {
			t.Fatalf("the new alice has the renamed account's ID %d", aliceID)
		}

		// The old cookie's subject now names the new account, at the same
		// session epoch and active: only the account ID it carries tells them
		// apart.
		if code, id := oldAlice.whoami(); code != http.StatusUnauthorized {
			t.Errorf("alice's old session once the name is reused: HTTP %d as account %d, want 401", code, id)
		}
		newAlice := newBrowser(t, base)
		if code := newAlice.signIn("alice", newAlicePass); code != http.StatusOK {
			t.Fatalf("the new alice signs in: HTTP %d, want 200", code)
		}
		if code, id := newAlice.whoami(); code != http.StatusOK || id != newAliceID {
			t.Errorf("the new alice's session: HTTP %d as account %d, want 200 as %d", code, id, newAliceID)
		}
		if id, ok := newAlice.sessionAccountID(); !ok || id != newAliceID {
			t.Errorf("the new alice's session carries account %d (%v), want %d", id, ok, newAliceID)
		}
		if code, _ := oldAlice.whoami(); code != http.StatusUnauthorized {
			t.Errorf("alice's old session after the new alice signs in: HTTP %d, want 401", code)
		}
		if code, id := alicia.whoami(); code != http.StatusOK || id != aliceID {
			t.Errorf("alicia's session after the reuse: HTTP %d as account %d, want 200 as %d", code, id, aliceID)
		}
	})

	t.Run("an SSO session carries the account ID and keeps working", func(t *testing.T) {
		user := newBrowser(t, base)

		// The platform starts the login and sends the browser to Keycloak.
		resp, _ := user.send(mustRequest(t, http.MethodGet, base+"/auth/login/oauth2", nil))
		authURL := resp.Header.Get("Location")
		if resp.StatusCode != http.StatusFound || !strings.Contains(authURL, "/protocol/openid-connect/auth") {
			t.Fatalf("login: HTTP %d to %q, want a redirect to Keycloak", resp.StatusCode, authURL)
		}

		// Keycloak's login form, filled in and posted.
		_, page := user.send(mustRequest(t, http.MethodGet, authURL, nil))
		m := regexp.MustCompile(`action="([^"]+)"`).FindSubmatch(page)
		if m == nil {
			t.Fatalf("no login form in Keycloak's page: %.300s", page)
		}
		form := url.Values{"username": {e2eUsers[0].username}, "password": {e2eUserPassword}}
		req := mustRequest(t, http.MethodPost, html.UnescapeString(string(m[1])), strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, _ = user.send(req)
		callback := resp.Header.Get("Location")
		if !strings.HasPrefix(callback, base+"/auth/callback?") {
			t.Fatalf("Keycloak sent the browser to %q (HTTP %d), want the platform callback", callback, resp.StatusCode)
		}

		// The platform's callback opens the session.
		resp, body := user.send(mustRequest(t, http.MethodGet, callback, nil))
		if resp.StatusCode >= 400 {
			t.Fatalf("callback: HTTP %d: %s", resp.StatusCode, string(body))
		}

		code, id := user.whoami()
		if code != http.StatusOK || id == 0 {
			t.Fatalf("the SSO session: HTTP %d as account %d, want 200 as a real account", code, id)
		}
		if got, ok := user.sessionAccountID(); !ok || got != id {
			t.Errorf("the SSO session carries account %d (%v), want %d", got, ok, id)
		}
		if again, againID := user.whoami(); again != http.StatusOK || againID != id {
			t.Errorf("the SSO session on a later request: HTTP %d as account %d, want 200 as %d", again, againID, id)
		}
	})
}

func mustRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
