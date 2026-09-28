# Feature: Events

## Domain
Event stream and activity logs from Keycloak, unified with AMFA login events.

## Responsibilities
- Event listing with pagination and filtering (type, realm, source, time range)
- Event detail dialog, including the AMFA section for risk-scored logins
- AMFA KPI strip and geolocation map for AMFA-enabled realms

## Structure
```
events/
├── pages/
│   ├── EventsPage.tsx          → the events list, filters, detail dialog
│   ├── EventsPage.test.tsx     → colocated tests for the above
│   └── index.ts
├── services/
│   ├── eventService.ts         → UNUSED, see below
│   └── index.ts
├── types/
│   ├── event.types.ts          → Event, EventsResponse, Stats
│   └── index.ts
└── index.ts
```

`services/eventService.ts` is dead code. Nothing imports it, `index.ts` does
not export it, and `EventsPage` imports no service at all. It is a leftover
duplicate of `@/shared/services/eventsService`, which is the one actually in
use — `useEvents` and `useEventStats` both call that. Read the shared service,
not this file, and delete this one rather than extending it.

There is no `components/` or `hooks/` directory here. The hooks `EventsPage`
uses live in `@/shared/hooks` — `useEvents`, `useAmfaStats`, `useAmfaGeo`,
`useAmfaRealms`, `useKeycloakDashboard`, `useRealmSelector`, `useSessionStorage`,
plus the `isAllRealmsUnsupportedError` and `isRealmScopeRequiresRealmError`
error helpers — and the presentational pieces it renders (`RiskBadge`,
`AmfaKpiRow`, `AmfaGeoMap`, `AlertBanner`, `PageHeader`, `RealmSelector`,
`TimeSelector`) come from `@/shared/components`.

`useEventStats` also lives in `@/shared/hooks` and consumes a `Stats` type of
the same shape, but it is called from the dashboard, not from this feature, so
statistics are not this feature's concern despite the type living here.

## Public API (index.ts)
```tsx
export { EventsPage } from "./pages";
export type { Event, EventsResponse, Stats } from "./types";
```

## Key Types
`Event` is deliberately an alias of the canonical shared `KeycloakEvent` rather
than its own definition — the two previously drifted as manually-synced copies.
Field names are snake_case because they come straight off the API.

`Stats` has not had the same treatment: an identical copy is declared in
`@/shared/services/eventsService`, and that is the one `useEventStats` returns.
The two are in sync today by hand, which is exactly the drift the `Event` alias
was introduced to stop.

```tsx
import type { KeycloakEvent } from "@/shared/types";

export type Event = KeycloakEvent;

export interface EventsResponse {
  events: Event[];
  count: number;
  total: number;
}

export interface Stats {
  total_events: number;
  by_severity?: Record<string, number>;
  by_type?: Record<string, number>;
  by_source?: Record<string, number>;
}
```

`KeycloakEvent` carries the Keycloak fields (`event_id`, `timestamp`, `type`,
`category`, `severity`, `source`, `source_ip`, `source_system`, `user_id`,
`username`, `client_id`, `status`, …) plus optional AMFA fields that are only
present once the backend has merged an AMFA counterpart, or when the login was
risk-scored: `amfa_event_id`, `risk_level`, `is_vpn`, `country`, `city`, `lat`,
`long`, `operating_system`, `browser`, `device`, `system_language`,
`screen_resolution`. See `@/shared/types/keycloak.types.ts` for the full shape.

Treat every AMFA field as possibly absent. The detail dialog gates the AMFA
section on `amfa_event_id || risk_level != null`, and the VPN chip on
`is_vpn != null`, because the open-source Adaptive MFA SPI does not stamp
`amfa_event_id` onto the login event — an event can carry a risk level and
nothing else.

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
