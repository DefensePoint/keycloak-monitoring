# Shared Hooks - Utility Hooks

## Purpose
Generic React hooks that can be used across any feature.

## Rules

### ✅ CAN
- Handle browser APIs
- Manage UI state patterns
- Abstract common behaviors
- Be used by any feature

### ❌ CANNOT
- Contain business logic
- Know about specific domains
- Make API calls
- Import from features

## Available Hooks

### useLocalStorage
```tsx
const [value, setValue] = useLocalStorage<T>(key: string, initialValue: T)
```
Persists state in localStorage with type safety.

### useDebounce
```tsx
const debouncedValue = useDebounce<T>(value: T, delay: number)
```
Debounces a value by specified delay in ms.

### useMediaQuery
```tsx
const isMobile = useMediaQuery('(max-width: 768px)')
```
Subscribes to a CSS media query.

### useClickOutside
```tsx
useClickOutside(ref: RefObject, handler: () => void)
```
Triggers handler when clicking outside referenced element.

### useAsync
```tsx
const { data, error, isLoading, execute } = useAsync<T>(asyncFn)
```
Manages async operation state.

## Hook Conventions

### Naming
- Always prefix with `use`
- Descriptive name: `useDebounce`, `useLocalStorage`

### Parameters
- Use options object for 3+ parameters
- Provide sensible defaults

### Return Values
- Prefer tuple for simple hooks: `[value, setValue]`
- Prefer object for complex hooks: `{ data, error, isLoading }`

### TypeScript
- Always generic when handling dynamic data
- Export hook type if consumers need it

## Example Implementation
```tsx
// useDebounce.ts
import { useState, useEffect } from 'react'

export function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value)

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedValue(value), delay)
    return () => clearTimeout(timer)
  }, [value, delay])

  return debouncedValue
}
```

## Testing
- Test hook behavior, not implementation
- Use `@testing-library/react-hooks` or `renderHook`
- Cover edge cases (empty values, rapid updates)
