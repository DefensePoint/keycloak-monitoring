package fx

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx/fxtest"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/tenant"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// unmatched returns the entries the log has warned about, as tenant/provider.
func (b *syncBuffer) unmatched() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, line := range strings.Split(b.buf.String(), "\n") {
		if !strings.Contains(line, "secrets entry names no enabled tenant") {
			continue
		}
		tenantAt := strings.Index(line, `"tenant":"`)
		idpAt := strings.Index(line, `"identity_provider":"`)
		if tenantAt < 0 || idpAt < 0 {
			continue
		}
		t := line[tenantAt+len(`"tenant":"`):]
		i := line[idpAt+len(`"identity_provider":"`):]
		out = append(out, t[:strings.Index(t, `"`)]+"/"+i[:strings.Index(i, `"`)])
	}
	return out
}

// viper lower-cases the tenant IDs of config.yaml, so a tenant written Prod:
// there has the ID prod, while an entry's tenant keeps the case it was
// written in. They must still match.
func TestBuildConfigChecks_SecretEntryTenantIgnoresCase(t *testing.T) {
	var cfg config.ConfigCheckerConfig
	cfg.Checks.IdentityProviderSecretExpiry.Enabled = true
	cfg.Checks.IdentityProviderSecretExpiry.WarningDays = 30
	cfg.Checks.IdentityProviderSecretExpiry.ErrorDays = 7
	cfg.Checks.IdentityProviderSecretExpiry.Secrets = []config.IdentityProviderSecret{
		{Tenant: "Prod", Realm: "customers", IdentityProvider: "entra", ExpiresOn: "2020-03-31"},
	}
	kc := secretStubClient{stubConfigCheckClient{realms: []*keycloakadmin.RealmRepresentation{{ID: "r", Realm: "customers", Enabled: true}}}}
	checks := buildConfigChecks(cfg, []*domain.KeycloakTenant{{TenantID: "prod"}},
		func(string) configcheck.KeycloakClient { return kc }, saferequest.Policy{}, logger.NewNoop())

	alerts, err := checks[0].Execute(t.Context())
	if err != nil || len(alerts) != 1 || alerts[0].ResourceName != "entra" {
		t.Fatalf("alerts %+v, err %v; want the expired alert of the entry written Prod", alerts, err)
	}
}

// An entry whose tenant is not one the checker runs for is not watched: a typo
// used to drop it without a word. It is logged once, when the tenants are
// known, and again if it matched a tenant for a while and then stops.
func TestConfigCheckWorkers_WarnAboutSecretEntriesOfNoTenant(t *testing.T) {
	var cfg config.ConfigCheckerConfig
	cfg.Enabled = true
	cfg.PollInterval = time.Hour
	cfg.Checks.IdentityProviderSecretExpiry.Enabled = true
	cfg.Checks.IdentityProviderSecretExpiry.WarningDays = 30
	cfg.Checks.IdentityProviderSecretExpiry.ErrorDays = 7
	cfg.Checks.IdentityProviderSecretExpiry.Secrets = []config.IdentityProviderSecret{
		{Tenant: "Prod", Realm: "customers", IdentityProvider: "entra", ExpiresOn: "2030-03-31"},
		{Tenant: "prdo", Realm: "customers", IdentityProvider: "okta", ExpiresOn: "2030-03-31"},
		{Tenant: "later", Realm: "staff", IdentityProvider: "google", ExpiresOn: "2030-03-31"},
	}
	var out syncBuffer
	w := newConfigCheckWorkers(cfg, func(string) configcheck.KeycloakClient { return nil }, saferequest.Policy{},
		ecFakeAlertStore{}, ecFakeNotifier{}, nil, logger.New(zerolog.New(&out)))
	tenants := &fakeAmfaTenantService{tenants: []*domain.KeycloakTenant{enabledTenant("prod")}}
	lc := fxtest.NewLifecycle(t)
	w.hook(lc, tenants)
	lc.RequireStart()
	t.Cleanup(func() { lc.RequireStop() })

	if got := strings.Join(out.unmatched(), ","); got != "prdo/okta,later/google" {
		t.Fatalf("at startup warned about %q, want the typo and the tenant not yet created", got)
	}

	// The tenant is created in the interface: its entry is watched now. An
	// unrelated change does not repeat the warnings.
	tenants.callback(tenant.EventAdded, enabledTenant("later"))
	tenants.callback(tenant.EventUpdated, enabledTenant("prod"))
	if got := strings.Join(out.unmatched(), ","); got != "prdo/okta,later/google" {
		t.Fatalf("after changes warned about %q, want no new warning", got)
	}

	// Disabled: its entry is no longer watched, and that is said again.
	disabled := enabledTenant("later")
	disabled.Enabled = false
	tenants.callback(tenant.EventDisabled, disabled)
	if got := strings.Join(out.unmatched(), ","); got != "prdo/okta,later/google,later/google" {
		t.Fatalf("after disabling warned about %q, want later/google again", got)
	}
}
