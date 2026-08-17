# App - Bootstrap and Configuration

## Purpose
Application initialization, provider composition, and route configuration.

## Structure
```
app/
├── providers/     → Global providers (Auth, Theme, Query, etc.)
├── routes/        → Route configuration and guards
├── App.tsx        → Root component
└── main.tsx       → Entry point
```

## Rules

### ✅ CAN
- Import from `features/`, `shared/`, `theme/`
- Compose global providers
- Define application routes
- Configure global error boundaries

### ❌ CANNOT
- Contain business logic
- Have UI components besides wrappers
- Make API calls directly

## Patterns

### Provider Composition
```tsx
// providers/index.tsx
export const AppProviders = ({ children }: PropsWithChildren) => (
  <QueryProvider>
    <ThemeProvider>
      <AuthProvider>
        {children}
      </AuthProvider>
    </ThemeProvider>
  </QueryProvider>
)
```

### Routes
- Lazy-loaded routes per feature
- Authentication guards via `ProtectedRoute`
- Paths centralized in `routePaths.ts`

```tsx
// routes/routes.tsx
const DashboardPage = lazy(() => import('@/features/dashboard'))

export const routes = [
  {
    path: routePaths.dashboard,
    element: <ProtectedRoute><DashboardPage /></ProtectedRoute>
  }
]
```
