package fx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/fx/fxtest"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/configcheck"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

type stubConfigCheckClient struct {
	realms []*keycloakadmin.RealmRepresentation
}

func (s stubConfigCheckClient) GetAllRealms(context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
	return s.realms, nil
}
func (stubConfigCheckClient) GetRealmInfo(context.Context, string) (*keycloakadmin.RealmRepresentation, error) {
	return nil, nil
}
func (stubConfigCheckClient) GetClients(context.Context, string) ([]*keycloakadmin.ClientRepresentation, error) {
	return nil, nil
}
func (stubConfigCheckClient) GetIdentityProviders(context.Context, string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	return nil, nil
}

func TestBuildConfigChecks(t *testing.T) {
	tenants := []*domain.KeycloakTenant{{TenantID: "a"}, {TenantID: "b"}}
	noClient := func(string) configcheck.KeycloakClient { return nil }

	for _, tc := range []struct {
		name               string
		idp, realm, client bool
		want               []string
	}{
		{name: "all checks", idp: true, realm: true, client: true,
			want: []string{"identity_provider_metadata", "realm_security", "client_security"}},
		{name: "only identity providers", idp: true, want: []string{"identity_provider_metadata"}},
		{name: "none", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg config.ConfigCheckerConfig
			cfg.Checks.IdentityProviderMetadata.Enabled = tc.idp
			cfg.Checks.RealmSecurity.Enabled = tc.realm
			cfg.Checks.ClientSecurity.Enabled = tc.client

			checks := buildConfigChecks(cfg, tenants, noClient, saferequest.Policy{}, logger.NewNoop())
			if len(checks) != len(tc.want)*len(tenants) {
				t.Fatalf("got %d checks, want %d", len(checks), len(tc.want)*len(tenants))
			}
			for i, check := range checks {
				if got := check.GetCheckType(); got != tc.want[i%len(tc.want)] {
					t.Fatalf("check %d type %q, want %q", i, got, tc.want[i%len(tc.want)])
				}
			}
		})
	}
}

func TestLateKeycloakClient(t *testing.T) {
	var current configcheck.KeycloakClient
	kc := lateKeycloakClient{tenantID: "a", lookup: func(tenantID string) configcheck.KeycloakClient {
		if tenantID != "a" {
			t.Fatalf("looked up tenant %q, want a", tenantID)
		}
		return current
	}}

	if _, err := kc.GetAllRealms(context.Background()); !errors.Is(err, keycloak.ErrClientNotAvailable) {
		t.Fatalf("before the pool has a client: err = %v, want ErrClientNotAvailable", err)
	}

	current = stubConfigCheckClient{realms: []*keycloakadmin.RealmRepresentation{{Realm: "r1"}}}
	realms, err := kc.GetAllRealms(context.Background())
	if err != nil || len(realms) != 1 || realms[0].Realm != "r1" {
		t.Fatalf("after the pool has a client: realms %v, err %v", realms, err)
	}
}

func TestRegisterConfigCheckHooks_DisabledRegistersNothing(t *testing.T) {
	lc := fxtest.NewLifecycle(t)
	cfg := &config.AppConfig{}
	cfg.Keycloak.Global.ConfigChecker.Checks.IdentityProviderMetadata.Enabled = true

	registerConfigCheckHooks(ConfigCheckParams{Lifecycle: lc, Config: cfg, Logger: logger.NewNoop()})

	// Tenants and Pool are nil, so a registered hook would panic on start.
	lc.RequireStart().RequireStop()
}

type samlStubClient struct {
	stubConfigCheckClient
	metadataURL string
}

func (s samlStubClient) GetIdentityProviders(context.Context, string) ([]*keycloakadmin.IdentityProviderRepresentation, error) {
	return []*keycloakadmin.IdentityProviderRepresentation{{
		Alias: "corp", ProviderID: "saml", InternalID: "idp-1",
		Config: map[string]string{"metadataDescriptorUrl": s.metadataURL},
	}}, nil
}

func TestBuildConfigChecks_MetadataFetchIsGuarded(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	var cfg config.ConfigCheckerConfig
	cfg.Checks.IdentityProviderMetadata.Enabled = true
	kc := samlStubClient{
		stubConfigCheckClient: stubConfigCheckClient{realms: []*keycloakadmin.RealmRepresentation{{ID: "r1-id", Realm: "r1", Enabled: true}}},
		metadataURL:           srv.URL,
	}
	appCfg := &config.AppConfig{}
	appCfg.Keycloak.Global.Connection.AllowPrivateRanges = true
	checks := buildConfigChecks(cfg, []*domain.KeycloakTenant{{TenantID: "a"}},
		func(string) configcheck.KeycloakClient { return kc }, keycloakSafeRequestPolicy(appCfg), logger.NewNoop())

	alerts, err := checks[0].Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the metadata fetch reached a loopback server")
	}
	if len(alerts) != 1 || !strings.Contains(alerts[0].Title, "metadata fetch failed") {
		t.Fatalf("alerts = %+v, want one metadata fetch failure", alerts)
	}
}
