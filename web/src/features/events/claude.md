# Feature: Events

## Domain
Event stream and activity logs from Keycloak.

## Responsibilities
- Event listing with pagination
- Event filtering by type, realm, time
- Event timeline visualization
- Event statistics

## Structure
```
events/
├── components/
│   ├── EventCard.tsx           → Event display
│   ├── EventTimeline.tsx       → Timeline visualization
│   ├── EventTypeBadge.tsx      → Event type indicator
│   └── EventFilters.tsx        → Filter controls
├── hooks/
│   ├── useEvents.ts            → Event list
│   └── useEventStats.ts        → Event statistics
├── services/
│   └── eventService.ts         → Event API calls
├── types/
│   └── event.types.ts          → Event, EventType, Stats
├── pages/
│   └── EventsPage.tsx          → Events list page
└── index.ts
```

## Public API (index.ts)
```tsx
export { EventsPage } from './pages'
export { EventCard, EventTimeline } from './components'
export { useEvents, useEventStats } from './hooks'
export type { Event, EventType, EventStats } from './types'
```

## Key Types
```tsx
interface Event {
  id: string
  type: string
  realmId: string
  userId?: string
  clientId?: string
  ipAddress?: string
  details: Record<string, unknown>
  time: string
}

interface EventStats {
  total: number
  byType: Record<string, number>
  byRealm: Record<string, number>
  timeline: TimelineBucket[]
}
```

## Event Types (Keycloak)
Common event types:
- `LOGIN` - User login
- `LOGIN_ERROR` - Failed login
- `LOGOUT` - User logout
- `REGISTER` - User registration
- `UPDATE_PASSWORD` - Password change
- `CLIENT_LOGIN` - Client authentication

## Filtering
- Time range (start/end)
- Event type
- Realm
- Source
- Pagination (limit/offset)

## Permissions
- `events:read` - View events
