# Metrics Feature

This feature contains functionalities related to operator performance metrics.

## Structure

```
features/metrics/
├── components/          # Metrics-specific components
│   ├── OperatorMetricsHeader.tsx
│   ├── OperatorDateRangeFilter.tsx
│   ├── OperatorSummaryCards.tsx
│   └── index.ts
├── pages/              # Metrics pages
│   └── OperatorMetricsPage.tsx
├── types/              # TypeScript types
│   ├── metrics.types.ts
│   └── index.ts
├── services/           # Services/API calls (future)
├── claude.md          # This documentation
└── index.ts           # Public exports
```

## Responsibilities

### OperatorMetricsPage
- **Responsibility**: Display operator performance metrics
- **Includes**:
  - Date range filter
  - Summary cards (total operators, alerts, hours, avg response time)
  - Individual operator performance table
  - Action timing breakdown table
  - Help information about metrics

### Components

#### OperatorMetricsHeader
- **Responsibility**: Display page title and description

#### OperatorDateRangeFilter
- **Responsibility**: Manage date filter (start/end) and refresh button

#### OperatorSummaryCards
- **Responsibility**: Display summary cards with aggregated metrics

## Types

### OperatorMetricsSummary
Interface defining the data structure for individual operator metrics.

## Guidelines

1. **Componentization**: Each visual section should be a separate component
2. **Types**: Always use types from `types/` folder, never inline
3. **Tables**: Use Material React Table (MRT) for complex tables
4. **Colors**: Always use theme colors, never hardcoded
5. **No `any`**: Types must always be specific
