# Hacking Guide

Guide for contributors and developers who want to understand and modify the Keycloak Monitoring Tool codebase.

## Table of Contents

- [Code Organization](#code-organization)
- [Coding Conventions](#coding-conventions)
- [Adding New Features](#adding-new-features)
- [Contribution Workflow](#contribution-workflow)
- [Testing Guidelines](#testing-guidelines)
- [Documentation Standards](#documentation-standards)
- [Performance Considerations](#performance-considerations)
- [Security Guidelines](#security-guidelines)

## Code Organization

### Package Structure

The project follows Go standard project layout:

- **`cmd/`**: Application entry points (binaries)
- **`internal/`**: Private application code (cannot be imported by external projects)
- **`pkg/`**: Public libraries (can be imported by external projects)
- **`web/`**: Frontend React application
- **`docs/`**: Documentation
- **`deployments/`**: Deployment configurations

### Internal vs Pkg vs Root-Level

**Use `internal/` for**:

- Application-specific logic
- Configuration loading (`internal/config/`)
- HTTP server setup (`internal/http/chi/`)
- Dependency injection (`internal/fx/`)
- Request/Response DTOs (`internal/http/dto/`)
- Logging infrastructure (`internal/logger/`)

**Use `pkg/` for**:

- Shared infrastructure components
- Database client and models (`pkg/database/`)
- External API clients (`pkg/keycloakadmin/`)

**Use root-level packages for**:

- Domain packages with services and repositories
- `alerts/`, `auth/`, `rbac/`, `tenant/`, `users/`
- `keycloak/`, `configcheck/`, `notifications/`
- `events/`, `reports/`, `operator/`

## Coding Conventions

### Go Code Style

#### Naming

```go
// Exported (public) - PascalCase
type UserRepository struct {}
func NewUserRepository() *UserRepository {}

// Unexported (private) - camelCase
type userCache struct {}
func newUserCache() *userCache {}

// Constants - SCREAMING_SNAKE_CASE or camelCase
const (
    DefaultTimeout = 30 * time.Second
    maxRetries     = 5
)

// Interface names
type Writer interface {}  // Not WriterInterface
type Reader interface {}  // Not IReader
```

#### Error Handling

```go
// Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to connect to database: %w", err)
}

// Bad: Lose error context
if err != nil {
    return err
}

// Good: Check and handle errors immediately
data, err := fetchData()
if err != nil {
    return nil, fmt.Errorf("fetch failed: %w", err)
}

// Bad: Defer error handling
data, err := fetchData()
// ... many lines later ...
if err != nil {
    return err
}
```

#### Logging

The project uses Go's `slog` for structured logging:

```go
// Good: Structured logging with slog
logger.Error("Failed to process request",
    "error", err,
    "user_id", userID,
    "attempt", retryCount)

// Bad: String concatenation
logger.Error("Failed to process request for user " + userID)

// Good: Use appropriate log levels
logger.Debug("Processing item", "id", id)  // Development
logger.Info("Server started", "port", port)  // Important info
logger.Warn("Rate limit approaching", "count", count)  // Warnings
logger.Error("Operation failed", "error", err)  // Errors

// Bad: Everything at Error level
logger.Error("Processing item 123")
```

#### Comments

```go
// Good: Explain WHY, not WHAT
// Use exponential backoff to avoid overwhelming the server during recovery
time.Sleep(backoff * time.Duration(attempt))

// Bad: Obvious comments
// Sleep for backoff time
time.Sleep(backoff * time.Duration(attempt))

// Good: Package documentation
// Package keycloak provides a client for interacting with Keycloak Admin API.
// It handles authentication, token management, and provides methods for
// collecting metrics and events from Keycloak instances.
package keycloak

// Good: Exported function documentation
// NewClient creates a new Keycloak client with the provided configuration.
// It automatically detects the Keycloak version and handles authentication.
// Returns an error if connection or authentication fails.
func NewClient(cfg *Config) (*Client, error) {
```

### TypeScript/React Code Style

#### Component Structure

```typescript
// Good: Functional components with TypeScript
interface MyComponentProps {
  title: string;
  onAction: () => void;
}

export const MyComponent: React.FC<MyComponentProps> = ({ title, onAction }) => {
  const [state, setState] = useState<string>('');

  useEffect(() => {
    // Effect logic
  }, []);

  return (
    <div>
      <h2>{title}</h2>
      <button onClick={onAction}>Action</button>
    </div>
  );
};
```

#### API Calls

```typescript
// Good: Centralized API service
class ApiService {
  private async fetchWithAuth(url: string): Promise<Response> {
    // Handle auth, errors, etc.
  }

  async getUsers(realm: string): Promise<User[]> {
    const response = await this.fetchWithAuth(`/api/users?realm=${realm}`);
    if (!response.ok) throw new Error('Failed to fetch users');
    return response.json();
  }
}

// Bad: Direct fetch in components
const MyComponent = () => {
  useEffect(() => {
    fetch('/api/users')
      .then(r => r.json())
      .then(data => setUsers(data));
  }, []);
};
```

## Adding New Features

### Step-by-Step Process

#### 1. Create Feature Branch

```bash
git checkout -b feature/my-awesome-feature
```

#### 2. Plan the Implementation

- Identify affected components
- Design the data model
- Plan the API endpoints
- Consider database schema changes

#### 3. Implement Backend

a. Add GORM Model

```go
// myfeature/model.go
type MyFeature struct {
    ID        uint           `gorm:"primaryKey" json:"id"`
    Name      string         `gorm:"size:255;not null" json:"name"`
    TenantID  string         `gorm:"size:255;not null;index" json:"tenant_id"`
    CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MyFeature) TableName() string {
    return "my_features"
}
```

b. Add Repository Interface and Implementation

```go
// myfeature/repository.go
type Repository interface {
    Create(ctx context.Context, feature *MyFeature) error
    FindByID(ctx context.Context, id uint) (*MyFeature, error)
    FindAll(ctx context.Context) ([]MyFeature, error)
}

type repository struct {
    db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
    return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, feature *MyFeature) error {
    return r.db.WithContext(ctx).Create(feature).Error
}

func (r *repository) FindByID(ctx context.Context, id uint) (*MyFeature, error) {
    var feature MyFeature
    if err := r.db.WithContext(ctx).First(&feature, id).Error; err != nil {
        return nil, err
    }
    return &feature, nil
}
```

c. Add Service

```go
// myfeature/service.go
type Service struct {
    repo   Repository
    logger *slog.Logger
}

func NewService(repo Repository, logger *slog.Logger) *Service {
    return &Service{repo: repo, logger: logger}
}

func (s *Service) Create(ctx context.Context, feature *MyFeature) error {
    return s.repo.Create(ctx, feature)
}
```

d. Add Request DTO

```go
// internal/http/dto/requests/myfeature.go
type CreateMyFeatureRequest struct {
    Name string `json:"name" validate:"required,min=1,max=255"`
}

func (r *CreateMyFeatureRequest) Validate() error {
    return validator.New().Struct(r)
}
```

e. Add API Handler

```go
// internal/http/chi/myfeature_handlers.go
type MyFeatureHandlers struct {
    service *myfeature.Service
    logger  *slog.Logger
}

func NewMyFeatureHandlers(service *myfeature.Service, logger *slog.Logger) *MyFeatureHandlers {
    return &MyFeatureHandlers{service: service, logger: logger}
}

// CreateMyFeature godoc
// @Summary Create a new feature
// @Tags my-feature
// @Accept json
// @Produce json
// @Param request body requests.CreateMyFeatureRequest true "Create request"
// @Success 201 {object} myfeature.MyFeature
// @Router /api/my-feature [post]
func (h *MyFeatureHandlers) CreateMyFeature(w http.ResponseWriter, r *http.Request) {
    var req requests.CreateMyFeatureRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "Invalid request body")
        return
    }

    if err := req.Validate(); err != nil {
        respondError(w, http.StatusBadRequest, err.Error())
        return
    }

    feature := &myfeature.MyFeature{Name: req.Name}
    if err := h.service.Create(r.Context(), feature); err != nil {
        h.logger.Error("Failed to create feature", "error", err)
        respondError(w, http.StatusInternalServerError, "Failed to create feature")
        return
    }

    respondJSON(w, http.StatusCreated, feature)
}
```

f. Register in Fx Module and Router

```go
// internal/fx/module.go
fx.Provide(myfeature.NewRepository),
fx.Provide(myfeature.NewService),
fx.Provide(chi.NewMyFeatureHandlers),

// internal/http/chi/router.go
r.Route("/my-feature", func(r chi.Router) {
    r.Post("/", rt.myFeatureHandlers.CreateMyFeature)
    r.Get("/{id}", rt.myFeatureHandlers.GetMyFeature)
})
```

#### 4. Implement Frontend

a. Add TypeScript Types

```typescript
// web/src/types/my-feature.ts
export interface MyFeature {
  id: number;
  name: string;
  created_at: string;
}
```

b. Add API Service Method

```typescript
// web/src/services/api.ts
async createMyFeature(name: string): Promise<MyFeature> {
  const response = await this.fetchWithAuth('/api/my-feature', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  });
  if (!response.ok) throw new Error('Failed to create feature');
  return response.json();
}
```

c. Create Component

```typescript
// web/src/components/MyFeatureForm.tsx
import React, { useState } from 'react';
import { apiService } from '../services/api';

export const MyFeatureForm: React.FC = () => {
  const [name, setName] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await apiService.createMyFeature(name);
      setName('');
      alert('Feature created successfully');
    } catch (error) {
      console.error('Failed to create feature:', error);
      alert('Failed to create feature');
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={handleSubmit}>
      <input
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Feature name"
        disabled={loading}
      />
      <button type="submit" disabled={loading}>
        {loading ? 'Creating...' : 'Create'}
      </button>
    </form>
  );
};
```

#### 5. Write Tests

```go
// pkg/database/repository_test.go
func TestCreateMyFeature(t *testing.T) {
    repo := setupTestRepository(t)

    feature := &domain.MyFeature{Name: "Test Feature"}
    err := repo.CreateMyFeature(context.Background(), feature)

    assert.NoError(t, err)
    assert.NotZero(t, feature.ID)
    assert.NotZero(t, feature.CreatedAt)
}
```

#### 6. Update Documentation

Add to relevant documentation files:

- API Reference
- User guide (if user-facing)
- Configuration (if new config options)

## Contribution Workflow

### 1. Fork and Clone

```bash
# Fork on GitHub/GitLab
git clone https://github.com/DefensePoint/keycloak-monitoring.git
cd keycloak-monitoring
git remote add upstream <original-repo-url>
```

### 2. Create Feature Branch

```bash
git checkout -b feature/my-feature
```

### 3. Make Changes

Follow coding conventions and test your changes.

### 4. Commit

Use conventional commits:

```bash
git commit -m "feat: add user profile feature"
git commit -m "fix: resolve database connection leak"
git commit -m "docs: update API documentation"
git commit -m "refactor: simplify authentication logic"
git commit -m "test: add integration tests for events"
```

**Commit Types**:

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style (formatting, no logic change)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Build, dependencies, tooling
- `perf`: Performance improvements

### 5. Push and Create PR

```bash
git push origin feature/my-feature
```

Create a pull request with:

- Clear description of changes
- Reference to related issues
- Screenshots (for UI changes)
- Test results

### 6. Code Review

Address feedback from reviewers.

### 7. Merge

After approval, maintainers will merge your PR.

## Testing Guidelines

This section covers **how to write** tests. For **how to run** them (Makefile targets, Docker workflow without a local Go install, env-gated and build-tagged tests, etc.), see [Testing in the Engineering Guide](engineering-guide.md#testing).

### Style rules

- **Backend tests use stdlib `testing` only.** No `testify`, no `gomock`. Write `if got != want { t.Errorf(...) }` and `t.Fatalf(...)` directly.
- **Frontend tests use Vitest + `@testing-library/react`.** Not Jest. Import from `vitest`: `import { describe, expect, it, vi } from "vitest"`.
- Co-locate tests next to the code: `foo.go` → `foo_test.go`; `Bar.tsx` → `Bar.test.tsx`.
- Name tests with the convention `TestThing_DoesX` / `TestThing_WhenY_DoesZ`.

### Unit tests (Go)

Test individual functions and methods. No DB, no network — use plain Go and stdlib testing:

```go
func TestValidateEmail(t *testing.T) {
    tests := []struct {
        name  string
        email string
        valid bool
    }{
        {"valid email", "user@example.com", true},
        {"invalid email", "notanemail", false},
        {"empty email", "", false},
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

### Integration tests (Go)

Tests that need external systems (a real Postgres, a real Keycloak, etc.) belong in `_integration_test.go` files double-gated by a build tag **and** an env-var check. This keeps them out of `go test ./...` by default and lets devs opt in explicitly.

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

The canonical example is `amfa/postgres/repository_integration_test.go`. See the engineering guide for the exact commands to bring up dependencies (AMFA's docker-compose) and run these tests via Docker or local Go.

### Frontend tests

```typescript
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MyComponent } from "./MyComponent";

describe("MyComponent", () => {
  it("renders and handles click", () => {
    const handleClick = vi.fn();
    render(<MyComponent onAction={handleClick} />);

    fireEvent.click(screen.getByRole("button"));

    expect(handleClick).toHaveBeenCalledTimes(1);
  });
});
```

For components that import JSDOM-hostile libraries (e.g., `react-leaflet`, `react-leaflet-cluster`), `vi.mock(...)` them at the top of the test file — see `web/src/features/amfa/components/AmfaGeoMap.test.tsx`.

## Documentation Standards

### Code Documentation

```go
// Package-level documentation
// Package api provides HTTP handlers and routing for the monitoring API.
// It includes handlers for events, statistics, and Keycloak monitoring endpoints.
package api

// Type documentation
// Server represents the HTTP API server with all its dependencies.
type Server struct {
    router     *mux.Router
    logger     *logger.Logger
    repository *database.Repository
}

// Function documentation
// NewServer creates a new API server instance with the provided configuration.
// It initializes routes, middleware, and all required dependencies.
//
// Parameters:
//   - cfg: Application configuration
//   - repo: Database repository
//   - logger: Application logger
//
// Returns:
//   - *Server: Configured server instance
//   - error: Error if initialization fails
func NewServer(cfg *config.Config, repo *database.Repository, logger *logger.Logger) (*Server, error) {
```

### README and Guides

- Use clear, concise language
- Include code examples
- Provide step-by-step instructions
- Add diagrams for complex concepts
- Keep documentation up-to-date

## Performance Considerations

### Database Queries

```go
// Good: Use connection pooling
pool, err := pgxpool.Connect(ctx, connString)

// Good: Use prepared statements for repeated queries
stmt := "SELECT * FROM events WHERE realm = $1 LIMIT $2"

// Good: Limit result sets
query := "SELECT * FROM events LIMIT 1000"

// Bad: Unbounded queries
query := "SELECT * FROM events"  // Could return millions of rows
```

### Concurrency

```go
// Good: Use context for cancellation
func (m *Monitor) Start(ctx context.Context) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.collect()
        }
    }
}

// Good: Use goroutines with proper synchronization
var wg sync.WaitGroup
for _, realm := range realms {
    wg.Add(1)
    go func(r string) {
        defer wg.Done()
        processRealm(r)
    }(realm)
}
wg.Wait()
```

### Frontend Performance

```typescript
// Good: Memoize expensive computations
const sortedData = useMemo(() => {
  return data.sort((a, b) => a.timestamp - b.timestamp);
}, [data]);

// Good: Debounce user input
const debouncedSearch = useMemo(
  () => debounce((query) => search(query), 300),
  []
);
```

## Security Guidelines

### Input Validation

```go
// Good: Validate all inputs
func validateUserInput(name string) error {
    if len(name) == 0 {
        return errors.New("name cannot be empty")
    }
    if len(name) > 255 {
        return errors.New("name too long")
    }
    return nil
}

// Good: Use parameterized queries
query := "SELECT * FROM users WHERE email = $1"
row := pool.QueryRow(ctx, query, email)
```

### Authentication

```go
// Good: Hash passwords
hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

// Good: Use secure session cookies
cookie := &http.Cookie{
    Name:     "session",
    Value:    sessionID,
    HttpOnly: true,  // Prevent XSS
    Secure:   true,  // HTTPS only
    SameSite: http.SameSiteLaxMode,  // CSRF protection
}
```

### Secrets Management

```go
// Good: Never log secrets
logger.Info("Connected to database", logger.Str("host", dbHost))

// Bad: Logging sensitive data
logger.Info("Connected", logger.Str("password", dbPassword))  // NEVER DO THIS

// Good: Use environment variables for secrets
password := os.Getenv("DATABASE_PASSWORD")

// Bad: Hardcoded secrets
password := "my_secret_password"  // NEVER DO THIS
```
