package chi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

type realmScopeKeycloakService struct {
	*stubKeycloakService
}

func (s *realmScopeKeycloakService) GetRealm(_ context.Context, tenantID, realm string) (*domain.KeycloakRealmInfo, error) {
	return &domain.KeycloakRealmInfo{TenantID: tenantID, RealmName: realm}, nil
}

func TestKeycloakSingleRealmPolicyShapes(t *testing.T) {
	routes := []string{
		"/keycloak/metrics?realm=realmB",
		"/keycloak/metrics/history?realm=realmB",
		"/keycloak/events?realm=realmB",
		"/keycloak/events/stats?realm=realmB",
		"/keycloak/realms/info?realm=realmB",
		"/keycloak/dashboard?realm=realmB",
		"/keycloak/realms/users?realm=realmB",
		"/keycloak/realms/clients?realm=realmB",
		"/keycloak/realms/users/details?realm=realmB&userId=user-1",
		"/realms/realmB/users/user-1/",
	}
	cases := []struct {
		name      string
		policies  []*domain.TenantPolicy
		admin     bool
		policyErr error
		status    int
	}{
		{name: "no policy", status: http.StatusOK},
		{name: "unrestricted row", policies: []*domain.TenantPolicy{{TenantID: "tenant-a", RealmsUnrestricted: true}}, status: http.StatusOK},
		{name: "allowed list", policies: policyFor("tenant-a", "realmB"), status: http.StatusOK},
		{name: "denied list", policies: policyFor("tenant-a", "realmA"), status: http.StatusForbidden},
		{name: "multiple rows union", policies: append(policyFor("tenant-a", "realmA"), policyFor("tenant-a", "realmB")...), status: http.StatusOK},
		{name: "unrestricted row wins", policies: append(policyFor("tenant-a", "realmA"), &domain.TenantPolicy{TenantID: "tenant-a", RealmsUnrestricted: true}), status: http.StatusOK},
		{name: "blank only", policies: policyFor("tenant-a", " ", ""), status: http.StatusForbidden},
		{name: "explicit empty list", policies: policyFor("tenant-a"), status: http.StatusForbidden},
		{name: "other tenant does not restrict", policies: policyFor("tenant-other", "realmA"), status: http.StatusOK},
		{name: "global admin", admin: true, policies: policyFor("tenant-a", "realmA"), status: http.StatusOK},
		{name: "policy read fails closed", policyErr: errors.New("unavailable"), status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, route := range routes {
				t.Run(route, func(t *testing.T) {
					access := &mockRBACChecker{isAdmin: tc.admin, policies: tc.policies, policiesErr: tc.policyErr}
					service := &realmScopeKeycloakService{stubKeycloakService: &stubKeycloakService{realmMetrics: realmMetrics("realmA", "realmB")}}
					router := setupTestKeycloakRouter(newTestKeycloakHandlers(service, access))
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a"+route, nil))
					if rec.Code != tc.status {
						t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.status, rec.Body.String())
					}
				})
			}
		})
	}
	for _, route := range routes {
		if strings.Contains(route, "realm=realmB") {
			t.Run("blank realm "+route, func(t *testing.T) {
				service := &realmScopeKeycloakService{stubKeycloakService: &stubKeycloakService{}}
				router := setupTestKeycloakRouter(newTestKeycloakHandlers(service, &mockRBACChecker{}))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tenants/tenant-a"+strings.Replace(route, "realm=realmB", "realm=%20", 1), nil))
				if rec.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403", rec.Code)
				}
			})
		}
	}
}
