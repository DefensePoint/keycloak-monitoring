# Role-Based Access Control (RBAC) Guide

> This document describes the RBAC system as implemented in the codebase. Authoritative source: `rbac/constants.go`, `rbac/service.go`, `internal/http/chi/middleware.go`, and the handler files under `internal/http/chi/`.

## Overview

The Keycloak Monitoring Tool enforces access control through **two independent layers**:

1. **Permissions** — gate *what actions* a user can perform (e.g. `alerts:read`, `tenants:write`).
2. **Tenant policies** — gate *which tenants' data* a non-admin user can access.

A user needs to pass **both** layers to access tenant-scoped data. Having `tenants:read` globally is **not** enough to read a specific tenant's alerts, events, or metrics — the user also needs an explicit policy for that tenant (or be an admin, which bypasses the policy check).

### Key concepts

- **Role** — named collection of permissions. Three system roles ship by default (`admin`, `operator`, `viewer`); custom roles can be created.
- **Permission** — a `resource:action` string (e.g. `alerts:acknowledge`). Defined as constants in `rbac/constants.go`.
- **User role assignment** — a binding between a user and a role. Can be global or scoped to a specific tenant (`user_role_assignments` table).
- **Tenant policy** — a binding between a user and a tenant, optionally restricting access to specific realms within that tenant (`policies` table).
- **Admin bypass** — users with the `admin` role bypass tenant-policy checks entirely (`rbac/service.go:86-100`).

---

## System Roles

Defined in `rbac/constants.go:84-129` (`GetSystemRoles()`).

### `admin` — Administrator

> *Full system access — can manage all resources, users, and configurations.*

Holds every permission defined in the system. Bypasses tenant-policy enforcement: admins can access data from any tenant regardless of policies.

**Typical users:** platform administrators, DevOps leads.

### `operator` — Operator (SOC Analyst)

> *Can view and act on alerts/events within assigned tenants and realms.*

Permissions:

- `tenants:read`, `realms:read`
- `users:read`, `clients:read`
- `events:read`
- `alerts:read`, `alerts:acknowledge`, `alerts:resolve`
- `metrics:read`, `keycloak:read`

Operators **cannot** create/modify/delete alerts, tenants, users, realms, or clients. They **do not** bypass tenant policies — they only see tenants for which they have an explicit policy.

**Typical users:** SOC analysts, on-call engineers.

### `viewer` — Viewer

> *Read-only access to assigned tenants and realms.*

Permissions:

- `tenants:read`, `realms:read`
- `users:read`, `clients:read`
- `events:read`, `alerts:read`
- `metrics:read`, `keycloak:read`

Cannot perform any write action (including alert ack/resolve). Subject to tenant-policy enforcement.

**Typical users:** auditors, stakeholders, junior team members.

---

## Permissions Reference

All permissions follow the format `resource:action`. Defined in `rbac/constants.go:5-66`, full list returned by `GetSystemPermissions()` (`rbac/constants.go:141-204`).

### Tenants

| Permission | Description |
|------------|-------------|
| `tenants:read` | View tenant information (list + detail + health) |
| `tenants:write` | Create and modify tenants |
| `tenants:delete` | Delete tenants |

### Realms

| Permission | Description |
|------------|-------------|
| `realms:read` | View realm information |
| `realms:write` | Modify realm configurations |
| `realms:delete` | Delete realms |

### Keycloak Users

> These permissions apply to **Keycloak-managed users** (users that live in the monitored Keycloak instance), not to platform users.

| Permission | Description |
|------------|-------------|
| `users:read` | View Keycloak user information |
| `users:write` | Create and modify Keycloak users |
| `users:delete` | Delete Keycloak users |

### Clients

| Permission | Description |
|------------|-------------|
| `clients:read` | View client information |
| `clients:write` | Create and modify clients |
| `clients:delete` | Delete clients |

### Events

| Permission | Description |
|------------|-------------|
| `events:read` | View event logs |

> Events are read-only in the platform — they are ingested from Keycloak, not created via the API.

### Alerts

| Permission | Description |
|------------|-------------|
| `alerts:read` | View alerts |
| `alerts:acknowledge` | Acknowledge alerts |
| `alerts:resolve` | Resolve alerts |
| `alerts:write` | Create and modify alerts |
| `alerts:delete` | Delete alerts |

### Metrics

| Permission | Description |
|------------|-------------|
| `metrics:read` | View metrics and dashboards |

### Keycloak (general)

| Permission | Description |
|------------|-------------|
| `keycloak:read` | View Keycloak health, metrics, and general information |

### Roles

| Permission | Description |
|------------|-------------|
| `roles:read` | View role information |
| `roles:write` | Create and modify roles |
| `roles:delete` | Delete roles |
| `roles:assign` | Assign roles to users |

### Permissions (meta)

| Permission | Description |
|------------|-------------|
| `permissions:read` | View permission information |
| `permissions:write` | Create and modify permissions |
| `permissions:delete` | Delete permissions |

### Platform Users

> These are **platform users** (users of Keycloak Monitoring Tool itself), not Keycloak users.

| Permission | Description |
|------------|-------------|
| `platform_users:read` | View platform user information |
| `platform_users:write` | Create and modify platform users |
| `platform_users:delete` | Delete platform users |

### Policies

> These permissions govern the **tenant policies** table that enforces tenant isolation (see [Tenant Isolation](#tenant-isolation)).

| Permission | Description |
|------------|-------------|
| `policies:read` | View tenant access policies |
| `policies:write` | Create and modify tenant access policies |
| `policies:delete` | Delete tenant access policies |

### System Configuration

| Permission | Description |
|------------|-------------|
| `system_config:read` | View system configuration |
| `system_config:write` | Modify system configuration |

---

## Tenant Isolation

Tenant isolation is enforced by a **separate layer** on top of permission checks. This is critical — understanding it is the difference between "user can reach a route" and "user can actually see a specific tenant's data."

### The `policies` table

Persisted via `rbac/postgres/repository.go:330-419` (`TenantPolicy` model). Each row maps a user to a tenant, optionally restricting access to specific realms:

| Column | Purpose |
|--------|---------|
| `user_id` | Platform user receiving access |
| `tenant_id` | Tenant being granted |
| `allowed_realms` | JSON array of realm names — empty array means "all realms of this tenant" |
| `granted_by` | Who granted the policy |
| `granted_at` | Timestamp |

### `HasAccessToTenant()`

`rbac/service.go:86-100`. Logic:

1. If the user has the `admin` role → **return true** (admin bypass).
2. Otherwise delegate to `policyRepo.HasAccessToTenant(userID, tenantID)` — checks the `policies` table.

### `HasAccessToRealm()`

`rbac/service.go:102-121`. Logic:

1. Admin bypass (same as above).
2. Must pass `HasAccessToTenant` first.
3. Checks whether `realmName` is in the policy's `allowed_realms` array. Empty array means "all realms."

### `RequireTenantAccess()` middleware

`internal/http/chi/middleware.go:566-606`. Applied to tenant-scoped routes. For the current request's `tenantID`, it calls `HasPermission(userID, "tenants:read", &tenantID)` — a **tenant-scoped** permission check. This leverages the fact that a user can be assigned a role **for a specific tenant**, so the permission resolution also considers the tenant policy for that user.

On failure, returns `403 {"error":"Access to tenant denied"}`.

### `TenantIDMiddleware`

`internal/http/chi/middleware.go:608-619`. Extracts the `tenantID` URL parameter (via `chi.URLParam`) and injects it into the request context for downstream middleware/handlers.

---

## API Endpoints

### RBAC management (`/api/rbac/*`)

All endpoints below require `RequireAdmin()` except `/api/rbac/me`. Source: `internal/http/chi/rbac_handlers.go:44-100`.

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/rbac/me` | authenticated user (no admin required) |
| GET | `/api/rbac/roles` | admin |
| POST | `/api/rbac/roles` | admin |
| GET | `/api/rbac/roles/{roleID}` | admin |
| PUT | `/api/rbac/roles/{roleID}` | admin (system roles cannot be modified) |
| DELETE | `/api/rbac/roles/{roleID}` | admin (system roles cannot be deleted) |
| GET | `/api/rbac/permissions` | admin (optional `?resource=` filter) |
| GET | `/api/rbac/users/{userID}/roles` | admin (optional `?tenant_id=` filter) |
| POST | `/api/rbac/users/{userID}/roles` | admin (body: `role_id`, optional `tenant_id`, `expires_at`) |
| DELETE | `/api/rbac/users/{userID}/roles/{roleID}` | admin |
| GET | `/api/rbac/users/{userID}/policies` | admin |
| POST | `/api/rbac/users/{userID}/policies` | admin (body: `tenant_id`, `allowed_realms`) |
| DELETE | `/api/rbac/policies/{policyID}` | admin |

The `GET /api/rbac/me` endpoint returns the current user's roles, permissions, and tenant policies. Accepts an optional `?tenant_id=` query parameter to scope the returned permissions to a specific tenant.

### Tenant routes (`/api/tenants/*`)

Source: `internal/http/chi/tenant_handlers.go:82-110`.

| Method | Path | Middleware | Permission |
|--------|------|------------|------------|
| GET | `/api/tenants` | — | `tenants:read` |
| POST | `/api/tenants` | — | `tenants:write` |
| POST | `/api/tenants/test-connection` | — | `tenants:write` |
| GET | `/api/tenants/{tenantID}` | `RequireTenantAccess` | `tenants:read` |
| PUT | `/api/tenants/{tenantID}` | `RequireTenantAccess` | `tenants:write` |
| DELETE | `/api/tenants/{tenantID}` | `RequireTenantAccess` | `tenants:delete` |
| GET | `/api/tenants/{tenantID}/health` | `RequireTenantAccess` | `tenants:read` |

All sub-routes under `/api/tenants/{tenantID}/...` (alerts, reports, keycloak, operator, system) inherit `RequireTenantAccess`.

### Alerts (`/api/tenants/{tenantID}/alerts`)

Source: `internal/http/chi/alerts_handlers.go:151-169`. Route group protected by `RequireTenantAccess`. Permission checks happen **inside handlers** via the `checkAlertPermission` helper (`alerts_handlers.go:726-728`) rather than as middleware.

| Method | Path | Permission checked |
|--------|------|---------------------|
| GET | `/alerts` | `alerts:read` |
| GET | `/alerts/stats` | `alerts:read` |
| GET | `/alerts/by-realm` | `alerts:read` |
| GET | `/alerts/get` | `alerts:read` |
| GET | `/alerts/{alertID}` | `alerts:read` |
| PUT | `/alerts/{alertID}/status` | `alerts:acknowledge` |
| POST | `/alerts/{alertID}/resolve` | `alerts:resolve` |
| POST | `/alerts/update-status` | `alerts:acknowledge` |
| POST | `/alerts/resolve` | `alerts:resolve` |
| DELETE | `/alerts/delete` | `alerts:delete` |

Non-admin users receive **sanitized responses**: the `Metadata`, `CheckType`, `RuleID`, and `EventID` fields are stripped from alerts (commit `281818d`, addressing pentest finding #5).

### Platform users (`/api/users/*`)

Source: `internal/http/chi/user_handlers.go:65-80`.

| Method | Path | Permission |
|--------|------|------------|
| GET | `/api/users` | `platform_users:read` |
| POST | `/api/users` | `platform_users:write` |
| PUT | `/api/users/{id}` | `platform_users:write` |
| DELETE | `/api/users/{id}` | `platform_users:delete` |

### Other tenant-scoped groups

All groups below require `RequireTenantAccess`. Handler-level permission enforcement varies per route — see source files for details:

- **Reports** — `internal/http/chi/report_handlers.go`
- **Keycloak** — `internal/http/chi/keycloak_handlers.go`
- **Operator (metrics)** — `internal/http/chi/operator_handlers.go`
- **System** — `internal/http/chi/system_handlers.go`

---

## Authentication and Initial Admin

### Simple authentication (username/password)

Configured under `auth.simple` (`internal/config/models.go:74-79`). Required environment variables when no admin exists:

```bash
export MONITORING_AUTH_SIMPLE_DEFAULT_USER="admin"
export MONITORING_AUTH_SIMPLE_DEFAULT_PASS="YourS3cur3P@ssw0rd!"
export MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL="admin@example.com"
```

Bootstrap flow (`internal/fx/bootstrap.go:97-157`):

1. If `default_user` is unset and no admin exists → startup fails with a security error.
2. Creates the initial user.
3. Calls `RBACSeeder.CreateDefaultAdminUser(userID)` (`rbac/seeder.go:177-178`) to assign the `admin` role.

The initial admin keeps the install-time password until it is changed manually. Change it after first login. Forced password rotation on first login was considered and deliberately not implemented.

### OAuth2 / OIDC

Configured under `auth.oauth2` (`internal/config/models.go:90+`). Admin bootstrap via `auth.oauth2.admin_users`:

```yaml
auth:
  oauth2:
    enabled: true
    admin_users:
      - "admin@company.com"
      - "ops-lead@company.com"
```

Or via environment variable:

```bash
export MONITORING_AUTH_OAUTH2_ADMIN_USERS="admin@company.com,ops-lead@company.com"
```

Behavior (`auth/oauth2_admin.go:58-134`):

- On first login, `ShouldBeAdmin` normalizes the user's email (trim + lowercase) and checks for an exact match against the normalized admin list.
- If matched, `EnsureAdminRole` calls `AssignRoleToUser(user.ID, adminRole.ID, nil, "system", nil)` — unscoped (global) admin role.
- A `SECURITY NOTICE` is logged at INFO level.
- If OAuth2 is the only enabled auth method, at least one admin email **must** be configured or startup fails.

### Password requirements

Enforced by `auth/password.go:92-130` (`ValidatePassword`). Called on user creation and password change.

- Minimum length: **12 characters**
- Must contain: uppercase letter, lowercase letter, number, special character
- Cannot match a blocklist of common weak passwords (e.g. `admin`, `admin123`, `welcome`, `qwerty`) — see `auth/password.go:174+`

### Session requirements

Enforced in `auth/session.go` and consumed by the auth middleware (`internal/http/chi/middleware.go:245-250`):

- Session secret must be at least 32 characters (enforced at startup).
- Session `max_age` is evaluated server-side on every request (`time.Now().Unix() - createdAt > maxAge`).
- Sessions can be revoked via a blacklist (`IsRevoked(sessionID)` check on each request).

The application **fails to start** if session, password, or OAuth2 configuration violates these requirements.

---

## User Types (Simple vs OAuth2)

Each platform user carries an `AuthMethod` field (`domain.AuthMethodSimple` or `domain.AuthMethodOAuth2`):

| Aspect | Simple auth users | OAuth2 users |
|--------|-------------------|--------------|
| Password hash | stored locally | empty (external provider) |
| `Subject` field | set to email | OAuth2 subject claim |
| Identity source | platform | OAuth2 provider |
| Modifiable fields | username, email, name, password, status, role assignments | **only** account status and role assignments |
| Identity on login | static | synced from provider on each login |

API endpoints that attempt to modify identity fields on an OAuth2 user respond `403 Forbidden`. The frontend disables identity fields in the user management UI for OAuth2 users.

Classification:

- **On creation** — set based on auth flow (simple auth bootstrap sets `simple`; OAuth2 login sets `oauth2`).
- **On login** — OAuth2 users have their type reaffirmed.

---

## Multi-Tenancy

Tenant-scoped role assignments and tenant policies together support multi-tenant deployments. Common patterns:

- **Geographic separation** — team A manages EU tenants, team B manages US tenants.
- **Customer isolation** — each customer has dedicated operators with policies limited to their tenant.
- **Environment separation** — dev/staging/prod tenants with separate operator groups.

Assigning an operator to a tenant requires two steps:

1. Assign the `operator` role to the user (global or scoped).
2. Create a tenant policy granting access to the specific tenant: `POST /api/rbac/users/{userID}/policies` with `tenant_id` and optionally `allowed_realms`.

---

## Frontend Integration

### Hooks

Source: `web/src/shared/hooks/usePermission.ts`.

| Hook | Returns |
|------|---------|
| `usePermission(permission: string \| string[])` | `{ hasPermission, isLoading }` — checks a single permission or any of an array |
| `useRole(roleName: string, tenantId?: string)` | `{ hasRole, isLoading }` — checks a specific role (optionally tenant-scoped) |
| `useUserRoles()` | `{ roles, isLoading }` |
| `useUserPermissions()` | `{ permissions, isLoading }` |
| `useIsAdmin()` | `{ isAdmin, isLoading }` |
| `useTenantAccess(tenantId: string)` | `{ hasAccess, isLoading }` |

### Route guards

- **`ProtectedRoute`** (`web/src/app/routes/ProtectedRoute.tsx`) — auth-only gate. Redirects unauthenticated users to login.
- **`PermissionRoute`** (`web/src/app/routes/PermissionRoute.tsx`) — permission gate. Renders `AccessDeniedPage` (403) if the user lacks the required permission. **Admins always bypass.**

### Permission constants

`web/src/shared/constants/permissions.ts` mirrors a subset of backend permissions. Coverage is partial — permissions for resources the UI does not expose (e.g. `policies:*`, `system_config:*`, `permissions:*`) are not present in the frontend constants. When adding frontend functionality that calls protected endpoints, add the relevant constant here.

---

## Recent Security Fixes

Two commits address cross-tenant isolation bugs identified during penetration testing:

### `281818d` (2026-02-19) — alerts, metrics, reports

- Added `RequireTenantAccess()` middleware to `alerts`, `operator` (metrics), and `report` handlers. Previously, any user with `alerts:read` could query alerts for **any** tenant by changing the tenant ID in the URL — pentest finding #2.
- Sanitized alert responses for non-admin users by stripping `Metadata`, `CheckType`, `RuleID`, and `EventID` — pentest finding #5.

### `7983324` (2026-03-06) — tenant detail routes

- Added `RequireTenantAccess()` to `GET/PUT/DELETE /api/tenants/{tenantID}` and `/health`. Commit message: *"Previously, users could access any tenant's information if they had the base permission, bypassing tenant policies."*

> **Design intent confirmed.** Tenant isolation is a security requirement, not a feature gap. Having a `*:read` permission is not equivalent to having access to a specific tenant's data — the `policies` table and `HasAccessToTenant` check are the authoritative mechanism.

---

## Audit Logging

At present, RBAC changes (role assignments, policy creation, role updates/deletes) are logged to the application logger at INFO level only (`rbac/service.go` `Assign*`, `CreatePolicy`, etc.). There is **no dedicated audit log table** or persistent audit trail for RBAC mutations. If compliance requirements demand a durable audit trail, this needs to be implemented separately.

---

## Best Practices

1. **Use least privilege.** Assign the most restrictive role that still lets the user do their job. Promote to `operator` or `admin` only when needed.
2. **Scope operators to tenants.** A bare `operator` role with no tenant policy grants zero data access. Always pair the role assignment with a tenant policy.
3. **Reserve simple auth for bootstrap.** In environments with OAuth2 available, use OAuth2 for regular users and keep simple auth only for emergency admin access.
4. **Add tenant policies through the API, not directly in the database.** The `/api/rbac/users/{userID}/policies` endpoint ensures proper validation.
5. **When adding new routes**, always apply `RequireTenantAccess()` to tenant-scoped endpoints in addition to permission checks. Permission checks alone do not enforce tenant isolation.
