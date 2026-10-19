package chi

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
)

var testClientCredentials = map[string]string{
	"client secret":          "client-secret-6f1d0c2e9a8b4d7f1c3e5a7b9d0f2a4c",
	"rotated secret":         "rotated-secret-3b5e7a9c1d2f4a6b8c0e",
	"signing private key":    "signing-private-key-MIIEvQIBADANBgkq",
	"encryption private key": "encryption-private-key-MIIEvgIBADAN",
	"custom attribute token": "webhook-token-9c1e3a5b7d",
	"mapper config key":      "mapper-api-key-2f4a6c8e0b",
}

func confidentialAndPublicClients() []*keycloakadmin.ClientRepresentation {
	return []*keycloakadmin.ClientRepresentation{
		{
			ID:                      "c-monitoring",
			ClientID:                "monitoring-service",
			Enabled:                 true,
			ClientAuthenticatorType: "client-secret",
			ServiceAccountsEnabled:  true,
			Protocol:                "openid-connect",
			Secret:                  testClientCredentials["client secret"],
			Attributes: map[string]string{
				"client.secret.creation.time": "1759300000",
				"client.secret.rotated":       testClientCredentials["rotated secret"],
				"saml.signing.private.key":    testClientCredentials["signing private key"],
				"saml.encryption.private.key": testClientCredentials["encryption private key"],
				"webhook.token":               testClientCredentials["custom attribute token"],
				"post.logout.redirect.uris":   "+",
			},
			ProtocolMappers: []interface{}{map[string]any{
				"name":   "partner-api",
				"config": map[string]any{"api.key": testClientCredentials["mapper config key"]},
			}},
		},
		{ID: "c-web", ClientID: "web-app", Enabled: true, PublicClient: true, Protocol: "openid-connect"},
	}
}

// systemRoleCaller answers RBAC checks as rbac.Service does for a caller holding
// one system role on tenant-a, with a realm policy narrowed to realmA.
func systemRoleCaller(t *testing.T, role string) *mockRBACChecker {
	t.Helper()
	for _, def := range rbac.GetSystemRoles() {
		if def.Name == role {
			return &mockRBACChecker{
				isAdmin: role == rbac.RoleAdmin,
				hasPermissionFn: func(_ context.Context, _ uint, permission string, _ *string) (bool, error) {
					return slices.Contains(def.Permissions, permission), nil
				},
				policies: policyFor("tenant-a", "realmA"),
			}
		}
	}
	t.Fatalf("no system role %q", role)
	return nil
}

func TestKeycloakHandlers_Clients_NeverReturnSecrets(t *testing.T) {
	for _, role := range []string{rbac.RoleAdmin, rbac.RoleOperator, rbac.RoleViewer} {
		t.Run(role, func(t *testing.T) {
			svc := &stubKeycloakService{clients: confidentialAndPublicClients()}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, systemRoleCaller(t, role)))

			req := httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a/keycloak/realms/clients?realm=realmA", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			body := rec.Body.String()
			for name, value := range testClientCredentials {
				if strings.Contains(body, value) {
					t.Errorf("response body carries the %s", name)
				}
			}
			if strings.Contains(body, `"secret"`) {
				t.Errorf(`response body has a "secret" key`)
			}

			var resp struct {
				Clients []map[string]json.RawMessage `json:"clients"`
				Count   int                          `json:"count"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Count != 2 || len(resp.Clients) != 2 {
				t.Fatalf("count = %d with %d clients, want 2 and 2", resp.Count, len(resp.Clients))
			}

			for i, raw := range confidentialAndPublicClients() {
				encoded, err := json.Marshal(raw)
				if err != nil {
					t.Fatalf("marshal representation: %v", err)
				}
				var wantFields map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &wantFields); err != nil {
					t.Fatalf("unmarshal representation: %v", err)
				}
				delete(wantFields, "secret")
				delete(wantFields, "attributes")
				delete(wantFields, "protocolMappers")
				gotKeys := slices.Sorted(maps.Keys(resp.Clients[i]))
				if wantKeys := slices.Sorted(maps.Keys(wantFields)); !slices.Equal(gotKeys, wantKeys) {
					t.Errorf("client %d fields = %v, want %v", i, gotKeys, wantKeys)
				}
			}
		})
	}
}
