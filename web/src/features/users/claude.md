# Feature: Users

## Domain
Keycloak user information and details.

## Responsibilities
- Display user details from Keycloak
- Show user roles and groups
- User session information
- User activity history

## Structure
```
users/
├── components/
│   ├── UserCard.tsx            → User info card
│   ├── UserRolesDisplay.tsx    → Roles and groups
│   ├── UserSessions.tsx        → Active sessions
│   └── UserActivity.tsx        → Recent activity
├── hooks/
│   └── useUserDetails.ts       → User data fetching
├── services/
│   └── userService.ts          → User API calls
├── types/
│   └── user.types.ts           → KeycloakUser, UserSession
├── pages/
│   └── UserDetailsPage.tsx     → User detail view
└── index.ts
```

## Public API (index.ts)
```tsx
export { UserDetailsPage } from './pages'
export { UserCard, UserRolesDisplay } from './components'
export { useUserDetails } from './hooks'
export type { KeycloakUser, UserSession } from './types'
```

## Key Types
```tsx
interface KeycloakUser {
  id: string
  username: string
  email?: string
  firstName?: string
  lastName?: string
  enabled: boolean
  emailVerified: boolean
  createdTimestamp: number
  attributes?: Record<string, string[]>
}

interface UserSession {
  id: string
  username: string
  ipAddress: string
  start: number
  lastAccess: number
  clients: Record<string, string>
}
```

## Navigation
Users are accessed from realm context:
`/:tenantId/realms/:realm/users/:userId`

## Data Source
User data comes from Keycloak Admin API:
- User info: `/admin/realms/:realm/users/:id`
- Sessions: `/admin/realms/:realm/users/:id/sessions`
- Role mappings: `/admin/realms/:realm/users/:id/role-mappings`

## Permissions
- `users:read` - View user details
- Inherits realm read permission
