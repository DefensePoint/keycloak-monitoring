package keycloak

import (
	"context"
	"reflect"
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
		{"prefixed id not configured", []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "custom-adaptive-auth", Requirement: "REQUIRED"}}, false},
	}
	ids := amfaAuthenticatorIDSet(nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := flowUsesAdaptiveAuth(tc.in, ids); got != tc.want {
				t.Errorf("flowUsesAdaptiveAuth = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFlowUsesAdaptiveAuth_ExtraIDs(t *testing.T) {
	ids := amfaAuthenticatorIDSet([]string{"custom-adaptive-auth", " custom-conditional-adaptive-auth ", "", "  "})
	cases := []struct {
		name string
		id   string
		req  string
		want bool
	}{
		{"extra id", "custom-adaptive-auth", "REQUIRED", true},
		{"extra id with surrounding spaces in config", "custom-conditional-adaptive-auth", "CONDITIONAL", true},
		{"extra id disabled", "custom-adaptive-auth", "DISABLED", false},
		{"built-in id still matched", "adaptive-auth", "REQUIRED", true},
		{"blank provider id", "", "REQUIRED", false},
		{"unrelated authenticator", "auth-otp-form", "REQUIRED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: tc.id, Requirement: tc.req}}
			if got := flowUsesAdaptiveAuth(in, ids); got != tc.want {
				t.Errorf("flowUsesAdaptiveAuth(%q, %s) = %v, want %v", tc.id, tc.req, got, tc.want)
			}
		})
	}
}

// A realm whose flow uses an extension's own authenticator ID is only listed
// once that ID is configured.
func TestListAmfaEnabledRealms_ExtraIDs(t *testing.T) {
	mock := newMockAdminAPI()
	mock.GetAllRealmsFunc = func(context.Context) ([]*keycloakadmin.RealmRepresentation, error) {
		return []*keycloakadmin.RealmRepresentation{{Realm: "extension"}, {Realm: "plugin"}, {Realm: "master"}}, nil
	}
	mock.GetAuthFlowsFunc = func(context.Context, string) ([]keycloakadmin.AuthenticationFlowRepresentation, error) {
		return []keycloakadmin.AuthenticationFlowRepresentation{{Alias: "browser", TopLevel: true}}, nil
	}
	mock.GetFlowExecutionsFunc = func(_ context.Context, realmName, _ string) ([]keycloakadmin.AuthenticationExecutionInfoRepresentation, error) {
		switch realmName {
		case "extension":
			return []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "custom-conditional-adaptive-auth", Requirement: "CONDITIONAL"}}, nil
		case "plugin":
			return []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "adaptive-auth", Requirement: "REQUIRED"}}, nil
		}
		return []keycloakadmin.AuthenticationExecutionInfoRepresentation{{ProviderID: "auth-username-password-form", Requirement: "REQUIRED"}}, nil
	}

	for _, tc := range []struct {
		name  string
		extra []string
		want  []string
	}{
		{"built-in ids only", nil, []string{"plugin"}},
		{"with the extension's ids", []string{"custom-adaptive-auth", "custom-conditional-adaptive-auth"}, []string{"extension", "plugin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil, nil, nil, nil, fakeClientProvider{client: mock}, nil, tc.extra)
			got, err := svc.ListAmfaEnabledRealms(context.Background(), "t1")
			if err != nil {
				t.Fatalf("ListAmfaEnabledRealms: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
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

	svc := NewService(nil, nil, nil, nil, fakeClientProvider{client: mock}, nil, nil)

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
	svc := NewService(nil, nil, nil, nil, nil, nil, nil)
	if _, err := svc.ListAmfaEnabledRealms(context.Background(), "t1"); err == nil {
		t.Error("expected error when no client provider is configured")
	}
}
