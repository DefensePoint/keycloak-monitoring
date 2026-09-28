# Feature: Auth

## Domain
Authentication, authorization, and user session management.

## Responsibilities
- Login (simple + OAuth2)
- Logout
- Session management
- Permission verification (RBAC)
- Authenticated user context

## Structure
```
auth/
├── components/
│   ├── LoginForm.tsx        → Login form
│   └── OAuthButton.tsx      → OAuth2 button
├── hooks/
│   ├── useAuth.ts           → Authentication state
│   └── usePermission.ts     → Permission verification
├── services/
│   └── authService.ts       → Authentication API
├── context/
│   └── AuthContext.tsx      → Global auth provider
├── types/
│   └── auth.types.ts        → User, Permission, Role
├── pages/
│   └── LoginPage.tsx        → Login page
└── index.ts
```

## Public API (index.ts)
```tsx
export { LoginPage } from './pages/LoginPage'
export { useAuth, usePermission, useIsAdmin } from './hooks'
export { AuthProvider, AuthContext } from './context'
export type { User, Permission, Role, AuthState } from './types'
```

## Integrations
- **Backend**: `/auth/*` endpoints
- **Methods**: Simple (username/password) + OAuth2/OIDC

## Usage Patterns

### Check authentication
```tsx
const { user, isAuthenticated, isLoading } = useAuth()
```

### Check permission
```tsx
const canEdit = usePermission('alerts:write')
const isAdmin = useIsAdmin()
```

### Component with guard
```tsx
import { PermissionGate } from '@/shared/components'

<PermissionGate permission="alerts:write">
  <EditButton />
</PermissionGate>
```

## RBAC Model
- **Permissions**: Granular actions (e.g., `alerts:read`, `alerts:write`)
- **Roles**: Groups of permissions (e.g., `admin`, `viewer`)
- **Tenant-scoped**: Permissions can be tenant-specific
