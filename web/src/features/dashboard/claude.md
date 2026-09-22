# Feature: Dashboard

## Domain
Tenant overview with aggregated metrics and quick insights.

## Responsibilities
- Display key metrics summary
- Recent alerts overview
- Activity charts
- Quick navigation to details
- Monthly report generation

## Structure
```
dashboard/
├── components/
│   ├── DashboardMetrics.tsx    → Key metrics cards
│   ├── RecentAlerts.tsx        → Latest alerts list
│   ├── StatsCard.tsx           → Individual stat card
│   ├── ActivityChart.tsx       → Activity over time
│   └── QuickActions.tsx        → Common action buttons
├── hooks/
│   └── useDashboardData.ts     → Aggregated dashboard data
├── pages/
│   └── DashboardPage.tsx       → Main dashboard
└── index.ts
```

## Public API (index.ts)
```tsx
export { DashboardPage } from './pages'
export { StatsCard } from './components'
export { useDashboardData } from './hooks'
```

## Dashboard Data
Aggregates data from multiple domains:
- Stats from events
- Recent alerts (limited)
- Keycloak health metrics
- Realm summaries

## Key Metrics
- Total events (today/week)
- Active alerts by severity
- Active sessions
- Login success rate
- System health status

## Charts
Use Recharts for visualization:
- Line chart for activity timeline
- Bar chart for events by type
- Pie chart for alert distribution

## Report Generation
- Monthly PDF reports
- Date range selection
- Includes all key metrics

## Performance
- Use React Query for caching
- Stale time: 60 seconds
- Background refetch on focus
