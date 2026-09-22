# Architecture Guide

This document describes the system architecture, design decisions, and component interactions of the Keycloak Monitoring Tool.

## Overview

The Keycloak Monitoring Tool is a distributed monitoring system designed to collect, store, and visualize metrics and events from Keycloak identity management systems. It follows a clean architecture approach with clear separation of concerns and dependency injection.

### Key Design Goals

1. **Separation of Concerns**: Clear boundaries between presentation, business logic, and data layers
2. **Maintainability**: Easy to understand, modify, and extend
3. **Testability**: Components are loosely coupled and easily testable
4. **Scalability**: Can handle multiple Keycloak instances and high event volumes
5. **Reliability**: Graceful error handling and automatic recovery
6. **Security**: Secure by default with multiple authentication options

## System Architecture

### High-Level Architecture

```bash
┌──────────────────────────────────────────────────────────────────┐
│                         User Browser                             │
└────────────────────────────┬─────────────────────────────────────┘
                             │ HTTPS
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│                      Web Server (Port 7880)                      │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Static File Server (React App)                            │  │
│  └────────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Reverse Proxy (/api → API Server, /auth → API Server)     │  │
│  └────────────────────────────────────────────────────────────┘  │
└────────────────────────────┬─────────────────────────────────────┘
                             │ Internal HTTP
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│                      API Server (Port 7888)                      │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────┐   │
│  │ Authentication  │  │  REST API        │  │  Background    │   │
│  │ Middleware      │  │  Handlers        │  │  Monitors      │   │
│  └─────────────────┘  └──────────────────┘  └────────────────┘   │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────┐   │
│  │ Session Manager │  │  Business Logic  │  │  Keycloak      │   │
│  │                 │  │  Services        │  │  Client        │   │
│  └─────────────────┘  └──────────────────┘  └────────────────┘   │
└──────────┬────────────────────────┬──────────────────┬───────────┘
           │                        │                  │
           │ PostgreSQL             │ Database ORM     │ HTTPS
           ▼                        ▼                  ▼
┌─────────────────────┐  ┌─────────────────────┐  ┌──────────────┐
│   PostgreSQL        │  │  Repository Layer   │  │  Keycloak    │
│   with TimescaleDB  │◀─┤  (Data Access)      │  │  Instances   │
│                     │  └─────────────────────┘  │  (Monitored) │
└─────────────────────┘                           └──────────────┘
```

### Component Diagram

```bash
┌─────────────────────────────────────────────────────────────┐
│                         cmd/                                │
│  ┌──────────────────┐              ┌──────────────────┐     │
│  │   cmd/server     │              │    cmd/web       │     │
│  │  (API Server)    │              │  (Web Server)    │     │
│  │  - main.go       │              │  - main.go       │     │
│  └────────┬─────────┘              └─────────┬────────┘     │
└───────────┼──────────────────────────────────┼──────────────┘
            │                                  │
            ▼                                  ▼
┌─────────────────────────────────────────────────────────────┐
│                       internal/                             │
│  ┌──────────────────┐  ┌──────────────────┐                 │
│  │  internal/fx     │  │ internal/config  │                 │
│  │  - Uber Fx DI    │  │ - Config Loader  │                 │
│  │  - Module Wiring │  │ - Validation     │                 │
│  │                  │  │ - Env Variables  │                 │
│  └────────┬─────────┘  └──────────────────┘                 │
│           │                                                 │
│    ┌──────┴─────────┬───────────┬─────────────┐             │
│    │                │           │             │             │
│    ▼                ▼           ▼             ▼             │
│  ┌─────────┐  ┌─────────┐  ┌──────────┐  ┌──────────┐       │
│  │ logger  │  │http/chi │  │  domain  │  │ version  │       │
│  │         │  │(router) │  │          │  │          │       │
│  └─────────┘  └─────────┘  └──────────┘  └──────────┘       │
│                    │                                        │
│              ┌─────┴─────┐                                  │
│              ▼           ▼                                  │
│         ┌────────┐  ┌──────────┐                            │
│         │  dto/  │  │handlers  │                            │
│         │requests│  │          │                            │
│         └────────┘  └──────────┘                            │
└─────────────────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Domain Packages (root-level)             │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │   alerts/    │  │    auth/     │  │     rbac/       │    │
│  │  - Service   │  │  - Service   │  │  - Service      │    │
│  │  - Domain    │  │  - OAuth2    │  │  - Permissions  │    │
│  │              │  │  - Sessions  │  │  - Roles        │    │
│  └──────────────┘  └──────────────┘  └─────────────────┘    │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │  keycloak/   │  │   tenant/    │  │     users/      │    │
│  │  - Service   │  │  - Service   │  │  - Service      │    │
│  │  - Monitor   │  │  - Multi-    │  │  - Management   │    │
│  │  - Events    │  │    tenant    │  │                 │    │
│  └──────────────┘  └──────────────┘  └─────────────────┘    │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │ configcheck/ │  │notifications/│  │    operator/    │    │
│  │  - Security  │  │  - Slack     │  │  - Metrics      │    │
│  │  - Scanner   │  │  - Email     │  │  - Performance  │    │
│  │              │  │  - GitLab    │  │                 │    │
│  └──────────────┘  └──────────────┘  └─────────────────┘    │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │   events/    │  │ eventscheck/ │  │  metricscheck/  │    │
│  │  - Domain    │  │  - IDP Error │  │  - Metrics      │    │
│  │  - Processing│  │    Checking  │  │    Validation   │    │
│  └──────────────┘  └──────────────┘  └─────────────────┘    │
│                                                             │
│  ┌──────────────┐                                           │
│  │   reports/   │                                           │
│  │  - HTML Gen  │                                           │
│  │  - Scheduling│                                           │
│  └──────────────┘                                           │
└─────────────────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│                          pkg/                               │
│  ┌──────────────────────┐  ┌────────────────────────────┐   │
│  │    pkg/database      │  │    pkg/keycloakadmin       │   │
│  │  - GORM Repository   │  │  - Admin API Client        │   │
│  │  - Models            │  │  - Token Management        │   │
│  │  - Migrations        │  │                            │   │
│  └──────────────────────┘  └────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────┐
│                         web/                                │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐    │
│  │    Pages     │  │  Components  │  │    Services     │    │
│  │  - Dashboard │  │  - Charts    │  │  - API Client   │    │
│  │  - Events    │  │  - Tables    │  │  - Auth Service │    │
│  │  - Keycloak  │  │  - Layout    │  │                 │    │
│  │  - Alerts    │  │  - Alerts    │  │                 │    │
│  │  - RBAC      │  │              │  │                 │    │
│  │  - Tenants   │  │              │  │                 │    │
│  │  - Users     │  │              │  │                 │    │
│  └──────────────┘  └──────────────┘  └─────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## Components

### 1. API Server (`cmd/server`)

**Purpose**: Main backend service handling API requests, authentication, and data processing.

**Responsibilities**:

- HTTP server lifecycle management
- Request routing and handling
- Authentication and authorization
- Database interactions
- Keycloak monitoring coordination

**Key Files**:

- `main.go`: Entry point, graceful shutdown handling

### 2. Web Server (`cmd/web`)

**Purpose**: Serves the React frontend and proxies API requests.

**Responsibilities**:

- Static file serving (React app)
- Reverse proxy for `/api` and `/auth` routes
- Session cookie management
- CORS handling

**Key Files**:

- `main.go`: Entry point, proxy configuration

### 3. Application Layer (`internal/fx`)

**Purpose**: Application initialization and dependency injection using Uber Fx.

**Responsibilities**:

- Module-based dependency injection with Uber Fx
- Service initialization and lifecycle management
- Automatic dependency resolution
- Graceful shutdown coordination

**Key Components**:

- `fx.Module`: Main application module definition
- `fx.Provide`: Constructor registration for services
- `fx.Invoke`: Service initialization hooks
- `fx.Lifecycle`: Startup and shutdown hooks

### 4. Configuration (`internal/config`)

**Purpose**: Configuration loading, validation, and management.

**Responsibilities**:

- Load configuration from YAML files
- Environment variable override support
- Configuration validation
- Default value handling

**Key Features**:

- Viper-based configuration
- Nested configuration structures
- Type-safe configuration access
- Environment variable mapping (`MONITORING_*`)

### 5. HTTP Layer (`internal/http/chi`)

**Purpose**: HTTP server setup using Chi router with middleware.

**Responsibilities**:

- Chi router configuration and route registration
- Middleware chain setup (logging, auth, CORS, recovery)
- Request/response DTOs (`internal/http/dto`)
- Handler implementations for all API endpoints
- Swagger/OpenAPI documentation serving

**Key Components**:

- `router.go`: Chi router setup and middleware configuration
- `*_handlers.go`: HTTP handlers for each domain (alerts, users, tenants, etc.)
- `dto/requests/`: Request DTOs with validation
- `dto/responses/`: Response DTOs (when needed)

### 6. Domain Layer (`internal/domain`)

**Purpose**: Core business models and interfaces.

**Responsibilities**:

- Domain model definitions
- Business logic interfaces
- Validation rules
- Type definitions

### 7. API Endpoints

The API is implemented in `internal/http/chi/` with handlers for each domain. Full API documentation is available via Swagger at `/swagger/`.

**Key Endpoints**:

**Events & Stats**
- `GET /api/events` - List events with time-based filtering
- `GET /api/stats` - Get statistics

**Keycloak**
- `GET /api/keycloak/dashboard` - Keycloak dashboard data
- `GET /api/keycloak/realms/{realm}/users` - List realm users
- `GET /api/keycloak/realms/{realm}/clients` - List realm clients

**Alerts**
- `GET /api/alerts` - List alerts with filtering (severity, status, realm, type)
- `GET /api/alerts/{alert_id}` - Get alert details
- `POST /api/alerts/{alert_id}/acknowledge` - Acknowledge an alert
- `POST /api/alerts/{alert_id}/resolve` - Resolve an alert
- `POST /api/alerts/{alert_id}/ignore` - Ignore an alert
- `POST /api/alerts/{alert_id}/comment` - Add comment to alert

**Operator Metrics**
- `GET /api/operator-metrics/summary` - Operator performance metrics
- `GET /api/operator-metrics/actions` - Operator action history
- `GET /api/operator-metrics/leaderboard` - Operator rankings

**Tenants**
- `GET /api/tenants` - List all tenants
- `GET /api/tenants/{tenant_id}` - Get tenant details
- `POST /api/tenants` - Create tenant
- `PUT /api/tenants/{tenant_id}` - Update tenant
- `DELETE /api/tenants/{tenant_id}` - Delete tenant

**Users**
- `GET /api/users` - List users
- `GET /api/users/{user_id}` - Get user details
- `POST /api/users` - Create user
- `PUT /api/users/{user_id}` - Update user
- `DELETE /api/users/{user_id}` - Delete user

**RBAC**
- `GET /api/rbac/roles` - List roles
- `GET /api/rbac/permissions` - List permissions
- `POST /api/rbac/roles` - Create role
- `PUT /api/rbac/roles/{role_id}` - Update role

**Auth**
- `POST /api/auth/login` - Login
- `POST /api/auth/logout` - Logout
- `GET /api/auth/me` - Get current user
- `POST /api/auth/refresh` - Refresh token

**System**
- `GET /health` - Health check
- `GET /ready` - Readiness check
- `GET /swagger/*` - Swagger documentation

### 8. Authentication Package (`auth/`)

**Purpose**: Authentication and authorization (root-level package).

**Responsibilities**:

- Multiple authentication providers (Simple, OAuth2/OIDC)
- Session management
- Password hashing (bcrypt)
- Middleware for protected routes
- Token management and refresh

**Authentication Providers**:

- **Simple Auth**: Username/password with database storage
- **OAuth2/OIDC**: Keycloak SSO integration

### 9. Database Package (`pkg/database`)

**Purpose**: Database access and persistence using GORM.

**Responsibilities**:

- PostgreSQL connection management with GORM
- Repository pattern implementation
- Model mapping and migrations
- Query execution with GORM query builder
- Connection pooling via pgx

**Key Models**:

- Events (with TimescaleDB hypertables)
- Users
- Sessions
- Alerts
- Tenants
- Roles and Permissions
- Keycloak metrics

### 10. Keycloak Packages

**`keycloak/`** (root-level) - Keycloak monitoring service

**Purpose**: Background monitoring of Keycloak instances.

**Responsibilities**:

- Event collection and processing
- Metrics gathering
- Health monitoring
- Multi-realm support

**Components**:

- **Service**: Main monitoring service
- **Monitor**: Background monitoring worker
- **Events**: Event fetching and processing

**`pkg/keycloakadmin`** - Keycloak Admin API client

**Purpose**: HTTP client for Keycloak Admin REST API.

**Responsibilities**:

- HTTP client authenticated via the OAuth2 `client_credentials` grant — a
  dedicated confidential client's service account, 
- TLS configuration

### 11. Configuration Checker Package (`configcheck/`)

**Purpose**: Automated security configuration scanning for Keycloak instances.

**Responsibilities**:

- Periodic scanning of Keycloak configuration
- Detection of security misconfigurations
- Alert creation and management
- Multi-tenant configuration validation
- Automated remediation guidance

**Components**:

- **Service**: Main configuration checker service with background worker
- **Realm Security Checker**: Validates realm security settings
  - SSL requirements
  - Brute force protection
  - Password policies (length, complexity)
  - Email verification
  - Event logging (admin and user events)
  - Duplicate email prevention
  - Token and session lifespans
- **Client Security Checker**: Validates OAuth2/OIDC client configurations
  - Redirect URI validation (wildcards, localhost in production)
  - Public client configurations
  - Deprecated flows (implicit flow, direct access grants)
  - Client secret requirements
  - HTTP detection in production
- **Identity Provider Checker**: Validates IDP configurations
  - SAML metadata descriptor URLs
  - Certificate expiration monitoring
  - Automatic certificate fetching and parsing
  - 30-day expiration warnings

**Check Flow**:

```bash
┌──────────────────┐
│ Config Checker   │
│ Background Job   │ (Runs every 5m)
└────────┬─────────┘
         │
         ├─── For each enabled tenant
         │
         ▼
┌──────────────────┐
│ Fetch Keycloak   │
│ Configuration    │
│ (Realms, Clients,│
│  IDPs)           │
└────────┬─────────┘
         │
         ├─── Run Realm Security Checks
         ├─── Run Client Security Checks
         └─── Run IDP Metadata Checks
         │
         ▼
┌──────────────────┐
│ Compare with     │
│ Expected Config  │
│ (from config.yaml)│
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Create/Update    │
│ Alerts in DB     │
│ (Active status)  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Trigger          │
│ Notifications    │
└──────────────────┘
```

**Key Features**:

- Configurable check intervals (default: 5 minutes)
- Per-check enable/disable toggles
- Customizable thresholds (password length, certificate expiration days)
- Production vs. development environment modes
- Excluded realms and providers lists
- Automatic alert deduplication (same issue = same alert)

### 12. Notifications Package (`notifications/`)

**Purpose**: Multi-channel alert notification system.

**Responsibilities**:

- Send alerts to external systems
- Severity-based filtering (info, warning, error, critical)
- Integration with Slack, Email, and GitLab
- Notification failure handling and retry logic
- Per-channel minimum severity thresholds

**Components**:

- **Service**: Main notification orchestration service
- **GitLab Notifier**: Creates GitLab issues for alerts
  - Automatic issue creation with alert details
  - Configurable labels, milestones, and assignees
  - Issue deduplication (one issue per unique alert)
  - Markdown formatting with remediation steps
- **Slack Notifier**: Sends messages to Slack channels
  - Webhook-based integration
  - Color-coded messages by severity
  - Rich formatting with alert metadata
  - Custom channel, username, and emoji support
- **Email Notifier**: Sends email notifications
  - SMTP with TLS support
  - HTML-formatted emails
  - Multiple recipients
  - Custom subject templates

**Notification Flow**:

```bash
┌──────────────────┐
│ Configuration    │
│ Alert Created    │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Notification     │
│ Service          │
└────────┬─────────┘
         │
         ├─── Check GitLab min_severity (critical)
         │    └─── Meets threshold? → Create GitLab Issue
         │
         ├─── Check Slack min_severity (warning)
         │    └─── Meets threshold? → Send Slack Message
         │
         └─── Check Email min_severity (error)
              └─── Meets threshold? → Send Email
```

**Severity Filtering**:

Each notification channel has independent severity thresholds:

- **GitLab**: `critical` - Only urgent issues requiring tracking
- **Slack**: `warning` - Most issues for team awareness
- **Email**: `error` - Important issues without notification fatigue

**Integration Configuration**:

- Environment variable support for secrets
- Configurable retry logic
- Error logging without exposing credentials
- Graceful degradation (continue if one channel fails)

### 13. Alerts Package (`alerts/`)

**Purpose**: Event-to-alert conversion and silence management.

**Responsibilities**:

- Automatic conversion of events to alerts based on severity
- Alert silence rule matching and filtering
- Wildcard pattern matching for silence rules
- Scheduled and permanent silence management
- Alert deduplication and suppression

**Components**:

- **Event Alert Converter**: Converts Keycloak events to alerts
  - Maps event types to alert types
  - Assigns severity levels based on event type
  - Enriches alerts with metadata from events
  - Handles both authentication and configuration events
- **Silence Manager**: Manages alert silences
  - Loads silence rules from configuration
  - Matches alerts against active silences
  - Supports wildcard patterns (`*`, `test-*`)
  - Handles scheduled silences (starts_at, ends_at)
  - Tenant-specific and global silences
  - Real-time silence evaluation

**Alert Conversion Flow**:

```bash
Keycloak Event → Event Alert Converter → Check Silences → Create Alert (if not silenced)
     │                    │                    │                     │
     │                    │                    │                     ▼
     │                    │                    │              Notification Service
     │                    ▼                    │                     │
     │            Extract metadata             │                     ▼
     │            Map event type               │              Slack/Email/GitLab
     │            Assign severity              │
     │                    │                    │
     │                    ▼                    │
     │              Alert Created              │
     │                    │                    │
     │                    └────────────────────┘
     │                          │
     └──────────────────────────┘
```

**Silence Matching Logic**:

1. Check if silence is enabled
2. Check if current time is within silence window (starts_at, ends_at)
3. Check tenant_id match (if specified)
4. Check all matchers (realm_name, client_id, alert_type, severity, etc.)
5. Support wildcard patterns in matcher values
6. Return true if all conditions match (alert is silenced)

**Key Features**:

- Configuration-based silence rules
- Wildcard pattern support for flexible matching
- Scheduled silences for planned maintenance
- Permanent silences for test environments
- Per-tenant silence scoping
- Silence metrics tracking

### 14. Alert Management System

**Purpose**: Centralized alert lifecycle management.

**Database Schema**:

```sql
CREATE TABLE configuration_alerts (
    alert_id VARCHAR(255) PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    realm_name VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,  -- 'realm', 'client', 'identity_provider'
    resource_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),
    check_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL,  -- 'info', 'warning', 'error', 'critical'
    status VARCHAR(20) NOT NULL,    -- 'active', 'acknowledged', 'resolved', 'ignored'
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    remediation TEXT,
    metadata JSONB,
    first_detected_at TIMESTAMPTZ NOT NULL,
    last_detected_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Alert Lifecycle**:

```bash
┌─────────┐
│ Active  │ ← New security issue detected
└────┬────┘
     │
     ├─── User clicks "Acknowledge"
     │    └───> ┌──────────────┐
     │          │ Acknowledged │ ← Being worked on
     │          └──────────────┘
     │
     ├─── User clicks "Resolve"
     │    └───> ┌──────────┐
     │          │ Resolved │ ← Configuration fixed
     │          └──────────┘
     │
     └─── User clicks "Ignore"
          └───> ┌─────────┐
                │ Ignored │ ← False positive or accepted risk
                └─────────┘
```

**Alert Actions**:

- **Acknowledge**: Mark alert as "being worked on" without removing from active view
- **Resolve**: Mark as fixed (configuration checker will verify on next run)
- **Ignore**: Suppress future alerts for this specific issue (permanent)
- **View**: See full details including remediation guidance

**UI Features**:

- Multi-level filtering (severity, realm, resource type, status)
- Tabbed interface (Active / Acknowledged / Resolved / Ignored)
- Expandable alert details with remediation steps
- Action dropdown per alert
- Real-time status updates
- Alert count badges
- Color-coded severity indicators
- Styled toast notifications for action feedback
- Confirmation dialogs for destructive actions

**Alert Deduplication**:

Alerts are uniquely identified by:

```go
alert_id = hash(tenant_id + realm_name + resource_type + resource_id + check_type)
```

This ensures:

- Same issue = same alert (updates, not duplicates)
- `first_detected_at` tracks original detection time
- `last_detected_at` tracks most recent detection
- Resolving an alert that recurs creates a new active alert

### 14. Operator Metrics System

**Purpose**: Track and analyze SOC operator performance and alert handling.

**Responsibilities**:

- Track all alert actions by operators (acknowledge, resolve, ignore, comment)
- Calculate response time metrics (time from detection to action)
- Calculate resolution time metrics (time from detection to resolution)
- Track transition times between alert states
- Generate operator performance summaries
- Support multiple time periods (daily, weekly, monthly, custom ranges)
- Per-tenant and cross-tenant metrics

**Database Schema**:

```sql
-- Tracks individual operator actions on alerts
CREATE TABLE operator_alert_actions (
    action_id BIGSERIAL PRIMARY KEY,
    alert_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    operator_email VARCHAR(255) NOT NULL,
    operator_name VARCHAR(255),
    action_type VARCHAR(50) NOT NULL,  -- 'acknowledge', 'resolve', 'ignore', 'comment'
    previous_status VARCHAR(20),
    new_status VARCHAR(20),
    comment TEXT,
    alert_severity VARCHAR(20),
    alert_type VARCHAR(100),
    alert_detection_time TIMESTAMPTZ,
    action_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_time_seconds INT,  -- Time from detection to this action
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_operator_actions_email ON operator_alert_actions(operator_email);
CREATE INDEX idx_operator_actions_tenant ON operator_alert_actions(tenant_id);
CREATE INDEX idx_operator_actions_time ON operator_alert_actions(action_time);
CREATE INDEX idx_operator_actions_type ON operator_alert_actions(action_type);
```

**Metrics Calculated**:

- **Action Counts**:
  - Total alerts handled
  - Alerts acknowledged, resolved, ignored, commented
  - Breakdown by severity (critical, high, medium, low)
  - Breakdown by alert type

- **Response Time Metrics**:
  - Average/median time to first action
  - Min/max response times
  - Average time to acknowledge
  - Average time to resolve
  - Average time to ignore

- **Transition Time Metrics**:
  - Average time from acknowledged → resolved
  - Average time from acknowledged → ignored
  - Average time from active → acknowledged
  - Average time from active → resolved (direct)
  - Average time from active → ignored (direct)

- **Work Time Metrics**:
  - Total work time in seconds/hours
  - Time spent per alert on average

**API Endpoints**:

- `GET /api/operator-metrics/summary` - Get operator metrics summary
  - Query params: `start_time`, `end_time`, `tenant_id`, `operator_email`
- `GET /api/operator-metrics/actions` - List operator actions
  - Query params: Filtering by operator, tenant, time range
- `GET /api/operator-metrics/leaderboard` - Top performing operators
  - Ranked by resolution count, response time, etc.

**UI Features**:

- Operator performance dashboard
- Time period selection (today, this week, this month, custom)
- Per-operator detailed metrics
- Tenant filtering
- Charts for response/resolution times
- Action type distribution
- Alert severity handling breakdown
- Leaderboard/ranking view

**Use Cases**:

- SOC team performance tracking
- Identify response time bottlenecks
- Workload distribution analysis
- Training and improvement opportunities
- SLA compliance monitoring
- Operator workload balancing

### 15. Reports Package (`reports/`)

**Purpose**: Generate scheduled reports with operator metrics and alert summaries.

**Responsibilities**:

- Generate HTML reports with comprehensive metrics
- Include operator performance data
- Alert summaries and trends
- Email delivery of scheduled reports
- Configurable report periods

**Report Contents**:

- Alert summary statistics
- Top alert types
- Operator performance metrics
- Response/resolution time trends
- Charts and visualizations
- Period-over-period comparisons

### 16. Tenant Package (`tenant/`)

**Purpose**: Multi-tenant management and isolation.

**Responsibilities**:

- Tenant CRUD operations
- Tenant-specific configuration
- Multi-tenant data isolation
- Tenant provisioning and deprovisioning

### 17. Users Package (`users/`)

**Purpose**: User management and administration.

**Responsibilities**:

- User CRUD operations
- User profile management
- User-tenant associations
- Password management

### 18. RBAC Package (`rbac/`)

**Purpose**: Role-based access control system.

**Responsibilities**:

- Role and permission management
- Role assignment to users
- Permission checking middleware
- Tenant-scoped permissions

**Components**:

- **Service**: RBAC service with permission checking
- **Roles**: Role definitions and management
- **Permissions**: Permission definitions and validation
- **Middleware**: HTTP middleware for permission enforcement

### 19. Events Check Package (`eventscheck/`)

**Purpose**: IDP error event monitoring and alerting.

**Responsibilities**:

- Monitor Keycloak events for IDP errors
- Detect identity provider failures
- Generate alerts for IDP issues

### 20. Metrics Check Package (`metricscheck/`)

**Purpose**: Metrics validation and anomaly detection.

**Responsibilities**:

- Monitor collected metrics
- Detect anomalies and threshold breaches
- Generate alerts for metric issues

### 21. Web Frontend (`web/`)

**Purpose**: React-based user interface.

**Technology**:

- React 18
- TypeScript
- Recharts for visualization
- React Router for navigation
- Vite for building

**Structure**:

```bash
web/src/
├── components/       # Reusable UI components
│   ├── EventsList.tsx
│   ├── StatsCard.tsx
│   ├── Layout.tsx
│   └── charts/
├── pages/           # Page components
│   ├── Dashboard.tsx              # Main dashboard with overview
│   ├── EventsPage.tsx             # Event history and filtering
│   ├── AlertsPage.tsx             # Alert management
│   ├── AlertDetailPage.tsx        # Individual alert details
│   ├── OperatorMetricsPage.tsx    # Operator performance metrics
│   ├── KeycloakPage.tsx           # Keycloak instance details
│   ├── UserDetailsPage.tsx        # User details
│   ├── AdminUsersPage.tsx         # User administration
│   ├── AdminRolesPage.tsx         # Role administration
│   ├── TenantsPage.tsx            # Tenant management
│   ├── LoginPage.tsx              # Authentication
│   ├── SettingsPage.tsx           # Application settings
│   └── HealthPage.tsx             # System health
├── services/        # API and service layers
│   ├── api.ts       # API client
│   └── auth.ts      # Auth service
├── types/           # TypeScript type definitions
├── contexts/        # React contexts
└── utils/           # Utility functions
```

**Pages and Features**:

- **AlertsPage**: Multi-tab interface for managing alerts
  - Tabs: Active, Acknowledged, Resolved, Ignored
  - Filtering by severity, realm, resource type, status
  - Bulk actions and individual alert actions
  - Real-time status updates
  - Expandable alert details with remediation steps

- **AlertDetailPage**: Detailed view of individual alerts
  - Full alert information and metadata
  - Action history and timeline
  - Comments and collaboration
  - Remediation guidance
  - Status change actions

- **OperatorMetricsPage**: SOC operator performance dashboard
  - Time period selection (today, week, month, custom)
  - Per-operator metrics and rankings
  - Response/resolution time charts
  - Action distribution visualizations
  - Severity handling breakdown
  - Tenant filtering

## Data Flow

### 1. User Authentication Flow

```bash
User → Web UI → POST /auth/login → Auth Middleware → Auth Provider
                                                            │
                                                            ▼
                                                    Check Credentials
                                                            │
                                                            ▼
                                                    Create Session
                                                            │
                                                            ▼
                                                    Set Session Cookie
                                                            │
                                                            ▼
                                                    Return User Info
```

### 2. Event Collection Flow

```bash
Keycloak → Admin API → Keycloak Client → Event Processor → Database
    │                         ▲                                │
    │                         │                                │
    │                   (polling every 30s)                    │
    │                         │                                │
    └─────── Events ──────────┘                                │
                                                               │
                                                               ▼
User Browser ← API Server ← GET /api/events ← Database Query ┘
```

**User enrichment:** 
The enricher prefers the values already in the event payload — Keycloak records
`username` in the `details` of user events (`LOGIN`, `LOGIN_ERROR`, …), and a
user always has a username — and only falls back to an Admin API user lookup
(`GetUserByID`, cached per poll cycle) when the payload has no username. The
lookup is expensive on federated realms (Keycloak resolves the user via the
storage provider per call), so skipping it for the
high-volume login events removes most of that load. 

### 3. Alert Lifecycle Flow (NEW)

```
Keycloak Event → Event Processor → Event Alert Converter
                                           │
                                           ▼
                                   Check Alert Silences
                                           │
                                           ├─ Matches silence? → Drop (not created)
                                           │
                                           ▼
                                   Create Alert in DB
                                           │
                                           ├─ Alert status: "active"
                                           │
                                           ▼
                                   Notification Service
                                           │
                                           ├─ Slack (if severity ≥ warning)
                                           ├─ Email (if severity ≥ error)
                                           └─ GitLab (if severity = critical)
                                           │
                                           ▼
User Views Alert in UI → Takes Action (acknowledge/resolve/ignore)
                                           │
                                           ▼
                           POST /api/alerts/{id}/acknowledge
                                           │
                                           ▼
                           Update Alert Status in DB
                                           │
                                           ▼
                           Record Operator Action
                           (operator_alert_actions table)
                                           │
                                           ├─ Calculate response_time_seconds
                                           ├─ Track action_type
                                           └─ Store metadata
                                           │
                                           ▼
                           Return Success to UI
                                           │
                                           ▼
                           UI Updates Alert Display
```

**Alert State Transitions**:

```
active → acknowledged → resolved
   ↓          ↓            ↓
   └────── ignored ────────┘
```

Each state transition is tracked in the operator_alert_actions table for metrics and audit purposes.

### 4. Metrics Collection Flow

```bash
┌─────────────┐
│  Keycloak   │
│   Instance  │
└──────┬──────┘
       │
       │ (Polling every 5m)
       │
       ▼
┌──────────────────┐
│ Keycloak Monitor │
│  Background Job  │
└────────┬─────────┘
         │
         ├─── Fetch Realms
         ├─── Fetch Users Count
         ├─── Fetch Clients Count
         ├─── Fetch Sessions Count
         └─── Check Health
         │
         ▼
┌─────────────────┐
│    Database     │
│ (TimescaleDB)   │
└────────┬────────┘
         │
         │ (Query)
         │
         ▼
┌─────────────────┐
│   API Handler   │
│ GET /api/stats  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Web Browser   │
└─────────────────┘
```

**User counts:** enabled/disabled user counts come from COUNT-only Admin API
queries rather than
listing users and counting in memory. This is a DB COUNT (no user-object
transfer) and, for federated realms, avoids Keycloak calling back into the
storage provider per user.

**Session counts:** active/offline session counts come from a single
realm-level call to `GET /admin/realms/{realm}/client-session-stats`, which
returns per-client active/offline counts; the monitor sums them. Counts arrive as strings or numbers and are parsed
tolerantly in case of future keycloak updates. Note: this metric is the **sum of client sessions** across clients
(one user session spanning N clients counts as N), not distinct user sessions.

### 5. Request/Response Flow

```bash
Browser → Web Server (Port 7880) → Reverse Proxy
                                         │
                                         ▼
                            API Server (Port 7888)
                                         │
                                         ▼
                            Auth Middleware (Check Session)
                                         │
                                         ▼
                                  Route Handler
                                         │
                                         ▼
                              Business Logic / Service
                                         │
                                         ▼
                                Database Repository
                                         │
                                         ▼
                                    PostgreSQL
                                         │
                                         ▼
                                 Response (JSON)
                                         │
                                         ▼
                                      Browser
```

## Technology Stack

### Backend

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Language | Go 1.23+ | High-performance, statically typed |
| HTTP Router | Chi (go-chi/chi/v5) | Lightweight, idiomatic HTTP router |
| Dependency Injection | Uber Fx | Dependency injection framework |
| Configuration | Viper | Configuration management |
| Logging | slog | Structured logging (JSON) |
| Database ORM | GORM | ORM with PostgreSQL support |
| Database Driver | pgx | PostgreSQL driver |
| Authentication | Custom + OAuth2/OIDC | Multiple auth strategies |
| Password Hashing | bcrypt | Secure password storage |
| API Documentation | Swaggo | OpenAPI/Swagger generation |

### Frontend

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Framework | React 18 | UI framework |
| Language | TypeScript | Type-safe JavaScript |
| Build Tool | Vite | Fast builds and HMR |
| Charts | Recharts | Data visualization |
| Routing | React Router | Client-side routing |
| HTTP Client | Fetch API | API communication |

### Database

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Database | PostgreSQL 17 | Relational database |
| Time-Series | TimescaleDB | Time-series data optimization |
| Connection Pooling | pgx | Connection management |

### Infrastructure

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Containerization | Docker | Application packaging |
| Orchestration | Docker Compose | Multi-container management |
| CI/CD | GitLab CI | Automated testing and deployment |
| Build System | Make | Build automation |

## Design Patterns

### 1. Dependency Injection (Uber Fx)

The application uses Uber Fx for dependency injection. Components are organized into modules and wired together automatically:

```go
// internal/fx/module.go
func Module() fx.Option {
    return fx.Options(
        fx.Provide(
            config.Load,
            logger.New,
            database.NewClient,
            NewRouter,
        ),
        fx.Invoke(RegisterRoutes),
    )
}

// Service constructor - dependencies are injected automatically
func NewAlertService(
    repo alerts.Repository,
    logger *slog.Logger,
) *alerts.Service {
    return &alerts.Service{
        repo:   repo,
        logger: logger,
    }
}

// cmd/server/main.go
func main() {
    fx.New(
        fx.Module(),
        fx.Invoke(func(lc fx.Lifecycle, srv *http.Server) {
            lc.Append(fx.Hook{
                OnStart: func(ctx context.Context) error {
                    go srv.ListenAndServe()
                    return nil
                },
                OnStop: func(ctx context.Context) error {
                    return srv.Shutdown(ctx)
                },
            })
        }),
    ).Run()
}
```

### 2. Repository Pattern

Data access is abstracted through repository interfaces:

```go
type Repository interface {
    SaveEvent(ctx context.Context, event *Event) error
    GetEvents(ctx context.Context, limit, offset int) ([]*Event, error)
    // ...
}
```

### 3. Provider Pattern

Authentication uses a provider pattern for multiple auth strategies:

```go
type AuthProvider interface {
    Authenticate(username, password string) (*User, error)
    CreateUser(user *User) error
    // ...
}
```

### 4. Middleware Pattern

HTTP middleware for cross-cutting concerns:

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check authentication
        // Call next handler if authenticated
        next.ServeHTTP(w, r)
    })
}
```

### 5. Background Worker Pattern

Long-running monitoring tasks run in goroutines:

```go
func (m *Monitor) Start(ctx context.Context) {
    go m.collectMetrics(ctx)
    go m.collectEvents(ctx)
    go m.checkHealth(ctx)
}
```

## Database Schema

### Events Table (TimescaleDB Hypertable)

```sql
CREATE TABLE events (
    id BIGSERIAL,
    timestamp TIMESTAMPTZ NOT NULL,
    source VARCHAR(255),
    type VARCHAR(255),
    realm VARCHAR(255),
    user_id VARCHAR(255),
    username VARCHAR(255),
    ip_address VARCHAR(45),
    client_id VARCHAR(255),
    error VARCHAR(255),
    details JSONB,
    PRIMARY KEY (id, timestamp)
);

-- Convert to hypertable for time-series optimization
SELECT create_hypertable('events', 'timestamp');
```

### Users Table

```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255),
    password_hash VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Sessions Table

```sql
CREATE TABLE sessions (
    id VARCHAR(255) PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    data JSONB,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Configuration Alerts Table (NEW)

```sql
CREATE TABLE configuration_alerts (
    alert_id VARCHAR(255) PRIMARY KEY,
    tenant_id VARCHAR(255) NOT NULL,
    realm_name VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,  -- 'realm', 'client', 'identity_provider'
    resource_id VARCHAR(255) NOT NULL,
    resource_name VARCHAR(255),
    check_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL,  -- 'info', 'warning', 'error', 'critical'
    status VARCHAR(20) NOT NULL,    -- 'active', 'acknowledged', 'resolved', 'ignored'
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    remediation TEXT,
    metadata JSONB,
    first_detected_at TIMESTAMPTZ NOT NULL,
    last_detected_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_alerts_tenant ON configuration_alerts(tenant_id);
CREATE INDEX idx_alerts_status ON configuration_alerts(status);
CREATE INDEX idx_alerts_severity ON configuration_alerts(severity);
CREATE INDEX idx_alerts_realm ON configuration_alerts(realm_name);
CREATE INDEX idx_alerts_type ON configuration_alerts(check_type);
CREATE INDEX idx_alerts_detected ON configuration_alerts(first_detected_at);
```

### Operator Alert Actions Table (NEW)

```sql
CREATE TABLE operator_alert_actions (
    action_id BIGSERIAL PRIMARY KEY,
    alert_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    operator_email VARCHAR(255) NOT NULL,
    operator_name VARCHAR(255),
    action_type VARCHAR(50) NOT NULL,  -- 'acknowledge', 'resolve', 'ignore', 'comment'
    previous_status VARCHAR(20),
    new_status VARCHAR(20),
    comment TEXT,
    alert_severity VARCHAR(20),
    alert_type VARCHAR(100),
    alert_detection_time TIMESTAMPTZ,
    action_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    response_time_seconds INT,  -- Time from detection to this action
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_operator_actions_email ON operator_alert_actions(operator_email);
CREATE INDEX idx_operator_actions_tenant ON operator_alert_actions(tenant_id);
CREATE INDEX idx_operator_actions_time ON operator_alert_actions(action_time);
CREATE INDEX idx_operator_actions_type ON operator_alert_actions(action_type);
CREATE INDEX idx_operator_actions_alert ON operator_alert_actions(alert_id);
```

## Security Architecture

### 1. Authentication

- **Session-based authentication** with secure HTTP-only cookies
- **Multiple authentication providers** (Simple, OAuth2)
- **Password hashing** using bcrypt with appropriate cost factor
- **Session expiration** and automatic cleanup

### 2. Authorization

- **Role-based access control**
- **Protected API endpoints** via middleware
- **Session validation** on every request

### 3. Data Security

- **PostgreSQL SSL/TLS support**
- **Secure session storage**
- **Password secrets in environment variables**
- **No credentials in logs**

### 4. Network Security

- **HTTPS support** (via reverse proxy)
- **CORS configuration** for frontend
- **SameSite cookies** for CSRF protection
- **Secure cookie flags** in production

### 5. Keycloak Integration Security

- **Client-credentials authentication**: the platform authenticates to the
  Keycloak Admin API as a dedicated confidential client (`client_credentials`
  grant). Its service account holds the permissions. 
- **Automatic token refresh**
- **TLS verification** (configurable)
- **Retry logic with backoff**

## Scalability Considerations

### Horizontal Scaling

- **Stateless API servers**: Can run multiple instances behind a load balancer
- **Shared database**: All instances connect to the same PostgreSQL cluster
- **Session storage**: Database-backed sessions for multi-instance support

### Vertical Scaling

- **Connection pooling**: Efficient database connection management
- **Configurable pool sizes**: Adjust based on load
- **Resource limits**: Container resource limits

### Database Optimization

- **TimescaleDB hypertables**: Automatic partitioning for time-series data
- **Indexes**: Optimized queries for common access patterns
- **Retention policies**: Automatic old data cleanup (can be configured)
- **Compression**: TimescaleDB compression for historical data

### Monitoring Efficiency

- **Configurable polling intervals**: Adjust based on needs
- **Batch event processing**: Fetch multiple events per poll
- **Connection reuse**: HTTP client connection pooling
- **Graceful degradation**: Continue monitoring other realms if one fails
- **Count/aggregate endpoints over enumeration**: user and session metrics use COUNT-only / realm-aggregate Admin API endpoints (`/users/count`, `/client-session-stats`) instead of listing users or sessions — avoiding large object transfers and, on federated realms, the per-entity storage-provider lookups (and WARN spam) that enumeration triggers
- **Payload-first event enrichment**: event username/email are taken from the event `details` when present, falling back to a per-user Admin API lookup (`GetUserByID`) only when needed — avoiding a storage-provider lookup per event on the 30s events cycle

### Future Scalability

- **Message queue** for event processing (RabbitMQ, Kafka)
- **Read replicas** for database scaling
- **Caching layer** (Redis) for frequently accessed data
- **Microservices** split for specific monitoring tasks

### 22. AMFA Module (`amfa/`)

**Purpose:** Read-only access to per-tenant AMFA (Adaptive Multi-Factor Authentication) PostgreSQL databases, surfacing AMFA login events, risk metrics, and geolocation in the KMT monitoring dashboard.

**Deployment model:** One AMFA service per Keycloak instance, owned by the same KMT tenant. KMT opens one read-only DB connection per tenant whose `amfa.enabled` is `true`. 

**Components:**

- `amfa.Registry` — per-tenant `Repository` registry built at startup by iterating `cfg.Keycloak.Tenants`. Tenants without `amfa.enabled = true` are absent; lookups for them return `ErrAmfaNotConfigured`.
- `amfa/postgres/` — read-only GORM client + repository. Uses the `gorm:"->"` read-only tag plus a dedicated least-privilege Postgres role (`kmt_ro`); `AutoMigrate` is never called on this connection. Per-tenant `EventsLookbackDays` (config) drives the default query window via `NewRepositoryWithLookback`; capped at 90 days regardless.
- `amfa/postgres/errors.go` — translates low-level driver / pgconn / network errors into `amfa.ErrAmfaUnavailable` so DB outages surface as HTTP 503 rather than 500. Query/syntax errors continue to bubble up as 500.
- `amfa/postgres/schema_check.go` — non-fatal `alembic_version` check at startup, with its own 5s deadline so a hung AMFA DB can never block KMT's monitoring boot. Logs WARN on drift, never returns a fatal error.
- `amfa.Enrichment` — Keycloak user-lookup cache backed by `hashicorp/golang-lru/v2` (bounded at 50k entries by default; capacity is configurable). 5-minute success TTL, 30-second negative TTL. Concurrent lookups for the same `(tenant, realm, user)` coalesce through `golang.org/x/sync/singleflight` so a cold cache or TTL boundary doesn't produce a thundering herd against Keycloak. Cache keys are typed structs (no delimiter collisions).
- `amfa.Service` — orchestrates `Registry` + `Enrichment` behind the `Service` interface.
- `internal/http/chi/amfa_handlers.go` — HTTP layer under `/api/tenants/{id}/amfa/{events,stats,geo}`, gated by `auth` → `RequireTenantAccess` → `RequirePermission("amfa:read")`. Input validation rejects malformed RFC3339 timestamps, inverted time ranges, and oversized offsets (`> 50_000`) with 400; `ErrAmfaUnavailable` from the data layer produces 503.
- `internal/fx/amfa.go` — fx module wiring the above; if no tenants enable AMFA, the registry is empty and all `/amfa/*` endpoints return 404 cleanly. The monitor-pool type-assertion logs a per-tenant WARN if a future Keycloak-client wrapper breaks the cast (regression guard).

**Permissions:** A new `amfa:read` permission is added by an idempotent data migration on KMT startup, granted to the default `admin`, `operator`, and `viewer` roles.

**Existing AMFA codebase is not modified.** All integration is purely additive on the KMT monitoring side; the only AMFA-side change is the operator-run `CREATE USER ... GRANT SELECT` block documented in `docs/amfa-integration.md`.

**Risk-alert checker (`amfacheck/`):** A per-tenant poll loop, sibling to `eventscheck/`, that turns high-risk AMFA login events into `domain.Alert`s routed through the existing alerts and notifications infrastructure (same lifecycle and notification fan-out as the Keycloak event checker). It reads AMFA's database through additive `amfa.Repository` methods, never modifying the AMFA schema. The package is deliberately kept parallel to `eventscheck/` rather than refactored behind a shared abstraction; see the design spec for the rationale. The checker is gated by the `keycloak.global.amfa_checker` config block (master switch off by default) and is documented operationally in `docs/amfa-integration.md`.

**Operator-facing setup guide:** [`docs/amfa-integration.md`](amfa-integration.md).

### 23. MCP Server (`cmd/mcp-server`)

**Purpose**: Read-only Model Context Protocol (MCP) server exposing tenants, realms, alerts, events and AMFA statistics to MCP clients such as Claude Code and Claude Desktop.

**Deployment model**: A third service alongside the API server and the web server, wired by its own Fx module (`internal/fx/mcp.go`) on a database client that never calls `AutoMigrate`. The optional `mcp.database` block points it at a dedicated read-only PostgreSQL role, so the process has no write capability at the database layer either. It is disabled by default: the binary exits immediately unless `mcp.enabled` is true.

**Transport**: Streamable HTTP in stateless mode on `POST /mcp`, port 7889 by default. Every request runs in its own temporary session, so no session identifier is issued and nothing accumulates server-side between calls. `/health`, `/ready` and, when `mcp.metrics.enabled` is set, a Prometheus `/metrics` endpoint share the port.

**Authentication**: Every request carries `Authorization: Bearer <personal access token>`, validated by the `apitoken` service against the stored SHA-256 digest. Tokens are created and revoked through the API server's `/api/tokens` endpoints; the MCP server holds no sessions and sets no cookies.

**Authorization**: Tools read through the same domain packages as the HTTP handlers (`tenant`, `keycloak`, `alerts`, `events`, `amfa`) and pass through the same `rbac.Service`. Effective scope is the intersection of the token's tenant allowlist and the acting user's RBAC roles and tenant policies, narrowed further to the realms that user's tenant policy allows, so a token can never reach further than the user it acts as.

**Key Files**:

- `main.go`: Entry point, configuration load, and the exit when the server is disabled
- `internal/mcp/`: Transport, authentication, per-tool authorization, and the guardrails every tool call passes through (error mapping, parameter caps, string sanitization, rate limiting, audit logging, per-tool metrics, response size cap)

**Operator guide and tool reference**: [`cmd/mcp-server/README.md`](../cmd/mcp-server/README.md).
