package mcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/internal/rbac"
)

// PermissionService is the slice of rbac.Service this package needs.
type PermissionService interface {
	HasPermission(ctx context.Context, userID uint, permission string, tenantID *string) (bool, error)
	GetUserPolicies(ctx context.Context, userID uint) ([]*domain.TenantPolicy, error)
	GetUserRoles(ctx context.Context, userID uint) ([]*domain.UserRole, error)
	IsAdmin(ctx context.Context, userID uint) (bool, error)
}

// TenantReader is the slice of tenant.Reader this package needs.
type TenantReader interface {
	GetByTenantID(ctx context.Context, tenantID string) (*domain.KeycloakTenant, error)
	ListEnabled(ctx context.Context) ([]*domain.KeycloakTenant, error)
}

// Authorizer performs the per-tool-call authorization checks. Every tool that
// takes a tenant argument must authorize it through AuthorizeTenant (or
// Authorize) before touching data. Authorization always derives from the
// per-request Caller, never from session or connection state.
type Authorizer struct {
	perms   PermissionService
	tenants TenantReader
	log     *logger.Logger
}

// NewAuthorizer creates an Authorizer.
func NewAuthorizer(perms PermissionService, tenants TenantReader, log *logger.Logger) *Authorizer {
	return &Authorizer{perms: perms, tenants: tenants, log: log}
}

// Authorize applies AuthorizeTenant and discards the tenant row.
func (a *Authorizer) Authorize(ctx context.Context, caller *Caller, tenantID, permission string) error {
	_, err := a.AuthorizeTenant(ctx, caller, tenantID, permission)
	return err
}

// TenantArg resolves which tenant a tool call operates on. A token
// allowlisted to exactly one tenant has that tenant injected server-side and
// must not name it, so the tenant is not a parameter the caller (or the model
// driving it) can choose at all. A token allowlisted to several keeps naming
// one of them, and AuthorizeTenant constrains the choice to the list.
func TenantArg(caller *Caller, arg string) (string, error) {
	if caller == nil {
		return "", ErrUnauthenticated
	}
	switch len(caller.TenantIDs) {
	case 0:
		return "", ErrForbidden
	case 1:
		if arg != "" {
			return "", Invalidf("tenant must be omitted: this token is allowlisted to a single tenant, which the server supplies")
		}
		return caller.TenantIDs[0], nil
	default:
		if arg == "" {
			return "", Invalidf("tenant is required: this token is allowlisted to %d tenants, so the call must name one of them", len(caller.TenantIDs))
		}
		return arg, nil
	}
}

// AuthorizeTenant denies unless the token's tenant allowlist admits the
// tenant (an absent allowlist admits nothing), RBAC grants every listed
// permission for it (none listed denies), and the tenant exists and is
// enabled. On success it returns the tenant row it validated so callers never
// fetch the tenant a second time. rbac.HasPermission treats global roles as
// valid for every tenant string, so the allowlist and the tenant lookup are
// what scope a globally-granted caller to real, enabled tenants.
func (a *Authorizer) AuthorizeTenant(ctx context.Context, caller *Caller, tenantID string, permissions ...string) (*domain.KeycloakTenant, error) {
	if caller == nil {
		return nil, ErrUnauthenticated
	}
	if len(permissions) == 0 {
		return nil, ErrForbidden
	}

	if !slices.Contains(caller.TenantIDs, tenantID) {
		return nil, ErrForbidden
	}

	for _, permission := range permissions {
		allowed, err := a.perms.HasPermission(ctx, caller.UserID, permission, &tenantID)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !allowed {
			return nil, ErrForbidden
		}
	}

	t, err := a.tenants.GetByTenantID(ctx, tenantID)
	if err != nil {
		a.log.Warn("MCP tenant lookup failed",
			logger.Str("tenant_id", tenantID),
			logger.Err(err))
		return nil, ErrTenantNotAvailable
	}
	if t == nil || !t.Enabled {
		return nil, ErrTenantNotAvailable
	}
	return t, nil
}

// VisibleTenants returns the enabled tenants the caller may list. Admins
// skip the permission gate here, mirroring the admin bypass in the web list
// endpoint's RequirePermission gate. The shared fail-closed role filter
// narrows the set and applies its own admin and global-role bypass; the
// token's tenant allowlist narrows it further, and a token carrying no
// allowlist sees nothing.
func (a *Authorizer) VisibleTenants(ctx context.Context, caller *Caller, permission string) ([]*domain.KeycloakTenant, error) {
	if caller == nil {
		return nil, ErrUnauthenticated
	}
	if len(caller.TenantIDs) == 0 {
		return nil, ErrForbidden
	}

	if !caller.IsAdmin {
		allowed, err := a.perms.HasPermission(ctx, caller.UserID, permission, nil)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !allowed {
			return nil, ErrForbidden
		}
	}

	tenants, err := a.tenants.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("tenant list failed: %w", err)
	}

	visible := rbac.FilterTenantsByUserRoles(ctx, a.perms, caller.UserID, tenants, a.log)
	narrowed := make([]*domain.KeycloakTenant, 0, len(visible))
	for _, t := range visible {
		if slices.Contains(caller.TenantIDs, t.TenantID) {
			narrowed = append(narrowed, t)
		}
	}
	return narrowed, nil
}

// AllowedRealms resolves the caller's allowed realm set for a tenant. A global
// admin gets every realm. Otherwise the tenant's policy rows decide, via
// rbac.ResolveRealmScope, which is the same function the web resolver in
// internal/http/chi uses so a policy means one thing on both transports.
//
// This used to reimplement the rules and disagreed with the web on three of
// the five shapes a row can take: it denied a row whose allowed_realms column
// was NULL (unrestricted on the web), it returned only the first matching row
// instead of the union across rows, and it passed blank realms straight
// through, which widened alert statistics to the whole tenant because the
// repository reads an all-blank scope as no realm filter.
//
// A policy that grants nothing is reported as ErrForbidden rather than an
// empty scope, which is this API's existing contract: callers treat an error
// as a refusal, and none of them has to remember that an empty realms slice
// with all = false means "no realm" rather than "no restriction".
func (a *Authorizer) AllowedRealms(ctx context.Context, caller *Caller, tenantID string) (realms []string, all bool, err error) {
	if caller == nil {
		return nil, false, ErrUnauthenticated
	}
	if caller.IsAdmin {
		return nil, true, nil
	}

	policies, err := a.perms.GetUserPolicies(ctx, caller.UserID)
	if err != nil {
		return nil, false, fmt.Errorf("tenant policy lookup failed: %w", err)
	}

	realms, all = rbac.ResolveRealmScope(policies, tenantID)
	if all {
		return nil, true, nil
	}
	if len(realms) == 0 {
		return nil, false, ErrForbidden
	}
	return realms, false, nil
}
