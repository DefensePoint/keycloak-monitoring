package fx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/fx/fxtest"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/metrics"
)

// An identity provider that is down while the server starts used to leave the
// provider nil, and every SSO login answered "OAuth2 service not configured"
// until the pod was restarted.
func TestProvideOIDCProvider_KeepsTheProviderWhenTheIdPIsDownAtStartup(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no available server", http.StatusServiceUnavailable)
	}))
	defer idp.Close()

	cfg := &config.AppConfig{}
	cfg.Auth.OAuth2.Enabled = true
	cfg.Auth.OAuth2.ProviderURL = idp.URL
	cfg.Auth.OAuth2.ClientID = "kmt"
	cfg.Auth.OAuth2.ClientSecret = "secret"
	cfg.Auth.OAuth2.RedirectURL = "https://kmt.invalid/auth/callback"

	authCfg := provideAuthConfig(cfg)
	provider := provideOIDCProvider(cfg, authCfg, logger.NewNoop())
	if provider == nil {
		t.Fatal("provider is nil after a failed startup discovery; SSO would stay broken until a restart")
	}

	svc := provideAuthService(AuthServiceParams{Provider: provider, Logger: logger.NewNoop(), Config: authCfg})
	if svc == nil {
		t.Fatal("auth service is nil after a failed startup discovery")
	}
}

func TestRegisterOIDCProviderRetry_ReportsSSODownAndStopsCleanly(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no available server", http.StatusServiceUnavailable)
	}))
	defer idp.Close()

	cfg := &config.AppConfig{}
	cfg.Auth.OAuth2.Enabled = true
	cfg.Auth.OAuth2.ProviderURL = idp.URL
	cfg.Auth.OAuth2.ClientID = "kmt"
	cfg.Auth.OAuth2.ClientSecret = "secret"
	cfg.Auth.OAuth2.RedirectURL = "https://kmt.invalid/auth/callback"

	provider := provideOIDCProvider(cfg, provideAuthConfig(cfg), logger.NewNoop())
	reg := metrics.New()
	lc := fxtest.NewLifecycle(t)
	registerOIDCProviderRetry(OIDCProviderRetryParams{Lifecycle: lc, Provider: provider, Metrics: reg})

	if got := testutil.CollectAndCount(reg.SSOProviderUp); got != 1 {
		t.Fatalf("sso_provider_up series = %d, want 1 once SSO is configured", got)
	}
	if got := testutil.ToFloat64(reg.SSOProviderUp.WithLabelValues()); got != 0 {
		t.Fatalf("sso_provider_up = %v with the identity provider down, want 0", got)
	}

	if err := lc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterOIDCProviderRetry_SSODisabledExportsNoGauge(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Auth.OAuth2.ProviderURL = "https://idp.invalid"

	reg := metrics.New()
	registerOIDCProviderRetry(OIDCProviderRetryParams{
		Lifecycle: fxtest.NewLifecycle(t),
		Config:    cfg,
		Provider:  provideOIDCProvider(cfg, provideAuthConfig(cfg), logger.NewNoop()),
		Metrics:   reg,
	})
	if got := testutil.CollectAndCount(reg.SSOProviderUp); got != 0 {
		t.Fatalf("sso_provider_up series = %d with SSO disabled, want 0", got)
	}
}

// These configurations leave no provider, so every login answers "OAuth2
// service not configured"; an absent gauge would never fire the SSO-down alert.
func TestRegisterOIDCProviderRetry_ReportsSSODownWhenNoProviderCouldBeBuilt(t *testing.T) {
	cases := map[string]func(*config.AppConfig){
		"no provider_url":  func(c *config.AppConfig) { c.Auth.OAuth2.ProviderURL = "" },
		"no client_id":     func(c *config.AppConfig) { c.Auth.OAuth2.ClientID = "" },
		"no client_secret": func(c *config.AppConfig) { c.Auth.OAuth2.ClientSecret = "" },
		"no redirect_url":  func(c *config.AppConfig) { c.Auth.OAuth2.RedirectURL = "" },
		"invalid scopes":   func(c *config.AppConfig) { c.Auth.OAuth2.Scopes = []string{"profile"} },
	}
	for name, breakConfig := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &config.AppConfig{}
			cfg.Auth.OAuth2.Enabled = true
			cfg.Auth.OAuth2.ProviderURL = "https://idp.invalid"
			cfg.Auth.OAuth2.ClientID = "kmt"
			cfg.Auth.OAuth2.ClientSecret = "secret"
			cfg.Auth.OAuth2.RedirectURL = "https://kmt.invalid/auth/callback"
			breakConfig(cfg)

			provider := provideOIDCProvider(cfg, provideAuthConfig(cfg), logger.NewNoop())
			if provider != nil {
				t.Fatal("an incomplete configuration produced a provider")
			}

			reg := metrics.New()
			registerOIDCProviderRetry(OIDCProviderRetryParams{
				Lifecycle: fxtest.NewLifecycle(t),
				Config:    cfg,
				Provider:  provider,
				Metrics:   reg,
			})
			if got := testutil.CollectAndCount(reg.SSOProviderUp); got != 1 {
				t.Fatalf("sso_provider_up series = %d with SSO enabled, want 1", got)
			}
			if got := testutil.ToFloat64(reg.SSOProviderUp.WithLabelValues()); got != 0 {
				t.Fatalf("sso_provider_up = %v with no provider, want 0", got)
			}
		})
	}
}

func TestProvideOIDCProvider_InvalidConfigStillDisablesSSO(t *testing.T) {
	cfg := &config.AppConfig{}
	cfg.Auth.OAuth2.Enabled = true
	cfg.Auth.OAuth2.ProviderURL = "https://idp.invalid"
	cfg.Auth.OAuth2.ClientID = "kmt"
	cfg.Auth.OAuth2.ClientSecret = "secret"
	cfg.Auth.OAuth2.RedirectURL = "https://kmt.invalid/auth/callback"
	cfg.Auth.OAuth2.Scopes = []string{"profile"}

	if provider := provideOIDCProvider(cfg, provideAuthConfig(cfg), logger.NewNoop()); provider != nil {
		t.Fatal("a configuration without the openid scope produced a provider")
	}
}
