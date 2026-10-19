package chi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// A caller allowed only realmA asks for realmA, but with a user ID that climbs
// into realmB. Keycloak would resolve it to realmB's user, so it must be refused
// before the service is called.
func TestKeycloakHandlers_UserDetails_RefuseAUserIDThatLeavesTheRealm(t *testing.T) {
	for _, path := range []string{
		"/api/tenants/tenant-a/keycloak/realms/users/details?realm=realmA&userId=../../realmB/users/u1",
		"/api/tenants/tenant-a/keycloak/realms/users/details?realm=realmA&userId=..%2F..%2FrealmB%2Fusers%2Fu1",
		"/api/tenants/tenant-a/realms/realmA/users/..%2F..%2FrealmB%2Fusers%2Fu1/",
	} {
		t.Run(path, func(t *testing.T) {
			svc := &stubKeycloakService{}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, systemRoleCaller(t, rbac.RoleOperator)))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code == http.StatusOK {
				t.Errorf("status = 200, want the request refused")
			}
			if svc.userDetailsCalls != 0 {
				t.Errorf("GetUserDetails called %d times, want 0", svc.userDetailsCalls)
			}
		})
	}
}

func TestKeycloakHandlers_UserDetails_AllowAPlainUserID(t *testing.T) {
	for _, path := range []string{
		"/api/tenants/tenant-a/keycloak/realms/users/details?realm=realmA&userId=f:7c8e:jdoe",
		"/api/tenants/tenant-a/realms/realmA/users/a1b2c3d4-0000-4000-8000-000000000000/",
	} {
		t.Run(path, func(t *testing.T) {
			svc := &stubKeycloakService{}
			router := setupTestKeycloakRouter(newTestKeycloakHandlers(svc, systemRoleCaller(t, rbac.RoleOperator)))

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusOK || svc.userDetailsCalls != 1 {
				t.Errorf("status = %d with %d service calls, want 200 and 1", rec.Code, svc.userDetailsCalls)
			}
		})
	}
}
