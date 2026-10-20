package chi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	httputil "github.com/DefensePoint/keycloak-monitoring/internal/http"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
	"github.com/DefensePoint/keycloak-monitoring/internal/users"
)

// fakeUserService keeps users in memory and records the writes it receives.
type fakeUserService struct {
	users       map[uint]*domain.User
	updated     []uint
	deleted     []uint
	lastUpdate  *domain.User
	usernameErr error // a failure to look a username up
}

func (f *fakeUserService) ListUsers(ctx context.Context, opts *httputil.ListOptions) ([]*domain.User, error) {
	out := make([]*domain.User, 0, len(f.users))
	for _, u := range f.users {
		out = append(out, u)
	}
	return out, nil
}

func (f *fakeUserService) GetByID(ctx context.Context, userID uint) (*domain.User, error) {
	u, ok := f.users[userID]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

// GetByUsername answers as the real repository does: an error wrapping
// users.ErrUserNotFound when no user has the name.
func (f *fakeUserService) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	if f.usernameErr != nil {
		return nil, f.usernameErr
	}
	for _, u := range f.users {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", users.ErrUserNotFound, username)
}

func (f *fakeUserService) CreateUser(ctx context.Context, user *domain.User) error { return nil }

func (f *fakeUserService) UpdateUser(ctx context.Context, user *domain.User) error {
	f.updated = append(f.updated, user.ID)
	cp := *user
	f.lastUpdate = &cp
	return nil
}

func (f *fakeUserService) DeleteUser(ctx context.Context, userID uint) error {
	f.deleted = append(f.deleted, userID)
	return nil
}

const (
	adminUserID    uint = 1
	managerUserID  uint = 2
	regularUserID  uint = 3
	operatorUserID uint = 4
)

// usersTestServer serves /api/users as callerID. admins lists the users that
// hold the global admin role; globalPerms lists the permissions the caller
// holds through a role granted for every tenant. Every tenant-scoped check
// (HasPermission) passes, so a refusal can only come from the global check or
// the admin guard.
func usersTestServer(t *testing.T, callerID uint, admins []uint, globalPerms ...string) (*httptest.Server, *fakeUserService) {
	t.Helper()
	return usersTestServerWith(t, callerID, admins, nil, globalPerms...)
}

// usersTestServerWith is usersTestServer with a hook that adjusts the RBAC
// fake before the server starts.
func usersTestServerWith(t *testing.T, callerID uint, admins []uint, adjust func(*mockRBACChecker), globalPerms ...string) (*httptest.Server, *fakeUserService) {
	t.Helper()

	svc := &fakeUserService{users: map[uint]*domain.User{
		adminUserID:    {ID: adminUserID, Username: "admin", AuthMethod: domain.AuthMethodSimple, IsActive: true},
		managerUserID:  {ID: managerUserID, Username: "manager", AuthMethod: domain.AuthMethodSimple, IsActive: true},
		regularUserID:  {ID: regularUserID, Username: "regular", AuthMethod: domain.AuthMethodSimple, IsActive: true},
		operatorUserID: {ID: operatorUserID, Username: "operator", AuthMethod: domain.AuthMethodSimple, IsActive: true},
	}}

	isAdmin := func(userID uint) bool {
		for _, id := range admins {
			if id == userID {
				return true
			}
		}
		return false
	}
	checker := &mockRBACChecker{
		isAdminFn: isAdmin,
		// Admins hold the admin role, the manager a custom role granting
		// platform_users permissions, and the operator the built-in operator
		// role. The regular user holds none.
		holdsRoleFn: func(userID uint) bool {
			return isAdmin(userID) || userID == managerUserID || userID == operatorUserID
		},
		hasGlobalPermissionFn: func(ctx context.Context, userID uint, permission string) (bool, error) {
			for _, p := range globalPerms {
				if p == permission {
					return true, nil
				}
			}
			return false, nil
		},
	}

	if adjust != nil {
		adjust(checker)
	}

	log := logger.NewNoop()
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), UserInfoKey, &UserDetails{ID: callerID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	h := NewUserHandlers(svc, checker, log, auth, NewRBACMiddleware(checker, log))

	r := chi.NewRouter()
	h.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, svc
}

func doUsersRequest(t *testing.T, srv *httptest.Server, method, path, body string) int {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestUsersRoutes_TenantScopedGrantIsNotEnough(t *testing.T) {
	// No global grant at all: the caller's platform_users permissions exist
	// only for some tenant, which HasPermission would have accepted.
	srv, svc := usersTestServer(t, managerUserID, nil)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/users/", ""},
		{http.MethodGet, "/api/users/3/", ""},
		{http.MethodPost, "/api/users/", `{"username":"newuser","email":"n@example.com","password":"Str0ng!Passw0rd"}`},
		{http.MethodPut, "/api/users/3/", `{"name":"Changed"}`},
		{http.MethodDelete, "/api/users/3/", ""},
	} {
		if got := doUsersRequest(t, srv, tc.method, tc.path, tc.body); got != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", tc.method, tc.path, got)
		}
	}
	if len(svc.updated) != 0 || len(svc.deleted) != 0 {
		t.Fatalf("writes reached the service: updated=%v deleted=%v", svc.updated, svc.deleted)
	}
}

func TestUpdateUser_NonAdminCannotChangeAnAdmin(t *testing.T) {
	srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite)

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/1/", `{"password":"An0ther!Passw0rd"}`); got != http.StatusForbidden {
		t.Fatalf("PUT on an admin = %d, want 403", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("the admin account was updated: %v", svc.updated)
	}
}

func TestUpdateUser_NonAdminCanChangeARegularUser(t *testing.T) {
	srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite)

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"name":"Changed"}`); got != http.StatusOK {
		t.Fatalf("PUT on a regular user = %d, want 200", got)
	}
	if len(svc.updated) != 1 || svc.updated[0] != regularUserID {
		t.Fatalf("updated = %v, want [%d]", svc.updated, regularUserID)
	}
}

func TestUpdateUser_AdminCanChangeAnAdmin(t *testing.T) {
	srv, svc := usersTestServer(t, adminUserID, []uint{adminUserID, managerUserID})

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/2/", `{"name":"Changed"}`); got != http.StatusOK {
		t.Fatalf("admin PUT on an admin = %d, want 200", got)
	}
	if len(svc.updated) != 1 {
		t.Fatalf("updated = %v, want one update", svc.updated)
	}
}

func TestDeleteUser_NonAdminCannotDeleteAnAdmin(t *testing.T) {
	srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersDelete)

	if got := doUsersRequest(t, srv, http.MethodDelete, "/api/users/1/", ""); got != http.StatusForbidden {
		t.Fatalf("DELETE on an admin = %d, want 403", got)
	}
	if len(svc.deleted) != 0 {
		t.Fatalf("the admin account was deleted: %v", svc.deleted)
	}
}

func TestDeleteUser_RequiresTheDeletePermission(t *testing.T) {
	writeOnly, svc := usersTestServer(t, managerUserID, nil,
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite)
	if got := doUsersRequest(t, writeOnly, http.MethodDelete, "/api/users/3/", ""); got != http.StatusForbidden {
		t.Fatalf("DELETE with write but not delete = %d, want 403", got)
	}
	if len(svc.deleted) != 0 {
		t.Fatalf("deleted without platform_users:delete: %v", svc.deleted)
	}

	withDelete, svc := usersTestServer(t, managerUserID, nil,
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersDelete)
	if got := doUsersRequest(t, withDelete, http.MethodDelete, "/api/users/3/", ""); got != http.StatusOK {
		t.Fatalf("DELETE with platform_users:delete = %d, want 200", got)
	}
	if len(svc.deleted) != 1 || svc.deleted[0] != regularUserID {
		t.Fatalf("deleted = %v, want [%d]", svc.deleted, regularUserID)
	}
}

// platform_users:write and :delete let a non-admin reset another user's
// password and sign in as them. The guard used to protect only admins, so a
// user manager could take over an operator and use every permission the
// operator role gives. Only an admin assigns roles, so only an admin manages
// anyone who holds one.

func TestManageUser_NonAdminCannotTouchARoleHolder(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
	}{
		{"reset the password", http.MethodPut, `{"password":"An0ther!Passw0rd"}`},
		{"change the email", http.MethodPut, `{"email":"taken-over@example.com"}`},
		{"change the username", http.MethodPut, `{"username":"someone-else"}`},
		{"block", http.MethodPut, `{"is_blocked":true}`},
		{"deactivate", http.MethodPut, `{"is_active":false}`},
		{"rename", http.MethodPut, `{"name":"Changed"}`},
		{"delete", http.MethodDelete, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
				rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite, rbac.PermissionPlatformUsersDelete)

			if got := doUsersRequest(t, srv, tc.method, "/api/users/4/", tc.body); got != http.StatusForbidden {
				t.Fatalf("%s an operator = %d, want 403", tc.name, got)
			}
			if len(svc.updated) != 0 || len(svc.deleted) != 0 {
				t.Fatalf("the operator account was changed: updated %v, deleted %v", svc.updated, svc.deleted)
			}
		})
	}
}

func TestManageUser_NonAdminCanStillManageAUserWithoutRoles(t *testing.T) {
	srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite, rbac.PermissionPlatformUsersDelete)

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"password":"An0ther!Passw0rd"}`); got != http.StatusOK {
		t.Fatalf("password reset on a user without roles = %d, want 200", got)
	}
	if got := doUsersRequest(t, srv, http.MethodDelete, "/api/users/3/", ""); got != http.StatusOK && got != http.StatusNoContent {
		t.Fatalf("delete of a user without roles = %d, want success", got)
	}
	if len(svc.updated) != 1 || len(svc.deleted) != 1 {
		t.Fatalf("updated %v, deleted %v; want one of each", svc.updated, svc.deleted)
	}
}

func TestManageUser_NonAdminCanStillManageThemselves(t *testing.T) {
	srv, svc := usersTestServer(t, managerUserID, []uint{adminUserID},
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite)

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/2/", `{"name":"Me"}`); got != http.StatusOK {
		t.Fatalf("PUT on themselves = %d, want 200", got)
	}
	if len(svc.updated) != 1 || svc.updated[0] != managerUserID {
		t.Fatalf("updated = %v, want [%d]", svc.updated, managerUserID)
	}
}

func TestManageUser_AdminCanManageARoleHolder(t *testing.T) {
	srv, svc := usersTestServer(t, adminUserID, []uint{adminUserID})

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/4/", `{"password":"An0ther!Passw0rd"}`); got != http.StatusOK {
		t.Fatalf("admin password reset on an operator = %d, want 200", got)
	}
	if len(svc.updated) != 1 || svc.updated[0] != operatorUserID {
		t.Fatalf("updated = %v, want [%d]", svc.updated, operatorUserID)
	}
}

func TestManageUser_ARoleLookupFailureRefuses(t *testing.T) {
	srv, svc := usersTestServerWith(t, managerUserID, []uint{adminUserID},
		func(m *mockRBACChecker) { m.holdsRoleErr = errors.New("database unavailable") },
		rbac.PermissionPlatformUsersRead, rbac.PermissionPlatformUsersWrite)

	if got := doUsersRequest(t, srv, http.MethodPut, "/api/users/3/", `{"password":"An0ther!Passw0rd"}`); got != http.StatusInternalServerError {
		t.Fatalf("PUT while roles cannot be read = %d, want 500", got)
	}
	if len(svc.updated) != 0 {
		t.Fatalf("a user was updated without the role check: %v", svc.updated)
	}
}
