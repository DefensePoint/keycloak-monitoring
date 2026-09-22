package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/apitoken"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// mockTokenValidator implements TokenValidator for testing.
type mockTokenValidator struct {
	validateFn func(ctx context.Context, plaintext string) (*apitoken.Identity, error)
}

func (m *mockTokenValidator) Validate(ctx context.Context, plaintext string) (*apitoken.Identity, error) {
	if m.validateFn != nil {
		return m.validateFn(ctx, plaintext)
	}
	return nil, apitoken.ErrTokenNotFound
}

// mockPermissionService implements PermissionService for testing.
type mockPermissionService struct {
	hasPermissionFn   func(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	getUserPoliciesFn func(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)
	getUserRolesFn    func(ctx context.Context, userID uint) ([]*domain.UserRole, error)
	isAdminFn         func(ctx context.Context, userID uint) (bool, error)
}

func (m *mockPermissionService) HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error) {
	if m.hasPermissionFn != nil {
		return m.hasPermissionFn(ctx, userID, permission, tenantID)
	}
	return false, nil
}

func (m *mockPermissionService) GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error) {
	if m.getUserPoliciesFn != nil {
		return m.getUserPoliciesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockPermissionService) GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error) {
	if m.getUserRolesFn != nil {
		return m.getUserRolesFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockPermissionService) IsAdmin(ctx context.Context, userID uint) (bool, error) {
	if m.isAdminFn != nil {
		return m.isAdminFn(ctx, userID)
	}
	return false, nil
}

func testLogger() *logger.Logger {
	return logger.NewConsole("error", false)
}

func validIdentity() *apitoken.Identity {
	return &apitoken.Identity{
		User:      &domain.User{ID: 7, Subject: "user-7", IsActive: true},
		TenantIDs: []string{"tenant-a"},
	}
}

func acceptingValidator(accepted string) *mockTokenValidator {
	return &mockTokenValidator{
		validateFn: func(_ context.Context, plaintext string) (*apitoken.Identity, error) {
			if plaintext == accepted {
				return validIdentity(), nil
			}
			return nil, apitoken.ErrTokenNotFound
		},
	}
}

func newAuthedHandler(t *testing.T, tokens TokenValidator, allowlist []string) http.Handler {
	t.Helper()
	middleware := AuthMiddleware(tokens, &mockPermissionService{}, allowlist, testLogger())
	return middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestAuthMiddlewareRejectsWithoutToken(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), nil)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without token: got status %d, want %d", method, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), nil)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer pat_wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with invalid token: got status %d, want %d", method, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestAuthMiddlewareRejectsExpiredToken(t *testing.T) {
	tokens := &mockTokenValidator{
		validateFn: func(context.Context, string) (*apitoken.Identity, error) {
			return nil, apitoken.ErrTokenExpired
		},
	}
	handler := newAuthedHandler(t, tokens, nil)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		req := httptest.NewRequest(method, "/mcp", nil)
		req.Header.Set("Authorization", "Bearer pat_expired")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with expired token: got status %d, want %d", method, rec.Code, http.StatusUnauthorized)
		}
		if body := rec.Body.String(); body != "invalid token\n" {
			t.Errorf("%s with expired token: body %q leaks validation detail", method, body)
		}
	}
}

func TestAuthMiddlewareAcceptsValidToken(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), nil)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer pat_good")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareRejectsOriginWithEmptyAllowlist(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), nil)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer pat_good")
	req.Header.Set("Origin", "https://anything.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin with empty allowlist: got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAuthMiddlewareRejectsOriginNotInAllowlist(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), []string{"https://allowed.example.com"})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer pat_good")
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin: got status %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAuthMiddlewareAcceptsAllowedOrigin(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), []string{"https://allowed.example.com"})

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer pat_good")
	req.Header.Set("Origin", "https://allowed.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("allowed origin: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareOriginCheckedBeforeAuth(t *testing.T) {
	handler := newAuthedHandler(t, acceptingValidator("pat_good"), nil)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin without token: got status %d, want %d (origin check first)", rec.Code, http.StatusForbidden)
	}
}
