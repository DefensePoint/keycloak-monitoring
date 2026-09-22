package mcp

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
)

// mockTenantReader implements TenantReader for testing.
type mockTenantReader struct {
	getByTenantIDFn func(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
	listEnabledFn   func(ctx context.Context) ([]*domain.KeycloakTenant, error)
}

func (m *mockTenantReader) GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error) {
	if m.getByTenantIDFn != nil {
		return m.getByTenantIDFn(ctx, tenantID)
	}
	return nil, errors.New("tenant not found")
}

func (m *mockTenantReader) ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error) {
	if m.listEnabledFn != nil {
		return m.listEnabledFn(ctx)
	}
	return nil, nil
}

func enabledTenant(tenantID string) *domain.KeycloakTenant {
	return &domain.KeycloakTenant{TenantID: tenantID, Enabled: true}
}

func grantAll() *mockPermissionService {
	return &mockPermissionService{
		hasPermissionFn: func(context.Context, uint, string, *string) (bool, error) {
			return true, nil
		},
	}
}

func readerFor(t *domain.KeycloakTenant) *mockTenantReader {
	return &mockTenantReader{
		getByTenantIDFn: func(_ context.Context, tenantID string) (*domain.KeycloakTenant, error) {
			if t != nil && t.TenantID == tenantID {
				return t, nil
			}
			return nil, errors.New("tenant not found")
		},
	}
}

func TestAuthorizeAllowsTenantInAllowlist(t *testing.T) {
	authz := NewAuthorizer(grantAll(), readerFor(enabledTenant("tenant-a")), testLogger())
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a", "tenant-b"}}

	if err := authz.Authorize(context.Background(), caller, "tenant-a", "events:read"); err != nil {
		t.Fatalf("expected allow, got %v", err)
	}
}

func TestAuthorizeDeniesEmptyAllowlist(t *testing.T) {
	authz := NewAuthorizer(grantAll(), readerFor(enabledTenant("tenant-a")), testLogger())
	caller := &Caller{UserID: 7}

	err := authz.Authorize(context.Background(), caller, "tenant-a", "events:read")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a token carrying no allowlist, got %v", err)
	}
}

func TestTenantArgInjectsTheOnlyAllowlistedTenant(t *testing.T) {
	got, err := TenantArg(&Caller{UserID: 7, TenantIDs: []string{"tenant-a"}}, "")
	if err != nil || got != "tenant-a" {
		t.Fatalf("TenantArg = %q, %v, want tenant-a, nil", got, err)
	}
}

func TestTenantArgRejectsASuppliedTenantForASingleTenantToken(t *testing.T) {
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a"}}
	for _, arg := range []string{"tenant-a", "tenant-b"} {
		if _, err := TenantArg(caller, arg); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("TenantArg(%q) error = %v, want ErrInvalidInput", arg, err)
		}
	}
}

func TestTenantArgKeepsTheArgumentForAMultiTenantToken(t *testing.T) {
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a", "tenant-b"}}
	got, err := TenantArg(caller, "tenant-b")
	if err != nil || got != "tenant-b" {
		t.Fatalf("TenantArg = %q, %v, want tenant-b, nil", got, err)
	}
	if _, err := TenantArg(caller, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("TenantArg without an argument = %v, want ErrInvalidInput", err)
	}
}

func TestTenantArgDeniesAnUnscopedToken(t *testing.T) {
	if _, err := TenantArg(&Caller{UserID: 7}, "tenant-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("TenantArg for a token carrying no allowlist = %v, want ErrForbidden", err)
	}
}

func TestAuthorizeDeniesTenantOutsideAllowlist(t *testing.T) {
	authz := NewAuthorizer(grantAll(), readerFor(enabledTenant("tenant-b")), testLogger())
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a"}}

	err := authz.Authorize(context.Background(), caller, "tenant-b", "events:read")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthorizeDeniesWithoutRBACPermission(t *testing.T) {
	perms := &mockPermissionService{
		hasPermissionFn: func(context.Context, uint, string, *string) (bool, error) {
			return false, nil
		},
	}
	authz := NewAuthorizer(perms, readerFor(enabledTenant("tenant-a")), testLogger())
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a"}}

	err := authz.Authorize(context.Background(), caller, "tenant-a", "events:read")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAuthorizeDeniesDisabledTenant(t *testing.T) {
	disabled := &domain.KeycloakTenant{TenantID: "tenant-a", Enabled: false}
	authz := NewAuthorizer(grantAll(), readerFor(disabled), testLogger())
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-a"}}

	err := authz.Authorize(context.Background(), caller, "tenant-a", "events:read")
	if !errors.Is(err, ErrTenantNotAvailable) {
		t.Fatalf("expected ErrTenantNotAvailable, got %v", err)
	}
}

func TestAuthorizeDeniesUnknownTenant(t *testing.T) {
	authz := NewAuthorizer(grantAll(), readerFor(nil), testLogger())
	caller := &Caller{UserID: 7, TenantIDs: []string{"tenant-missing"}}

	err := authz.Authorize(context.Background(), caller, "tenant-missing", "events:read")
	if !errors.Is(err, ErrTenantNotAvailable) {
		t.Fatalf("expected ErrTenantNotAvailable, got %v", err)
	}
}

func TestAuthorizeDeniesNilCaller(t *testing.T) {
	authz := NewAuthorizer(grantAll(), readerFor(enabledTenant("tenant-a")), testLogger())

	err := authz.Authorize(context.Background(), nil, "tenant-a", "events:read")
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestAllowedRealms(t *testing.T) {
	tests := []struct {
		name       string
		caller     *Caller
		policies   []*domain.TenantPolicy
		wantRealms []string
		wantAll    bool
		wantErr    error
	}{
		{
			name:    "no policy row means all realms",
			caller:  &Caller{UserID: 7},
			wantAll: true,
		},
		{
			name:     "policy row with nil realm list denies",
			caller:   &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: nil}},
			wantErr:  ErrForbidden,
		},
		{
			name:     "policy row with empty realm list denies",
			caller:   &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{}}},
			wantErr:  ErrForbidden,
		},
		{
			name:   "policy row with realms grants exactly those",
			caller: &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{
				{TenantID: "tenant-b", AllowedRealms: []string{"other"}},
				{TenantID: "tenant-a", AllowedRealms: []string{"prod", "staging"}},
			},
			wantRealms: []string{"prod", "staging"},
		},
		{
			name:     "global admin gets all realms",
			caller:   &Caller{UserID: 7, IsAdmin: true},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{}}},
			wantAll:  true,
		},
		{
			// The shape this resolver used to deny while the web allowed it.
			// A SQL NULL allowed_realms column is an absent restriction, not
			// an empty one: tenant_policies is opt-in, so a row that names no
			// list grants every realm. Nothing here covered RealmsUnrestricted
			// before, which is how the two sides drifted apart unnoticed.
			name:     "a row whose allowed_realms column holds no list is unrestricted",
			caller:   &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", RealmsUnrestricted: true}},
			wantAll:  true,
		},
		{
			name:   "an unrestricted row wins over a restricting one",
			caller: &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{
				{TenantID: "tenant-a", AllowedRealms: []string{"prod"}},
				{TenantID: "tenant-a", RealmsUnrestricted: true},
			},
			wantAll: true,
		},
		{
			// The fail-open shape. A blank names no realm, but passed through
			// it reads as "no realm filter" in the alerts repository and
			// widened alert statistics to the whole tenant.
			name:     "a single blank realm denies rather than widening the scope",
			caller:   &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{""}}},
			wantErr:  ErrForbidden,
		},
		{
			name:     "nothing but blank realms denies",
			caller:   &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"", "  "}}},
			wantErr:  ErrForbidden,
		},
		{
			name:       "a blank beside a real realm keeps only the real one",
			caller:     &Caller{UserID: 7},
			policies:   []*domain.TenantPolicy{{TenantID: "tenant-a", AllowedRealms: []string{"", "prod"}}},
			wantRealms: []string{"prod"},
		},
		{
			// This resolver used to return the first matching row and stop.
			// (user_id, tenant_id) is not unique and GetUserPolicies has no
			// ORDER BY, so which row won was whatever Postgres returned first.
			name:   "every matching row contributes, not just the first",
			caller: &Caller{UserID: 7},
			policies: []*domain.TenantPolicy{
				{TenantID: "tenant-a", AllowedRealms: []string{"prod"}},
				{TenantID: "tenant-a", AllowedRealms: []string{"staging"}},
			},
			wantRealms: []string{"prod", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := &mockPermissionService{
				getUserPoliciesFn: func(context.Context, uint) ([]*domain.TenantPolicy, error) {
					return tt.policies, nil
				},
			}
			authz := NewAuthorizer(perms, readerFor(nil), testLogger())

			realms, all, err := authz.AllowedRealms(context.Background(), tt.caller, "tenant-a")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if all != tt.wantAll {
				t.Fatalf("all = %v, want %v", all, tt.wantAll)
			}
			if tt.wantAll {
				if realms != nil {
					t.Fatalf("expected nil realms with all=true, got %v", realms)
				}
				return
			}
			if !slices.Equal(realms, tt.wantRealms) {
				t.Fatalf("realms = %v, want %v", realms, tt.wantRealms)
			}
		})
	}
}
