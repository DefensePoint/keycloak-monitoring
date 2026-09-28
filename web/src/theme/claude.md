# Theme - Design System

## Purpose
Design tokens, MUI and Tailwind configuration.

## Structure
```
theme/
├── index.ts        → Main MUI theme export
├── palette.ts      → Colors and palette
├── typography.ts   → Fonts and typography
└── components.ts   → MUI component overrides
```

## Color Palette (DefensePoint)

### Backgrounds
```
defense.bg.primary:   #0E0E0E  (main background)
defense.bg.secondary: #1a1a1a  (secondary background)
defense.bg.elevated:  #212121  (cards, modals)
```

### Brand
```
defense.red.primary:  #DB2833  (primary red)
defense.red.dark:     #b01f28  (hover)
defense.red.light:    #e63946  (accent)
```

### Severities
```
defense.severity.critical: #7f1d1d
defense.severity.high:     #c2410c
defense.severity.medium:   #854d0e
defense.severity.low:      #1e40af
defense.severity.info:     #0e7490
```

### Status
```
defense.status.success: #10b981
defense.status.warning: #f59e0b
defense.status.error:   #ef4444
```

## Usage

### In MUI components
```tsx
<Button color="primary">  // Uses defense.red.primary
<Alert severity="error">  // Uses defense.status.error
```

### In Tailwind
```tsx
<div className="bg-defense-bg-primary text-defense-red-primary">
<span className="bg-defense-severity-critical">
```

## Typography

### Font Families
- **Sans**: Inter (general UI)
- **Mono**: JetBrains Mono (code, metrics)

### Scale
- `text-xs`: 12px
- `text-sm`: 14px
- `text-base`: 16px
- `text-lg`: 18px
- `text-xl`: 20px

## Rules

### ✅ CAN
- Define design tokens
- Configure MUI theme
- Extend Tailwind

### ❌ CANNOT
- Import from features or shared
- Contain React components
- Have runtime logic
