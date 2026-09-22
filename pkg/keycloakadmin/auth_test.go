package keycloakadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// noopLogger satisfies the Logger interface without producing output.
type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Debug(string, ...any) {}

func TestClientConfig_Validate(t *testing.T) {
	base := func() *ClientConfig {
		return &ClientConfig{
			ServerURL:    "https://kc.example.com",
			AdminRealm:   "master",
			ClientID:     "monitoring-service",
			ClientSecret: "shh",
		}
	}

	tests := []struct {
		name    string
		mutate  func(*ClientConfig)
		wantErr error
	}{
		{"valid", func(*ClientConfig) {}, nil},
		{"missing server url", func(c *ClientConfig) { c.ServerURL = "" }, ErrServerURLRequired},
		{"missing admin realm", func(c *ClientConfig) { c.AdminRealm = "" }, ErrAdminRealmRequired},
		{"missing client id", func(c *ClientConfig) { c.ClientID = "" }, ErrClientIDRequired},
		{"missing client secret", func(c *ClientConfig) { c.ClientSecret = "" }, ErrClientSecretRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(cfg)
			if err := cfg.Validate(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthenticate_UsesClientCredentials(t *testing.T) {
	var (
		mu   sync.Mutex
		form url.Values
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
			http.NotFound(w, r)
			return
		}
		_ = r.ParseForm()
		mu.Lock()
		form = r.Form
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 300})
	}))
	defer srv.Close()

	cfg := &ClientConfig{
		ServerURL:    srv.URL,
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: "shh",
		HTTPClient:   srv.Client(),
	}

	// NewClient authenticates during construction.
	if _, err := NewClient(context.Background(), cfg, noopLogger{}); err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if got := form.Get("grant_type"); got != "client_credentials" {
		t.Errorf("grant_type = %q, want client_credentials", got)
	}
	if got := form.Get("client_id"); got != "monitoring-service" {
		t.Errorf("client_id = %q, want monitoring-service", got)
	}
	if got := form.Get("client_secret"); got != "shh" {
		t.Errorf("client_secret = %q, want shh", got)
	}
	if form.Has("username") || form.Has("password") {
		t.Errorf("client_credentials request must not carry username/password; got username=%q password=%q",
			form.Get("username"), form.Get("password"))
	}
}
