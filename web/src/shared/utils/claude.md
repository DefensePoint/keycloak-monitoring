# Shared Utils - Utility Functions

## Purpose
Pure utility functions without side effects.

## Rules

### ✅ CAN
- Be pure functions
- Transform data
- Format values
- Validate inputs

### ❌ CANNOT
- Have side effects
- Make API calls
- Access browser APIs (use hooks instead)
- Import from features

## Available Utilities

### date.ts
```tsx
// Format dates
formatDate(date: Date | string, format?: string): string
formatRelativeTime(date: Date | string): string
parseDate(dateString: string): Date

// Date calculations
isToday(date: Date): boolean
daysBetween(start: Date, end: Date): number
```

### format.ts
```tsx
// Number formatting
formatNumber(n: number, options?: FormatOptions): string
formatPercentage(n: number): string
formatBytes(bytes: number): string

// String formatting
capitalize(str: string): string
truncate(str: string, maxLength: number): string
slugify(str: string): string
```

### cn.ts
```tsx
// Classname utility (clsx + tailwind-merge)
cn(...classes: ClassValue[]): string

// Usage:
cn('base-class', conditional && 'optional', { 'object-syntax': true })
```

### validation.ts
```tsx
// Common validators
isEmail(value: string): boolean
isUrl(value: string): boolean
isEmpty(value: unknown): boolean
isValidUUID(value: string): boolean
```

## Conventions

### Function Signatures
- Single responsibility
- Descriptive names
- TypeScript generics when needed
- JSDoc for complex functions

### Example
```tsx
/**
 * Formats a byte value to human readable string
 * @param bytes - Number of bytes
 * @param decimals - Decimal places (default: 2)
 * @returns Formatted string (e.g., "1.5 MB")
 */
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes === 0) return '0 Bytes'

  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))

  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(decimals))} ${sizes[i]}`
}
```

## Testing
- Every util function should have unit tests
- Test edge cases (null, undefined, empty)
- Test boundary conditions
