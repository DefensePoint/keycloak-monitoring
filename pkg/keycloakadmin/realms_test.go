package keycloakadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A missing realm is ErrNotFound, so callers can tell a deleted realm from one
// the client may not read.
func TestGetRealmInfo_OnlyAMissingRealmIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "t", "expires_in": 300})
		case strings.HasSuffix(r.URL.Path, "/admin/realms/gone"):
			http.Error(w, `{"error":"Realm not found."}`, http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/admin/realms/hidden"):
			http.Error(w, `{"error":"HTTP 403 Forbidden"}`, http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), &ClientConfig{
		ServerURL:    srv.URL,
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "s",
		HTTPClient:   srv.Client(),
	}, noopLogger{})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := c.GetRealmInfo(context.Background(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing realm: err = %v, want ErrNotFound", err)
	}
	if _, err := c.GetRealmInfo(context.Background(), "hidden"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("forbidden realm: err = %v, want an error that is not ErrNotFound", err)
	}
}
