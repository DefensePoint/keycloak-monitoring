# Shared Components - UI Kit

## Purpose
Library of reusable and generic UI components.

## Principles
1. **Domain-agnostic** - No knowledge of business logic
2. **Composable** - Small and combinable
3. **Accessible** - Follow WCAG guidelines
4. **Tested** - Each component with tests

## Component Structure
```
ComponentName/
├── ComponentName.tsx       → Implementation
├── ComponentName.test.tsx  → Tests
├── ComponentName.types.ts  → Types (if complex)
└── index.ts                → Re-export
```

## Conventions

### Props Interface
```tsx
interface ButtonProps {
  variant?: 'primary' | 'secondary' | 'ghost'
  size?: 'sm' | 'md' | 'lg'
  isLoading?: boolean
  children: React.ReactNode
  onClick?: () => void
}

export const Button = ({
  variant = 'primary',
  size = 'md',
  ...props
}: ButtonProps) => { }
```

### Composition with MUI + Tailwind
```tsx
import { Button as MuiButton } from '@mui/material'
import { cn } from '@/shared/utils'

export const Button = ({ className, ...props }: ButtonProps) => (
  <MuiButton
    className={cn('rounded-lg font-medium', className)}
    {...props}
  />
)
```

### Forwarding Refs
Input components should use forwardRef:
```tsx
export const Input = forwardRef<HTMLInputElement, InputProps>(
  (props, ref) => <input ref={ref} {...props} />
)
```

## Available Components

### Layout & Navigation
- `Layout` - Main layout with sidebar
- `PageHeader` - Standardized page header with automatic breadcrumbs
- `DialogHeader` - Header for dialogs with title, subtitle, and close button

### Permission & Auth
- `PermissionGate` - Permission guard component

### Dialogs & Modals
- `ConfirmDialog` - Confirmation modal

### Notifications & Feedback
- `ToastProvider` - Context provider for toast notifications
- `ToastContainer` - Toast notification display container
- `useToast` - Hook to show toast notifications (`showToast({ message, type })`)
- `AlertBanner` - Alert banner for inline messages

### Form Components
- `FormTextField` - Text field with react-hook-form integration
- `FormSelect` - Select with react-hook-form integration
- `FormCheckbox` - Checkbox with react-hook-form integration
- `FormSwitch` - Switch with react-hook-form integration

### Async State Components
- `LoadingSkeleton` - Loading skeleton with variants: `table`, `card`, `list`, `text`, `spinner`
- `ErrorState` - Error state display with retry functionality
- `EmptyState` - Empty state display

### Error Boundaries
- `GlobalErrorBoundary` - Catches errors at app root level
- `RouteErrorBoundary` - Catches errors at route level
- `FeatureErrorBoundary` - Catches errors at feature/widget level (use for isolated widgets)

### Data Display
- `SectionCard` - Card container for content sections
- `StatusBadge` - Status indicator badge
- `SeverityIndicator` - Severity level indicator
- `FieldDisplay` - Field label and value display
- `MetricCard` - Individual metric display card
- `StatsCardGrid` - Responsive grid for metric cards
- `ResponsiveGrid` - Generic responsive grid layout

### Selectors
- `TimeSelector` - Time range picker with session storage persistence
- `TenantSelector` - Tenant selection dropdown
- `RealmSelector` - Realm selection dropdown
- `PageSizeSelector` - Pagination size selector

### Other
- `FilterCard` - Container for filter controls

## Rules

### ✅ CAN
- Use MUI components as base
- Use Tailwind for styling
- Accept className prop for customization
- Have internal state for UI behavior

### ❌ CANNOT
- Fetch data from APIs
- Know about specific domains
- Import from features
- Have business logic
