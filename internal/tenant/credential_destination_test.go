package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// The stored secret is sent to server_url for tokens, and the tokens go to the
// AMFA API URL. An update that moves either to another server without the
// secret would hand the secret, or a token made with it, to that server.

const storedTenantSecret = "stored-secret"

func credentialTestService(t *testing.T, withAmfa bool) (Service, *[]*domain.KeycloakTenant) {
	t.Helper()
	var persisted []*domain.KeycloakTenant
	repo := &mockRepository{
		updateFn: func(ctx context.Context, tenant *domain.KeycloakTenant) error {
			cp := *tenant
			persisted = append(persisted, &cp)
			return nil
		},
		getByTenantIDFn: func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			return nil, errors.New("not found")
		},
	}
	svc := NewService(repo)
	stored := &domain.KeycloakTenant{
		TenantID:     "tenant-1",
		Name:         "Original",
		Enabled:      true,
		ServerURL:    "https://kc.example.com",
		AdminRealm:   "master",
		ClientID:     "monitoring-service",
		ClientSecret: storedTenantSecret,
	}
	if withAmfa {
		stored.Amfa = &domain.TenantAmfa{Enabled: true, APIBaseURL: "https://amfa.example.com/api", EventsLookbackDays: 30, APITimeoutSeconds: 10}
	}
	svc.(*service).cache.Store("tenant-1", stored)
	return svc, &persisted
}

func amfaTo(base string) *AmfaRequest {
	return &AmfaRequest{Enabled: true, APIBaseURL: base, EventsLookbackDays: 30, APITimeoutSeconds: 10}
}

func TestUpdateTenant_NewDestinationWithoutSecretIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name     string
		withAmfa bool
		req      *UpdateRequest
	}{
		{"server_url to another host", true, &UpdateRequest{ServerURL: strPtr("https://evil.example")}},
		{"server_url to another host, form sends an empty secret", true, &UpdateRequest{ServerURL: strPtr("https://evil.example"), ClientSecret: strPtr("")}},
		{"server_url https down to http", true, &UpdateRequest{ServerURL: strPtr("http://kc.example.com")}},
		{"server_url to another port", true, &UpdateRequest{ServerURL: strPtr("https://kc.example.com:8443")}},
		{"server_url to a subdomain", true, &UpdateRequest{ServerURL: strPtr("https://kc.example.com.evil.example")}},
		{"server_url without a scheme", true, &UpdateRequest{ServerURL: strPtr("kc.example.com")}},
		{"AMFA API URL to another host", true, &UpdateRequest{Amfa: amfaTo("https://evil.example/api")}},
		{"AMFA API URL set where there was none", false, &UpdateRequest{Amfa: amfaTo("https://amfa.example.com/api")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, persisted := credentialTestService(t, tc.withAmfa)
			_, err := svc.UpdateTenant(context.Background(), "tenant-1", tc.req)
			if !errors.Is(err, ErrClientSecretRequired) {
				t.Fatalf("err = %v, want ErrClientSecretRequired", err)
			}
			if len(*persisted) != 0 {
				t.Fatalf("the tenant was saved: %+v", (*persisted)[0])
			}
			got, _ := svc.GetTenant(context.Background(), "tenant-1")
			if got.ServerURL != "https://kc.example.com" || got.ClientSecret != storedTenantSecret {
				t.Fatalf("cached tenant changed: server_url=%q secret=%q", got.ServerURL, got.ClientSecret)
			}
		})
	}
}

func TestUpdateTenant_SameServerNeedsNoSecret(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  *UpdateRequest
	}{
		{"unrelated edit", &UpdateRequest{Name: strPtr("Renamed")}},
		{"server_url sent unchanged", &UpdateRequest{ServerURL: strPtr("https://kc.example.com")}},
		{"server_url gains a path", &UpdateRequest{ServerURL: strPtr("https://kc.example.com/auth")}},
		{"server_url trailing slash", &UpdateRequest{ServerURL: strPtr("https://kc.example.com/")}},
		{"server_url default port written out", &UpdateRequest{ServerURL: strPtr("https://kc.example.com:443")}},
		{"server_url host in capitals", &UpdateRequest{ServerURL: strPtr("https://KC.Example.com")}},
		{"AMFA API URL path change", &UpdateRequest{Amfa: amfaTo("https://amfa.example.com/v2")}},
		{"AMFA API URL cleared", &UpdateRequest{Amfa: &AmfaRequest{Enabled: false, APIBaseURL: ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, persisted := credentialTestService(t, true)
			if _, err := svc.UpdateTenant(context.Background(), "tenant-1", tc.req); err != nil {
				t.Fatalf("UpdateTenant: %v", err)
			}
			if len(*persisted) != 1 || (*persisted)[0].ClientSecret != storedTenantSecret {
				t.Fatalf("persisted = %+v, want one save keeping the stored secret", *persisted)
			}
		})
	}
}

func TestUpdateTenant_NewDestinationWithSecretIsSaved(t *testing.T) {
	svc, persisted := credentialTestService(t, true)
	req := &UpdateRequest{
		ServerURL:    strPtr("https://kc2.example.org"),
		ClientSecret: strPtr("new-secret"),
		Amfa:         amfaTo("https://amfa2.example.org/api"),
	}
	if _, err := svc.UpdateTenant(context.Background(), "tenant-1", req); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}
	if len(*persisted) != 1 {
		t.Fatalf("persisted %d times, want 1", len(*persisted))
	}
	got := (*persisted)[0]
	if got.ServerURL != "https://kc2.example.org" || got.ClientSecret != "new-secret" || got.Amfa.APIBaseURL != "https://amfa2.example.org/api" {
		t.Fatalf("persisted %+v", got)
	}
}
