# Feature: Alerts

## Domain
Alert management and alert rules for the monitoring system.

## Responsibilities
- Alert listing with filters
- Individual alert details
- Status updates (resolve, acknowledge)
- Alert rules CRUD
- Severity-based categorization

## Structure
```
alerts/
├── components/
│   ├── AlertsHeader.tsx        → Page header
│   ├── AlertStatsCards.tsx     → Statistics display
│   ├── AlertsFilters.tsx       → List filters
│   ├── AlertActionsMenu.tsx    → Actions dropdown menu
│   ├── AlertDetailHeader.tsx   → Detail page header
│   ├── AlertDetailModal.tsx    → Alert detail modal
│   ├── AlertBadges.tsx         → Status/severity badges
│   └── AlertActions.tsx        → Action buttons
├── hooks/
│   ├── useAlerts.ts            → Alert list
│   ├── useAlert.ts             → Single alert
│   └── useAlertRules.ts        → Alert rules
├── services/
│   └── alertService.ts         → Alert API calls
├── types/
│   └── alert.types.ts          → Alert, AlertRule, Severity
├── pages/
│   ├── AlertsPage.tsx          → Alert list
│   └── AlertDetailPage.tsx     → Alert details
└── index.ts
```

## Public API (index.ts)
```tsx
export { AlertsPage, AlertDetailPage } from './pages'
export { AlertCard, AlertSeverityBadge } from './components'
export { useAlerts, useAlertRules } from './hooks'
export type { Alert, AlertRule, Severity, AlertStatus } from './types'
```

## Key Types
```tsx
type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'
type AlertStatus = 'open' | 'acknowledged' | 'resolved'

interface Alert {
  id: string
  title: string
  description: string
  severity: Severity
  status: AlertStatus
  tenantId: string
  realmName?: string
  source: string
  createdAt: string
  resolvedAt?: string
}

interface AlertRule {
  id: string
  name: string
  condition: string
  severity: Severity
  enabled: boolean
  tenantId: string
}
```

## Severity Colors
Use theme tokens:
- critical: `defense.severity.critical` (#7f1d1d)
- high: `defense.severity.high` (#c2410c)
- medium: `defense.severity.medium` (#854d0e)
- low: `defense.severity.low` (#1e40af)
- info: `defense.severity.info` (#0e7490)

## Permissions
- `alerts:read` - View alerts
- `alerts:write` - Update status
- `alert-rules:read` - View rules
- `alert-rules:write` - Create/edit rules

## Filtering
Supports filtering by:
- Status (open, acknowledged, resolved)
- Severity (critical, high, medium, low, info)
- Realm
- Time range
