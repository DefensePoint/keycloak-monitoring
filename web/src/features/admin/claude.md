# Feature: Admin

## Domain
Administrative functions for platform management.

## Responsibilities
- Platform user management
- Role and permission management
- System configuration
- Audit logs

## Structure
```
admin/
├── users/
│   ├── components/
│   │   ├── UserManagement.tsx    → User CRUD table
│   │   ├── UserForm.tsx          → Create/edit form
│   │   └── UserRoleAssign.tsx    → Role assignment
│   ├── hooks/
│   │   └── useAdminUsers.ts      → Admin user operations
│   ├── pages/
│   │   └── AdminUsersPage.tsx    → Users management page
│   └── index.ts
│
├── roles/
│   ├── components/
│   │   ├── RoleManagement.tsx    → Role CRUD table
│   │   ├── RoleForm.tsx          → Create/edit form
│   │   └── PermissionMatrix.tsx  → Permission assignment
│   ├── hooks/
│   │   └── useAdminRoles.ts      → Admin role operations
│   ├── pages/
│   │   └── AdminRolesPage.tsx    → Roles management page
│   └── index.ts
│
└── index.ts
```

## Public API (index.ts)
```tsx
// Users
export { AdminUsersPage } from './users/pages'
export { useAdminUsers } from './users/hooks'

// Roles
export { AdminRolesPage } from './roles/pages'
export { useAdminRoles } from './roles/hooks'
```

## Access Control
Admin features require `admin` role or specific permissions:
- `admin:users:read` - View platform users
- `admin:users:write` - Manage users
- `admin:roles:read` - View roles
- `admin:roles:write` - Manage roles

## Routes
Admin routes are nested under `/admin`:
- `/admin/users` - User management
- `/admin/roles` - Role management

## Platform Users vs Keycloak Users
**Important distinction:**
- **Platform users**: Users of this monitoring platform (managed here)
- **Keycloak users**: Users in monitored Keycloak instances (read-only, in `features/users`)

## RBAC Model
```tsx
interface PlatformRole {
  id: string
  name: string
  description: string
  permissions: string[]
  isSystem: boolean  // Built-in roles cannot be deleted
}

interface PlatformUser {
  id: string
  username: string
  email: string
  roles: PlatformRole[]
  tenantAccess: string[]  // Which tenants can access
  createdAt: string
  lastLogin?: string
}
```

## Built-in Roles
- `admin` - Full access to everything
- `operator` - Can manage alerts and view all data
- `viewer` - Read-only access
