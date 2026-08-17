# AMFA Events — Design Spec


|             |                                                                                                                          |
| ----------- | ------------------------------------------------------------------------------------------------------------------------ |
| **Date**    | 2026-05-26                                                                                                               |
| **Author**  | Jonas Figu (with Claude)                                                                                                 |
| **Status**  | Draft — awaiting user review                                                                                             |
| **Scope**   | Add an "AMFA Events" section to Keycloak Monitoring Tool with KPI metrics, paginated event logs, and a geolocation map. |
| **Touches** | Keycloak Monitoring Tool only. AMFA codebase is **not** modified.                                                       |


---

## 1. Goal

Keycloak Monitoring Tool administrators need a single page in the dashboard that surfaces authentication-risk activity coming from the AMFA (Adaptive MFA) service, so they can:

- See at a glance how many login attempts are happening and how many are risky.
- Audit recent AMFA login events with full context (user, IP, client, risk level).
- Visually identify the geographic distribution of login attempts to spot anomalies.

The page must be tenant-scoped (so per-customer admins see only their data), gated by RBAC, and follow the existing Keycloak Monitoring Tool architecture and patterns without breaking existing functionality.

## 2. Non-goals

- **Writes to AMFA.** Read-only first; actions on events (block IP, force MFA, etc.) are future work.
- **Real-time push** (websockets/SSE). 30 s polling via React Query is sufficient.
- **Historical backfill.** KMT shows whatever AMFA already has; no import step.
- **Cross-realm aggregation.** Page shows one realm at a time, matching the existing `EventsPage` behaviour.
- **Editing AMFA risk-evaluation parameters.** AMFA owns that UI/API; out of scope here.

## 3. Decisions made during brainstorming


| Decision           | Choice                                                                      | Rationale                                                                                                                                                                                                                                                                                                                             |
| ------------------ | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AMFA data access   | **Direct read from AMFA's PostgreSQL** (no AMFA code changes)               | User chose DB-direct over HTTP API for speed-to-ship and single-repo PR; coupling risks mitigated by read-only role + schema check                                                                                                                                                                                                    |
| AMFA DB credential | **Dedicated `kmt_ro` read-only role**                          | Defense in depth; ~30 s of DBA work for least-privilege guarantee                                                                                                                                                                                                                                                                     |
| Tenant model       | **One AMFA per Keycloak instance, owned by the same KMT tenant**          | Confirmed by user: deployments will run one AMFA service alongside each Keycloak. KMT therefore connects to a *different* AMFA database for each tenant. Realm-name collisions across tenants become impossible by topology (Keycloak enforces realm-name uniqueness within itself, and no other tenant's data lives in this AMFA). |
| User display       | UUIDs enriched with username/email via Keycloak admin client                | Better UX; reuses existing `pkg/keycloakadmin`                                                                                                                                                                                                                                                                                        |
| Map library        | `react-leaflet` + `leaflet.markercluster` with CartoDB Dark Matter tiles    | Free, no API key, mature, dark theme matches mockup, supports dynamic world→country→city zoom                                                                                                                                                                                                                                         |
| RBAC               | New `amfa:read` permission, granted to admin / operator / viewer by default | Future-proof: AMFA-specific data can be restricted later without affecting `events:read`                                                                                                                                                                                                                                              |
| Backend approach   | Pure pass-through (no KMT-side caching of AMFA data)                      | Simplest; AMFA stays source of truth; can add cache layer later if AMFA load becomes a concern                                                                                                                                                                                                                                        |


## 4. Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│  Keycloak Monitoring Tool Web (React)                                    │
│  features/amfa/  ── new isolated feature folder                  │
│    pages/AmfaEventsPage.tsx                                      │
│    components/  AmfaKpiRow, AmfaEventsTable, AmfaGeoMap, ...     │
│    hooks/       useAmfaEvents, useAmfaStats, useAmfaGeo          │
│    services/    amfaService.ts                                   │
│    types/       amfa.types.ts                                    │
└─────────────────────────────┬────────────────────────────────────┘
                              │  GET /api/tenants/{id}/amfa/events
                              │  GET /api/tenants/{id}/amfa/stats
                              │  GET /api/tenants/{id}/amfa/geo
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│  Keycloak Monitoring Tool API (Go, Chi)                                  │
│                                                                  │
│  internal/http/chi/amfa_handlers.go                              │
│    handlers gated by validateTenant + RequireTenantAccess +      │
│    RequirePermission("amfa:read")                                │
│                                                                  │
│  amfa/                          ── new root package              │
│    service.go                   orchestrates query + enrichment  │
│    types.go                     domain types                     │
│    enrichment.go                Keycloak user lookup + TTL cache │
│    interfaces.go                Repository interface             │
│    registry.go                  per-tenant GORM client registry  │
│    postgres/                                                     │
│      client.go                  per-tenant read-only GORM client │
│      repository.go              AMFA-table queries               │
│      models.go                  Read-only GORM models            │
│      schema_check.go            startup alembic_version assert   │
│                                                                  │
│  internal/config/models.go      Per-tenant amfa block on tenant  │
│  internal/fx/amfa.go            DI wiring                        │
│  rbac/constants.go              new amfa:read permission         │
└──────┬─────────────────────────────────────────┬─────────────────┘
       │                                         │
       │ Read-only Postgres                      │ Keycloak Admin API
       │ ONE connection per KMT tenant         │ (user enrichment, cached)
       │ Dispatched at request time              │ Per-tenant credentials
       ▼                                         ▼
┌──────────────────────────────────┐    ┌─────────────────────────┐
│  AMFA PostgreSQL                 │    │  Keycloak (per tenant)  │
│  ONE INSTANCE PER KMT TENANT   │    │  /admin/realms/{r}/users│
│  Connected via tenant-specific   │    │  via existing           │
│  read-only role                  │    │  keycloakadmin package  │
│                                  │    └─────────────────────────┘
│  Tables (read-only):             │
│  - auth_event                    │   Tenant ↔ AMFA mapping is
│  - auth_process                  │   1:1. Tenant A's AMFA never
│  - auth_context                  │   contains Tenant B's data.
│  - device                        │   Realm-name collisions are
│  - location_network              │   impossible by topology.
└──────────────────────────────────┘
```

## 5. Backend design

### 5.1 Configuration

AMFA is configured **per tenant**, alongside that tenant's existing Keycloak settings. Tenants without an `amfa` block simply don't get the feature — the page returns a "not configured" empty state for those tenants.

The new `AmfaTenantConfig` struct is added next to the existing per-tenant Keycloak fields in `internal/config/models.go`. No new top-level config section is needed.

```go
// internal/config/models.go (extending existing KeycloakTenant struct)
type KeycloakTenantConfig struct {
    // ... existing fields (ServerURL, AdminUsername, AdminPassword, etc.) ...

    // NEW: optional per-tenant AMFA configuration
    Amfa *AmfaTenantConfig `mapstructure:"amfa,omitempty"`
}

type AmfaTenantConfig struct {
    Enabled              bool               `mapstructure:"enabled"`
    Database             AmfaDatabaseConfig `mapstructure:"database"`
    EventsLookbackDays   int                `mapstructure:"events_lookback_days"`    // default 30, hard-capped at 90
    ExpectedSchemaVersion string            `mapstructure:"expected_schema_version"` // alembic head this AMFA was tested against
}

type AmfaDatabaseConfig struct {
    Host             string        `mapstructure:"host"`
    Port             int           `mapstructure:"port"`
    Database         string        `mapstructure:"database"`
    User             string        `mapstructure:"user"`
    Password         string        `mapstructure:"password"`        // from env: MONITORING_KEYCLOAK_TENANTS_<TENANT>_AMFA_DATABASE_PASSWORD
    SSLMode          string        `mapstructure:"sslmode"`         // default "require"
    MaxConns         int           `mapstructure:"max_conns"`       // default 5 per tenant
    MinConns         int           `mapstructure:"min_conns"`       // default 1 per tenant
    Timeout          time.Duration `mapstructure:"timeout"`         // default 10s
    ReadReplicaHost  string        `mapstructure:"read_replica_host"` // optional; preferred when set
}
```

`config.yaml.example` documents the new per-tenant `amfa:` block. Env-var overrides follow KMT's existing per-tenant Keycloak env-var pattern, so secrets like the AMFA DB password stay outside YAML.

### 5.2 New `amfa/` root package

Mirrors the layout of existing root packages (`events/`, `alerts/`, `keycloak/`):

```
amfa/
├── service.go          exposes Service interface used by HTTP handlers
├── types.go            domain types (Event, Stats, GeoBucket)
├── enrichment.go       Keycloak user lookup with TTL cache
├── interfaces.go       Repository interface
└── postgres/
    ├── client.go       opens a second GORM connection
    ├── repository.go   implements amfa.Repository
    ├── models.go       GORM models mirroring AMFA's schema (read-only)
    └── schema_check.go reads alembic_version, warns on mismatch
```

### 5.3 GORM models (read-only, mirror AMFA's schema)

All columns use `gorm:"->"` tag to enforce read-only at the ORM level, in addition to the DB-role-level read-only enforcement.

```go
// amfa/postgres/models.go
type AuthEvent struct {
    ID                  string    `gorm:"->;column:id;primaryKey"`
    AuthProcess         *string   `gorm:"->;column:auth_process"`
    UserID              *string   `gorm:"->;column:user_id"`
    EventType           string    `gorm:"->;column:event_type"`
    Details             *string   `gorm:"->;column:details"`
    AuthContextHash     *string   `gorm:"->;column:auth_context_hash"`
    DeviceInfoHash      *string   `gorm:"->;column:device_info_hash"`
    NetworkLocationHash *string   `gorm:"->;column:network_location_hash"`
    EventTime           time.Time `gorm:"->;column:event_time"`
}
func (AuthEvent) TableName() string { return "auth_event" }

type AuthProcess struct {
    ID                   string         `gorm:"->;column:id;primaryKey"`
    UserID               string         `gorm:"->;column:user_id"`
    AuthContextHash      string         `gorm:"->;column:auth_context_hash"`
    DeviceCredibility    *float64       `gorm:"->;column:device_credibility"`
    NetLocCredibility    *float64       `gorm:"->;column:net_loc_credibility"`
    AuthContextJSON      datatypes.JSON `gorm:"->;column:auth_context_json"`
    PreAuthRiskDecision  int            `gorm:"->;column:pre_auth_risk_decision"`
    FinalStatus          string         `gorm:"->;column:final_status"`
    StartedAt            time.Time      `gorm:"->;column:started_at"`
    FinishedAt           *time.Time     `gorm:"->;column:finished_at"`
}
func (AuthProcess) TableName() string { return "auth_process" }

type AuthContext struct {
    Hash            string   `gorm:"->;column:hash;primaryKey"`
    Client          string   `gorm:"->;column:client"`
    IPAddress       string   `gorm:"->;column:ip_address"`
    Lat             *float64 `gorm:"->;column:lat"`
    Long            *float64 `gorm:"->;column:long"`
    IsVPN           *bool    `gorm:"->;column:is_vpn"`
    CountryName     *string  `gorm:"->;column:country_name"`
    OperatingSystem *string  `gorm:"->;column:operating_system"`
    Browser         *string  `gorm:"->;column:browser"`
}
func (AuthContext) TableName() string { return "auth_context" }
```

`AutoMigrate` is **never** called on this DB client.

### 5.4 Repository interface and queries

```go
// amfa/interfaces.go
type Repository interface {
    ListEvents(ctx context.Context, opts ListEventsOptions) ([]EventRow, int64, error)
    GetStats(ctx context.Context, opts StatsOptions) (Stats, error)
    GetGeoBuckets(ctx context.Context, opts GeoOptions) ([]GeoBucket, error)
}

type ListEventsOptions struct {
    RealmID    string     // required; scopes via auth_context_json->>'realm_id'
    StartTime  *time.Time
    EndTime    *time.Time
    RiskLevel  *int       // optional; treated as ">=" filter
    EventType  string     // optional; LOGIN | LOGIN_ERROR | CLIENT_LOGIN | CLIENT_LOGIN_ERROR
    Limit      int
    Offset     int
}
```

#### Events list query

```sql
SELECT
    e.id, e.event_type, e.event_time, e.user_id,
    ac.ip_address, ac.country_name, ac.lat, ac.long, ac.client, ac.is_vpn,
    p.pre_auth_risk_decision AS risk_level,
    p.final_status
FROM auth_event e
LEFT JOIN auth_process p ON p.id = e.auth_process
LEFT JOIN auth_context ac ON ac.hash = e.auth_context_hash
WHERE p.auth_context_json->>'realm_id' = $1
  AND e.event_time BETWEEN $2 AND $3
ORDER BY e.event_time DESC
LIMIT $4 OFFSET $5
```

#### KPI stats query (single round-trip)

```sql
SELECT
    COUNT(*)                                                                            AS total,
    COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)                                AS risky,
    COUNT(DISTINCT e.user_id) FILTER (WHERE e.event_time >= NOW() - INTERVAL '24 hours') AS unique_users_24h,
    COUNT(DISTINCT ac.ip_address) FILTER (WHERE ac.is_vpn = true)                        AS flagged_ips
FROM auth_event e
LEFT JOIN auth_process p ON p.id = e.auth_process
LEFT JOIN auth_context ac ON ac.hash = e.auth_context_hash
WHERE p.auth_context_json->>'realm_id' = $1
  AND e.event_time BETWEEN $2 AND $3
```

#### Geo aggregation query

```sql
SELECT
    ac.country_name,
    ROUND(ac.lat::numeric,  1) AS lat_bucket,
    ROUND(ac.long::numeric, 1) AS long_bucket,
    COUNT(*)                                                       AS count,
    COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)          AS risky_count
FROM auth_event e
LEFT JOIN auth_process p  ON p.id = e.auth_process
LEFT JOIN auth_context ac ON ac.hash = e.auth_context_hash
WHERE p.auth_context_json->>'realm_id' = $1
  AND e.event_time BETWEEN $2 AND $3
  AND ac.lat IS NOT NULL AND ac.long IS NOT NULL
GROUP BY ac.country_name, lat_bucket, long_bucket
ORDER BY count DESC
```

Bucketing lat/long to 0.1° (~11 km cells) keeps map responses small while preserving city-level resolution.

### 5.5 Schema-version check

`amfa/postgres/schema_check.go` runs once during fx `OnStart`:

```go
const ExpectedAmfaSchemaVersion = "c00d6d7d197c" // populated from config

func CheckAmfaSchema(db *gorm.DB, expected string, log *logger.Logger) {
    var actual string
    if err := db.Raw("SELECT version_num FROM alembic_version").Scan(&actual).Error; err != nil {
        log.Warn("Could not verify AMFA schema version", logger.Err(err))
        return
    }
    if actual != expected {
        log.Warn("AMFA schema version drift detected",
            logger.Str("expected", expected),
            logger.Str("actual",   actual))
        return
    }
    log.Info("AMFA schema version matches", logger.Str("version", actual))
}
```

Outcomes:

- Match → INFO, no further action.
- Drift → WARN; service continues. UI shows an admin-visible banner via a `/amfa/health` endpoint (small follow-up).
- Read error → WARN; treated as "could not verify."

Never fatal — KMT always boots.

### 5.6 Service layer

`amfa/service.go` orchestrates each request. Because each tenant has its own AMFA, the service uses a **per-tenant repository registry** (`amfa/registry.go`) to dispatch to the correct database connection:

1. Resolve `tenantID` → tenant-specific `Repository` from the registry.
  - If the tenant has no AMFA config: return `ErrAmfaNotConfigured` (handler maps to HTTP 404 with a clear message).
2. Call `Repository.ListEvents(...)` / `GetStats(...)` / `GetGeoBuckets(...)` against that tenant's AMFA DB.
3. For `ListEvents`, collect unique `user_id`s and call `enrichment.BatchEnrich(ctx, tenantID, realm, userIDs)` — enrichment uses the existing per-tenant Keycloak admin client, naturally aligned with the per-tenant AMFA.
4. Zip enrichment into rows; return DTOs.

The registry (`amfa/registry.go`) is constructed once at startup. For each tenant with `amfa.enabled = true` in config, it opens a GORM read-only connection and runs the schema check. Tenants with no AMFA block or `enabled: false` aren't represented in the registry — the service simply returns the not-configured error for those tenant IDs.

### 5.7 Keycloak user enrichment (`amfa/enrichment.go`)

Reuses existing `pkg/keycloakadmin` client. Adds:

- In-memory TTL cache keyed by `tenantID|realm|userID`.
- 5-minute TTL on successful lookups, 30 s TTL on 404s (deleted users) to prevent retry storms.
- Per-page dedup: a user appearing 5 times on one page costs one Keycloak call.
- No persistence; per-replica state.

### 5.8 HTTP handlers

`internal/http/chi/amfa_handlers.go` follows the existing tenant-scoped handler shape (mirrors `system_handlers.go`):

```go
func (h *AmfaHandlers) RegisterTenantRoutes(r chi.Router) {
    r.Group(func(r chi.Router) {
        r.Use(h.authMiddleware)
        r.Use(h.validateTenantMiddleware)
        r.Use(h.rbacMiddleware.RequireTenantAccess())
        r.Use(h.rbacMiddleware.RequirePermission("amfa:read"))

        r.Get("/amfa/events", h.handleAmfaEvents)
        r.Get("/amfa/stats",  h.handleAmfaStats)
        r.Get("/amfa/geo",    h.handleAmfaGeo)
    })
}
```

Query params on all three: `realm_id` (required), `start_time`, `end_time`. `/events` additionally accepts `limit`, `offset`, `event_type`, `risk_level`.

If `amfa.enabled = false`, routes are not registered, and the page is gated by absence of the permission.

### 5.9 Dependency injection (`internal/fx/amfa.go`)

A new fx module provides:

- `amfa.Registry` — built by iterating `cfg.Keycloak.Tenants`. For each tenant whose `amfa.enabled == true`, it opens a read-only GORM connection, runs `schema_check.CheckAmfaSchema`, and registers the resulting `amfa.Repository` keyed by `tenant_id`. Tenants without an `amfa:` block are skipped silently. Connection or schema-check failures are logged but don't abort startup — only that specific tenant's AMFA endpoints will return 503.
- `amfa.UserEnrichment` — wraps the existing per-tenant `pkg/keycloakadmin` clients with the TTL cache.
- `amfa.Service` — combines registry + enrichment behind the `Service` interface used by handlers.
- `*chi.AmfaHandlers` — always registered by the router (sidebar/page remain visible even when no tenants have AMFA).
- An `fx.Invoke` that logs at startup how many tenants have AMFA enabled, for operator visibility.

If **no** tenants have `amfa.enabled == true`, the registry is empty but the module still wires cleanly. Handlers respond "not configured" (404) for every tenant. The rest of the system is entirely unaffected.

### 5.10 RBAC

`rbac/constants.go` gets:

```go
const PermAmfaRead = "amfa:read"
```

Added to the permission list returned by `GetSystemPermissions()`. The `admin`, `operator`, and `viewer` entries in `GetSystemRoles()` each receive `PermAmfaRead`. Existing role-permission rows in the database are migrated to include `amfa:read` via a new data-migration in `pkg/database/client.go` (idempotent, runs once).

## 6. Frontend design

### 6.1 Feature folder

```
web/src/features/amfa/
├── pages/
│   ├── AmfaEventsPage.tsx
│   └── index.ts
├── components/
│   ├── AmfaKpiRow.tsx
│   ├── AmfaEventsTable.tsx
│   ├── AmfaGeoMap.tsx
│   ├── AmfaEventDetailsDialog.tsx
│   └── AmfaRiskBadge.tsx
├── hooks/
│   ├── useAmfaEvents.ts
│   ├── useAmfaStats.ts
│   └── useAmfaGeo.ts
├── services/
│   └── amfaService.ts
├── types/
│   └── amfa.types.ts
├── claude.md
└── index.ts
```

### 6.2 Types

```ts
export interface AmfaEvent {
  event_id: string;
  event_time: string;                           // ISO 8601
  event_type: "LOGIN" | "LOGIN_ERROR" | "CLIENT_LOGIN" | "CLIENT_LOGIN_ERROR";
  user_id: string | null;
  username: string | null;                      // enriched by KMT
  email: string | null;                         // enriched by KMT
  client: string;
  ip_address: string;
  country: string | null;
  city: string | null;
  lat: number | null;
  long: number | null;
  is_vpn: boolean;
  risk_level: 1 | 2 | 3 | 4 | null;
  final_status: "LOGIN" | "LOGIN_ERROR" | null;
}

export interface AmfaStats {
  total: number;
  risky: number;                                // risk_level >= 3
  unique_users_24h: number;
  flagged_ips: number;
}

export interface AmfaGeoBucket {
  country: string;
  city: string | null;
  lat: number;
  long: number;
  count: number;
  risky_count: number;
}

export interface AmfaEventsResponse {
  items: AmfaEvent[];
  total: number;
  limit: number;
  offset: number;
}
```

### 6.3 Page layout

```
┌─ PageHeader ─────────────────────────────────────────────────────┐
│ AMFA Events    [RealmSelector] [TimeSelector] [Refresh]          │
└──────────────────────────────────────────────────────────────────┘

┌─ AmfaKpiRow (StatsCardGrid) ─────────────────────────────────────┐
│ ┌───────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐       │
│ │ Total     │ │ Risky 3+   │ │ Unique     │ │ Flagged    │       │
│ │ Login     │ │            │ │ Users 24h  │ │ IPs        │       │
│ │ 1,247     │ │ 38         │ │ 312        │ │ 7          │       │
│ └───────────┘ └────────────┘ └────────────┘ └────────────┘       │
└──────────────────────────────────────────────────────────────────┘

┌─ md+ : two columns; xs/sm : stacked ─────────────────────────────┐
│ ┌─ AmfaEventsTable ────────┐  ┌─ AmfaGeoMap ──────────────────┐  │
│ │ Time | User | Type | IP  │  │  react-leaflet + CartoDB Dark │  │
│ │ Client | Risk            │  │  marker clusters by location  │  │
│ │ pagination               │  │  zoom controls + fullscreen   │  │
│ └──────────────────────────┘  └───────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

### 6.4 Hooks (React Query)

```ts
export const useAmfaEvents = (params) =>
  useQuery({
    queryKey: ["amfa", "events", params],
    queryFn: () => amfaService.getEvents(params),
    enabled: !!params.tenantId && !!params.realm,
    refetchInterval: 30_000,
  });
```

`useAmfaStats` and `useAmfaGeo` follow the same shape. 30 s refetch interval matches the existing Events page.

### 6.5 Service layer

```ts
class AmfaService {
  getEvents(p) { return apiClient.get(`/tenants/${p.tenantId}/amfa/events`, toQueryParams(p)); }
  getStats(p)  { return apiClient.get(`/tenants/${p.tenantId}/amfa/stats`,  toQueryParams(p)); }
  getGeo(p)    { return apiClient.get(`/tenants/${p.tenantId}/amfa/geo`,    toQueryParams(p)); }
}
```

Pure HTTP, no React. Matches `eventService.ts`.

### 6.6 Table

`MaterialReactTable` with the same `MRT_OPTIONS_WITH_TOOLBAR` and pagination affordances as `EventsPage.tsx`. Columns:


| Column     | Source       | Notes                                             |
| ---------- | ------------ | ------------------------------------------------- |
| Time       | `event_time` | Pre-formatted, monospace                          |
| User       | `username    |                                                   |
| Event Type | `event_type` | Colored MUI Chip                                  |
| IP Address | `ip_address` | Monospace                                         |
| Client     | `client`     | Monospace, dimmed                                 |
| Risk Level | `risk_level` | `AmfaRiskBadge`: 1 green, 2 blue, 3 orange, 4 red |
| Actions    | —            | Eye icon → details dialog                         |


Row click opens `AmfaEventDetailsDialog` (same pattern as the existing Events page).

### 6.7 Map

```tsx
<MapContainer center={[20, 0]} zoom={2} scrollWheelZoom>
  <TileLayer
    attribution="&copy; OpenStreetMap, &copy; CARTO"
    url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
  />
  <MarkerClusterGroup>
    {geoBuckets.map(b => (
      <Marker key={`${b.lat},${b.long}`} position={[b.lat, b.long]} icon={iconFor(b)}>
        <Tooltip permanent direction="top">
          {b.city ?? b.country}: {b.count}
          {b.risky_count > 0 && ` (⚠ ${b.risky_count})`}
        </Tooltip>
        <Popup>
          <strong>{b.city ?? b.country}</strong><br/>
          Total events: {b.count}<br/>
          Risky (3+): {b.risky_count}
        </Popup>
      </Marker>
    ))}
  </MarkerClusterGroup>
</MapContainer>
```

- World view on load (`center=[20,0], zoom=2`); panning and dynamic zoom via Leaflet.
- Clustering collapses dense locations at low zoom, decomposes as the user zooms in.
- Marker icon turns orange when `risky_count > 0`, default blue otherwise.
- Parent grid cell sets `minHeight: 600px`.

### 6.8 Routing

`app/routes/routes.tsx` adds:

```tsx
<Route
  path="/:tenantId/amfa-events"
  element={
    <PermissionRoute permission={PERMISSIONS.AMFA.READ}>
      <RouteErrorBoundary routeId="AmfaEvents">
        <AmfaEventsPage />
      </RouteErrorBoundary>
    </PermissionRoute>
  }
/>
```

`shared/constants/permissions.ts` adds:

```ts
export const AMFA_PERMISSIONS = { READ: "amfa:read" } as const;
// folded into PERMISSIONS
```

### 6.9 Sidebar

`shared/components/Layout/Layout.tsx`, the `navigation` array gets one new entry after `Events`:

```ts
{ name: "AMFA Events", path: "/amfa-events", icon: <Shield /> },
```

Users without `amfa:read` get the existing `AccessDenied` page on click. Sidebar permission-aware hiding is optional follow-up.

### 6.10 Dependencies to add

`web/package.json`:

```
"leaflet": "^1.9.x",
"react-leaflet": "^4.x",
"leaflet.markercluster": "^1.5.x",
"@types/leaflet": "^1.9.x",
"@types/leaflet.markercluster": "^1.5.x"
```

Leaflet CSS is imported once at the feature page level.

## 7. Data flow (happy path)

```
User opens /tenants/prod-keycloak/amfa-events
  ├─► AmfaEventsPage mounts; reads tenantId, realm
  ├─► 3 parallel React Query GETs
  │
  ▼
Web Server (7880) → API Server (7888)
  ├─ authMiddleware
  ├─ validateTenantMiddleware
  ├─ RequireTenantAccess
  └─ RequirePermission("amfa:read")
  │
  ▼
AmfaHandlers.handle* → amfa.Service.*
  ├─► registry.RepositoryFor(tenantID)
  │     └─ resolves to this tenant's AMFA GORM connection
  │        (or returns ErrAmfaNotConfigured → 404 if tenant has no amfa: block)
  ├─► repo.ListEvents (Postgres SELECT against THIS tenant's AMFA tables)
  └─► enrichment.BatchEnrich (Keycloak admin lookups via THIS tenant's
                              keycloakadmin client, cached)
  │
  ▼
JSON response → React Query → MaterialReactTable / KPI cards / Leaflet
```

## 8. Errors and edge cases


| Failure point                                   | Behavior                                                                                                            | User sees                                                                                 |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Tenant has no `amfa:` block in config           | Handler returns 404 `{error: "amfa_not_configured"}`                                                                | Empty-state: "AMFA Events is not configured for this tenant. Contact your administrator." |
| AMFA DB unreachable at startup (for one tenant) | KMT still boots; that tenant's AMFA registry entry is absent; its endpoints return 503. Other tenants unaffected. | KPIs `—`; table empty state "AMFA data source unavailable"; map empty                     |
| AMFA DB unreachable mid-request                 | Handler returns 503 `{error: "amfa_unavailable"}`                                                                   | Toast + React Query retry on next 30 s tick                                               |
| Schema-version drift                            | Boots; WARN log; admin-visible banner                                                                               | "AMFA schema may have changed" banner; page still works for known fields                  |
| Keycloak admin call fails                       | WARN log; fall back to UUID only                                                                                    | UUID with "(?)" hint; rest of row renders                                                 |
| Permission denied                               | `PermissionRoute` short-circuits                                                                                    | Existing `<AccessDenied>` page                                                            |
| No realm selected                               | Hooks `enabled: false` — no requests                                                                                | "Select a realm to view AMFA data"                                                        |
| Historical row missing `realm_id` in JSON       | Skipped by realm filter                                                                                             | Counts may be lower than raw event count — documented                                     |
| Row with null `lat`/`long`                      | Excluded from `/geo`; still in events table                                                                         | Map count ≤ events count                                                                  |
| Time range > 90 d                               | Server clamps to 90 d; warning in response                                                                          | TimeSelector clamps; small notice "Showing last 90 days max"                              |


## 9. Performance budgets


| Endpoint                     | Median target | P95 target |
| ---------------------------- | ------------- | ---------- |
| `GET /amfa/stats`            | < 150 ms      | < 500 ms   |
| `GET /amfa/events` (25 rows) | < 300 ms      | < 800 ms   |
| `GET /amfa/geo`              | < 300 ms      | < 700 ms   |


Knobs to turn if missed:

- Recommended functional index (operations-time, on AMFA's DB):
  ```sql
  CREATE INDEX IF NOT EXISTS idx_auth_process_realm_id
      ON auth_process ((auth_context_json->>'realm_id'));
  ```
- Move KMT to AMFA's read replica.
- Add a short-TTL cache layer in KMT (deferred until needed).

## 10. Deployment

### 10.1 One-time setup on **each** AMFA's PostgreSQL (run by DBA, once per AMFA instance)

```sql
-- Run on every AMFA database that KMT will read from.
-- Each KMT tenant has its own AMFA, so this is run N times for N tenants.
CREATE USER kmt_ro WITH PASSWORD '<strong-pw>';
GRANT CONNECT ON DATABASE adaptive_mfa TO kmt_ro;
GRANT USAGE ON SCHEMA public TO kmt_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO kmt_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO kmt_ro;

-- Recommended for query performance:
CREATE INDEX IF NOT EXISTS idx_auth_process_realm_id
    ON auth_process ((auth_context_json->>'realm_id'));
```

### 10.2 Keycloak Monitoring Tool per-tenant config

AMFA is configured per-tenant inside the existing `keycloak.tenants` section. Example for two tenants — one with AMFA, one without:

```yaml
keycloak:
  tenants:
    prod-keycloak:
      enabled: true
      server_url: "https://kc-prod.example.com"
      admin_username: "admin"
      admin_password: "${KC_PROD_ADMIN_PASSWORD}"
      amfa:
        enabled: true
        events_lookback_days: 30
        expected_schema_version: "c00d6d7d197c"
        database:
          host: amfa-prod.internal
          port: 5432
          database: adaptive_mfa
          user: kmt_ro
          password: "${PROD_AMFA_DB_PASSWORD}"
          sslmode: require
          read_replica_host: amfa-prod-replica.internal   # optional
    legacy-tenant:
      enabled: true
      server_url: "https://kc-legacy.example.com"
      admin_password: "${KC_LEGACY_ADMIN_PASSWORD}"
      # no amfa: block → AMFA Events page returns "not configured" for this tenant
```

Secrets (DB passwords) come from env vars using KMT's existing per-tenant override pattern. AMFA-related env vars only — no existing env-var conventions are changed.

### 10.3 Verification

Expected KMT startup log lines:

```
[INFO] AMFA enabled for 1 tenant(s)
[INFO] AMFA database connected: tenant=prod-keycloak host=amfa-prod.internal db=adaptive_mfa
[INFO] AMFA schema version matches: tenant=prod-keycloak version=c00d6d7d197c
```

If a tenant has `amfa.enabled: true` but its AMFA is unreachable:

```
[ERROR] AMFA database connection failed: tenant=prod-keycloak err=<...>
[WARN]  AMFA endpoints for tenant=prod-keycloak will return 503
```

### 10.4 Rollback (per tenant)

To disable for a single tenant, set its `amfa.enabled: false`, restart KMT. Other tenants keep working. To fully disable, remove or set `enabled: false` on every tenant's `amfa:` block. AMFA itself is never touched, so there's nothing to roll back on the AMFA side. Each AMFA's read-only DB role can stay or be revoked at leisure.

### 10.5 Feature flag

`amfa.enabled` defaults to `false` per tenant. Tenants without an `amfa:` block are equivalent to disabled. Standard "enabled per tenant per environment" pattern, consistent with how KMT already gates other per-tenant features.

## 11. Testing strategy

**Backend:**

- `amfa/postgres/repository_test.go` — integration test against a Dockerized AMFA-shaped Postgres (use AMFA's docker-compose for fixtures); covers events list, stats, geo aggregation.
- `amfa/service_test.go` — unit tests with mocked `Repository` and mocked `keycloakadmin` client; covers enrichment, dedup, fallback on Keycloak failure.
- `amfa/enrichment_test.go` — cache hit/miss/expiry behavior; negative cache for 404s.
- `internal/http/chi/amfa_handlers_test.go` — mirrors `system_handlers_test.go`; covers auth chain, query-param parsing, error mapping, response shape.
- `amfa/postgres/schema_check_test.go` — match / drift / table-missing paths.

**Frontend:**

- `AmfaEventsPage.test.tsx` — renders with mocked hooks; KPIs, table, map container present.
- `AmfaKpiRow.test.tsx` — formatting (thousands separators).
- `AmfaEventsTable.test.tsx` — row click opens dialog; risk badge colors per level.
- `AmfaGeoMap.test.tsx` — `react-leaflet` exports mocked; verifies `Marker` count matches props.
- `amfaService.test.ts` — URL and query-param construction for each endpoint.

## 12. Files changed (summary)

**New (Keycloak Monitoring Tool):**

```
amfa/
  service.go
  types.go
  enrichment.go
  interfaces.go
  registry.go            ── per-tenant repository registry
  postgres/
    client.go
    repository.go
    models.go
    schema_check.go

internal/http/chi/amfa_handlers.go
internal/http/chi/amfa_handlers_test.go
internal/fx/amfa.go

docs/amfa-integration.md            (new operator-facing doc)

web/src/features/amfa/
  pages/AmfaEventsPage.tsx
  pages/AmfaEventsPage.test.tsx
  pages/index.ts
  components/AmfaKpiRow.tsx
  components/AmfaKpiRow.test.tsx
  components/AmfaEventsTable.tsx
  components/AmfaEventsTable.test.tsx
  components/AmfaGeoMap.tsx
  components/AmfaGeoMap.test.tsx
  components/AmfaEventDetailsDialog.tsx
  components/AmfaRiskBadge.tsx
  hooks/useAmfaEvents.ts
  hooks/useAmfaStats.ts
  hooks/useAmfaGeo.ts
  services/amfaService.ts
  services/amfaService.test.ts
  types/amfa.types.ts
  claude.md
  index.ts
```

**Modified (Keycloak Monitoring Tool, additive only — no existing logic changes):**

```
internal/config/models.go              + optional Amfa *AmfaTenantConfig field on
                                         existing KeycloakTenantConfig (pointer,
                                         omitempty; nil for tenants without AMFA)
internal/fx/app.go                     + new module import
internal/fx/repositories.go            + amfa registry wiring (or new file)
rbac/constants.go                      + new PermAmfaRead constant
rbac/service.go (or seeder)            + grant amfa:read to system roles (additive)
pkg/database/client.go                 + new idempotent data migration to grant
                                         amfa:read on existing role rows

config.yaml.example                    + documented per-tenant amfa block

web/src/app/routes/routes.tsx          + new route entry
web/src/shared/constants/permissions.ts + new AMFA_PERMISSIONS constant
web/src/shared/components/Layout/Layout.tsx + new sidebar entry
web/package.json                       + leaflet, react-leaflet, leaflet.markercluster (+ types)

docs/architecture.md                   + new section describing AMFA module
```

**Not modified (zero changes):**

- AMFA codebase: zero.
- Existing KMT logic for auth, tenants, events, alerts, keycloak monitoring, RBAC enforcement: zero behavior changes.
- Existing tenant configurations (without an `amfa:` block) continue to work exactly as before.

## 13. Open items / known follow-ups

- Sidebar permission filtering (cosmetic — hide "AMFA Events" when permission missing). Optional.
- Cache layer between KMT and AMFA's DB (only if performance becomes an issue).
- AMFA write actions from KMT (e.g., "block this IP") — explicit follow-up feature.
- **Automatic KMT alerts from high-risk AMFA events** — wire AMFA events with `risk_level >= 3` (or repeated `LOGIN_ERROR` patterns) into KMT's existing `alerts/` package so they flow through the standard alert lifecycle (Slack / Email / GitLab notifications, ack/resolve workflow). This MVP surfaces risky events in the dashboard only; alert generation is a clean follow-up that builds on the existing notification infrastructure.
- Real-time updates via SSE — only if 30 s polling proves insufficient.

## 14. Risks acknowledged


| Risk                                                                                                          | Mitigation in this design                                                                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| AMFA renames columns / restructures tables                                                                    | Per-tenant schema-version check at startup logs immediate WARN; read-only role prevents accidental writes; spec mandates KMT CI runs against expected AMFA schema                                                                                            |
| AMFA changes the JSON key name in `auth_context_json` (e.g. `realm_id` → `realmId`) — silently returns 0 rows | Known risk introduced by DB-direct integration. Documented as future improvement; alembic version check catches DDL but not JSON-shape changes. A stricter probe could be added (`SELECT auth_context_json->>'realm_id' FROM auth_process LIMIT 1`) if needed. |
| Heavy KMT queries impact AMFA's live `/decision` latency                                                    | Per-tenant `read_replica_host` config option; recommended `idx_auth_process_realm_id` functional index on each AMFA's database                                                                                                                                 |
| Persistent DB credentials per tenant in KMT config                                                          | Per-tenant least-privilege read-only role; rotatable per tenant without coordinating with AMFA                                                                                                                                                                 |
| User UUID lookup hammering Keycloak                                                                           | TTL cache + per-page dedup + negative caching of 404s                                                                                                                                                                                                          |
| Historical AMFA rows without `realm_id` in JSON                                                               | Documented limitation; counts may be lower than raw event count                                                                                                                                                                                                |
| One tenant's AMFA being down affects only that tenant                                                         | Registry isolates per-tenant connections; failures in one tenant's AMFA never affect others                                                                                                                                                                    |






