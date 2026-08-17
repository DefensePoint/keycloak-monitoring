# Keycloak Monitoring Tool - Frontend

## Overview
Multi-tenant Keycloak monitoring system with RBAC.

## Tech Stack
- React 18 + TypeScript 5
- Vite (build tool)
- React Query v5 (data fetching)
- Material-UI v7 + Tailwind CSS (styling)
- React Router v6 (routing)

## Architecture: Feature-Based

```
src/
├── app/        → Bootstrap and global configuration
├── features/   → Domain modules (each feature is isolated)
├── shared/     → Reusable code without business logic
├── theme/      → Design system and tokens
└── assets/     → Static files
```

## Dependency Rules

```
app      → can import from: features, shared, theme
features → can import from: shared, theme (NEVER from other features)
shared   → can import from: theme (NEVER from features)
theme    → does not import from any internal folder
```

## Global Conventions

### Imports
- Use path alias `@/` for absolute imports
- Order: external libs → shared → features → relative

### Naming
- Components: PascalCase (`UserCard.tsx`)
- Hooks: camelCase with `use` prefix (`useAuth.ts`)
- Services: camelCase with `Service` suffix (`authService.ts`)
- Types: PascalCase with descriptive suffix (`user.types.ts`)
- Constants: SCREAMING_SNAKE_CASE

### Components
- Always functional with hooks
- Props typed with interface (not type)
- Named exports (not default)

### TypeScript
- Strict mode enabled
- Avoid `any` - use `unknown` if necessary
- Prefer interfaces for objects, types for unions

### Testing
- Files `.test.tsx` next to component
- Vitest + Testing Library
- Minimum: render tests and critical interactions

### Git
- Commits in English, conventional commits
- Branch naming: `feat/`, `fix/`, `refactor/`, `chore/`

## Data Fetching with React Query

### Fundamental Rules
- NEVER make API calls directly in components
- NEVER use try/catch in components for API calls
- NEVER use useState to manage loading/error states for requests
- ALWAYS encapsulate API calls in custom hooks with useQuery/useMutation

### Structure per Feature
```
feature/
├── hooks/
│   ├── useItems.ts        # useQuery for lists
│   ├── useItem.ts         # useQuery for detail
│   ├── useCreateItem.ts   # useMutation for create
│   ├── useUpdateItem.ts   # useMutation for update
│   └── useDeleteItem.ts   # useMutation for delete
├── services/
│   └── itemService.ts     # Pure HTTP calls (no React)
└── pages/
    └── ItemsPage.tsx      # Uses hooks, not services directly
```

### Hook Pattern with useQuery
```typescript
export const useItems = (tenantId: string) => {
  return useQuery({
    queryKey: ['items', tenantId],
    queryFn: () => itemService.getItems(tenantId),
    enabled: !!tenantId,
  })
}
```

### Hook Pattern with useMutation
```typescript
export const useCreateItem = () => {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateItemDto) => itemService.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['items'] })
    },
  })
}
```

### In Components
```typescript
// CORRECT
const { data, isLoading, error } = useItems(tenantId)
const { mutate, isPending } = useCreateItem()

// INCORRECT - Never do this
const [data, setData] = useState(null)
const [loading, setLoading] = useState(false)
try { const result = await service.get() } catch (e) { }
```
