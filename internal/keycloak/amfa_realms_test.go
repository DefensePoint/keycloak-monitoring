package keycloak

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

// fakeClientProvider hands out a fixed AdminAPI for any tenant.
type fakeClientProvider struct{ client AdminAPI }

func (f fakeClientProvider) GetClient(string) AdminAPI { return f.client }

func TestFlowUsesAdaptiveAuth(t *testing.T) {
	cases := []struct {
		name string
		in   []keycloakadmin.AuthenticationExecutionInfoRepresentation
		want bool
	}{
		{"adaptive required", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "adaptive-auth", Requirement: "REQUIRED"}}, true},
		{"conditional alternative", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "conditional-adaptive-auth", Requirement: "ALTERNATIVE"}}, true},
		{"adaptive disabled", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "adaptive-auth", Requirement: "DISABLED"}}, false},
		{"unrelated authenticator", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "auth-otp-form", Requirement: "REQUIRED"}}, false},
		{"non-adaptive only", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "auth-username-password-form", Requirement: "REQUIRED"}}, false},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := flowUsesAdaptiveAuth(tc.in); got != tc.want {
				t.Errorf("flowUsesAdaptiveAuth = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestListAmfaEnabledRealms(t *testing.T) {
	mock := newMockAdminAPI()
	mock.GetAllRealmsFunc = func(context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
		return []*keycloakadmin.RealmRepresentation{
			{Realm: "AdaptiveAuth"},
			{Realm: "master"},
		}, nil
	}
	// AdaptiveAuth has a custom "hybrid-flow" carrying the adaptive-auth
	// authenticator (AMFA is bound per-client, not as the realm browser flow);
	// master only has the default flows.
	mock.GetAuthFlowsFunc = func(_ context.Context, realmName string) ([]keycloakadmin.AuthenticationFlowRepresentation, error) {
		flows := []keycloakadmin.AuthenticationFlowRepresentation{
			{Alias: "browser", TopLevel: true, BuiltIn: true},
		}
		if realmName == "AdaptiveAuth" {
			flows = append(flows, keycloakadmin.AuthenticationFlowRepresentation{Alias: "hybrid-flow", TopLevel: true})
		}
		return flows, nil
	}
	mock.GetFlowExecutionsFunc = func(_ context.Context, realmName, flowAlias string) ([]keycloakadmin.AuthenticationExecutionInfoRepresentation, error) {
		if realmName == "AdaptiveAuth" && flowAlias == "hybrid-flow" {
			return []keycloakadmin.AuthenticationExecutionInfoRepresentation{
				{ProviderID: "adaptive-auth", Requirement: "REQUIRED"},
			}, nil
		}
		return []keycloakadmin.AuthenticationExecutionInfoRepresentation{
			{ProviderID: "auth-username-password-form", Requirement: "REQUIRED"},
		}, nil
	}

	svc := NewService(nil, nil, nil, nil, fakeClientProvider{client: mock}, nil)

	got, err := svc.ListAmfaEnabledRealms(context.Background(), "t1")
	if err != nil {
		t.Fatalf("ListAmfaEnabledRealms: %v", err)
	}
	if len(got) != 1 || got[0] != "AdaptiveAuth" {
		t.Fatalf("got %v, want [AdaptiveAuth]", got)
	}

	// A second call within the TTL must be served from cache (no extra walk).
	if _, err := svc.ListAmfaEnabledRealms(context.Background(), "t1"); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if n := mock.getCalls("GetAllRealms"); n != 1 {
		t.Errorf("GetAllRealms called %d times, want 1 (second call should be cached)", n)
	}
}

func TestListAmfaEnabledRealms_NoClientProvider(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	if _, err := svc.ListAmfaEnabledRealms(context.Background(), "t1"); err == nil {
		t.Error("expected error when no client provider is configured")
	}
}
