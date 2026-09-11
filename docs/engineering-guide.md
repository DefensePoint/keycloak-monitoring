# Engineering Guide

Developer guide for working with the Keycloak Monitoring Tool codebase.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Environment](#development-environment)
- [Project Structure](#project-structure)
- [Development Workflow](#development-workflow)
- [Testing](#testing)
- [Code Quality](#code-quality)
- [Frontend Development](#frontend-development)
- [Backend Development](#backend-development)
- [Database Development](#database-development)
- [Debugging](#debugging)
- [Common Tasks](#common-tasks)

## Getting Started

### Prerequisites

- **Go**: 1.23 or later
- **Node.js**: 18+ and npm
- **PostgreSQL**: 17 with TimescaleDB extension
- **Git**: For version control
- **Make**: Build automation
- **Docker**: (Optional) For containerized development
- **Swag**: (Optional) For Swagger documentation generation (`go install github.com/swaggo/swag/cmd/swag@latest`)

### Quick Setup

```bash
# Clone repository
git clone https://github.com/DefensePoint/keycloak-monitoring.git
cd keycloak-monitoring

# Install dependencies
make deps
make install-frontend

# Start PostgreSQL (Docker)
docker run -d --name postgres -p 5432:5432 \
  -e POSTGRES_DB=monitoring \
  -e POSTGRES_USER=monitoring \
  -e POSTGRES_PASSWORD=monitoring_password \
  timescale/timescaledb:latest-pg16

# Copy configuration
cp config.yaml.example config.yaml

# Run backend
make run

# In another terminal, run frontend
make dev-frontend
```

Access the application at <http://localhost:5173>

## Development Environment

### IDE Setup

#### VS Code

Recommended extensions:

- **Go** (golang.go)
- **ESLint** (dbaeumer.vscode-eslint)
- **Prettier** (esbenp.prettier-vscode)
- **TypeScript** (ms-vscode.vscode-typescript-next)

**.vscode/settings.json**:

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "editor.formatOnSave": true,
  "[go]": {
    "editor.defaultFormatter": "golang.go"
  },
  "[typescript]": {
    "editor.defaultFormatter": "esbenp.prettier-vscode"
  },
  "[typescriptreact]": {
    "editor.defaultFormatter": "esbenp.prettier-vscode"
  }
}
```

#### GoLand / IntelliJ IDEA

- Enable Go modules support
- Configure gofmt on save
- Enable golangci-lint integration

### Environment Variables

Create a `.env.local` file for development:

```bash
# Database
export MONITORING_DATABASE_HOST=localhost
export MONITORING_DATABASE_PASSWORD=monitoring_password

# Keycloak (client_credentials grant — dedicated confidential client)
export MONITORING_KEYCLOAK_SERVER_URL=http://localhost:8080
export MONITORING_KEYCLOAK_CLIENT_ID=monitoring-service
export MONITORING_KEYCLOAK_CLIENT_SECRET=your-client-secret

# Auth
export MONITORING_AUTH_SESSION_SECRET=dev-secret-key-minimum-32-characters

# Logging
export MONITORING_LOGGING_LEVEL=debug
export MONITORING_LOGGING_FORMAT=console
```

Load before running:

```bash
source .env.local
make run
```

## Project Structure

```bash
.
├── cmd/                     # Application entry points
│   ├── server/              # API server
│   └── web/                 # Web server
├── internal/                # Internal packages (private)
│   ├── config/              # Configuration loading
│   ├── fx/                  # Uber Fx dependency injection modules
│   ├── http/
│   │   ├── chi/             # Chi router, handlers
│   │   └── dto/             # Request/Response DTOs
│   ├── domain/              # Domain models and interfaces
│   ├── logger/              # Structured logging (slog)
│   └── version/             # Version information
├── pkg/                     # Shared packages
│   ├── database/            # GORM database client and models
│   └── keycloakadmin/       # Keycloak Admin API client
├── alerts/                  # Alerts domain package
├── auth/                    # Authentication package
├── rbac/                    # Role-based access control
├── tenant/                  # Multi-tenant management
├── users/                   # User management
├── keycloak/                # Keycloak monitoring service
├── configcheck/             # Security configuration checker
├── eventscheck/             # IDP event checking
├── metricscheck/            # Metrics validation
├── notifications/           # Notification channels (Slack, Email, GitLab)
├── operator/                # Operator metrics
├── reports/                 # Report generation
├── events/                  # Events domain
├── web/                     # Frontend
│   ├── src/
│   │   ├── components/      # React components
│   │   ├── pages/           # Page components
│   │   ├── services/        # API services
│   │   ├── types/           # TypeScript types
│   │   ├── contexts/        # React contexts
│   │   └── utils/           # Utilities
│   ├── public/              # Static assets
│   ├── package.json
│   └── vite.config.ts
├── deployments/             # Deployment configs
│   ├── local/               # Local docker-compose
│   └── server/              # Production docker-compose
├── docs/                    # Documentation
├── config.yaml.example      # Config template
├── Dockerfile               # Multi-stage build
├── Makefile                 # Build targets
└── go.mod                   # Go dependencies
```

## Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/my-feature
```

### 2. Make Changes

Be creative :)

### 3. Run Tests

```bash
# Backend tests
make test-backend

# Frontend tests
make test-frontend

# Or run all tests
make test-all
```

### 4. Format Code

```bash
# Go formatting
make fmt

# Go vetting
make vet

# Frontend formatting (if configured)
cd web && npm run lint
```

### 5. Commit Changes

```bash
git add .
git commit -m "feat: add new feature"
```

Use conventional commits:

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation
- `refactor:` Code refactoring
- `test:` Tests
- `chore:` Maintenance

### 6. Push and Create PR

```bash
git push origin feature/my-feature
```

Create a pull request on GitHub/GitLab.

## Testing

### Test categories at a glance

| Category | What it covers | Default run | How to opt in to skipped ones |
|---|---|---|---|
| **Unit tests** (Go) | Logic in isolation. No DB, no network. Stdlib `testing` only — no `testify`. | `go test -short ./...` | Always run |
| **Unit tests** (frontend) | React components + service URL/param construction. JSDOM, mocked deps. | `cd web && npm test -- --run` | Always run |
| **DB-backed tests** (env-gated) | Repository queries against a real PostgreSQL, using only fixtures they clean up, plus the MCP security posture suite. | Skip when `KMT_TEST_DATABASE_DSN` unset. | Export `KMT_TEST_DATABASE_DSN`. |
| **Destructive DB tests** (env-gated) | KMT's own migrations / RBAC seeders and the tenant purge path. Drop the schema or delete role rows globally. | Skip when `KMT_DESTRUCTIVE_TEST_DATABASE_DSN` unset; CI deliberately does not set it. | Point `KMT_DESTRUCTIVE_TEST_DATABASE_DSN` at a throwaway database. |
| **AMFA integration tests** (build-tag + env-gated) | AMFA repository SQL queries against a real AMFA Postgres. Tagged `//go:build integration`. | Skip in default run. | `go test -tags=integration ./amfa/postgres/...` with `AMFA_TEST_DSN` set. The schema check in the same package drops `alembic_version` and reads `AMFA_DESTRUCTIVE_TEST_DSN` instead. |
| **Alert pipeline e2e** (build-tag + env-gated) | The alert pipeline against a whole running deployment. Tagged `//go:build integration`. | Skip in default run. | `go test -tags=integration ./tests/...` with `KMT_E2E_TEST_DATABASE_DSN` set. See `tests/test_README.md`. |
| **Full e2e smoke** (manual) | UI walkthrough against running stacks. Not automated. | N/A | See `docs/amfa-integration.md` § Verification. |

> **No testify in this project.** All Go tests use stdlib `testing` (`if got != want { t.Errorf(...) }` style). Don't add `github.com/stretchr/testify` as a dependency.

### Backend — running with the Makefile

```bash
make test-backend           # go test -short -v -race -coverprofile=coverage.out ./...
make test-frontend          # cd web && npm test -- --run
make test-all               # both
```

`make test-backend` requires Go ≥1.25 on your `PATH`. If you don't have Go installed locally, use the Docker workflow below.

### Backend — running with Docker (no local Go required)

The project builds with `golang:1.25.14-alpine` (see `build/Dockerfile`). Same image runs tests:

```bash
# Warm cache once (downloads modules, ~1 min):
docker run --rm \
  -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
  golang:1.25.14-alpine \
  sh -c "apk add --no-cache git build-base && go mod download"

# Subsequent runs are fast (~30s):
docker run --rm \
  -v "$PWD:/app" -v go-mod-cache:/go/pkg/mod -w /app \
  golang:1.25.14-alpine \
  sh -c "apk add --no-cache git build-base >/dev/null 2>&1 && go build ./... && go test -short ./..."
```

The `go-mod-cache` named volume persists across runs so subsequent invocations don't re-download dependencies.

### Backend — running specific packages

```bash
# One package
go test -v ./amfa/postgres/...

# Race detector (CI default)
go test -race ./...

# Coverage
go test -cover ./...

# Verbose
go test -v ./internal/http/chi/...
```

### Backend — DB-backed unit tests

A small number of tests want a real PostgreSQL. The two below are read by the package tests and are separated by how much damage they do, each `t.Skip(...)`ing cleanly when its own variable is absent. Two more exist outside this table: `KMT_E2E_TEST_DATABASE_DSN` for the alert pipeline e2e, and `AMFA_TEST_DSN` / `AMFA_DESTRUCTIVE_TEST_DSN` for the AMFA integration tests. Four variables in total, and no test reads more than one of them.

| Variable | Read by | Point it at |
|---|---|---|
| `KMT_TEST_DATABASE_DSN` | The events repository tests in `events/postgres`, and the client-secret migration test in `pkg/database`. | Any dev database. These remove only the fixtures they create. |
| `KMT_DESTRUCTIVE_TEST_DATABASE_DSN` | The RBAC permission migration in `pkg/database/client_test.go`, and the tenant purge tests in `pkg/database` and `tenant/postgres`. | A throwaway database only. These run `DROP SCHEMA public CASCADE` and delete the admin, operator and viewer rows globally. |

Keep them separate. The destructive helpers also refuse to start unless the database name carries a `test`, `tmp` or `scratch` component, so a DSN aimed at `monitoring` fails loudly instead of wiping it, but that is a backstop and not a licence to reuse one variable for both.

Omitting `-p 1` below does not fail with a message about parallelism: one
package drops the schema while the other is mid-migration, and what you see is
`relation "user_roles" does not exist`. If a destructive run fails that way,
check `-p 1` before believing the schema is at fault.

Host port 55432 below is deliberate. The local stack publishes its `monitoring`
database on 5433 and a server deployment publishes one on 5432, so binding the
throwaway container on either would either fail to start or leave a DSN that
reaches a real database.

```bash
# Spin up a disposable Postgres (or use any existing dev DB):
docker run -d --name kmt-test-db --rm \
  -e POSTGRES_DB=kmt_test -e POSTGRES_USER=kmt -e POSTGRES_PASSWORD=kmt \
  -p 55432:5432 postgres:17-alpine

# Run with the DSN set:
KMT_DESTRUCTIVE_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
  go test -v ./pkg/database/...

# Tear down:
docker stop kmt-test-db
```

`pkg/database` and `tenant/postgres` both run `DROP SCHEMA public CASCADE` against `KMT_DESTRUCTIVE_TEST_DATABASE_DSN`, so they cannot share a database with a concurrently running package. Pass `-p 1` whenever more than one of them is in the same invocation:

```bash
KMT_DESTRUCTIVE_TEST_DATABASE_DSN="host=localhost port=55432 dbname=kmt_test user=kmt password=kmt sslmode=disable" \
  go test -p 1 ./pkg/database/... ./tenant/postgres/...
```

### Frontend tests

```bash
cd web

npm test -- --run               # all tests, single run
npm test                        # watch mode
npm run test:coverage           # with coverage
npx vitest run src/features/amfa/   # one feature
npx tsc --noEmit                # type-check (no tests, no emit)
npm run build                   # production build (catches more than tsc alone)
```

Frontend tests use **Vitest + @testing-library/react** in JSDOM. JSDOM-hostile libraries (`react-leaflet`, `react-leaflet-cluster`) must be `vi.mock()`-ed in any test that imports a component using them — see `web/src/features/amfa/components/AmfaGeoMap.test.tsx` and `web/src/features/amfa/pages/AmfaEventsPage.test.tsx` for examples.

### Writing tests

#### Backend — stdlib style

```go
package mypkg

import "testing"

func TestThing_DoesX(t *testing.T) {
    got := Thing{Name: "foo"}.DoX()
    if got != "expected" {
        t.Errorf("DoX(): got %q, want %q", got, "expected")
    }
}

// Table-driven
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name  string
        email string
        valid bool
    }{
        {"valid", "user@example.com", true},
        {"missing @", "notanemail", false},
        {"empty", "", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := ValidateEmail(tt.email); got != tt.valid {
                t.Errorf("ValidateEmail(%q) = %v, want %v", tt.email, got, tt.valid)
            }
        })
    }
}
```

#### Backend — integration test with build tag

```go
//go:build integration

package mypkg

import (
    "os"
    "testing"
)

func openTestDB(t *testing.T) *gorm.DB {
    dsn := os.Getenv("MY_TEST_DSN")
    if dsn == "" {
        t.Skip("MY_TEST_DSN not set; skipping integration test.")
    }
    db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
    if err != nil {
        t.Fatalf("connect: %v", err)
    }
    return db
}

func TestSomething_AgainstRealDB(t *testing.T) {
    db := openTestDB(t)
    // ... real query, real assertions ...
}
```

The build tag keeps the file out of compilation for default `go test ./...`. The `t.Skip()` keeps the test from failing when devs forget to set the env var. Both layers are intentional.

#### Frontend — component test

```typescript
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AmfaRiskBadge } from "./AmfaRiskBadge";

describe("AmfaRiskBadge", () => {
  it("renders the risk number", () => {
    render(<AmfaRiskBadge risk={3} />);
    expect(screen.getByText("3")).toBeInTheDocument();
  });
  it("renders a dash when risk is null", () => {
    render(<AmfaRiskBadge risk={null} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
```

### Common workflows

```bash
# Before committing — minimum bar
make test-all

# Before opening a PR — also include integration if touching AMFA
make test-all
# ...bring up AMFA, then:
AMFA_TEST_DSN="..." go test -tags=integration ./amfa/postgres/...

# Quick "did I break anything" while iterating (no race, no coverage):
go test -short ./...

# Just the package you're working on:
go test -v ./amfa/...
```

## Common pitfalls

A short list of footguns and the fixes. Add to this section when you find new ones.

### 1. `docker restart` does NOT pick up new images

After `make docker-build`, running `docker restart kmt-server` (or `-web`) restarts the **same container instance** with the **same image** it was originally created from. Tagging a new image as `:latest` doesn't migrate the running container to it.

To actually pick up a freshly rebuilt image:

```bash
make docker-down && make docker-up
```

That tears down the containers and `docker compose up` recreates them, picking up the latest tagged image.

Quick verification — these two SHAs should match after `down/up`:

```bash
docker inspect <container> --format 'Container Image: {{.Image}}'
docker images <image>:latest --format 'Latest Image ID: sha256:{{.ID}}'
```

### 2. `web/package-lock.json` must be regenerated inside the same Node image the Dockerfile uses

The Dockerfile's frontend stage uses `node:20-alpine` (npm 10). If you regenerate the lockfile on a macOS host running npm 11+, `npm ci` in the Docker build will fail with messages like:

```
Missing: yaml@2.9.0 from lock file
```

or the build will succeed but rollup will crash at runtime with:

```
Cannot find module '@rollup/rollup-linux-arm64-musl'
```

(because the host-generated lockfile only recorded the host's native rollup binary, not the linux-arm64-musl one the container needs.)

**Fix — always regenerate the lockfile in the same image:**

```bash
cd /path/to/kmt
docker run --rm \
  -v "$PWD/web:/web" -w /web \
  --platform linux/arm64 \
  node:20-alpine \
  sh -c "rm -f package-lock.json && npm install --no-audit --no-fund"
```

This produces a lockfile that lists every platform's optional deps (rollup ships per-platform binaries), so subsequent `npm ci` in the Docker build can install whatever the build container needs.

### 3. Docker Desktop VM disk fills up silently

`docker system df` shows hundreds of GB inflated by deduped layer-counting. The number that matters is **inside** the Linux VM. If a container starts crashing with `No space left on device` and your Mac shows plenty of free space, check the VM:

```bash
docker run --rm --volumes-from <a-running-container> alpine df -h /
```

If the filesystem is at 100%, free space without losing data:

```bash
docker builder prune -af   # safe — only slows next build (typically frees the most)
docker image prune -f      # safe — removes dangling images
```

If still tight, review unused volumes before pruning them (`docker volume ls --filter dangling=true`). Or bump the VM disk in Docker Desktop → Settings → Resources → Disk image size.

### 4. Leaflet maps need an explicit pixel height

If you're maintaining a Leaflet (or react-leaflet) component, **never give `MapContainer` a percentage height**. Two reasons:

1. **react-leaflet 4 only forwards `height` and `width` from the `style` prop.** Other keys (`minHeight`, etc.) are silently dropped.
2. **`height: 100%` races MUI's layout pass at first paint.** Leaflet measures itself in a `useEffect` immediately after mount; if the parent hasn't resolved its final height yet, Leaflet sees `0×0` and never re-measures. The map stays invisible — no tiles, no controls, no markers, no console errors.

Always pass pixels:

```tsx
<MapContainer style={{ height: 600, width: "100%" }} ...>
```

The canonical example is `web/src/features/amfa/components/AmfaGeoMap.tsx`, which carries an inline comment as a warning to future maintainers.

### 5. Distinguish DB availability errors from query/logic errors

When a handler queries a backing database, callers want to know the difference between *"the database is down right now, retry"* and *"the query was bad, fix your code/config"*. The former should map to HTTP 503 and trigger ops alerting; the latter should map to HTTP 500 and trigger a bug ticket.

The cleanest pattern is a small classifier at the repository boundary that translates connection-level errors to a sentinel:

```go
// amfa/postgres/errors.go (canonical example)
func classifyDBError(err error, op string) error {
    if err == nil { return nil }
    if errors.Is(err, context.DeadlineExceeded) ||
       errors.Is(err, context.Canceled) ||
       errors.Is(err, driver.ErrBadConn) {
        return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && strings.HasPrefix(pgErr.Code, "08") {
        return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
    }
    var netErr *net.OpError
    if errors.As(err, &netErr) {
        return fmt.Errorf("%s: %w: %v", op, amfa.ErrAmfaUnavailable, err)
    }
    return fmt.Errorf("%s: %w", op, err)
}
```

The handler then has a clean three-way branch:

```go
events, _, err := h.service.ListEvents(ctx, tenantID, opts)
if errors.Is(err, amfa.ErrAmfaNotConfigured) {
    httputil.RespondJSON(w, h.log, http.StatusNotFound, ...)
    return
}
if errors.Is(err, amfa.ErrAmfaUnavailable) {
    httputil.RespondJSON(w, h.log, http.StatusServiceUnavailable, ...)
    return
}
if err != nil {
    httputil.RespondInternalError(w, h.log, "...")
    return
}
```

Without classification, handlers tend to grow either dead `503` branches (if a sentinel is checked but never produced) or undifferentiated `500`s for everything. See `amfa/postgres/errors.go` for the full version including SQLSTATE class `08` (Connection Exception) and a heuristic substring fallback for plain-text errors from older drivers.

## Code Quality

### Linting

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
make lint

# Or directly
golangci-lint run
```

### Code Formatting

```bash
# Format Go code
make fmt

# Check formatting
gofmt -l .

# Format TypeScript/JavaScript
cd web && npm run lint:fix
```

## Frontend Development

### Running Dev Server

```bash
cd web
npm run dev
```

Access at <http://localhost:5173> with hot module replacement.

### Adding a New Component

```typescript
// web/src/components/MyComponent.tsx
import React from 'react';

interface MyComponentProps {
  title: string;
  count: number;
}

export const MyComponent: React.FC<MyComponentProps> = ({ title, count }) => {
  return (
    <div>
      <h2>{title}</h2>
      <p>Count: {count}</p>
    </div>
  );
};
```

### API Integration

```typescript
// web/src/services/api.ts
class ApiService {
  async getMyData(): Promise<MyData[]> {
    const response = await this.fetchWithAuth('/api/my-endpoint');
    if (!response.ok) throw new Error('Failed to fetch');
    return response.json();
  }
}
```

### State Management

Using React Context:

```typescript
// web/src/contexts/MyContext.tsx
import React, { createContext, useContext, useState } from 'react';

interface MyContextType {
  data: string;
  setData: (data: string) => void;
}

const MyContext = createContext<MyContextType | undefined>(undefined);

export const MyProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [data, setData] = useState('');

  return (
    <MyContext.Provider value={{ data, setData }}>
      {children}
    </MyContext.Provider>
  );
};

export const useMyContext = () => {
  const context = useContext(MyContext);
  if (!context) throw new Error('useMyContext must be used within MyProvider');
  return context;
};
```

## Backend Development

### Adding a New API Endpoint

The project uses Chi router with Uber Fx for dependency injection and Swagger for API documentation.

#### 1. Define Request/Response DTOs

```go
// internal/http/dto/requests/my_requests.go
package requests

type CreateMyDataRequest struct {
    Name        string `json:"name" validate:"required,min=1,max=255"`
    Description string `json:"description" validate:"max=1000"`
}

func (r *CreateMyDataRequest) Validate() error {
    return validator.New().Struct(r)
}
```

#### 2. Define the Handler

```go
// internal/http/chi/my_handlers.go
package chi

type MyHandlers struct {
    service *mypackage.Service
    logger  *slog.Logger
}

func NewMyHandlers(service *mypackage.Service, logger *slog.Logger) *MyHandlers {
    return &MyHandlers{service: service, logger: logger}
}

// GetMyData godoc
// @Summary Get my data
// @Description Retrieves all my data items
// @Tags my-data
// @Accept json
// @Produce json
// @Success 200 {array} mypackage.MyData
// @Failure 500 {object} ErrorResponse
// @Router /api/my-data [get]
func (h *MyHandlers) GetMyData(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    data, err := h.service.GetAll(ctx)
    if err != nil {
        h.logger.Error("Failed to get data", "error", err)
        respondError(w, http.StatusInternalServerError, "Internal server error")
        return
    }

    respondJSON(w, http.StatusOK, data)
}
```

#### 3. Register Routes

```go
// internal/http/chi/router.go
func (rt *Router) setupRoutes() {
    rt.chi.Route("/api", func(r chi.Router) {
        // ... existing routes ...
        r.Route("/my-data", func(r chi.Router) {
            r.Get("/", rt.myHandlers.GetMyData)
            r.Post("/", rt.myHandlers.CreateMyData)
            r.Get("/{id}", rt.myHandlers.GetMyDataByID)
        })
    })
}
```

#### 4. Add Service Method

```go
// mypackage/service.go
type Service struct {
    repo   Repository
    logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *Service {
    return &Service{repo: repo, logger: logger}
}

func (s *Service) GetAll(ctx context.Context) ([]MyData, error) {
    return s.repo.FindAll(ctx)
}
```

#### 5. Register in Fx Module

```go
// internal/fx/module.go
func Module() fx.Option {
    return fx.Options(
        fx.Provide(
            // ... existing providers ...
            mypackage.NewService,
            chi.NewMyHandlers,
        ),
    )
}
```

### Dependency Injection with Uber Fx

The application uses Uber Fx for dependency injection. Components are organized into modules:

```go
// internal/fx/module.go
func Module() fx.Option {
    return fx.Options(
        // Configuration
        fx.Provide(config.Load),

        // Infrastructure
        fx.Provide(logger.New),
        fx.Provide(database.NewClient),

        // Services
        fx.Provide(alerts.NewService),
        fx.Provide(users.NewService),
        fx.Provide(tenant.NewService),
        fx.Provide(rbac.NewService),

        // HTTP
        fx.Provide(chi.NewRouter),
        fx.Provide(chi.NewAlertHandlers),
        fx.Provide(chi.NewUserHandlers),
        fx.Provide(chi.NewTenantHandlers),

        // Start server
        fx.Invoke(RegisterRoutes),
    )
}

// cmd/server/main.go
func main() {
    fx.New(
        fx.Module(),
        fx.Invoke(func(lc fx.Lifecycle, router *chi.Router, cfg *config.Config) {
            lc.Append(fx.Hook{
                OnStart: func(ctx context.Context) error {
                    go router.ListenAndServe(cfg.Server.Port)
                    return nil
                },
                OnStop: func(ctx context.Context) error {
                    return router.Shutdown(ctx)
                },
            })
        }),
    ).Run()
}
```

### Generating Swagger Documentation

```bash
# Install swag
go install github.com/swaggo/swag/cmd/swag@latest

# Generate docs
swag init -g cmd/server/main.go -o docs/swagger

# Access swagger UI at /swagger/index.html
```

## Database Development

The project uses GORM for database operations.

### Adding a New Model

```go
// pkg/database/models.go
type MyData struct {
    ID        uint           `gorm:"primaryKey"`
    Name      string         `gorm:"size:255;not null"`
    TenantID  string         `gorm:"size:255;not null;index"`
    CreatedAt time.Time      `gorm:"autoCreateTime"`
    UpdatedAt time.Time      `gorm:"autoUpdateTime"`
    DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (MyData) TableName() string {
    return "my_data"
}
```

### Adding a Repository

```go
// mypackage/repository.go
type Repository interface {
    FindAll(ctx context.Context) ([]MyData, error)
    FindByID(ctx context.Context, id uint) (*MyData, error)
    Create(ctx context.Context, data *MyData) error
    Update(ctx context.Context, data *MyData) error
    Delete(ctx context.Context, id uint) error
}

type repository struct {
    db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
    return &repository{db: db}
}

func (r *repository) FindAll(ctx context.Context) ([]MyData, error) {
    var results []MyData
    if err := r.db.WithContext(ctx).Find(&results).Error; err != nil {
        return nil, err
    }
    return results, nil
}

func (r *repository) Create(ctx context.Context, data *MyData) error {
    return r.db.WithContext(ctx).Create(data).Error
}
```

### Running Migrations

GORM auto-migrates models on startup:

```go
// pkg/database/client.go
func (c *Client) AutoMigrate() error {
    return c.db.AutoMigrate(
        &MyData{},
        &Alert{},
        &User{},
        // ... other models
    )
}
```

### Using TimescaleDB

For time-series data, use raw SQL to create hypertables:

```go
const createTimeseriesTable = `
CREATE TABLE IF NOT EXISTS metrics (
    time TIMESTAMPTZ NOT NULL,
    metric_name VARCHAR(255),
    value DOUBLE PRECISION,
    labels JSONB
);

SELECT create_hypertable('metrics', 'time', if_not_exists => TRUE);
`

func (c *Client) CreateHypertables() error {
    return c.db.Exec(createTimeseriesTable).Error
}
```

## Debugging

### Backend Debugging

#### Using Delve

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug server
dlv debug ./cmd/server

# Or attach to running process
dlv attach <pid>
```

#### VS Code Debug Configuration

**.vscode/launch.json**:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug API Server",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/server",
      "env": {
        "MONITORING_LOGGING_LEVEL": "debug"
      }
    }
  ]
}
```

#### Logging

```go
// Add debug logging
s.logger.Debug("Processing request",
    logger.Str("path", r.URL.Path),
    logger.Str("method", r.Method),
    logger.Any("params", params))
```

### Frontend Debugging

#### Browser DevTools

- Use React DevTools extension
- Check Network tab for API calls
- Use Console for errors

#### Source Maps

Vite automatically generates source maps in development.

## Common Tasks

### Adding a New Configuration Option

#### 1. Update Config Struct

```go
// internal/config/config.go
type Config struct {
    // ... existing fields ...
    MyNewOption string `mapstructure:"my_new_option"`
}
```

#### 2. Update Example Config

```yaml
# config.yaml.example
my_new_option: "default_value"
```

#### 3. Update Documentation

Add to [docs/configuration.md](configuration.md)

### Adding a New Keycloak Metric

#### 1. Fetch from Keycloak API

```go
// pkg/keycloak/metrics.go
func (c *Client) GetMyMetric(ctx context.Context, realm string) (int, error) {
    path := fmt.Sprintf("/admin/realms/%s/my-metric", realm)
    resp, err := c.get(ctx, path)
    // ... parse and return
}
```

#### 2. Store in Database

```go
// pkg/database/repository.go
func (r *Repository) SaveMyMetric(ctx context.Context, metric *MyMetric) error {
    query := `INSERT INTO my_metrics (realm, value, timestamp) VALUES ($1, $2, $3)`
    _, err := r.pool.Exec(ctx, query, metric.Realm, metric.Value, time.Now())
    return err
}
```

#### 3. Add API Endpoint

```go
// pkg/api/keycloak_handlers.go
func (s *Server) handleGetMyMetric(w http.ResponseWriter, r *http.Request) {
    // ... implementation
}
```

### Updating Dependencies

```bash
# Go dependencies
go get -u ./...
go mod tidy

# Frontend dependencies
cd web
npm update
npm audit fix
```
