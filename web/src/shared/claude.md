# Shared - Shared Code

## Purpose
Reusable code across features, WITHOUT business logic.

## Structure
```
shared/
├── components/   → Generic UI components
├── hooks/        → Utility hooks
├── lib/          → External library configuration
├── utils/        → Pure utility functions
├── types/        → Global types
└── constants/    → Application constants
```

## Fundamental Rules

### ✅ CAN
- Import from `@/theme`
- Import external libraries
- Be imported by any feature

### ❌ CANNOT
- Import from `@/features/*` (NEVER)
- Contain business logic
- Have knowledge of specific domains
- Make API calls (except base apiClient)

## "Shared" Test
Before adding something here, ask:
> "Does this code make sense in ANY React project?"

- If YES → can go in shared
- If NO → probably belongs to a feature

## Subfolder Conventions

### components/
- One directory per component
- Include index.ts for re-export
- Tests alongside component

### hooks/
- Generic hooks (useDebounce, useLocalStorage)
- NOT domain hooks (useAlerts ❌)

### lib/
- External library wrappers
- Configurations (apiClient, queryClient)

### utils/
- Pure functions without side effects
- Easily unit testable

### types/
- Types used across multiple features
- Generic API types
