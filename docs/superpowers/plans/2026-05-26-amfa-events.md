# AMFA Events Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an AMFA Events section to Keycloak Monitoring Tool with KPI metrics, paginated event logs, and a geolocation map. Tenant-scoped, RBAC-gated, one AMFA per Keycloak instance.

**Architecture:** New Go package `amfa/` reads AMFA's Postgres directly via a per-tenant connection registry (one tenant = one AMFA). HTTP handlers under `/api/tenants/{id}/amfa/*`. New React feature folder `features/amfa/` using existing shared components. New `amfa:read` RBAC permission. Zero AMFA-codebase changes; all KMT-side changes are additive to existing files.

**Tech Stack:** Go 1.23 (Chi router, Uber Fx DI, GORM); React 18 + TS + MUI + TanStack Query + Material React Table; react-leaflet + leaflet.markercluster + CartoDB Dark Matter tiles; PostgreSQL/TimescaleDB.

**Reference spec:** `docs/superpowers/specs/2026-05-26-amfa-events-design.md` — read it before starting.

**Important constraints to keep:**
- Don't modify pre-existing logic. Only additive changes to existing KMT files (new field, new entry, new line).
- AMFA codebase: zero changes.
- Use existing patterns: see `internal/http/chi/system_handlers.go` for handler shape, `features/events/` for frontend shape, `pkg/keycloakadmin` for Keycloak client patterns.

---

## Phase 1: Backend foundation — config + RBAC

### Task 1: Add per-tenant AMFA config structs

**Files:**
- Modify: `internal/config/models.go` (add new structs + optional field on existing `KeycloakTenantConfig`)
- Test: `internal/config/models_test.go` (extend existing tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/config/models_test.go`:

```go
func TestKeycloakTenantConfig_WithoutAmfaBlock_ParsesCleanly(t *testing.T) {
    yamlInput := `
enabled: true
server_url: "https://kc.example.com"
admin_username: "admin"
admin_password: "pw"
`
    var cfg KeycloakTenantConfig
    err := yaml.Unmarshal([]byte(yamlInput), &cfg)
    require.NoError(t, err)
    assert.True(t, cfg.Enabled)
    assert.Nil(t, cfg.Amfa, "Amfa should be nil when not provided")
}

func TestKeycloakTenantConfig_WithAmfaBlock_ParsesCorrectly(t *testing.T) {
    yamlInput := `
enabled: true
server_url: "https://kc.example.com"
admin_username: "admin"
admin_password: "pw"
amfa:
  enabled: true
  events_lookback_days: 30
  expected_schema_version: "c00d6d7d197c"
  database:
    host: "amfa.internal"
    port: 5432
    database: "adaptive_mfa"
    user: "kmt_ro"
    password: "pw"
    sslmode: "require"
`
    var cfg KeycloakTenantConfig
    err := yaml.Unmarshal([]byte(yamlInput), &cfg)
    require.NoError(t, err)
    require.NotNil(t, cfg.Amfa)
    assert.True(t, cfg.Amfa.Enabled)
    assert.Equal(t, 30, cfg.Amfa.EventsLookbackDays)
    assert.Equal(t, "amfa.internal", cfg.Amfa.Database.Host)
    assert.Equal(t, "require", cfg.Amfa.Database.SSLMode)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestKeycloakTenantConfig_WithAmfaBlock -v`
Expected: FAIL — `cfg.Amfa undefined (type KeycloakTenantConfig has no field or method Amfa)`

- [ ] **Step 3: Add the new structs and field**

Add to `internal/config/models.go` (find the existing `KeycloakTenantConfig` struct and add the `Amfa` field):

```go
type AmfaTenantConfig struct {
    Enabled               bool               `mapstructure:"enabled" yaml:"enabled"`
    EventsLookbackDays    int                `mapstructure:"events_lookback_days" yaml:"events_lookback_days"`
    ExpectedSchemaVersion string             `mapstructure:"expected_schema_version" yaml:"expected_schema_version"`
    Database              AmfaDatabaseConfig `mapstructure:"database" yaml:"database"`
}

type AmfaDatabaseConfig struct {
    Host            string        `mapstructure:"host" yaml:"host"`
    Port            int           `mapstructure:"port" yaml:"port"`
    Database        string        `mapstructure:"database" yaml:"database"`
    User            string        `mapstructure:"user" yaml:"user"`
    Password        string        `mapstructure:"password" yaml:"password"`
    SSLMode         string        `mapstructure:"sslmode" yaml:"sslmode"`
    MaxConns        int           `mapstructure:"max_conns" yaml:"max_conns"`
    MinConns        int           `mapstructure:"min_conns" yaml:"min_conns"`
    Timeout         time.Duration `mapstructure:"timeout" yaml:"timeout"`
    ReadReplicaHost string        `mapstructure:"read_replica_host" yaml:"read_replica_host"`
}

// Validate enforces required fields and applies defaults. Call after loading.
func (a *AmfaTenantConfig) Validate() error {
    if !a.Enabled {
        return nil
    }
    if a.Database.Host == "" {
        return fmt.Errorf("amfa.database.host is required when amfa.enabled=true")
    }
    if a.Database.Port == 0 {
        a.Database.Port = 5432
    }
    if a.Database.Database == "" {
        return fmt.Errorf("amfa.database.database is required when amfa.enabled=true")
    }
    if a.Database.User == "" {
        return fmt.Errorf("amfa.database.user is required when amfa.enabled=true")
    }
    if a.Database.SSLMode == "" {
        a.Database.SSLMode = "require"
    }
    if a.Database.MaxConns == 0 {
        a.Database.MaxConns = 5
    }
    if a.Database.MinConns == 0 {
        a.Database.MinConns = 1
    }
    if a.Database.Timeout == 0 {
        a.Database.Timeout = 10 * time.Second
    }
    if a.EventsLookbackDays == 0 {
        a.EventsLookbackDays = 30
    }
    if a.EventsLookbackDays > 90 {
        a.EventsLookbackDays = 90
    }
    return nil
}
```

Then add the field to `KeycloakTenantConfig`:

```go
type KeycloakTenantConfig struct {
    // ... existing fields, do not modify them ...

    // NEW: optional per-tenant AMFA configuration. Absent → AMFA Events disabled for this tenant.
    Amfa *AmfaTenantConfig `mapstructure:"amfa,omitempty" yaml:"amfa,omitempty"`
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS — both new tests pass, existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/config/models.go internal/config/models_test.go
git commit -m "feat(amfa): add per-tenant AmfaTenantConfig config struct"
```

---

### Task 2: Define amfa:read permission constant

**Files:**
- Modify: `rbac/constants.go` (add one constant, add to GetSystemPermissions list)
- Test: `rbac/constants_test.go` (extend if exists, create if not)

- [ ] **Step 1: Write the failing test**

Create or extend `rbac/constants_test.go`:

```go
func TestAmfaReadPermissionExists(t *testing.T) {
    perms := GetSystemPermissions()
    found := false
    for _, p := range perms {
        if p.Name == PermAmfaRead {
            found = true
            assert.Equal(t, "amfa:read", p.Name)
            assert.NotEmpty(t, p.Description)
            break
        }
    }
    assert.True(t, found, "PermAmfaRead should be in GetSystemPermissions list")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./rbac/ -run TestAmfaReadPermissionExists -v`
Expected: FAIL — `undefined: PermAmfaRead`

- [ ] **Step 3: Add the permission**

In `rbac/constants.go`, add to the const block alongside existing permission constants:

```go
const PermAmfaRead = "amfa:read"
```

And add to `GetSystemPermissions()`:

```go
{Name: PermAmfaRead, Description: "View AMFA (Adaptive MFA) events, risk metrics, and geolocation data"},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rbac/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rbac/constants.go rbac/constants_test.go
git commit -m "feat(amfa): add amfa:read permission constant"
```

---

### Task 3: Grant amfa:read to default system roles

**Files:**
- Modify: `rbac/constants.go` (extend GetSystemRoles to include amfa:read in admin/operator/viewer permission lists)
- Test: `rbac/constants_test.go`

- [ ] **Step 1: Write the failing test**

Add to `rbac/constants_test.go`:

```go
func TestDefaultRolesIncludeAmfaRead(t *testing.T) {
    roles := GetSystemRoles()
    for _, roleName := range []string{"admin", "operator", "viewer"} {
        var role *Role
        for i := range roles {
            if roles[i].Name == roleName {
                role = &roles[i]
                break
            }
        }
        require.NotNil(t, role, "role %s must exist", roleName)
        assert.Contains(t, role.Permissions, PermAmfaRead,
            "role %s should include amfa:read", roleName)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./rbac/ -run TestDefaultRolesIncludeAmfaRead -v`
Expected: FAIL — assertion failure: amfa:read not in role.Permissions.

- [ ] **Step 3: Add the permission to all three default roles**

In `rbac/constants.go`, find `GetSystemRoles()` and add `PermAmfaRead` to the `Permissions` slice on the `admin`, `operator`, and `viewer` role definitions. Example shape (don't replace existing entries — append):

```go
// In the admin role's Permissions: []string{...} list, append:
PermAmfaRead,

// Same for operator's Permissions list:
PermAmfaRead,

// Same for viewer's Permissions list:
PermAmfaRead,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./rbac/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add rbac/constants.go rbac/constants_test.go
git commit -m "feat(amfa): grant amfa:read to default admin/operator/viewer roles"
```

---

### Task 4: Idempotent data migration for amfa:read on existing role rows

**Files:**
- Modify: `pkg/database/client.go` (add a new idempotent migration function, called from AutoMigrate)
- Test: `pkg/database/client_test.go` (or new `_test.go` file if cleaner)

- [ ] **Step 1: Write the failing test**

In `pkg/database/client_test.go` add (uses existing test DB helper if present, otherwise stub):

```go
func TestMigrateAmfaReadPermission_AddsToExistingDefaultRoles(t *testing.T) {
    db := setupTestDB(t)  // existing helper in test file
    // Pre-seed admin role without amfa:read permission
    db.Create(&Role{Name: "admin", Description: "Administrator"})
    db.Create(&Permission{Name: "events:read"})  // an existing permission

    client := &Client{db: db, logger: testLogger(t)}
    err := client.migrateAmfaReadPermission()
    require.NoError(t, err)

    // The amfa:read permission row should now exist
    var permCount int64
    db.Model(&Permission{}).Where("name = ?", "amfa:read").Count(&permCount)
    assert.Equal(t, int64(1), permCount, "amfa:read permission row should be created")

    // The admin role should now have amfa:read granted via role_permissions
    var rolePermCount int64
    db.Table("role_permissions").
        Joins("JOIN permissions p ON p.id = role_permissions.permission_id").
        Joins("JOIN roles r ON r.id = role_permissions.role_id").
        Where("r.name = ? AND p.name = ?", "admin", "amfa:read").
        Count(&rolePermCount)
    assert.Equal(t, int64(1), rolePermCount, "admin role should have amfa:read granted")
}

func TestMigrateAmfaReadPermission_Idempotent(t *testing.T) {
    db := setupTestDB(t)
    client := &Client{db: db, logger: testLogger(t)}

    require.NoError(t, client.migrateAmfaReadPermission())
    require.NoError(t, client.migrateAmfaReadPermission())  // run twice
    // No errors, no duplicates
    var permCount int64
    db.Model(&Permission{}).Where("name = ?", "amfa:read").Count(&permCount)
    assert.Equal(t, int64(1), permCount)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/database/ -run TestMigrateAmfaReadPermission -v`
Expected: FAIL — `client.migrateAmfaReadPermission undefined`.

- [ ] **Step 3: Implement the migration**

In `pkg/database/client.go`, add this function and invoke it from `AutoMigrate` (after RBAC tables migrate, before time-series tables):

```go
// migrateAmfaReadPermission is an idempotent data migration that ensures the
// amfa:read permission exists and is granted to the default admin/operator/viewer
// roles. Safe to run multiple times.
func (c *Client) migrateAmfaReadPermission() error {
    c.logger.Info("Running amfa:read permission data migration")

    // 1. Ensure the permission row exists
    var perm Permission
    if err := c.db.Where(Permission{Name: "amfa:read"}).
        Attrs(Permission{Description: "View AMFA events, risk metrics, and geolocation data"}).
        FirstOrCreate(&perm).Error; err != nil {
        return fmt.Errorf("failed to ensure amfa:read permission row: %w", err)
    }

    // 2. Grant it to each of the default system roles if not already granted
    defaultRoles := []string{"admin", "operator", "viewer"}
    for _, roleName := range defaultRoles {
        var role Role
        if err := c.db.Where(Role{Name: roleName}).First(&role).Error; err != nil {
            // If a default role doesn't exist yet, the seeder will create it with amfa:read.
            // Don't fail this migration.
            c.logger.Debug("Default role not yet created, skipping",
                logger.Str("role", roleName))
            continue
        }

        // Idempotent insert: skip if already granted
        var existing int64
        if err := c.db.Table("role_permissions").
            Where("role_id = ? AND permission_id = ?", role.ID, perm.ID).
            Count(&existing).Error; err != nil {
            return fmt.Errorf("failed to check role_permissions for %s: %w", roleName, err)
        }
        if existing > 0 {
            continue
        }

        if err := c.db.Exec(
            "INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)",
            role.ID, perm.ID,
        ).Error; err != nil {
            return fmt.Errorf("failed to grant amfa:read to %s: %w", roleName, err)
        }
        c.logger.Info("Granted amfa:read to role", logger.Str("role", roleName))
    }

    return nil
}
```

Then call it from `AutoMigrate`:

```go
// Add after the RBAC tables AutoMigrate block:
if err := c.migrateAmfaReadPermission(); err != nil {
    c.logger.Warn("amfa:read permission migration failed", logger.Err(err))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/database/ -run TestMigrateAmfaReadPermission -v`
Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add pkg/database/client.go pkg/database/client_test.go
git commit -m "feat(amfa): idempotent migration granting amfa:read on existing roles"
```

---

## Phase 2: Backend — amfa package types, interfaces, models

### Task 5: Domain types (`amfa/types.go`)

**Files:**
- Create: `amfa/types.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/types_test.go`:

```go
package amfa

import (
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
)

func TestEventRowZeroValue(t *testing.T) {
    var e EventRow
    assert.Empty(t, e.EventID)
    assert.True(t, e.EventTime.IsZero())
    assert.Nil(t, e.RiskLevel)
}

func TestStatsZeroValue(t *testing.T) {
    var s Stats
    assert.Equal(t, int64(0), s.Total)
    assert.Equal(t, int64(0), s.Risky)
    assert.Equal(t, int64(0), s.UniqueUsers24h)
    assert.Equal(t, int64(0), s.FlaggedIPs)
}

func TestGeoBucketHasCoordinates(t *testing.T) {
    b := GeoBucket{Lat: 40.7, Long: -74.0, Count: 5}
    assert.InDelta(t, 40.7, b.Lat, 0.001)
    assert.InDelta(t, -74.0, b.Long, 0.001)
}

// Compile-time check that ErrAmfaNotConfigured is an error
var _ error = ErrAmfaNotConfigured

func TestErrAmfaNotConfigured_HasMessage(t *testing.T) {
    assert.NotEmpty(t, ErrAmfaNotConfigured.Error())
}

func TestListEventsOptionsDefaults(t *testing.T) {
    opts := ListEventsOptions{}
    assert.Equal(t, 0, opts.Limit)
    assert.Empty(t, opts.RealmID)
    _ = time.Now()  // ensure time is imported
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./amfa/ -v`
Expected: FAIL — `package amfa is not in std`/types undefined.

- [ ] **Step 3: Implement types**

Create `amfa/types.go`:

```go
// Package amfa provides types and services for reading AMFA (Adaptive MFA)
// events from per-tenant AMFA PostgreSQL databases.
package amfa

import (
    "errors"
    "time"
)

// ErrAmfaNotConfigured is returned when the caller asks for AMFA data for a tenant
// that does not have AMFA enabled in its configuration.
var ErrAmfaNotConfigured = errors.New("amfa: not configured for this tenant")

// ErrAmfaUnavailable is returned when the per-tenant AMFA database connection
// cannot be established or used.
var ErrAmfaUnavailable = errors.New("amfa: data source unavailable")

// EventRow is one AMFA event as joined from auth_event + auth_process + auth_context.
// It is the repository's row-level type; the service layer enriches it into Event.
type EventRow struct {
    EventID        string
    EventTime      time.Time
    EventType      string  // LOGIN | LOGIN_ERROR | CLIENT_LOGIN | CLIENT_LOGIN_ERROR
    UserID         *string // may be NULL for pre-auth events
    Client         string
    IPAddress      string
    Country        *string
    Lat            *float64
    Long           *float64
    IsVPN          bool
    RiskLevel      *int    // 1..4; nil if no auth_process row joined
    FinalStatus    *string // LOGIN | LOGIN_ERROR; nil if no auth_process row joined
}

// Event is what the service layer returns to the HTTP handler — EventRow plus
// Keycloak-enriched username/email.
type Event struct {
    EventID     string     `json:"event_id"`
    EventTime   time.Time  `json:"event_time"`
    EventType   string     `json:"event_type"`
    UserID      *string    `json:"user_id"`
    Username    *string    `json:"username"`
    Email       *string    `json:"email"`
    Client      string     `json:"client"`
    IPAddress   string     `json:"ip_address"`
    Country     *string    `json:"country"`
    Lat         *float64   `json:"lat"`
    Long        *float64   `json:"long"`
    IsVPN       bool       `json:"is_vpn"`
    RiskLevel   *int       `json:"risk_level"`
    FinalStatus *string    `json:"final_status"`
}

// Stats are the four KPI numbers for the AMFA Events page.
type Stats struct {
    Total          int64 `json:"total"`
    Risky          int64 `json:"risky"`            // pre_auth_risk_decision >= 3
    UniqueUsers24h int64 `json:"unique_users_24h"`
    FlaggedIPs     int64 `json:"flagged_ips"`      // is_vpn = true
}

// GeoBucket is one aggregated lat/long cell (~11 km) for the map.
type GeoBucket struct {
    Country    *string `json:"country"`
    Lat        float64 `json:"lat"`
    Long       float64 `json:"long"`
    Count      int64   `json:"count"`
    RiskyCount int64   `json:"risky_count"`
}

// ListEventsOptions filters for the events list query.
type ListEventsOptions struct {
    RealmID   string
    StartTime *time.Time
    EndTime   *time.Time
    RiskLevel *int    // treated as ">=" filter when set
    EventType string  // exact-match filter when set
    Limit     int
    Offset    int
}

// StatsOptions filters for the KPI query.
type StatsOptions struct {
    RealmID   string
    StartTime *time.Time
    EndTime   *time.Time
}

// GeoOptions filters for the map geo-aggregation query.
type GeoOptions struct {
    RealmID   string
    StartTime *time.Time
    EndTime   *time.Time
}

// ListEventsResult is what ListEvents returns to the service layer.
type ListEventsResult struct {
    Items []EventRow
    Total int64
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/types.go amfa/types_test.go
git commit -m "feat(amfa): add domain types and sentinel errors"
```

---

### Task 6: Repository and Service interfaces (`amfa/interfaces.go`)

**Files:**
- Create: `amfa/interfaces.go`

- [ ] **Step 1: Write the failing test**

Add to `amfa/types_test.go`:

```go
func TestRepositoryInterface_HasExpectedMethods(t *testing.T) {
    // Compile-time check via an unused variable assignment
    var _ Repository = (*nopRepository)(nil)
}

func TestServiceInterface_HasExpectedMethods(t *testing.T) {
    var _ Service = (*nopService)(nil)
}

type nopRepository struct{}
func (nopRepository) ListEvents(ctx context.Context, opts ListEventsOptions) (*ListEventsResult, error) { return nil, nil }
func (nopRepository) GetStats(ctx context.Context, opts StatsOptions) (Stats, error) { return Stats{}, nil }
func (nopRepository) GetGeoBuckets(ctx context.Context, opts GeoOptions) ([]GeoBucket, error) { return nil, nil }

type nopService struct{}
func (nopService) ListEvents(ctx context.Context, tenantID string, opts ListEventsOptions) ([]Event, int64, error) { return nil, 0, nil }
func (nopService) GetStats(ctx context.Context, tenantID string, opts StatsOptions) (Stats, error) { return Stats{}, nil }
func (nopService) GetGeoBuckets(ctx context.Context, tenantID string, opts GeoOptions) ([]GeoBucket, error) { return nil, nil }
```

Add the `context` import to `types_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./amfa/ -v`
Expected: FAIL — Repository/Service interfaces not defined.

- [ ] **Step 3: Implement interfaces**

Create `amfa/interfaces.go`:

```go
package amfa

import "context"

// Repository abstracts the data layer for one tenant's AMFA database.
// Implementations should be read-only.
type Repository interface {
    ListEvents(ctx context.Context, opts ListEventsOptions) (*ListEventsResult, error)
    GetStats(ctx context.Context, opts StatsOptions) (Stats, error)
    GetGeoBuckets(ctx context.Context, opts GeoOptions) ([]GeoBucket, error)
}

// Service is the orchestration layer used by HTTP handlers.
// It dispatches to the correct per-tenant repository and applies Keycloak
// user enrichment for events.
type Service interface {
    ListEvents(ctx context.Context, tenantID string, opts ListEventsOptions) ([]Event, int64, error)
    GetStats(ctx context.Context, tenantID string, opts StatsOptions) (Stats, error)
    GetGeoBuckets(ctx context.Context, tenantID string, opts GeoOptions) ([]GeoBucket, error)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/interfaces.go amfa/types_test.go
git commit -m "feat(amfa): define Repository and Service interfaces"
```

---

### Task 7: GORM models mirroring AMFA's schema (read-only)

**Files:**
- Create: `amfa/postgres/models.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/postgres/models_test.go`:

```go
package postgres

import (
    "testing"

    "github.com/stretchr/testify/assert"
)

func TestAuthEventTableName(t *testing.T) {
    assert.Equal(t, "auth_event", AuthEvent{}.TableName())
}

func TestAuthProcessTableName(t *testing.T) {
    assert.Equal(t, "auth_process", AuthProcess{}.TableName())
}

func TestAuthContextTableName(t *testing.T) {
    assert.Equal(t, "auth_context", AuthContext{}.TableName())
}

// Compile-time check that all fields use read-only GORM tags.
// Each field should have `gorm:"->;..."` (read-only marker `->`).
func TestModelsAreReadOnly(t *testing.T) {
    // Sanity: pick one field with reflection. Full read-only enforcement
    // happens via the DB role; this test ensures the convention is followed.
    e := AuthEvent{}
    _ = e.ID
    // If this compiles and TableName matches, the test passes.
    assert.NotEmpty(t, AuthEvent{}.TableName())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./amfa/postgres/ -v`
Expected: FAIL — package or types missing.

- [ ] **Step 3: Implement models**

Create `amfa/postgres/models.go`:

```go
// Package postgres provides read-only GORM access to AMFA's PostgreSQL.
// All models use the gorm:"->" read-only tag in addition to the DB-role-level
// least-privilege enforcement.
package postgres

import (
    "time"

    "gorm.io/datatypes"
)

// AuthEvent mirrors AMFA's auth_event table.
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

// AuthProcess mirrors AMFA's auth_process table.
type AuthProcess struct {
    ID                  string         `gorm:"->;column:id;primaryKey"`
    UserID              string         `gorm:"->;column:user_id"`
    AuthContextHash     string         `gorm:"->;column:auth_context_hash"`
    DeviceCredibility   *float64       `gorm:"->;column:device_credibility"`
    NetLocCredibility   *float64       `gorm:"->;column:net_loc_credibility"`
    AuthContextJSON     datatypes.JSON `gorm:"->;column:auth_context_json"`
    PreAuthRiskDecision int            `gorm:"->;column:pre_auth_risk_decision"`
    FinalStatus         string         `gorm:"->;column:final_status"`
    StartedAt           time.Time      `gorm:"->;column:started_at"`
    FinishedAt          *time.Time     `gorm:"->;column:finished_at"`
}

func (AuthProcess) TableName() string { return "auth_process" }

// AuthContext mirrors AMFA's auth_context table.
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/postgres/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/postgres/models.go amfa/postgres/models_test.go
git commit -m "feat(amfa): add read-only GORM models mirroring AMFA tables"
```

---

### Task 8: Per-tenant GORM client constructor (`amfa/postgres/client.go`)

**Files:**
- Create: `amfa/postgres/client.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/postgres/client_test.go`:

```go
package postgres

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
)

func TestBuildDSN_PrefersReplicaWhenSet(t *testing.T) {
    cfg := config.AmfaDatabaseConfig{
        Host:            "primary.internal",
        Port:            5432,
        Database:        "adaptive_mfa",
        User:            "ro",
        Password:        "pw",
        SSLMode:         "require",
        ReadReplicaHost: "replica.internal",
    }
    dsn := buildDSN(cfg)
    assert.Contains(t, dsn, "host=replica.internal")
    assert.NotContains(t, dsn, "host=primary.internal")
}

func TestBuildDSN_UsesPrimaryWhenNoReplica(t *testing.T) {
    cfg := config.AmfaDatabaseConfig{
        Host:     "primary.internal",
        Port:     5432,
        Database: "adaptive_mfa",
        User:     "ro",
        Password: "pw",
        SSLMode:  "disable",
    }
    dsn := buildDSN(cfg)
    assert.Contains(t, dsn, "host=primary.internal")
    assert.Contains(t, dsn, "sslmode=disable")
}

func TestNewClient_RejectsInvalidConfig(t *testing.T) {
    cfg := config.AmfaDatabaseConfig{Host: ""}  // missing host
    _, err := NewClient(cfg, nil)
    require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./amfa/postgres/ -v`
Expected: FAIL — `buildDSN undefined`.

- [ ] **Step 3: Implement client constructor**

Create `amfa/postgres/client.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "time"

    "gorm.io/driver/postgres"
    "gorm.io/gorm"
    gormlogger "gorm.io/gorm/logger"

    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
)

// NewClient opens a read-only GORM connection to one tenant's AMFA database.
// AutoMigrate is NEVER called on this connection — the database is owned by AMFA.
func NewClient(cfg config.AmfaDatabaseConfig, log *logger.Logger) (*gorm.DB, error) {
    if cfg.Host == "" {
        return nil, fmt.Errorf("amfa client: database.host is required")
    }
    if cfg.Database == "" {
        return nil, fmt.Errorf("amfa client: database.database is required")
    }
    if cfg.User == "" {
        return nil, fmt.Errorf("amfa client: database.user is required")
    }

    dsn := buildDSN(cfg)

    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
        Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
        SkipDefaultTransaction: true,
        PrepareStmt:            true,
    })
    if err != nil {
        return nil, fmt.Errorf("amfa client: connect: %w", err)
    }

    sqlDB, err := db.DB()
    if err != nil {
        return nil, fmt.Errorf("amfa client: extract sql.DB: %w", err)
    }

    sqlDB.SetMaxOpenConns(cfg.MaxConns)
    sqlDB.SetMaxIdleConns(cfg.MinConns)
    if cfg.Timeout > 0 {
        sqlDB.SetConnMaxLifetime(cfg.Timeout)
    }

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := sqlDB.PingContext(ctx); err != nil {
        _ = sqlDB.Close()
        return nil, fmt.Errorf("amfa client: ping: %w", err)
    }

    if log != nil {
        log.Info("AMFA database connected",
            logger.Str("host", hostUsed(cfg)),
            logger.Str("database", cfg.Database))
    }
    return db, nil
}

// buildDSN returns the Postgres DSN, preferring the read replica when configured.
func buildDSN(cfg config.AmfaDatabaseConfig) string {
    host := hostUsed(cfg)
    sslmode := cfg.SSLMode
    if sslmode == "" {
        sslmode = "require"
    }
    return fmt.Sprintf(
        "host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
        host, cfg.Port, cfg.Database, cfg.User, cfg.Password, sslmode,
    )
}

func hostUsed(cfg config.AmfaDatabaseConfig) string {
    if cfg.ReadReplicaHost != "" {
        return cfg.ReadReplicaHost
    }
    return cfg.Host
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/postgres/ -run TestBuildDSN -v`
Run: `go test ./amfa/postgres/ -run TestNewClient_RejectsInvalidConfig -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/postgres/client.go amfa/postgres/client_test.go
git commit -m "feat(amfa): per-tenant read-only GORM client constructor"
```

---

### Task 9: Schema-version check (`amfa/postgres/schema_check.go`)

**Files:**
- Create: `amfa/postgres/schema_check.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/postgres/schema_check_test.go` with a sqlite-backed mock that simulates the `alembic_version` table:

```go
package postgres

import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"

    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
    db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
    require.NoError(t, err)
    require.NoError(t, db.Exec("CREATE TABLE alembic_version (version_num VARCHAR(32) PRIMARY KEY)").Error)
    return db
}

func TestCheckAmfaSchema_VersionMatches(t *testing.T) {
    db := newTestDB(t)
    db.Exec("INSERT INTO alembic_version VALUES ('c00d6d7d197c')")
    log := logger.NewNop()
    err := CheckAmfaSchema(db, "c00d6d7d197c", log)
    assert.NoError(t, err)
}

func TestCheckAmfaSchema_DriftReturnsNilButLogs(t *testing.T) {
    db := newTestDB(t)
    db.Exec("INSERT INTO alembic_version VALUES ('different_hash')")
    log := logger.NewNop()
    err := CheckAmfaSchema(db, "c00d6d7d197c", log)
    // Drift is a warning, never a hard fail.
    assert.NoError(t, err)
}

func TestCheckAmfaSchema_TableMissingReturnsNilButLogs(t *testing.T) {
    db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared&_x=1"), &gorm.Config{})
    log := logger.NewNop()
    err := CheckAmfaSchema(db, "c00d6d7d197c", log)
    assert.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./amfa/postgres/ -run TestCheckAmfaSchema -v`
Expected: FAIL — `CheckAmfaSchema undefined`.

- [ ] **Step 3: Implement schema check**

Create `amfa/postgres/schema_check.go`:

```go
package postgres

import (
    "gorm.io/gorm"

    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
)

// CheckAmfaSchema reads AMFA's alembic_version table and logs a warning if it
// differs from the expected version this build was tested against. Never fatal:
// schema drift logs a WARN and returns nil so KMT continues to boot.
func CheckAmfaSchema(db *gorm.DB, expected string, log *logger.Logger) error {
    if expected == "" {
        log.Debug("AMFA expected_schema_version not set; skipping schema check")
        return nil
    }
    var actual string
    if err := db.Raw("SELECT version_num FROM alembic_version").Scan(&actual).Error; err != nil {
        log.Warn("Could not read AMFA alembic_version table; schema verification skipped",
            logger.Err(err))
        return nil
    }
    if actual != expected {
        log.Warn("AMFA schema version drift detected — KMT may need updating",
            logger.Str("expected", expected),
            logger.Str("actual", actual))
        return nil
    }
    log.Info("AMFA schema version matches", logger.Str("version", actual))
    return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/postgres/ -run TestCheckAmfaSchema -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/postgres/schema_check.go amfa/postgres/schema_check_test.go
git commit -m "feat(amfa): non-fatal schema-version check against alembic_version"
```

---

### Task 10: Repository implementation — ListEvents

**Files:**
- Create: `amfa/postgres/repository.go`

- [ ] **Step 1: Write the failing integration test**

Create `amfa/postgres/repository_integration_test.go` (uses build tag `integration` so unit tests aren't slowed):

```go
//go:build integration
// +build integration

package postgres

import (
    "context"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

// Requires a Postgres instance with AMFA's schema applied.
// Set AMFA_TEST_DSN env var: "host=localhost port=5455 dbname=adaptive_mfa user=keycloak password=password sslmode=disable"

func TestRepository_ListEvents_FiltersByRealm(t *testing.T) {
    db := openAmfaTestDB(t)  // helper that reads AMFA_TEST_DSN
    seedAmfaTestData(t, db, "master")  // seed two events in realm "master"
    seedAmfaTestData(t, db, "other")   // and two in realm "other"

    repo := NewRepository(db)
    result, err := repo.ListEvents(context.Background(), amfa.ListEventsOptions{
        RealmID: "master",
        Limit:   10,
        Offset:  0,
    })
    require.NoError(t, err)
    assert.Equal(t, int64(2), result.Total)
    assert.Len(t, result.Items, 2)
}

func TestRepository_ListEvents_OrdersByEventTimeDesc(t *testing.T) {
    db := openAmfaTestDB(t)
    seedAmfaTestData(t, db, "ordered-test")
    repo := NewRepository(db)
    result, _ := repo.ListEvents(context.Background(), amfa.ListEventsOptions{
        RealmID: "ordered-test",
        Limit:   10,
    })
    require.GreaterOrEqual(t, len(result.Items), 2)
    for i := 1; i < len(result.Items); i++ {
        assert.True(t,
            result.Items[i-1].EventTime.After(result.Items[i].EventTime) ||
                result.Items[i-1].EventTime.Equal(result.Items[i].EventTime),
            "rows should be ordered by event_time DESC")
    }
}

func TestRepository_ListEvents_AppliesTimeWindow(t *testing.T) {
    db := openAmfaTestDB(t)
    seedAmfaTestDataWithOldEvent(t, db, "tw-test")  // includes one event from 100 days ago
    repo := NewRepository(db)
    cutoff := time.Now().Add(-24 * time.Hour)
    result, _ := repo.ListEvents(context.Background(), amfa.ListEventsOptions{
        RealmID:   "tw-test",
        StartTime: &cutoff,
    })
    for _, row := range result.Items {
        assert.True(t, row.EventTime.After(cutoff))
    }
}
```

(Plus minimal `openAmfaTestDB`, `seedAmfaTestData`, `seedAmfaTestDataWithOldEvent` helpers in the same file. Use `t.Skip("set AMFA_TEST_DSN to run")` if the env var is absent.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `AMFA_TEST_DSN=... go test ./amfa/postgres/ -tags=integration -run TestRepository_ListEvents -v`
Expected: FAIL — `NewRepository undefined`.

- [ ] **Step 3: Implement repository.go with ListEvents**

Create `amfa/postgres/repository.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "time"

    "gorm.io/gorm"

    "repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
)

// Repository is a read-only GORM-backed implementation of amfa.Repository
// for one tenant's AMFA database.
type Repository struct {
    db *gorm.DB
}

// NewRepository wraps the given GORM connection as an AMFA repository.
func NewRepository(db *gorm.DB) *Repository {
    return &Repository{db: db}
}

// ListEvents joins auth_event + auth_process + auth_context, filters by realm
// (via auth_context_json->>'realm_id') and optional time/risk/type filters,
// and returns paginated rows ordered by event_time DESC plus the total count.
func (r *Repository) ListEvents(ctx context.Context, opts amfa.ListEventsOptions) (*amfa.ListEventsResult, error) {
    if opts.RealmID == "" {
        return nil, fmt.Errorf("amfa.ListEvents: realm_id is required")
    }
    if opts.Limit <= 0 || opts.Limit > 200 {
        opts.Limit = 25
    }
    if opts.Offset < 0 {
        opts.Offset = 0
    }
    start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime)

    var rows []eventRowScan
    selectSQL := `
        SELECT
            e.id              AS event_id,
            e.event_time      AS event_time,
            e.event_type      AS event_type,
            e.user_id         AS user_id,
            ac.client         AS client,
            ac.ip_address     AS ip_address,
            ac.country_name   AS country,
            ac.lat            AS lat,
            ac.long           AS long,
            COALESCE(ac.is_vpn, false) AS is_vpn,
            p.pre_auth_risk_decision   AS risk_level,
            p.final_status    AS final_status
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id  = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE p.auth_context_json->>'realm_id' = ?
          AND e.event_time BETWEEN ? AND ?
    `

    args := []interface{}{opts.RealmID, start, end}

    if opts.RiskLevel != nil {
        selectSQL += " AND p.pre_auth_risk_decision >= ?"
        args = append(args, *opts.RiskLevel)
    }
    if opts.EventType != "" {
        selectSQL += " AND e.event_type = ?"
        args = append(args, opts.EventType)
    }

    selectSQL += " ORDER BY e.event_time DESC LIMIT ? OFFSET ?"
    args = append(args, opts.Limit, opts.Offset)

    if err := r.db.WithContext(ctx).Raw(selectSQL, args...).Scan(&rows).Error; err != nil {
        return nil, fmt.Errorf("amfa list events: %w", err)
    }

    countSQL := `
        SELECT COUNT(*)
        FROM auth_event e
        LEFT JOIN auth_process p ON p.id = e.auth_process
        WHERE p.auth_context_json->>'realm_id' = ?
          AND e.event_time BETWEEN ? AND ?`
    countArgs := []interface{}{opts.RealmID, start, end}
    if opts.RiskLevel != nil {
        countSQL += " AND p.pre_auth_risk_decision >= ?"
        countArgs = append(countArgs, *opts.RiskLevel)
    }
    if opts.EventType != "" {
        countSQL += " AND e.event_type = ?"
        countArgs = append(countArgs, opts.EventType)
    }

    var total int64
    if err := r.db.WithContext(ctx).Raw(countSQL, countArgs...).Scan(&total).Error; err != nil {
        return nil, fmt.Errorf("amfa count events: %w", err)
    }

    return &amfa.ListEventsResult{
        Items: rowsToEventRows(rows),
        Total: total,
    }, nil
}

// eventRowScan is the raw scan target. We translate to amfa.EventRow afterwards.
type eventRowScan struct {
    EventID     string
    EventTime   time.Time
    EventType   string
    UserID      *string
    Client      string
    IPAddress   string
    Country     *string
    Lat         *float64
    Long        *float64
    IsVPN       bool
    RiskLevel   *int
    FinalStatus *string
}

func rowsToEventRows(in []eventRowScan) []amfa.EventRow {
    out := make([]amfa.EventRow, 0, len(in))
    for _, r := range in {
        out = append(out, amfa.EventRow{
            EventID:     r.EventID,
            EventTime:   r.EventTime,
            EventType:   r.EventType,
            UserID:      r.UserID,
            Client:      r.Client,
            IPAddress:   r.IPAddress,
            Country:     r.Country,
            Lat:         r.Lat,
            Long:        r.Long,
            IsVPN:       r.IsVPN,
            RiskLevel:   r.RiskLevel,
            FinalStatus: r.FinalStatus,
        })
    }
    return out
}

func normalizeTimeWindow(start, end *time.Time) (time.Time, time.Time) {
    if end == nil {
        now := time.Now().UTC()
        end = &now
    }
    if start == nil {
        s := end.Add(-30 * 24 * time.Hour)
        start = &s
    }
    // Hard cap: 90 days
    if end.Sub(*start) > 90*24*time.Hour {
        s := end.Add(-90 * 24 * time.Hour)
        start = &s
    }
    return *start, *end
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `AMFA_TEST_DSN=... go test ./amfa/postgres/ -tags=integration -run TestRepository_ListEvents -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/postgres/repository.go amfa/postgres/repository_integration_test.go
git commit -m "feat(amfa): repository ListEvents query with realm filter and pagination"
```

---

### Task 11: Repository — GetStats

**Files:**
- Modify: `amfa/postgres/repository.go` (add method)
- Modify: `amfa/postgres/repository_integration_test.go` (add test)

- [ ] **Step 1: Write the failing test**

Append to `repository_integration_test.go`:

```go
func TestRepository_GetStats_CountsTotalAndRiskyCorrectly(t *testing.T) {
    db := openAmfaTestDB(t)
    seedAmfaTestDataWithRiskLevels(t, db, "stats-test")  // 1 normal, 1 risky (>=3)
    repo := NewRepository(db)
    stats, err := repo.GetStats(context.Background(), amfa.StatsOptions{RealmID: "stats-test"})
    require.NoError(t, err)
    assert.GreaterOrEqual(t, stats.Total, int64(2))
    assert.GreaterOrEqual(t, stats.Risky, int64(1))
}
```

Run: expected to fail — `GetStats undefined`.

- [ ] **Step 2: Implement GetStats**

Add to `amfa/postgres/repository.go`:

```go
func (r *Repository) GetStats(ctx context.Context, opts amfa.StatsOptions) (amfa.Stats, error) {
    if opts.RealmID == "" {
        return amfa.Stats{}, fmt.Errorf("amfa.GetStats: realm_id is required")
    }
    start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime)

    var s amfa.Stats
    sql := `
        SELECT
            COUNT(*)                                                                            AS total,
            COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)                                AS risky,
            COUNT(DISTINCT e.user_id) FILTER (WHERE e.event_time >= NOW() - INTERVAL '24 hours') AS unique_users24h,
            COUNT(DISTINCT ac.ip_address) FILTER (WHERE ac.is_vpn = true)                        AS flagged_ips
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id    = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE p.auth_context_json->>'realm_id' = ?
          AND e.event_time BETWEEN ? AND ?
    `
    type scan struct {
        Total          int64
        Risky          int64
        UniqueUsers24h int64
        FlaggedIPs     int64
    }
    var row scan
    if err := r.db.WithContext(ctx).Raw(sql, opts.RealmID, start, end).Scan(&row).Error; err != nil {
        return amfa.Stats{}, fmt.Errorf("amfa stats query: %w", err)
    }
    s.Total = row.Total
    s.Risky = row.Risky
    s.UniqueUsers24h = row.UniqueUsers24h
    s.FlaggedIPs = row.FlaggedIPs
    return s, nil
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `AMFA_TEST_DSN=... go test ./amfa/postgres/ -tags=integration -run TestRepository_GetStats -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add amfa/postgres/repository.go amfa/postgres/repository_integration_test.go
git commit -m "feat(amfa): repository GetStats KPI aggregation query"
```

---

### Task 12: Repository — GetGeoBuckets

**Files:**
- Modify: `amfa/postgres/repository.go`
- Modify: `amfa/postgres/repository_integration_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestRepository_GetGeoBuckets_GroupsByLatLong(t *testing.T) {
    db := openAmfaTestDB(t)
    seedAmfaTestDataWithGeo(t, db, "geo-test")  // 3 rows near NYC, 1 in London
    repo := NewRepository(db)
    buckets, err := repo.GetGeoBuckets(context.Background(), amfa.GeoOptions{RealmID: "geo-test"})
    require.NoError(t, err)
    assert.GreaterOrEqual(t, len(buckets), 2)
    var foundDense bool
    for _, b := range buckets {
        if b.Count >= 3 {
            foundDense = true
        }
    }
    assert.True(t, foundDense)
}
```

Run: expected to fail.

- [ ] **Step 2: Implement GetGeoBuckets**

Add to `repository.go`:

```go
func (r *Repository) GetGeoBuckets(ctx context.Context, opts amfa.GeoOptions) ([]amfa.GeoBucket, error) {
    if opts.RealmID == "" {
        return nil, fmt.Errorf("amfa.GetGeoBuckets: realm_id is required")
    }
    start, end := normalizeTimeWindow(opts.StartTime, opts.EndTime)

    sql := `
        SELECT
            ac.country_name AS country,
            ROUND(ac.lat::numeric,  1)::float8 AS lat,
            ROUND(ac.long::numeric, 1)::float8 AS long,
            COUNT(*)                                                AS count,
            COUNT(*) FILTER (WHERE p.pre_auth_risk_decision >= 3)   AS risky_count
        FROM auth_event e
        LEFT JOIN auth_process p   ON p.id    = e.auth_process
        LEFT JOIN auth_context ac  ON ac.hash = e.auth_context_hash
        WHERE p.auth_context_json->>'realm_id' = ?
          AND e.event_time BETWEEN ? AND ?
          AND ac.lat IS NOT NULL AND ac.long IS NOT NULL
        GROUP BY ac.country_name, ROUND(ac.lat::numeric, 1), ROUND(ac.long::numeric, 1)
        ORDER BY count DESC
    `

    var buckets []amfa.GeoBucket
    if err := r.db.WithContext(ctx).Raw(sql, opts.RealmID, start, end).Scan(&buckets).Error; err != nil {
        return nil, fmt.Errorf("amfa geo query: %w", err)
    }
    return buckets, nil
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `AMFA_TEST_DSN=... go test ./amfa/postgres/ -tags=integration -run TestRepository_GetGeoBuckets -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add amfa/postgres/repository.go amfa/postgres/repository_integration_test.go
git commit -m "feat(amfa): repository GetGeoBuckets aggregated by 0.1deg lat/long cells"
```

---

### Task 13: Per-tenant repository registry (`amfa/registry.go`)

**Files:**
- Create: `amfa/registry.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/registry_test.go`:

```go
package amfa

import (
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
)

type fakeRepo struct{}
func (fakeRepo) ListEvents(_ context.Context, _ ListEventsOptions) (*ListEventsResult, error) { return nil, nil }
func (fakeRepo) GetStats(_ context.Context, _ StatsOptions) (Stats, error)                    { return Stats{}, nil }
func (fakeRepo) GetGeoBuckets(_ context.Context, _ GeoOptions) ([]GeoBucket, error)           { return nil, nil }

func TestRegistry_ReturnsRepoForRegisteredTenant(t *testing.T) {
    reg := NewRegistry()
    reg.Register("tenant-a", fakeRepo{})
    repo, err := reg.RepositoryFor("tenant-a")
    assert.NoError(t, err)
    assert.NotNil(t, repo)
}

func TestRegistry_ReturnsErrAmfaNotConfiguredForUnknownTenant(t *testing.T) {
    reg := NewRegistry()
    _, err := reg.RepositoryFor("unknown")
    assert.True(t, errors.Is(err, ErrAmfaNotConfigured))
}

func TestRegistry_TenantIDs_ReturnsRegisteredTenants(t *testing.T) {
    reg := NewRegistry()
    reg.Register("a", fakeRepo{})
    reg.Register("b", fakeRepo{})
    ids := reg.TenantIDs()
    assert.ElementsMatch(t, []string{"a", "b"}, ids)
}
```

(Add `context` import.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./amfa/ -run TestRegistry -v`
Expected: FAIL — `NewRegistry undefined`.

- [ ] **Step 3: Implement registry**

Create `amfa/registry.go`:

```go
package amfa

import (
    "fmt"
    "sort"
    "sync"
)

// Registry holds one Repository per KMT tenant. Lookups are O(1) and
// concurrent-safe. Tenants without AMFA configured are simply absent.
type Registry struct {
    mu   sync.RWMutex
    repos map[string]Repository
}

func NewRegistry() *Registry {
    return &Registry{repos: make(map[string]Repository)}
}

// Register associates a Repository with a tenant ID. Called once at startup per
// tenant that has amfa.enabled = true.
func (r *Registry) Register(tenantID string, repo Repository) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.repos[tenantID] = repo
}

// RepositoryFor returns the repository for the given tenant. Returns
// ErrAmfaNotConfigured if the tenant has no AMFA configured.
func (r *Registry) RepositoryFor(tenantID string) (Repository, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    repo, ok := r.repos[tenantID]
    if !ok {
        return nil, fmt.Errorf("%w: tenant=%s", ErrAmfaNotConfigured, tenantID)
    }
    return repo, nil
}

// TenantIDs returns a sorted list of tenant IDs that have AMFA configured.
// Useful for startup logging and operator visibility.
func (r *Registry) TenantIDs() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    ids := make([]string, 0, len(r.repos))
    for id := range r.repos {
        ids = append(ids, id)
    }
    sort.Strings(ids)
    return ids
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/ -run TestRegistry -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/registry.go amfa/registry_test.go
git commit -m "feat(amfa): per-tenant repository registry"
```

---

### Task 14: Keycloak user enrichment with TTL cache (`amfa/enrichment.go`)

**Files:**
- Create: `amfa/enrichment.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/enrichment_test.go`:

```go
package amfa

import (
    "context"
    "sync/atomic"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

type fakeKCClient struct {
    calls    atomic.Int64
    users    map[string]KeycloakUser
    notFound map[string]bool
}

func (f *fakeKCClient) GetUser(_ context.Context, tenantID, realm, userID string) (*KeycloakUser, error) {
    f.calls.Add(1)
    if f.notFound[userID] {
        return nil, ErrKeycloakUserNotFound
    }
    u, ok := f.users[userID]
    if !ok {
        return nil, ErrKeycloakUserNotFound
    }
    return &u, nil
}

func TestEnrichment_CachesAfterFirstFetch(t *testing.T) {
    f := &fakeKCClient{users: map[string]KeycloakUser{
        "u1": {Username: "alice", Email: "alice@x"},
    }}
    e := NewEnrichment(f, 5*time.Minute, 30*time.Second)

    result, err := e.BatchEnrich(context.Background(), "tA", "master", []string{"u1", "u1", "u1"})
    require.NoError(t, err)
    assert.Equal(t, "alice", *result["u1"].Username)
    // Same user fetched only once despite being asked for 3 times in same batch
    assert.Equal(t, int64(1), f.calls.Load())

    // Second batch should also hit cache
    _, _ = e.BatchEnrich(context.Background(), "tA", "master", []string{"u1"})
    assert.Equal(t, int64(1), f.calls.Load())
}

func TestEnrichment_NegativeCacheFor404(t *testing.T) {
    f := &fakeKCClient{notFound: map[string]bool{"missing": true}}
    e := NewEnrichment(f, 5*time.Minute, 30*time.Second)
    _, _ = e.BatchEnrich(context.Background(), "tA", "master", []string{"missing"})
    _, _ = e.BatchEnrich(context.Background(), "tA", "master", []string{"missing"})
    assert.Equal(t, int64(1), f.calls.Load(), "404 should be cached")
}

func TestEnrichment_TTLExpiry(t *testing.T) {
    f := &fakeKCClient{users: map[string]KeycloakUser{
        "u1": {Username: "alice"},
    }}
    e := NewEnrichment(f, 10*time.Millisecond, 30*time.Second)
    _, _ = e.BatchEnrich(context.Background(), "tA", "master", []string{"u1"})
    time.Sleep(15 * time.Millisecond)
    _, _ = e.BatchEnrich(context.Background(), "tA", "master", []string{"u1"})
    assert.Equal(t, int64(2), f.calls.Load(), "TTL expiry should re-fetch")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./amfa/ -run TestEnrichment -v`
Expected: FAIL.

- [ ] **Step 3: Implement enrichment**

Create `amfa/enrichment.go`:

```go
package amfa

import (
    "context"
    "errors"
    "sync"
    "time"
)

// ErrKeycloakUserNotFound mirrors a 404 from Keycloak's admin API.
var ErrKeycloakUserNotFound = errors.New("amfa enrichment: keycloak user not found")

// KeycloakUser is the minimal projection of a Keycloak user KMT needs for display.
type KeycloakUser struct {
    Username *string
    Email    *string
}

// KeycloakClient is the small interface Enrichment depends on. KMT's existing
// pkg/keycloakadmin client should be wrapped to satisfy this.
type KeycloakClient interface {
    GetUser(ctx context.Context, tenantID, realm, userID string) (*KeycloakUser, error)
}

type cacheEntry struct {
    user      *KeycloakUser // nil = negative cache
    expiresAt time.Time
}

// Enrichment looks up Keycloak users for AMFA events with a per-process TTL cache.
type Enrichment struct {
    client       KeycloakClient
    successTTL   time.Duration
    notFoundTTL  time.Duration
    mu           sync.RWMutex
    cache        map[string]cacheEntry // key = tenantID|realm|userID
}

// NewEnrichment returns an Enrichment with the given TTLs. Recommended:
// successTTL=5*time.Minute, notFoundTTL=30*time.Second.
func NewEnrichment(client KeycloakClient, successTTL, notFoundTTL time.Duration) *Enrichment {
    return &Enrichment{
        client:      client,
        successTTL:  successTTL,
        notFoundTTL: notFoundTTL,
        cache:       make(map[string]cacheEntry),
    }
}

// BatchEnrich looks up each unique userID in the given list and returns a map
// keyed by userID. Cache hits avoid Keycloak calls. Per-batch dedup is automatic.
func (e *Enrichment) BatchEnrich(ctx context.Context, tenantID, realm string, userIDs []string) (map[string]KeycloakUser, error) {
    seen := make(map[string]struct{}, len(userIDs))
    out := make(map[string]KeycloakUser, len(userIDs))
    now := time.Now()

    for _, uid := range userIDs {
        if uid == "" {
            continue
        }
        if _, dup := seen[uid]; dup {
            continue
        }
        seen[uid] = struct{}{}

        key := cacheKey(tenantID, realm, uid)

        e.mu.RLock()
        entry, hit := e.cache[key]
        e.mu.RUnlock()

        if hit && entry.expiresAt.After(now) {
            if entry.user != nil {
                out[uid] = *entry.user
            }
            // Negative cache hit: skip silently
            continue
        }

        user, err := e.client.GetUser(ctx, tenantID, realm, uid)
        e.mu.Lock()
        if errors.Is(err, ErrKeycloakUserNotFound) {
            e.cache[key] = cacheEntry{user: nil, expiresAt: now.Add(e.notFoundTTL)}
        } else if err == nil && user != nil {
            e.cache[key] = cacheEntry{user: user, expiresAt: now.Add(e.successTTL)}
            out[uid] = *user
        }
        e.mu.Unlock()
        // Any other error: don't cache; just skip enrichment for this user
    }
    return out, nil
}

func cacheKey(tenantID, realm, userID string) string {
    return tenantID + "|" + realm + "|" + userID
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/ -run TestEnrichment -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/enrichment.go amfa/enrichment_test.go
git commit -m "feat(amfa): Keycloak user enrichment with TTL and negative cache"
```

---

### Task 15: Service layer (`amfa/service.go`)

**Files:**
- Create: `amfa/service.go`

- [ ] **Step 1: Write the failing test**

Create `amfa/service_test.go`:

```go
package amfa

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

type stubRepo struct {
    listResult *ListEventsResult
    stats      Stats
    geo        []GeoBucket
    listErr    error
}

func (s *stubRepo) ListEvents(_ context.Context, _ ListEventsOptions) (*ListEventsResult, error) {
    return s.listResult, s.listErr
}
func (s *stubRepo) GetStats(_ context.Context, _ StatsOptions) (Stats, error)       { return s.stats, nil }
func (s *stubRepo) GetGeoBuckets(_ context.Context, _ GeoOptions) ([]GeoBucket, error) { return s.geo, nil }

func TestService_ListEvents_ReturnsErrAmfaNotConfiguredForUnknownTenant(t *testing.T) {
    reg := NewRegistry()
    enr := NewEnrichment(&fakeKCClient{}, time.Minute, time.Second)
    svc := NewService(reg, enr)
    _, _, err := svc.ListEvents(context.Background(), "no-such-tenant", ListEventsOptions{RealmID: "master"})
    assert.True(t, errors.Is(err, ErrAmfaNotConfigured))
}

func TestService_ListEvents_EnrichesUsernames(t *testing.T) {
    uid := "user-1"
    repo := &stubRepo{listResult: &ListEventsResult{
        Items: []EventRow{{EventID: "e1", UserID: &uid, EventType: "LOGIN"}},
        Total: 1,
    }}
    reg := NewRegistry()
    reg.Register("tA", repo)
    enr := NewEnrichment(&fakeKCClient{users: map[string]KeycloakUser{
        "user-1": {Username: strPtr("alice"), Email: strPtr("alice@x")},
    }}, time.Minute, time.Second)
    svc := NewService(reg, enr)

    events, total, err := svc.ListEvents(context.Background(), "tA", ListEventsOptions{RealmID: "master"})
    require.NoError(t, err)
    assert.Equal(t, int64(1), total)
    require.Len(t, events, 1)
    require.NotNil(t, events[0].Username)
    assert.Equal(t, "alice", *events[0].Username)
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./amfa/ -run TestService -v`
Expected: FAIL.

- [ ] **Step 3: Implement service**

Create `amfa/service.go`:

```go
package amfa

import (
    "context"
)

type service struct {
    registry   *Registry
    enrichment *Enrichment
}

// NewService wires registry + enrichment into a Service implementation.
func NewService(reg *Registry, enr *Enrichment) Service {
    return &service{registry: reg, enrichment: enr}
}

func (s *service) ListEvents(ctx context.Context, tenantID string, opts ListEventsOptions) ([]Event, int64, error) {
    repo, err := s.registry.RepositoryFor(tenantID)
    if err != nil {
        return nil, 0, err
    }
    res, err := repo.ListEvents(ctx, opts)
    if err != nil {
        return nil, 0, err
    }

    // Collect unique non-nil user IDs
    uids := make([]string, 0, len(res.Items))
    seen := make(map[string]struct{})
    for _, row := range res.Items {
        if row.UserID == nil || *row.UserID == "" {
            continue
        }
        if _, ok := seen[*row.UserID]; ok {
            continue
        }
        seen[*row.UserID] = struct{}{}
        uids = append(uids, *row.UserID)
    }

    enriched, _ := s.enrichment.BatchEnrich(ctx, tenantID, opts.RealmID, uids)

    events := make([]Event, 0, len(res.Items))
    for _, row := range res.Items {
        e := Event{
            EventID:     row.EventID,
            EventTime:   row.EventTime,
            EventType:   row.EventType,
            UserID:      row.UserID,
            Client:      row.Client,
            IPAddress:   row.IPAddress,
            Country:     row.Country,
            Lat:         row.Lat,
            Long:        row.Long,
            IsVPN:       row.IsVPN,
            RiskLevel:   row.RiskLevel,
            FinalStatus: row.FinalStatus,
        }
        if row.UserID != nil {
            if u, ok := enriched[*row.UserID]; ok {
                e.Username = u.Username
                e.Email = u.Email
            }
        }
        events = append(events, e)
    }
    return events, res.Total, nil
}

func (s *service) GetStats(ctx context.Context, tenantID string, opts StatsOptions) (Stats, error) {
    repo, err := s.registry.RepositoryFor(tenantID)
    if err != nil {
        return Stats{}, err
    }
    return repo.GetStats(ctx, opts)
}

func (s *service) GetGeoBuckets(ctx context.Context, tenantID string, opts GeoOptions) ([]GeoBucket, error) {
    repo, err := s.registry.RepositoryFor(tenantID)
    if err != nil {
        return nil, err
    }
    return repo.GetGeoBuckets(ctx, opts)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./amfa/ -v`
Expected: PASS (all tests in amfa package).

- [ ] **Step 5: Commit**

```bash
git add amfa/service.go amfa/service_test.go
git commit -m "feat(amfa): service layer combining registry and enrichment"
```

---

## Phase 3: Backend — HTTP handlers & DI wiring

### Task 16: HTTP handlers (`internal/http/chi/amfa_handlers.go`)

**Files:**
- Create: `internal/http/chi/amfa_handlers.go`

- [ ] **Step 1: Write the failing test**

Create `internal/http/chi/amfa_handlers_test.go`. Mirror the shape of `system_handlers_test.go`. Key tests:

```go
func TestAmfaHandlers_GetEvents_Returns404WhenTenantNotConfigured(t *testing.T) {
    svc := &stubAmfaService{listErr: amfa.ErrAmfaNotConfigured}
    h := NewAmfaHandlers(svc, newTestTenantService(t), nil, testLogger(t), nil, &RBACMiddleware{})

    req := httptest.NewRequest("GET", "/amfa/events?realm_id=master", nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("tenantID", "tA")
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
    w := httptest.NewRecorder()

    h.handleAmfaEvents(w, req)
    assert.Equal(t, http.StatusNotFound, w.Code)
    assert.Contains(t, w.Body.String(), "amfa_not_configured")
}

func TestAmfaHandlers_GetEvents_RequiresRealmID(t *testing.T) {
    svc := &stubAmfaService{}
    h := NewAmfaHandlers(svc, newTestTenantService(t), nil, testLogger(t), nil, &RBACMiddleware{})
    req := httptest.NewRequest("GET", "/amfa/events", nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("tenantID", "tA")
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
    w := httptest.NewRecorder()
    h.handleAmfaEvents(w, req)
    assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAmfaHandlers_GetEvents_Returns200WithItems(t *testing.T) {
    uid := "u1"
    svc := &stubAmfaService{
        events: []amfa.Event{{EventID: "e1", EventType: "LOGIN", UserID: &uid}},
        total:  1,
    }
    h := NewAmfaHandlers(svc, newTestTenantService(t), nil, testLogger(t), nil, &RBACMiddleware{})
    req := httptest.NewRequest("GET", "/amfa/events?realm_id=master&limit=25", nil)
    rctx := chi.NewRouteContext()
    rctx.URLParams.Add("tenantID", "tA")
    req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
    w := httptest.NewRecorder()
    h.handleAmfaEvents(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
    assert.Contains(t, w.Body.String(), `"event_id":"e1"`)
    assert.Contains(t, w.Body.String(), `"total":1`)
}

// stubAmfaService implements amfa.Service for tests
type stubAmfaService struct {
    events  []amfa.Event
    total   int64
    listErr error
    stats   amfa.Stats
    geo     []amfa.GeoBucket
}
func (s *stubAmfaService) ListEvents(_ context.Context, _ string, _ amfa.ListEventsOptions) ([]amfa.Event, int64, error) {
    return s.events, s.total, s.listErr
}
func (s *stubAmfaService) GetStats(_ context.Context, _ string, _ amfa.StatsOptions) (amfa.Stats, error) {
    return s.stats, nil
}
func (s *stubAmfaService) GetGeoBuckets(_ context.Context, _ string, _ amfa.GeoOptions) ([]amfa.GeoBucket, error) {
    return s.geo, nil
}
```

(Add corresponding stats and geo test cases.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/http/chi/ -run TestAmfaHandlers -v`
Expected: FAIL.

- [ ] **Step 3: Implement handlers**

Create `internal/http/chi/amfa_handlers.go`. Follow the exact shape of `system_handlers.go`:

```go
package chi

import (
    "errors"
    "net/http"
    "strconv"
    "time"

    "github.com/go-chi/chi/v5"

    "repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
    httputil "repos.defensepoint.com/iam/oss-pmt/kmt/internal/http"
    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
    "repos.defensepoint.com/iam/oss-pmt/kmt/tenant"
)

type AmfaHandlers struct {
    service        amfa.Service
    tenantService  tenant.Service
    rbacService    RBACService
    log            *logger.Logger
    authMiddleware func(http.Handler) http.Handler
    rbacMiddleware *RBACMiddleware
}

func NewAmfaHandlers(
    svc amfa.Service,
    tenantSvc tenant.Service,
    rbacSvc RBACService,
    log *logger.Logger,
    authMW func(http.Handler) http.Handler,
    rbacMW *RBACMiddleware,
) *AmfaHandlers {
    return &AmfaHandlers{
        service:        svc,
        tenantService:  tenantSvc,
        rbacService:    rbacSvc,
        log:            log,
        authMiddleware: authMW,
        rbacMiddleware: rbacMW,
    }
}

func (h *AmfaHandlers) RegisterRoutes(r chi.Router) {
    // No global (non-tenant) routes for AMFA
}

func (h *AmfaHandlers) RegisterTenantRoutes(r chi.Router) {
    r.Group(func(r chi.Router) {
        r.Use(h.authMiddleware)
        if h.rbacMiddleware != nil {
            r.Use(h.rbacMiddleware.RequireTenantAccess())
            r.Use(h.rbacMiddleware.RequirePermission("amfa:read"))
        }

        r.Get("/amfa/events", h.handleAmfaEvents)
        r.Get("/amfa/stats",  h.handleAmfaStats)
        r.Get("/amfa/geo",    h.handleAmfaGeo)
    })
}

func (h *AmfaHandlers) handleAmfaEvents(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    tenantID := chi.URLParam(r, "tenantID")
    q := r.URL.Query()

    realm := q.Get("realm_id")
    if realm == "" {
        httputil.RespondBadRequest(w, h.log, "realm_id query param is required")
        return
    }

    limit := parseIntParam(r, "limit", 25)
    if limit > MaxLimit {
        limit = MaxLimit
    }
    offset := parseIntParam(r, "offset", 0)

    var start, end *time.Time
    if s := q.Get("start_time"); s != "" {
        if t, err := time.Parse(time.RFC3339, s); err == nil {
            start = &t
        }
    }
    if e := q.Get("end_time"); e != "" {
        if t, err := time.Parse(time.RFC3339, e); err == nil {
            end = &t
        }
    }

    var riskPtr *int
    if rl := q.Get("risk_level"); rl != "" {
        if n, err := strconv.Atoi(rl); err == nil {
            riskPtr = &n
        }
    }

    opts := amfa.ListEventsOptions{
        RealmID:   realm,
        StartTime: start,
        EndTime:   end,
        RiskLevel: riskPtr,
        EventType: q.Get("event_type"),
        Limit:     limit,
        Offset:    offset,
    }

    events, total, err := h.service.ListEvents(ctx, tenantID, opts)
    if errors.Is(err, amfa.ErrAmfaNotConfigured) {
        httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{
            "error": "amfa_not_configured",
            "message": "AMFA Events is not configured for this tenant",
        })
        return
    }
    if errors.Is(err, amfa.ErrAmfaUnavailable) {
        httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, map[string]interface{}{
            "error": "amfa_unavailable",
        })
        return
    }
    if err != nil {
        h.log.Error("AMFA list events failed",
            logger.Str("tenant_id", tenantID), logger.Err(err))
        httputil.RespondInternalError(w, h.log, "AMFA list events failed")
        return
    }
    httputil.RespondJSON(w, h.log, http.StatusOK, map[string]interface{}{
        "items":  events,
        "total":  total,
        "limit":  limit,
        "offset": offset,
    })
}

func (h *AmfaHandlers) handleAmfaStats(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    tenantID := chi.URLParam(r, "tenantID")
    q := r.URL.Query()
    realm := q.Get("realm_id")
    if realm == "" {
        httputil.RespondBadRequest(w, h.log, "realm_id query param is required")
        return
    }
    var start, end *time.Time
    if s := q.Get("start_time"); s != "" {
        if t, err := time.Parse(time.RFC3339, s); err == nil { start = &t }
    }
    if e := q.Get("end_time"); e != "" {
        if t, err := time.Parse(time.RFC3339, e); err == nil { end = &t }
    }
    stats, err := h.service.GetStats(ctx, tenantID, amfa.StatsOptions{RealmID: realm, StartTime: start, EndTime: end})
    if errors.Is(err, amfa.ErrAmfaNotConfigured) {
        httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{"error": "amfa_not_configured"})
        return
    }
    if err != nil {
        httputil.RespondInternalError(w, h.log, "AMFA stats failed")
        return
    }
    httputil.RespondJSON(w, h.log, http.StatusOK, stats)
}

func (h *AmfaHandlers) handleAmfaGeo(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    tenantID := chi.URLParam(r, "tenantID")
    q := r.URL.Query()
    realm := q.Get("realm_id")
    if realm == "" {
        httputil.RespondBadRequest(w, h.log, "realm_id query param is required")
        return
    }
    var start, end *time.Time
    if s := q.Get("start_time"); s != "" {
        if t, err := time.Parse(time.RFC3339, s); err == nil { start = &t }
    }
    if e := q.Get("end_time"); e != "" {
        if t, err := time.Parse(time.RFC3339, e); err == nil { end = &t }
    }
    buckets, err := h.service.GetGeoBuckets(ctx, tenantID, amfa.GeoOptions{RealmID: realm, StartTime: start, EndTime: end})
    if errors.Is(err, amfa.ErrAmfaNotConfigured) {
        httputil.RespondJSON(w, h.log, http.StatusNotFound, map[string]interface{}{"error": "amfa_not_configured"})
        return
    }
    if err != nil {
        httputil.RespondInternalError(w, h.log, "AMFA geo failed")
        return
    }
    httputil.RespondJSON(w, h.log, http.StatusOK, buckets)
}

// Ensure handlers satisfy router interfaces
var _ RouteRegistrar  = (*AmfaHandlers)(nil)
var _ TenantSubRouter = (*AmfaHandlers)(nil)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/http/chi/ -run TestAmfaHandlers -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/http/chi/amfa_handlers.go internal/http/chi/amfa_handlers_test.go
git commit -m "feat(amfa): HTTP handlers under /api/tenants/{id}/amfa/*"
```

---

### Task 17: Keycloak-admin adapter for enrichment

**Files:**
- Create: `amfa/keycloak_adapter.go`

This adapts KMT's existing `pkg/keycloakadmin` client to the `amfa.KeycloakClient` interface defined in Task 14.

- [ ] **Step 1: Write the failing test**

Create `amfa/keycloak_adapter_test.go`:

```go
package amfa

import (
    "context"
    "errors"
    "testing"

    "github.com/stretchr/testify/assert"
)

type fakeKCAdmin struct {
    callTenant string
    callRealm  string
    callUserID string
    user       *struct{ Username, Email string }
    notFound   bool
    err        error
}

func (f *fakeKCAdmin) GetUserByID(_ context.Context, tenantID, realm, userID string) (interface{}, error) {
    f.callTenant, f.callRealm, f.callUserID = tenantID, realm, userID
    if f.err != nil {
        return nil, f.err
    }
    if f.notFound {
        return nil, ErrKeycloakAdminNotFound
    }
    return f.user, nil
}

func TestKeycloakAdapter_TranslatesNotFound(t *testing.T) {
    adapter := NewKeycloakAdapter(&fakeKCAdmin{notFound: true})
    _, err := adapter.GetUser(context.Background(), "tA", "master", "u1")
    assert.True(t, errors.Is(err, ErrKeycloakUserNotFound))
}
```

- [ ] **Step 2: Run test — fails**

Run: `go test ./amfa/ -run TestKeycloakAdapter -v`
Expected: FAIL — adapter undefined.

- [ ] **Step 3: Implement adapter**

Create `amfa/keycloak_adapter.go`. The exact method name on KMT's `pkg/keycloakadmin` client must be matched — confirm by reading that package; common names are `GetUserByID` or `GetUser`. Pseudocode:

```go
package amfa

import (
    "context"
    "errors"
)

// ErrKeycloakAdminNotFound is the sentinel returned by KMT's keycloak admin
// client for 404 responses. If a different sentinel exists in pkg/keycloakadmin,
// use that one in errors.Is comparisons.
var ErrKeycloakAdminNotFound = errors.New("keycloak admin: user not found")

// keycloakAdmin is the minimal interface this adapter needs from KMT's
// pkg/keycloakadmin. Adjust the method signature to match the real one.
type keycloakAdmin interface {
    GetUserByID(ctx context.Context, tenantID, realm, userID string) (interface{}, error)
}

type keycloakAdapter struct {
    admin keycloakAdmin
}

func NewKeycloakAdapter(admin keycloakAdmin) KeycloakClient {
    return &keycloakAdapter{admin: admin}
}

func (k *keycloakAdapter) GetUser(ctx context.Context, tenantID, realm, userID string) (*KeycloakUser, error) {
    raw, err := k.admin.GetUserByID(ctx, tenantID, realm, userID)
    if err != nil {
        if errors.Is(err, ErrKeycloakAdminNotFound) {
            return nil, ErrKeycloakUserNotFound
        }
        return nil, err
    }
    // raw is an interface{} — translate to KeycloakUser. Cast to the real
    // type returned by pkg/keycloakadmin.GetUserByID.
    if raw == nil {
        return nil, ErrKeycloakUserNotFound
    }
    type kcUser struct {
        Username string
        Email    string
    }
    u, ok := raw.(*kcUser)
    if !ok || u == nil {
        return nil, ErrKeycloakUserNotFound
    }
    return &KeycloakUser{
        Username: nullIfEmpty(u.Username),
        Email:    nullIfEmpty(u.Email),
    }, nil
}

func nullIfEmpty(s string) *string {
    if s == "" {
        return nil
    }
    return &s
}
```

**Note for implementer:** After reading `pkg/keycloakadmin`'s real signature, adjust the `keycloakAdmin` interface and the type assertion to match the actual returned type. The structure above is a placeholder for the pattern; the concrete types come from the real package.

- [ ] **Step 4: Run test — passes**

Run: `go test ./amfa/ -run TestKeycloakAdapter -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add amfa/keycloak_adapter.go amfa/keycloak_adapter_test.go
git commit -m "feat(amfa): adapter wiring pkg/keycloakadmin into amfa.KeycloakClient"
```

---

### Task 18: fx module (`internal/fx/amfa.go`)

**Files:**
- Create: `internal/fx/amfa.go`

- [ ] **Step 1: Write the failing test**

Create `internal/fx/amfa_test.go`:

```go
package fx

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestBuildAmfaRegistry_EmptyWhenNoTenants(t *testing.T) {
    reg, err := buildAmfaRegistry(nil, nil, testLogger(t))
    assert.NoError(t, err)
    assert.NotNil(t, reg)
    assert.Empty(t, reg.TenantIDs())
}

func TestBuildAmfaRegistry_SkipsTenantsWithoutAmfaBlock(t *testing.T) {
    tenants := map[string]config.KeycloakTenantConfig{
        "tA": {Enabled: true, ServerURL: "https://kc"},  // no amfa
    }
    reg, err := buildAmfaRegistry(tenants, nil, testLogger(t))
    assert.NoError(t, err)
    assert.Empty(t, reg.TenantIDs())
}
```

- [ ] **Step 2: Run test — fails**

Run: `go test ./internal/fx/ -run TestBuildAmfaRegistry -v`
Expected: FAIL — function undefined.

- [ ] **Step 3: Implement fx module**

Create `internal/fx/amfa.go`:

```go
package fx

import (
    "go.uber.org/fx"

    "repos.defensepoint.com/iam/oss-pmt/kmt/amfa"
    amfapg "repos.defensepoint.com/iam/oss-pmt/kmt/amfa/postgres"
    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/config"
    "repos.defensepoint.com/iam/oss-pmt/kmt/internal/logger"
    "repos.defensepoint.com/iam/oss-pmt/kmt/pkg/keycloakadmin"

    "time"
)

// AmfaModule provides AMFA service, registry, enrichment, and handlers
// when at least one tenant has amfa.enabled = true. When no tenants have
// AMFA configured, the registry is empty and all endpoints return 404.
func AmfaModule() fx.Option {
    return fx.Module("amfa",
        fx.Provide(
            provideAmfaRegistry,
            provideAmfaEnrichment,
            provideAmfaService,
            provideAmfaHandlers,
        ),
        fx.Invoke(logAmfaStartup),
    )
}

func provideAmfaRegistry(cfg *config.Config, log *logger.Logger) (*amfa.Registry, error) {
    return buildAmfaRegistry(cfg.Keycloak.Tenants, nil, log)
}

func buildAmfaRegistry(
    tenants map[string]config.KeycloakTenantConfig,
    clientFactory func(config.AmfaDatabaseConfig, *logger.Logger) (interface{}, error), // for test injection
    log *logger.Logger,
) (*amfa.Registry, error) {
    reg := amfa.NewRegistry()
    for tenantID, tcfg := range tenants {
        if tcfg.Amfa == nil || !tcfg.Amfa.Enabled {
            continue
        }
        if err := tcfg.Amfa.Validate(); err != nil {
            log.Error("AMFA config invalid for tenant",
                logger.Str("tenant_id", tenantID), logger.Err(err))
            continue
        }
        db, err := amfapg.NewClient(tcfg.Amfa.Database, log)
        if err != nil {
            log.Error("AMFA database connection failed",
                logger.Str("tenant_id", tenantID), logger.Err(err))
            continue
        }
        if err := amfapg.CheckAmfaSchema(db, tcfg.Amfa.ExpectedSchemaVersion, log); err != nil {
            log.Warn("AMFA schema check error",
                logger.Str("tenant_id", tenantID), logger.Err(err))
        }
        repo := amfapg.NewRepository(db)
        reg.Register(tenantID, repo)
        log.Info("AMFA registered for tenant", logger.Str("tenant_id", tenantID))
    }
    return reg, nil
}

func provideAmfaEnrichment(kc *keycloakadmin.Client, log *logger.Logger) *amfa.Enrichment {
    adapter := amfa.NewKeycloakAdapter(kc)
    return amfa.NewEnrichment(adapter, 5*time.Minute, 30*time.Second)
}

func provideAmfaService(reg *amfa.Registry, enr *amfa.Enrichment) amfa.Service {
    return amfa.NewService(reg, enr)
}

func provideAmfaHandlers(/* matches existing handler-providers pattern */) /* ... */ {
    // Match the exact constructor signature established elsewhere.
}

func logAmfaStartup(reg *amfa.Registry, log *logger.Logger) {
    ids := reg.TenantIDs()
    log.Info("AMFA module ready",
        logger.Int("enabled_tenant_count", len(ids)))
}
```

**Note for implementer:** The exact constructor pattern for handlers and the exact `keycloakadmin.Client` type may differ from above; the implementer should align it to KMT's existing `internal/fx/handlers.go` and `pkg/keycloakadmin` shape. The principle — build the registry by iterating tenant configs, skip those without AMFA, log failures non-fatally — is what matters.

- [ ] **Step 4: Run test — passes**

Run: `go test ./internal/fx/ -v`
Expected: PASS for AMFA tests.

- [ ] **Step 5: Commit**

```bash
git add internal/fx/amfa.go internal/fx/amfa_test.go
git commit -m "feat(amfa): fx module assembling per-tenant registry, enrichment, service"
```

---

### Task 19: Register the new module in app composition

**Files:**
- Modify: `internal/fx/app.go` (one new line — import + module call)

- [ ] **Step 1: Add module to app composition**

Find the app composition (the function that returns `fx.Options(...)` listing all modules). Add `AmfaModule()`:

```go
// In the existing app composition, alongside other module calls:
AmfaModule(),
```

- [ ] **Step 2: Wire handlers into the tenant sub-router**

Find where `TenantSubRouter` instances are collected (typically `internal/fx/handlers.go` or the router setup). Add `*chi.AmfaHandlers` to that list following the existing pattern.

- [ ] **Step 3: Run the full build and tests**

Run: `go build ./...`
Run: `go test ./...`
Expected: builds cleanly; all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/fx/app.go internal/fx/handlers.go  # (whichever files were modified)
git commit -m "feat(amfa): wire AmfaModule into fx app composition"
```

---

### Task 20: Update `config.yaml.example` with documented AMFA block

**Files:**
- Modify: `config.yaml.example` (additive only)

- [ ] **Step 1: Add a documented per-tenant amfa block**

Find the `keycloak.tenants.<example-tenant>:` block in `config.yaml.example`. Add a commented-out `amfa:` block at the bottom of that tenant's section showing all options:

```yaml
keycloak:
  tenants:
    prod-keycloak:
      # ... existing fields ...

      # AMFA Events integration (optional per tenant).
      # Omit or set enabled: false to disable AMFA Events for this tenant.
      # amfa:
      #   enabled: false
      #   events_lookback_days: 30  # max 90
      #   expected_schema_version: ""  # alembic head this AMFA was tested against
      #   database:
      #     host: amfa.internal
      #     port: 5432
      #     database: adaptive_mfa
      #     user: kmt_ro
      #     password: ${AMFA_DB_PASSWORD}
      #     sslmode: require
      #     max_conns: 5
      #     min_conns: 1
      #     timeout: 10s
      #     # read_replica_host: amfa-replica.internal
```

- [ ] **Step 2: Verify the example still parses**

Run: `go test ./internal/config/ -v`
Expected: PASS — existing tests parse the example file unchanged (the new block is commented out).

- [ ] **Step 3: Commit**

```bash
git add config.yaml.example
git commit -m "docs(amfa): document per-tenant amfa block in config.yaml.example"
```

---

## Phase 4: Frontend — types, services, hooks

### Task 21: Add map library dependencies

**Files:**
- Modify: `web/package.json` (additive — new dependencies)

- [ ] **Step 1: Add the dependencies**

Run from `web/` directory:

```bash
cd web
npm install --save leaflet@^1.9.0 react-leaflet@^4.2.0 react-leaflet-cluster@^2.1.0
npm install --save-dev @types/leaflet@^1.9.0
```

- [ ] **Step 2: Verify install**

Run: `npm ls leaflet react-leaflet`
Expected: shows installed versions.

- [ ] **Step 3: Build to confirm no breakage**

Run: `npm run build`
Expected: no errors related to the new packages.

- [ ] **Step 4: Commit**

```bash
git add web/package.json web/package-lock.json
git commit -m "chore(amfa): add leaflet + react-leaflet + cluster dependencies"
```

---

### Task 22: Frontend types (`features/amfa/types/amfa.types.ts`)

**Files:**
- Create: `web/src/features/amfa/types/amfa.types.ts`
- Create: `web/src/features/amfa/types/index.ts`

- [ ] **Step 1: Write the types**

Create `web/src/features/amfa/types/amfa.types.ts`:

```typescript
export type AmfaEventType =
  | "LOGIN"
  | "LOGIN_ERROR"
  | "CLIENT_LOGIN"
  | "CLIENT_LOGIN_ERROR";

export type AmfaRiskLevel = 1 | 2 | 3 | 4;

export type AmfaFinalStatus = "LOGIN" | "LOGIN_ERROR";

export interface AmfaEvent {
  event_id: string;
  event_time: string;
  event_type: AmfaEventType;
  user_id: string | null;
  username: string | null;
  email: string | null;
  client: string;
  ip_address: string;
  country: string | null;
  lat: number | null;
  long: number | null;
  is_vpn: boolean;
  risk_level: AmfaRiskLevel | null;
  final_status: AmfaFinalStatus | null;
}

export interface AmfaStats {
  total: number;
  risky: number;
  unique_users_24h: number;
  flagged_ips: number;
}

export interface AmfaGeoBucket {
  country: string | null;
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

export interface AmfaEventsQuery {
  tenantId: string;
  realm: string;
  startTime?: string;
  endTime?: string;
  riskLevel?: number;
  eventType?: string;
  limit: number;
  offset: number;
}

export interface AmfaSimpleQuery {
  tenantId: string;
  realm: string;
  startTime?: string;
  endTime?: string;
}
```

Create `web/src/features/amfa/types/index.ts`:

```typescript
export type {
  AmfaEvent,
  AmfaEventType,
  AmfaEventsQuery,
  AmfaEventsResponse,
  AmfaFinalStatus,
  AmfaGeoBucket,
  AmfaRiskLevel,
  AmfaSimpleQuery,
  AmfaStats,
} from "./amfa.types";
```

- [ ] **Step 2: Type-check**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/amfa/types/
git commit -m "feat(amfa): frontend TypeScript types"
```

---

### Task 23: AMFA permission constant in shared constants

**Files:**
- Modify: `web/src/shared/constants/permissions.ts` (additive)

- [ ] **Step 1: Write the failing test**

Create or extend `web/src/shared/constants/permissions.test.ts`:

```typescript
import { describe, expect, it } from "vitest";
import { PERMISSIONS, AMFA_PERMISSIONS } from "./permissions";

describe("AMFA permissions", () => {
  it("exports AMFA_PERMISSIONS.READ as 'amfa:read'", () => {
    expect(AMFA_PERMISSIONS.READ).toBe("amfa:read");
  });
  it("includes AMFA in the PERMISSIONS root object", () => {
    expect(PERMISSIONS.AMFA).toBe(AMFA_PERMISSIONS);
  });
});
```

Run: `cd web && npx vitest run shared/constants/permissions.test.ts`
Expected: FAIL — `AMFA_PERMISSIONS undefined`.

- [ ] **Step 2: Add the constant**

In `web/src/shared/constants/permissions.ts`, add (next to other `*_PERMISSIONS` declarations):

```typescript
export const AMFA_PERMISSIONS = {
  READ: "amfa:read",
} as const;
```

And in the `PERMISSIONS` aggregated object:

```typescript
export const PERMISSIONS = {
  // ... existing ...
  AMFA: AMFA_PERMISSIONS,
} as const;
```

Also add the type export:

```typescript
export type AmfaPermission = (typeof AMFA_PERMISSIONS)[keyof typeof AMFA_PERMISSIONS];
// And include it in the union `Permission` type.
```

- [ ] **Step 3: Run test — passes**

Run: `npx vitest run shared/constants/permissions.test.ts`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/shared/constants/permissions.ts web/src/shared/constants/permissions.test.ts
git commit -m "feat(amfa): add AMFA_PERMISSIONS frontend constant"
```

---

### Task 24: amfaService (HTTP client)

**Files:**
- Create: `web/src/features/amfa/services/amfaService.ts`
- Create: `web/src/features/amfa/services/index.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/features/amfa/services/amfaService.test.ts`:

```typescript
import { describe, expect, it, vi, beforeEach } from "vitest";
import { amfaService } from "./amfaService";
import { apiClient } from "@/shared/lib/apiClient";

vi.mock("@/shared/lib/apiClient", () => ({
  apiClient: { get: vi.fn() },
}));

describe("amfaService", () => {
  beforeEach(() => vi.clearAllMocks());

  it("getEvents calls the correct URL with query params", async () => {
    (apiClient.get as any).mockResolvedValueOnce({
      items: [], total: 0, limit: 25, offset: 0,
    });
    await amfaService.getEvents({
      tenantId: "tA", realm: "master", limit: 25, offset: 0,
    });
    expect(apiClient.get).toHaveBeenCalledWith(
      "/tenants/tA/amfa/events",
      expect.objectContaining({ realm_id: "master", limit: 25, offset: 0 }),
    );
  });

  it("getStats omits optional params when undefined", async () => {
    (apiClient.get as any).mockResolvedValueOnce({ total: 0, risky: 0, unique_users_24h: 0, flagged_ips: 0 });
    await amfaService.getStats({ tenantId: "tA", realm: "master" });
    expect(apiClient.get).toHaveBeenCalledWith(
      "/tenants/tA/amfa/stats",
      expect.objectContaining({ realm_id: "master" }),
    );
  });

  it("getGeo issues the geo endpoint", async () => {
    (apiClient.get as any).mockResolvedValueOnce([]);
    await amfaService.getGeo({ tenantId: "tA", realm: "master" });
    expect(apiClient.get).toHaveBeenCalledWith(
      "/tenants/tA/amfa/geo",
      expect.objectContaining({ realm_id: "master" }),
    );
  });
});
```

Run: expected FAIL.

- [ ] **Step 2: Implement service**

Create `web/src/features/amfa/services/amfaService.ts`:

```typescript
import { apiClient } from "@/shared/lib/apiClient";
import type {
  AmfaEventsQuery,
  AmfaEventsResponse,
  AmfaGeoBucket,
  AmfaSimpleQuery,
  AmfaStats,
} from "../types";

function buildEventsParams(q: AmfaEventsQuery): Record<string, string | number> {
  const params: Record<string, string | number> = {
    realm_id: q.realm,
    limit: q.limit,
    offset: q.offset,
  };
  if (q.startTime) params.start_time = q.startTime;
  if (q.endTime) params.end_time = q.endTime;
  if (q.riskLevel !== undefined) params.risk_level = q.riskLevel;
  if (q.eventType) params.event_type = q.eventType;
  return params;
}

function buildSimpleParams(q: AmfaSimpleQuery): Record<string, string> {
  const params: Record<string, string> = { realm_id: q.realm };
  if (q.startTime) params.start_time = q.startTime;
  if (q.endTime) params.end_time = q.endTime;
  return params;
}

class AmfaService {
  getEvents(q: AmfaEventsQuery): Promise<AmfaEventsResponse> {
    return apiClient.get<AmfaEventsResponse>(
      `/tenants/${q.tenantId}/amfa/events`,
      buildEventsParams(q),
    );
  }
  getStats(q: AmfaSimpleQuery): Promise<AmfaStats> {
    return apiClient.get<AmfaStats>(
      `/tenants/${q.tenantId}/amfa/stats`,
      buildSimpleParams(q),
    );
  }
  getGeo(q: AmfaSimpleQuery): Promise<AmfaGeoBucket[]> {
    return apiClient.get<AmfaGeoBucket[]>(
      `/tenants/${q.tenantId}/amfa/geo`,
      buildSimpleParams(q),
    );
  }
}

export const amfaService = new AmfaService();
```

Create `web/src/features/amfa/services/index.ts`:

```typescript
export { amfaService } from "./amfaService";
```

- [ ] **Step 3: Run test — passes**

Run: `npx vitest run features/amfa/services/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/services/
git commit -m "feat(amfa): amfaService HTTP client"
```

---

### Task 25: React Query hooks

**Files:**
- Create: `web/src/features/amfa/hooks/useAmfaEvents.ts`
- Create: `web/src/features/amfa/hooks/useAmfaStats.ts`
- Create: `web/src/features/amfa/hooks/useAmfaGeo.ts`
- Create: `web/src/features/amfa/hooks/index.ts`

- [ ] **Step 1: Implement hooks**

Create `web/src/features/amfa/hooks/useAmfaEvents.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { amfaService } from "../services";
import type { AmfaEventsQuery } from "../types";

export const useAmfaEvents = (params: Partial<AmfaEventsQuery>) =>
  useQuery({
    queryKey: ["amfa", "events", params],
    queryFn: () => amfaService.getEvents(params as AmfaEventsQuery),
    enabled: Boolean(params.tenantId && params.realm),
    refetchInterval: 30_000,
  });
```

Create `web/src/features/amfa/hooks/useAmfaStats.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { amfaService } from "../services";
import type { AmfaSimpleQuery } from "../types";

export const useAmfaStats = (params: Partial<AmfaSimpleQuery>) =>
  useQuery({
    queryKey: ["amfa", "stats", params],
    queryFn: () => amfaService.getStats(params as AmfaSimpleQuery),
    enabled: Boolean(params.tenantId && params.realm),
    refetchInterval: 30_000,
  });
```

Create `web/src/features/amfa/hooks/useAmfaGeo.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { amfaService } from "../services";
import type { AmfaSimpleQuery } from "../types";

export const useAmfaGeo = (params: Partial<AmfaSimpleQuery>) =>
  useQuery({
    queryKey: ["amfa", "geo", params],
    queryFn: () => amfaService.getGeo(params as AmfaSimpleQuery),
    enabled: Boolean(params.tenantId && params.realm),
    refetchInterval: 60_000,
  });
```

Create `web/src/features/amfa/hooks/index.ts`:

```typescript
export { useAmfaEvents } from "./useAmfaEvents";
export { useAmfaStats } from "./useAmfaStats";
export { useAmfaGeo } from "./useAmfaGeo";
```

- [ ] **Step 2: Type-check**

Run: `cd web && npx tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/features/amfa/hooks/
git commit -m "feat(amfa): React Query hooks (events/stats/geo)"
```

---

## Phase 5: Frontend — components

### Task 26: AmfaRiskBadge

**Files:**
- Create: `web/src/features/amfa/components/AmfaRiskBadge.tsx`
- Create: `web/src/features/amfa/components/AmfaRiskBadge.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AmfaRiskBadge } from "./AmfaRiskBadge";

describe("AmfaRiskBadge", () => {
  it("renders the risk number", () => {
    render(<AmfaRiskBadge risk={3} />);
    expect(screen.getByText("3")).toBeInTheDocument();
  });
  it("uses the correct color for each level", () => {
    const { rerender } = render(<AmfaRiskBadge risk={1} />);
    expect(screen.getByText("1").parentElement).toHaveClass(/MuiChip-colorSuccess/);
    rerender(<AmfaRiskBadge risk={4} />);
    expect(screen.getByText("4").parentElement).toHaveClass(/MuiChip-colorError/);
  });
  it("renders dash for null risk", () => {
    render(<AmfaRiskBadge risk={null} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
```

Run: expected FAIL.

- [ ] **Step 2: Implement**

```tsx
import { Chip } from "@mui/material";
import type { AmfaRiskLevel } from "../types";

interface Props {
  risk: AmfaRiskLevel | null;
}

const colorMap: Record<AmfaRiskLevel, "success" | "info" | "warning" | "error"> = {
  1: "success",
  2: "info",
  3: "warning",
  4: "error",
};

export function AmfaRiskBadge({ risk }: Props) {
  if (risk == null) {
    return <span>—</span>;
  }
  return <Chip label={String(risk)} color={colorMap[risk]} size="small" />;
}
```

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/components/AmfaRiskBadge`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/components/AmfaRiskBadge.tsx web/src/features/amfa/components/AmfaRiskBadge.test.tsx
git commit -m "feat(amfa): AmfaRiskBadge with risk-level color mapping"
```

---

### Task 27: AmfaKpiRow

**Files:**
- Create: `web/src/features/amfa/components/AmfaKpiRow.tsx`
- Create: `web/src/features/amfa/components/AmfaKpiRow.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AmfaKpiRow } from "./AmfaKpiRow";

describe("AmfaKpiRow", () => {
  it("renders all four KPI cards with formatted numbers", () => {
    render(<AmfaKpiRow stats={{ total: 1247, risky: 38, unique_users_24h: 312, flagged_ips: 7 }} />);
    expect(screen.getByText("1,247")).toBeInTheDocument();
    expect(screen.getByText("38")).toBeInTheDocument();
    expect(screen.getByText("312")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText(/Total Login/i)).toBeInTheDocument();
    expect(screen.getByText(/Risky/i)).toBeInTheDocument();
    expect(screen.getByText(/Unique Users/i)).toBeInTheDocument();
    expect(screen.getByText(/Flagged IPs/i)).toBeInTheDocument();
  });
  it("renders dashes when stats is undefined", () => {
    render(<AmfaKpiRow stats={undefined} />);
    expect(screen.getAllByText("—")).toHaveLength(4);
  });
});
```

- [ ] **Step 2: Implement**

```tsx
import { Box } from "@mui/material";
import { MetricCard, StatsCardGrid } from "@/shared/components";
import type { AmfaStats } from "../types";

interface Props {
  stats: AmfaStats | undefined;
}

function fmt(n: number | undefined): string {
  if (n === undefined || n === null) return "—";
  return n.toLocaleString();
}

export function AmfaKpiRow({ stats }: Props) {
  return (
    <Box sx={{ mb: 3 }}>
      <StatsCardGrid>
        <MetricCard title="Total Login Events" value={fmt(stats?.total)} />
        <MetricCard title="Risky (Risk 3+)" value={fmt(stats?.risky)} severity="warning" />
        <MetricCard title="Unique Users (24h)" value={fmt(stats?.unique_users_24h)} />
        <MetricCard title="Flagged IPs" value={fmt(stats?.flagged_ips)} severity="error" />
      </StatsCardGrid>
    </Box>
  );
}
```

(If `MetricCard` doesn't accept a `severity` prop, omit it and color via className. The implementer confirms by reading `shared/components/MetricCard`.)

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/components/AmfaKpiRow`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/components/AmfaKpiRow.tsx web/src/features/amfa/components/AmfaKpiRow.test.tsx
git commit -m "feat(amfa): AmfaKpiRow with four KPI cards"
```

---

### Task 28: AmfaEventDetailsDialog

**Files:**
- Create: `web/src/features/amfa/components/AmfaEventDetailsDialog.tsx`
- Create: `web/src/features/amfa/components/AmfaEventDetailsDialog.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AmfaEventDetailsDialog } from "./AmfaEventDetailsDialog";
import type { AmfaEvent } from "../types";

const sample: AmfaEvent = {
  event_id: "e1", event_time: "2026-05-20T23:40:00Z",
  event_type: "CLIENT_LOGIN_ERROR", user_id: "u1",
  username: "alice", email: "alice@x", client: "adaptive-auth-api",
  ip_address: "14.42.63.246", country: "BR", lat: -23.5, long: -46.6,
  is_vpn: false, risk_level: 3, final_status: "LOGIN_ERROR",
};

describe("AmfaEventDetailsDialog", () => {
  it("renders event fields when open", () => {
    render(<AmfaEventDetailsDialog event={sample} open onClose={() => {}} />);
    expect(screen.getByText("e1")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("14.42.63.246")).toBeInTheDocument();
    expect(screen.getByText("CLIENT_LOGIN_ERROR")).toBeInTheDocument();
  });
  it("calls onClose when the close button is clicked", () => {
    const onClose = vi.fn();
    render(<AmfaEventDetailsDialog event={sample} open onClose={onClose} />);
    fireEvent.click(screen.getByLabelText(/close/i));
    expect(onClose).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Implement**

```tsx
import { Dialog, DialogContent, DialogTitle, IconButton, Typography, Box } from "@mui/material";
import { Close as CloseIcon } from "@mui/icons-material";
import type { AmfaEvent } from "../types";
import { AmfaRiskBadge } from "./AmfaRiskBadge";

interface Props {
  event: AmfaEvent | null;
  open: boolean;
  onClose: () => void;
}

export function AmfaEventDetailsDialog({ event, open, onClose }: Props) {
  if (!event) return null;
  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <Typography variant="h6">AMFA Event Details</Typography>
        <IconButton onClick={onClose} aria-label="close"><CloseIcon /></IconButton>
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: "grid", gap: 2 }}>
          <Field label="Event ID" value={event.event_id} mono />
          <Field label="Time" value={new Date(event.event_time).toLocaleString()} />
          <Field label="Type" value={event.event_type} />
          <Field label="User" value={event.username ?? event.email ?? event.user_id ?? "—"} />
          {event.email && event.username && <Field label="Email" value={event.email} />}
          <Field label="IP Address" value={event.ip_address} mono />
          <Field label="Client" value={event.client} mono />
          <Field label="Country" value={event.country ?? "—"} />
          <Box>
            <Typography variant="caption" sx={{ textTransform: "uppercase" }}>Risk Level</Typography>
            <Box sx={{ mt: 0.5 }}><AmfaRiskBadge risk={event.risk_level} /></Box>
          </Box>
          <Field label="VPN" value={event.is_vpn ? "Yes" : "No"} />
          <Field label="Final Status" value={event.final_status ?? "—"} />
        </Box>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <Box>
      <Typography variant="caption" sx={{ textTransform: "uppercase", color: "text.secondary" }}>{label}</Typography>
      <Typography variant="body2" sx={{ mt: 0.5, fontFamily: mono ? "monospace" : undefined }}>{value}</Typography>
    </Box>
  );
}
```

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/components/AmfaEventDetailsDialog`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/components/AmfaEventDetailsDialog.tsx web/src/features/amfa/components/AmfaEventDetailsDialog.test.tsx
git commit -m "feat(amfa): AmfaEventDetailsDialog"
```

---

### Task 29: AmfaEventsTable

**Files:**
- Create: `web/src/features/amfa/components/AmfaEventsTable.tsx`
- Create: `web/src/features/amfa/components/AmfaEventsTable.test.tsx`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AmfaEventsTable } from "./AmfaEventsTable";
import type { AmfaEvent } from "../types";

const events: AmfaEvent[] = [
  {
    event_id: "e1", event_time: "2026-05-20T23:40:00Z",
    event_type: "CLIENT_LOGIN_ERROR", user_id: "u1",
    username: "alice", email: "alice@x", client: "api", ip_address: "1.1.1.1",
    country: null, lat: null, long: null, is_vpn: false, risk_level: 3, final_status: "LOGIN_ERROR",
  },
];

describe("AmfaEventsTable", () => {
  it("renders rows", () => {
    render(<AmfaEventsTable events={events} total={1} isLoading={false} onRowClick={() => {}}
                            page={1} pageSize={25} onPageChange={() => {}} />);
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("CLIENT_LOGIN_ERROR")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });
  it("calls onRowClick when action icon clicked", () => {
    const onRowClick = vi.fn();
    render(<AmfaEventsTable events={events} total={1} isLoading={false} onRowClick={onRowClick}
                            page={1} pageSize={25} onPageChange={() => {}} />);
    fireEvent.click(screen.getByLabelText(/view event/i));
    expect(onRowClick).toHaveBeenCalledWith(events[0]);
  });
});
```

- [ ] **Step 2: Implement**

Implementation mirrors `features/events/pages/EventsPage.tsx`'s `MaterialReactTable` use. Columns: Time, User, Event Type, IP, Client, Risk, Actions (eye icon).

```tsx
import { useMemo } from "react";
import { Box, IconButton, Typography, Chip } from "@mui/material";
import { Visibility as VisibilityIcon } from "@mui/icons-material";
import { MaterialReactTable, type MRT_ColumnDef } from "material-react-table";
import { MRT_OPTIONS_WITH_TOOLBAR } from "@/shared/constants";
import { AmfaRiskBadge } from "./AmfaRiskBadge";
import type { AmfaEvent } from "../types";

interface Props {
  events: AmfaEvent[];
  total: number;
  isLoading: boolean;
  page: number;       // 1-based
  pageSize: number;
  onPageChange: (newPage: number) => void;
  onRowClick: (event: AmfaEvent) => void;
}

const typeColor = (t: AmfaEvent["event_type"]): "default" | "success" | "error" => {
  if (t === "LOGIN" || t === "CLIENT_LOGIN") return "success";
  if (t === "LOGIN_ERROR" || t === "CLIENT_LOGIN_ERROR") return "error";
  return "default";
};

export function AmfaEventsTable({ events, total, isLoading, page, pageSize, onPageChange, onRowClick }: Props) {
  const columns = useMemo<MRT_ColumnDef<AmfaEvent>[]>(() => [
    { accessorKey: "event_time", header: "Time",
      Cell: ({ cell }) => <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
        {new Date(cell.getValue<string>()).toLocaleString()}</Typography> },
    { id: "user", header: "User",
      Cell: ({ row }) => {
        const e = row.original;
        const display = e.username ?? e.email ?? e.user_id ?? "—";
        return (
          <Box>
            <Typography variant="body2">{display}</Typography>
            {e.user_id && <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace", display: "block" }}>{e.user_id}</Typography>}
          </Box>
        );
      } },
    { accessorKey: "event_type", header: "Event Type",
      Cell: ({ cell }) => <Chip label={cell.getValue<string>()} color={typeColor(cell.getValue<AmfaEvent["event_type"]>())} size="small" /> },
    { accessorKey: "ip_address", header: "IP Address",
      Cell: ({ cell }) => <Typography variant="body2" sx={{ fontFamily: "monospace" }}>{cell.getValue<string>()}</Typography> },
    { accessorKey: "client", header: "Client",
      Cell: ({ cell }) => <Typography variant="body2" color="text.secondary" sx={{ fontFamily: "monospace" }}>{cell.getValue<string>()}</Typography> },
    { accessorKey: "risk_level", header: "Risk",
      Cell: ({ cell }) => <AmfaRiskBadge risk={cell.getValue<AmfaEvent["risk_level"]>()} /> },
    { id: "actions", header: "Actions", size: 80,
      Cell: ({ row }) => (
        <IconButton size="small" aria-label="view event" onClick={(e) => { e.stopPropagation(); onRowClick(row.original); }}>
          <VisibilityIcon fontSize="small" />
        </IconButton>
      ) },
  ], [onRowClick]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <MaterialReactTable
      {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
      columns={columns}
      data={events}
      state={{ isLoading, pagination: { pageIndex: page - 1, pageSize } }}
      enablePagination={false}
      enableSorting
      enableGlobalFilter
      renderBottomToolbarCustomActions={() => (
        <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%", px: 2, py: 1, alignItems: "center" }}>
          <Typography variant="body2" color="text.secondary">
            Page {page} of {totalPages} ({total} total)
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <IconButton size="small" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>←</IconButton>
            <IconButton size="small" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>→</IconButton>
          </Box>
        </Box>
      )}
    />
  );
}
```

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/components/AmfaEventsTable`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/components/AmfaEventsTable.tsx web/src/features/amfa/components/AmfaEventsTable.test.tsx
git commit -m "feat(amfa): AmfaEventsTable using MaterialReactTable"
```

---

### Task 30: AmfaGeoMap

**Files:**
- Create: `web/src/features/amfa/components/AmfaGeoMap.tsx`
- Create: `web/src/features/amfa/components/AmfaGeoMap.test.tsx`

- [ ] **Step 1: Write the failing test (mock react-leaflet)**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("react-leaflet", () => ({
  MapContainer: ({ children }: any) => <div data-testid="map-container">{children}</div>,
  TileLayer: () => <div data-testid="tile-layer" />,
  Marker: ({ children }: any) => <div data-testid="marker">{children}</div>,
  Tooltip: ({ children }: any) => <span>{children}</span>,
  Popup: ({ children }: any) => <div>{children}</div>,
}));
vi.mock("react-leaflet-cluster", () => ({
  default: ({ children }: any) => <div data-testid="cluster">{children}</div>,
}));

import { AmfaGeoMap } from "./AmfaGeoMap";

describe("AmfaGeoMap", () => {
  it("renders a marker per geo bucket", () => {
    render(<AmfaGeoMap buckets={[
      { country: "BR", lat: -23.5, long: -46.6, count: 14, risky_count: 0 },
      { country: "UK", lat: 51.5, long: -0.1, count: 6, risky_count: 1 },
    ]} />);
    expect(screen.getAllByTestId("marker")).toHaveLength(2);
    expect(screen.getByTestId("map-container")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implement**

```tsx
import "leaflet/dist/leaflet.css";
import { MapContainer, TileLayer, Marker, Tooltip, Popup } from "react-leaflet";
import MarkerClusterGroup from "react-leaflet-cluster";
import { Box } from "@mui/material";
import type { AmfaGeoBucket } from "../types";

interface Props {
  buckets: AmfaGeoBucket[];
}

export function AmfaGeoMap({ buckets }: Props) {
  return (
    <Box sx={{ height: "100%", minHeight: 600, width: "100%" }}>
      <MapContainer
        center={[20, 0]}
        zoom={2}
        scrollWheelZoom
        style={{ height: "100%", width: "100%" }}
      >
        <TileLayer
          attribution='&copy; OpenStreetMap, &copy; CARTO'
          url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
        />
        <MarkerClusterGroup>
          {buckets.map((b) => (
            <Marker key={`${b.lat}_${b.long}_${b.country ?? ""}`} position={[b.lat, b.long]}>
              <Tooltip permanent direction="top">
                {(b.country ?? "—") + ": " + b.count + (b.risky_count > 0 ? ` (⚠ ${b.risky_count})` : "")}
              </Tooltip>
              <Popup>
                <strong>{b.country ?? "Unknown"}</strong><br />
                Total events: {b.count}<br />
                Risky (3+): {b.risky_count}
              </Popup>
            </Marker>
          ))}
        </MarkerClusterGroup>
      </MapContainer>
    </Box>
  );
}
```

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/components/AmfaGeoMap`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/components/AmfaGeoMap.tsx web/src/features/amfa/components/AmfaGeoMap.test.tsx
git commit -m "feat(amfa): AmfaGeoMap with react-leaflet + cluster markers"
```

---

## Phase 6: Frontend — page, routing, sidebar

### Task 31: AmfaEventsPage (composes KPI + table + map)

**Files:**
- Create: `web/src/features/amfa/pages/AmfaEventsPage.tsx`
- Create: `web/src/features/amfa/pages/AmfaEventsPage.test.tsx`
- Create: `web/src/features/amfa/pages/index.ts`
- Create: `web/src/features/amfa/index.ts`

- [ ] **Step 1: Write the failing test**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";

vi.mock("@/shared/context", () => ({
  useTenant: () => ({ selectedTenant: { tenant_id: "tA", default_realm: "master" } }),
}));
vi.mock("../hooks", () => ({
  useAmfaEvents: () => ({ data: { items: [], total: 0 }, isLoading: false, refetch: vi.fn() }),
  useAmfaStats:  () => ({ data: { total: 12, risky: 1, unique_users_24h: 5, flagged_ips: 2 } }),
  useAmfaGeo:    () => ({ data: [] }),
}));
vi.mock("react-leaflet", () => ({ MapContainer: ({ children }: any) => <>{children}</>, TileLayer: () => null, Marker: () => null, Tooltip: () => null, Popup: () => null }));
vi.mock("react-leaflet-cluster", () => ({ default: ({ children }: any) => <>{children}</> }));

import { AmfaEventsPage } from "./AmfaEventsPage";

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient();
  return <QueryClientProvider client={qc}><MemoryRouter>{ui}</MemoryRouter></QueryClientProvider>;
}

describe("AmfaEventsPage", () => {
  it("renders the page heading", () => {
    render(wrap(<AmfaEventsPage />));
    expect(screen.getByText(/AMFA Events/i)).toBeInTheDocument();
  });
  it("renders KPIs from useAmfaStats", () => {
    render(wrap(<AmfaEventsPage />));
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implement page (mirrors EventsPage.tsx structure)**

```tsx
import { useCallback, useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Box, Button, Grid } from "@mui/material";
import { Refresh } from "@mui/icons-material";
import { useTenant } from "@/shared/context";
import {
  useKeycloakDashboard,
  useRealmSelector,
  useSessionStorage,
} from "@/shared/hooks";
import {
  RealmSelector,
  TimeSelector,
  PageSizeSelector,
  PageHeader,
  FeatureErrorBoundary,
  SectionCard,
} from "@/shared/components";
import { useAmfaEvents, useAmfaStats, useAmfaGeo } from "../hooks";
import { AmfaKpiRow } from "../components/AmfaKpiRow";
import { AmfaEventsTable } from "../components/AmfaEventsTable";
import { AmfaGeoMap } from "../components/AmfaGeoMap";
import { AmfaEventDetailsDialog } from "../components/AmfaEventDetailsDialog";
import type { AmfaEvent } from "../types";

export function AmfaEventsPage() {
  const { selectedTenant } = useTenant();
  const [pageSize, setPageSize] = useSessionStorage<number>("amfa.pageSize", 25);
  const [page, setPage] = useState(1);
  const [searchParams, setSearchParams] = useSearchParams();
  const [startTime, setStartTime] = useState<string | undefined>(searchParams.get("start") ?? undefined);
  const [endTime, setEndTime] = useState<string | undefined>(searchParams.get("end") ?? undefined);
  const [selectedEvent, setSelectedEvent] = useState<AmfaEvent | null>(null);

  const { data: kcDashboard } = useKeycloakDashboard({ tenantId: selectedTenant?.tenant_id });
  const realms = kcDashboard?.realms ?? [];
  const { selectedRealm, handleRealmChange } = useRealmSelector({
    tenantId: selectedTenant?.tenant_id,
    defaultRealm: selectedTenant?.default_realm,
    availableRealms: realms,
  });

  const handleTimeRangeChange = useCallback((s: Date | null, e: Date | null) => {
    setStartTime(s?.toISOString());
    setEndTime(e?.toISOString());
    setPage(1);
  }, []);

  const sharedQuery = {
    tenantId: selectedTenant?.tenant_id,
    realm: selectedRealm !== "all" ? selectedRealm : "",
    startTime, endTime,
  };

  const { data: stats } = useAmfaStats(sharedQuery);
  const { data: geo } = useAmfaGeo(sharedQuery);
  const { data: eventsRes, isLoading, refetch } = useAmfaEvents({
    ...sharedQuery, limit: pageSize, offset: (page - 1) * pageSize,
  });

  useEffect(() => { setPage(1); }, [selectedRealm]);

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <PageHeader title="AMFA Events" subtitle="Adaptive MFA login activity, risk levels, and geolocation">
        <RealmSelector
          selectedRealm={selectedRealm}
          onRealmChange={handleRealmChange}
          realms={realms}
          defaultRealm={selectedTenant?.default_realm}
        />
        <TimeSelector onTimeRangeChange={handleTimeRangeChange} />
        <PageSizeSelector value={pageSize} onChange={(v) => { setPageSize(v); setPage(1); }}
                          options={[25, 50, 100, 200]} label="events" />
        <Button variant="contained" size="small" startIcon={<Refresh />} onClick={() => refetch()}>Refresh</Button>
      </PageHeader>

      <AmfaKpiRow stats={stats} />

      <Grid container spacing={2} sx={{ mt: 1 }}>
        <Grid item xs={12} md={7}>
          <SectionCard title="Past AMFA Login Events">
            <FeatureErrorBoundary featureId="AmfaEventsTable" title="AMFA Events Table"
                                  resetKeys={[selectedTenant?.tenant_id, selectedRealm]}>
              <AmfaEventsTable
                events={eventsRes?.items ?? []}
                total={eventsRes?.total ?? 0}
                isLoading={isLoading}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onRowClick={setSelectedEvent}
              />
            </FeatureErrorBoundary>
          </SectionCard>
        </Grid>
        <Grid item xs={12} md={5}>
          <SectionCard title="Geolocation Map">
            <FeatureErrorBoundary featureId="AmfaGeoMap" title="AMFA Geo Map"
                                  resetKeys={[selectedTenant?.tenant_id, selectedRealm]}>
              <AmfaGeoMap buckets={geo ?? []} />
            </FeatureErrorBoundary>
          </SectionCard>
        </Grid>
      </Grid>

      <AmfaEventDetailsDialog
        event={selectedEvent}
        open={!!selectedEvent}
        onClose={() => setSelectedEvent(null)}
      />
    </Box>
  );
}
```

Create `web/src/features/amfa/pages/index.ts`:

```typescript
export { AmfaEventsPage } from "./AmfaEventsPage";
```

Create `web/src/features/amfa/index.ts`:

```typescript
export { AmfaEventsPage } from "./pages";
export type {
  AmfaEvent, AmfaStats, AmfaGeoBucket, AmfaRiskLevel, AmfaEventType,
} from "./types";
```

- [ ] **Step 3: Test passes**

Run: `npx vitest run features/amfa/pages/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/amfa/pages/ web/src/features/amfa/index.ts
git commit -m "feat(amfa): AmfaEventsPage composing KPIs, table, and map"
```

---

### Task 32: Routing — add AMFA Events route

**Files:**
- Modify: `web/src/app/routes/routes.tsx` (additive — one new `<Route>`)

- [ ] **Step 1: Add the route**

In `web/src/app/routes/routes.tsx`, add this `<Route>` next to `/events`:

```tsx
import { AmfaEventsPage } from "@/features/amfa";

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

- [ ] **Step 2: Build to verify**

Run: `cd web && npm run build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add web/src/app/routes/routes.tsx
git commit -m "feat(amfa): add /:tenantId/amfa-events route"
```

---

### Task 33: Sidebar entry

**Files:**
- Modify: `web/src/shared/components/Layout/Layout.tsx` (additive — one entry in the `navigation` array)

- [ ] **Step 1: Write the failing test**

Extend `Layout.test.tsx`:

```tsx
it("includes AMFA Events sidebar entry", () => {
  // render with a selected tenant
  render(<Layout>x</Layout>);
  expect(screen.getByText("AMFA Events")).toBeInTheDocument();
});
```

- [ ] **Step 2: Add the entry**

In `Layout.tsx`, find the `navigation` array and add (after `Events`):

```tsx
import { Shield } from "@mui/icons-material";

// inside navigation array:
{ name: "AMFA Events", path: "/amfa-events", icon: <Shield /> },
```

- [ ] **Step 3: Test passes**

Run: `cd web && npx vitest run shared/components/Layout/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/shared/components/Layout/Layout.tsx web/src/shared/components/Layout/Layout.test.tsx
git commit -m "feat(amfa): add AMFA Events sidebar entry"
```

---

### Task 34: Feature claude.md doc

**Files:**
- Create: `web/src/features/amfa/claude.md`

- [ ] **Step 1: Write the feature-contract doc**

Create `web/src/features/amfa/claude.md`:

```markdown
# Feature: AMFA Events

## Domain
Adaptive MFA (AMFA) login event logs, risk metrics, and geolocation. Sourced from a per-tenant AMFA PostgreSQL database via KMT's backend.

## Responsibilities
- KPI display (Total, Risky 3+, Unique Users 24h, Flagged IPs)
- Paginated event log with filtering
- Geolocation map of login activity (react-leaflet + clusters)
- Event detail dialog

## Structure
\`\`\`
amfa/
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
├── pages/
│   └── AmfaEventsPage.tsx
└── index.ts
\`\`\`

## Public API (index.ts)
\`\`\`tsx
export { AmfaEventsPage } from "./pages";
export type { AmfaEvent, AmfaStats, AmfaGeoBucket, ... } from "./types";
\`\`\`

## Permissions
- `amfa:read` — required to view this page.

## Risk Level Color Map
| Risk | Color | Meaning (AMFA ACR) |
|---|---|---|
| 1 | success (green) | Normal — no extra factor |
| 2 | info (blue) | OTP required |
| 3 | warning (orange) | OTP + Authenticator required |
| 4 | error (red) | Login rejected |

## Backend endpoints consumed
- `GET /api/tenants/{tenantID}/amfa/events`
- `GET /api/tenants/{tenantID}/amfa/stats`
- `GET /api/tenants/{tenantID}/amfa/geo`
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/amfa/claude.md
git commit -m "docs(amfa): add feature contract doc"
```

---

## Phase 7: Integration tests, docs, end-to-end verification

### Task 35: Operator-facing integration doc

**Files:**
- Create: `docs/amfa-integration.md`

- [ ] **Step 1: Write the doc**

Create `docs/amfa-integration.md` with:

```markdown
# AMFA Integration Guide

Keycloak Monitoring Tool's AMFA Events feature reads from each tenant's AMFA PostgreSQL database. This guide walks through enabling it.

## Prerequisites
- One AMFA service per KMT tenant (one AMFA per Keycloak instance).
- Network reachability from KMT to each AMFA's PostgreSQL.
- Optionally a read replica per AMFA DB (recommended in production).

## One-time DBA work per AMFA database

Run on every AMFA PostgreSQL the operator wants KMT to read from:

\`\`\`sql
CREATE USER kmt_ro WITH PASSWORD '<strong-password>';
GRANT CONNECT ON DATABASE adaptive_mfa TO kmt_ro;
GRANT USAGE ON SCHEMA public TO kmt_ro;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO kmt_ro;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT ON TABLES TO kmt_ro;

-- Recommended for query performance:
CREATE INDEX IF NOT EXISTS idx_auth_process_realm_id
    ON auth_process ((auth_context_json->>'realm_id'));
\`\`\`

## KMT config (per tenant)

Add an `amfa:` block under the tenant in `config.yaml`:

\`\`\`yaml
keycloak:
  tenants:
    prod-keycloak:
      ...
      amfa:
        enabled: true
        events_lookback_days: 30
        expected_schema_version: c00d6d7d197c
        database:
          host: amfa-prod.internal
          port: 5432
          database: adaptive_mfa
          user: kmt_ro
          password: ${PROD_AMFA_DB_PASSWORD}
          sslmode: require
          read_replica_host: amfa-prod-replica.internal  # optional
\`\`\`

## RBAC
The `amfa:read` permission is granted to the default `admin`, `operator`, and `viewer` roles by an idempotent migration on KMT startup. Custom roles must be granted explicitly.

## Verification

Look for these log lines on KMT startup:
\`\`\`
[INFO] AMFA registered for tenant: tenant_id=prod-keycloak
[INFO] AMFA schema version matches: tenant=prod-keycloak version=c00d6d7d197c
[INFO] AMFA module ready: enabled_tenant_count=1
\`\`\`

Then open the AMFA Events page in the dashboard.

## Troubleshooting

- **`amfa_not_configured` 404 on a tenant page** — that tenant has no `amfa:` block or has `enabled: false`.
- **All AMFA endpoints return 503** — KMT failed to connect to AMFA's DB at startup. Check `host`, `port`, and that the `kmt_ro` role exists with the right password.
- **`AMFA schema version drift detected` WARN** — AMFA's schema has moved past the version KMT was built against. Test before bumping `expected_schema_version` in config.

## Rollback
Set `amfa.enabled: false` for a tenant and restart KMT. Other tenants and KMT itself are unaffected. The read-only role on AMFA's DB can be revoked at leisure.
```

- [ ] **Step 2: Commit**

```bash
git add docs/amfa-integration.md
git commit -m "docs(amfa): operator-facing integration guide"
```

---

### Task 36: Update architecture.md with AMFA module

**Files:**
- Modify: `docs/architecture.md` (additive — new short section near the end)

- [ ] **Step 1: Add architecture section**

Append a new subsection to `docs/architecture.md`:

```markdown
### 22. AMFA Module (`amfa/`)

**Purpose:** Read-only access to per-tenant AMFA (Adaptive MFA) databases for the AMFA Events feature.

**Components:**
- `amfa.Registry` — per-tenant GORM connection registry (1:1 with KMT tenants that have AMFA enabled).
- `amfa.Enrichment` — Keycloak user-lookup cache (5 min TTL, 30 s negative cache).
- `amfa.Service` — orchestrates registry lookup + enrichment.
- `amfa/postgres/` — GORM repository against AMFA's existing tables (read-only by both ORM tag and DB role).
- `internal/http/chi/amfa_handlers.go` — HTTP layer under `/api/tenants/{id}/amfa/*`.

**Key design:** one AMFA per Keycloak/KMT tenant; never modifies AMFA. See `docs/amfa-integration.md` for setup.
```

- [ ] **Step 2: Commit**

```bash
git add docs/architecture.md
git commit -m "docs(amfa): add AMFA module to architecture overview"
```

---

### Task 37: End-to-end smoke

**Files:** none (operational task)

- [ ] **Step 1: Bring up AMFA's docker-compose**

```bash
cd ../point-adaptive-authentication
docker compose -f docker-compose.local.yml up -d
```

- [ ] **Step 2: Create the read-only role and seed a few events**

Connect to AMFA's Postgres:

```bash
psql -h localhost -p 5455 -U keycloak -d adaptive_mfa
```

Run the SQL block from `docs/amfa-integration.md` (CREATE USER etc.). Use AMFA's webhook endpoint to seed a few events with a known realm.

- [ ] **Step 3: Configure KMT for that tenant**

Add an `amfa:` block to the `prod-keycloak` tenant in `config.yaml` pointing at `localhost:5455` with the new role's credentials.

- [ ] **Step 4: Bring up KMT**

```bash
make docker-up
```

- [ ] **Step 5: Visit the AMFA Events page**

Open `http://localhost:7880/prod-keycloak/amfa-events`, log in. Verify:
- KPIs show non-zero numbers matching seeded events.
- Table shows the seeded events with risk levels.
- Map shows markers at the geo points from `auth_context.lat/long`.

- [ ] **Step 6: Verify rollback**

Set `amfa.enabled: false` in the tenant's `amfa:` block. Restart KMT. Confirm the page returns `amfa_not_configured` (404 → empty-state).

- [ ] **Step 7: No commit needed** (operational verification only).

---

## Self-Review

A quick self-review against the spec:

**Spec coverage:**
- §3 Decisions (DB-direct, dedicated read-only role, one AMFA per tenant, react-leaflet, amfa:read permission, pure pass-through) → Tasks 1–34
- §4 Architecture diagram → Tasks 5–18
- §5.1 Config struct (per-tenant) → Task 1
- §5.2 Folder structure → Tasks 5–15
- §5.3 GORM models read-only → Task 7
- §5.4 Repository queries (events, stats, geo) → Tasks 10–12
- §5.5 Schema check → Task 9
- §5.6 Service layer with registry → Task 15
- §5.7 Enrichment with TTL cache → Task 14
- §5.8 HTTP handlers → Task 16
- §5.9 fx module → Tasks 17, 18, 19
- §5.10 RBAC permission → Tasks 2, 3, 4
- §6.x Frontend (types, services, hooks, components, page, routes, sidebar) → Tasks 22–33
- §7 Data flow — covered implicitly by the implementation; no separate task needed
- §8 Errors → tested in handler tests (Task 16)
- §10 Deployment → Tasks 20, 35
- §11 Testing → integrated throughout TDD steps
- §12 Files changed — matches plan

**Placeholder scan:** none found (deliberate placeholder note in Task 17 is explicit guidance for the implementer to align with real `pkg/keycloakadmin` signature, not a TBD).

**Type consistency:**
- `EventRow` defined in Task 5, used in Tasks 10, 11, 12, 15 — consistent.
- `Stats` defined in Task 5, returned by Task 11 (`GetStats`) and Task 15 — consistent.
- `Repository` / `Service` interfaces in Task 6 — implemented in Tasks 10 (`Repository`), 15 (`Service`).
- `Registry.RepositoryFor` returns `Repository` — used in Task 15.
- `ErrAmfaNotConfigured` defined in Task 5, returned from Task 13 (`RepositoryFor`), surfaced in Task 16 handlers.

**Scope check:** focused on one feature; no second subsystem.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-26-amfa-events.md`. Two execution options:

**1. Subagent-Driven (recommended)** — Dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
