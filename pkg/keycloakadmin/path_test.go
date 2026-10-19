package keycloakadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCheckPathSegment(t *testing.T) {
	for _, ok := range []string{"demo", "a1b2c3d4-0000-4000-8000-000000000000", "f:7c8e:jdoe", "my realm"} {
		if err := CheckPathSegment(ok); err != nil {
			t.Errorf("CheckPathSegment(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../../master/users/u1", "a/b", `a\b`, "users/"} {
		if err := CheckPathSegment(bad); !errors.Is(err, ErrInvalidPathSegment) {
			t.Errorf("CheckPathSegment(%q) = %v, want ErrInvalidPathSegment", bad, err)
		}
	}
}

// adminPathRecorder answers the token request and records every Admin API path
// the client asks for, escaped as sent.
func adminPathRecorder(t *testing.T) (*Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
			return
		}
		mu.Lock()
		paths = append(paths, r.URL.EscapedPath())
		mu.Unlock()
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL: srv.URL, AdminRealm: "master", ClientID: "monitoring-service", ClientSecret: "s", HTTPClient: srv.Client(),
	}, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return c, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

// A user ID that climbs out of the requested realm is refused before any
// request, so a caller allowed one realm cannot read a user of another.
func TestGetUserDetails_RefusesAUserIDThatLeavesTheRealm(t *testing.T) {
	c, paths := adminPathRecorder(t)

	_, err := c.GetUserDetails(context.Background(), "demo", "../../master/users/u1")
	if !errors.Is(err, ErrInvalidPathSegment) {
		t.Fatalf("GetUserDetails() error = %v, want ErrInvalidPathSegment", err)
	}
	if got := paths(); len(got) != 0 {
		t.Errorf("Admin API paths requested = %v, want none", got)
	}
}

func TestGetUserDetails_EscapesTheUserID(t *testing.T) {
	c, paths := adminPathRecorder(t)

	if _, err := c.GetUserDetails(context.Background(), "demo", "f:7c8e:j doe"); err != nil {
		t.Fatalf("GetUserDetails() error = %v", err)
	}
	for _, p := range paths() {
		if !strings.HasPrefix(p, "/admin/realms/demo/users/f:7c8e:j%20doe") {
			t.Errorf("path %q, want it under /admin/realms/demo/users/f:7c8e:j%%20doe", p)
		}
	}
	if got := len(paths()); got != 3 {
		t.Errorf("requests = %d, want 3: the user, its groups and its role mappings", got)
	}
}
