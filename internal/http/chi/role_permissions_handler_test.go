package chi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// Role create and update used to log a failed permission save and still
// answer success, and update only ever added permissions.

type roleCall struct {
	created   []string
	active    *bool
	set       *[]string
	updated   *domain.Role
	setErr    error
	createErr error
}

type fakeRoleService struct {
	rbac.Service
	role  domain.Role
	calls roleCall
}

func (f *fakeRoleService) CreateRoleWithPermissions(_ context.Context, name, displayName, description string, perms []string, active bool) (*domain.Role, error) {
	f.calls.created = perms
	f.calls.active = &active
	if f.calls.createErr != nil {
		return nil, f.calls.createErr
	}
	return &domain.Role{ID: 9, Name: name, DisplayName: displayName}, nil
}

func (f *fakeRoleService) GetRoleWithPermissions(context.Context, uint) (*domain.Role, error) {
	r := f.role
	return &r, nil
}

func (f *fakeRoleService) SetRolePermissions(_ context.Context, _ uint, perms []string) error {
	f.calls.set = &perms
	return f.calls.setErr
}

func (f *fakeRoleService) UpdateRole(_ context.Context, role *domain.Role) error {
	r := *role
	f.calls.updated = &r
	f.role = r
	return nil
}

func roleRouter(svc *fakeRoleService) http.Handler {
	h := &RBACHandlers{service: svc, log: logger.NewNoop()}
	r := chi.NewRouter()
	r.Post("/roles", h.createRole)
	r.Put("/roles/{roleID}", h.updateRole)
	return r
}

func sendRole(t *testing.T, svc *fakeRoleService, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	roleRouter(svc).ServeHTTP(rec, req)
	return rec
}

func customRole() domain.Role {
	return domain.Role{ID: 4, Name: "ack_only", DisplayName: "Ack Only", Description: "keep me"}
}

func TestCreateRole_PassesThePermissionNames(t *testing.T) {
	svc := &fakeRoleService{}
	rec := sendRole(t, svc, http.MethodPost, "/roles",
		`{"name":"ack_only","display_name":"Ack Only","description":"d","permissions":["alerts:read","alerts:acknowledge"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d (%s)", rec.Code, rec.Body)
	}
	if fmt.Sprint(svc.calls.created) != "[alerts:read alerts:acknowledge]" {
		t.Fatalf("service got %v", svc.calls.created)
	}
}

func TestCreateRole_UnknownPermissionIs400(t *testing.T) {
	svc := &fakeRoleService{calls: roleCall{createErr: fmt.Errorf("%w: %q", rbac.ErrUnknownPermission, "alerts:nope")}}
	rec := sendRole(t, svc, http.MethodPost, "/roles", `{"name":"x","display_name":"X","permissions":["alerts:nope"]}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "alerts:nope") {
		t.Fatalf("status %d body %s, want 400 naming the permission", rec.Code, rec.Body)
	}
}

func TestCreateRole_SaveFailureIs500(t *testing.T) {
	svc := &fakeRoleService{calls: roleCall{createErr: fmt.Errorf("db down")}}
	rec := sendRole(t, svc, http.MethodPost, "/roles", `{"name":"x","display_name":"X","permissions":["alerts:read"]}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
}

func TestUpdateRole_PermissionListReplaces(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantSet string // "" means SetRolePermissions must not be called
	}{
		{"list given", `{"permissions":["alerts:read"]}`, "[alerts:read]"},
		{"empty list removes all", `{"permissions":[]}`, "[]"},
		{"list left out", `{"display_name":"Renamed"}`, ""},
		{"null list", `{"permissions":null,"display_name":"Renamed"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeRoleService{role: customRole()}
			rec := sendRole(t, svc, http.MethodPut, "/roles/4", tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d (%s)", rec.Code, rec.Body)
			}
			got := ""
			if svc.calls.set != nil {
				got = fmt.Sprint(*svc.calls.set)
			}
			if got != tc.wantSet {
				t.Fatalf("SetRolePermissions got %q, want %q", got, tc.wantSet)
			}
		})
	}
}

// Sending only permissions must not blank the name or description.
func TestUpdateRole_FieldsLeftOutAreKept(t *testing.T) {
	svc := &fakeRoleService{role: customRole()}
	rec := sendRole(t, svc, http.MethodPut, "/roles/4", `{"permissions":["alerts:read"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if svc.calls.updated != nil {
		t.Fatalf("role fields rewritten: %+v", svc.calls.updated)
	}

	svc = &fakeRoleService{role: customRole()}
	sendRole(t, svc, http.MethodPut, "/roles/4", `{"display_name":"Renamed"}`)
	if svc.calls.updated == nil || svc.calls.updated.DisplayName != "Renamed" || svc.calls.updated.Description != "keep me" {
		t.Fatalf("updated = %+v, want new name and the old description", svc.calls.updated)
	}
}

func TestUpdateRole_UnknownPermissionChangesNothing(t *testing.T) {
	svc := &fakeRoleService{role: customRole(),
		calls: roleCall{setErr: fmt.Errorf("%w: %q", rbac.ErrUnknownPermission, "alerts:nope")}}
	rec := sendRole(t, svc, http.MethodPut, "/roles/4", `{"display_name":"Renamed","permissions":["alerts:nope"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	if svc.calls.updated != nil {
		t.Fatal("the role was renamed by a rejected request")
	}
}

func TestUpdateRole_SaveFailureIs500(t *testing.T) {
	svc := &fakeRoleService{role: customRole(), calls: roleCall{setErr: fmt.Errorf("db down")}}
	rec := sendRole(t, svc, http.MethodPut, "/roles/4", `{"permissions":["alerts:read"]}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
}

func TestUpdateRole_SystemRoleIsRefused(t *testing.T) {
	role := customRole()
	role.IsSystem = true
	svc := &fakeRoleService{role: role}
	rec := sendRole(t, svc, http.MethodPut, "/roles/4", `{"permissions":[]}`)
	if rec.Code != http.StatusForbidden || svc.calls.set != nil {
		t.Fatalf("status %d, set called = %v; want 403 and no change", rec.Code, svc.calls.set != nil)
	}
}

// A tab still running the old UI sends permission_ids. Ignoring it saved an
// empty role and answered 201, so it is refused and nothing is written.
func TestRoleRequests_PermissionIDsAreRefused(t *testing.T) {
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/roles", `{"name":"x","display_name":"X","permission_ids":[1,2]}`},
		{http.MethodPost, "/roles", `{"name":"x","display_name":"X","permissions":["alerts:read"],"permission_ids":[]}`},
		{http.MethodPut, "/roles/4", `{"permission_ids":[1]}`},
	} {
		svc := &fakeRoleService{role: customRole()}
		rec := sendRole(t, svc, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "permission names") {
			t.Errorf("%s %s: status %d body %s, want 400 asking for names", tc.method, tc.body, rec.Code, rec.Body)
		}
		if svc.calls.created != nil || svc.calls.set != nil || svc.calls.updated != nil {
			t.Errorf("%s %s: the service was called", tc.method, tc.body)
		}
	}
}

func TestCreateRole_ActiveFlag(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"name":"x","display_name":"X"}`, true},
		{`{"name":"x","display_name":"X","is_active":true}`, true},
		{`{"name":"x","display_name":"X","is_active":false}`, false},
	} {
		svc := &fakeRoleService{}
		rec := sendRole(t, svc, http.MethodPost, "/roles", tc.body)
		if rec.Code != http.StatusCreated || svc.calls.active == nil || *svc.calls.active != tc.want {
			t.Fatalf("%s: status %d, active %v; want 201 and %v", tc.body, rec.Code, svc.calls.active, tc.want)
		}
	}
}

func TestCreateRole_TakenNameIs409(t *testing.T) {
	svc := &fakeRoleService{calls: roleCall{createErr: rbac.ErrRoleNameTaken}}
	rec := sendRole(t, svc, http.MethodPost, "/roles", `{"name":"analyst","display_name":"Analyst"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("status %d body %s, want 409 saying the name is taken", rec.Code, rec.Body)
	}
}

func TestUpdateRole_DeactivateAndReactivate(t *testing.T) {
	svc := &fakeRoleService{role: customRole()}
	svc.role.IsActive = true
	for _, want := range []bool{false, true} {
		rec := sendRole(t, svc, http.MethodPut, "/roles/4", fmt.Sprintf(`{"is_active":%v}`, want))
		if rec.Code != http.StatusOK || svc.calls.updated == nil || svc.calls.updated.IsActive != want {
			t.Fatalf("is_active=%v: status %d, saved %+v", want, rec.Code, svc.calls.updated)
		}
		if svc.calls.updated.DisplayName != "Ack Only" || svc.calls.updated.Description != "keep me" {
			t.Fatalf("is_active=%v changed other fields: %+v", want, svc.calls.updated)
		}
	}
}

func TestUpdateRole_SystemRoleCannotBeDeactivated(t *testing.T) {
	svc := &fakeRoleService{role: domain.Role{ID: 1, Name: "admin", IsSystem: true, IsActive: true}}
	rec := sendRole(t, svc, http.MethodPut, "/roles/1", `{"is_active":false}`)
	if rec.Code != http.StatusForbidden || svc.calls.updated != nil {
		t.Fatalf("status %d, saved %+v; want 403 and nothing saved", rec.Code, svc.calls.updated)
	}
}
