# Feature: Health

## Domain
System health monitoring and infrastructure metrics.

## Responsibilities
- Overall health status display
- Infinispan cache metrics
- High availability status
- Keycloak operator metrics
- Connection status to monitored instances

## Structure
```
health/
├── components/
│   ├── HealthStatus.tsx        → Overall health indicator
│   ├── InfinispanMetrics.tsx   → Cache metrics display
│   ├── HighAvailability.tsx    → HA status and nodes
│   └── ConnectionStatus.tsx    → Connection to Keycloak
├── hooks/
│   ├── useHealth.ts            → Health data
│   └── useInfinispanMetrics.ts → Cache metrics
├── services/
│   └── healthService.ts        → Health API calls
├── types/
│   └── health.types.ts         → Health, CacheMetrics
├── pages/
│   ├── HealthPage.tsx          → Health overview
│   └── OperatorMetricsPage.tsx → Operator details
└── index.ts
```

## Public API (index.ts)
```tsx
export { HealthPage, OperatorMetricsPage } from './pages'
export { HealthStatus, InfinispanMetrics } from './components'
export { useHealth } from './hooks'
export type { HealthStatus, CacheMetrics } from './types'
```

## Key Types
```tsx
type HealthState = 'healthy' | 'degraded' | 'unhealthy' | 'unknown'

interface TenantHealth {
  status: HealthState
  keycloak: {
    reachable: boolean
    version?: string
    uptime?: number
  }
  database: {
    connected: boolean
    latency?: number
  }
  lastCheck: string
}

interface CacheMetrics {
  name: string
  size: number
  hits: number
  misses: number
  evictions: number
}
```

## Health Indicators
Visual status:
- 🟢 Healthy - All systems operational
- 🟡 Degraded - Some issues detected
- 🔴 Unhealthy - Critical problems
- ⚪ Unknown - Cannot determine

## Auto-refresh
- Health data refreshes every 30 seconds
- Visual indicator when checking
- Last check timestamp displayed

## Infinispan Metrics
Cache performance monitoring:
- Cache hit ratio
- Memory usage
- Eviction rate
- Cluster node status
