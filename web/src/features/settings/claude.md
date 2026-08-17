# Feature: Settings

## Domain
Application and user preferences configuration.

## Responsibilities
- User preferences (theme, notifications)
- Display settings
- Session preferences

## Structure
```
settings/
├── components/
│   └── SettingsForm.tsx        → Settings form
├── hooks/
│   └── useSettings.ts          → Settings state
├── types/
│   └── settings.types.ts       → Settings shape
├── pages/
│   └── SettingsPage.tsx        → Settings page
└── index.ts
```

## Public API (index.ts)
```tsx
export { SettingsPage } from './pages'
export { useSettings } from './hooks'
export type { UserSettings } from './types'
```

## Key Types
```tsx
interface UserSettings {
  theme: 'light' | 'dark' | 'system'
  notifications: {
    email: boolean
    browser: boolean
  }
  display: {
    compactMode: boolean
    timezone: string
    dateFormat: string
  }
}
```

## Storage
- Settings stored in localStorage
- Synced across tabs
- Default values on first load

## Settings Sections
1. **Appearance**
   - Theme selection
   - Compact mode toggle

2. **Notifications**
   - Email notifications
   - Browser notifications

3. **Display**
   - Timezone
   - Date/time format

## Implementation Notes
- Use `useLocalStorage` hook from shared
- React to system theme changes
- Validate settings on load
