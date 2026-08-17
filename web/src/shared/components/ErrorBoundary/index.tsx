import { ReactNode } from "react";
import { useLocation } from "react-router-dom";
import { ErrorBoundary } from "./ErrorBoundary";
import {
  GlobalErrorFallback,
  RouteErrorFallback,
  FeatureErrorFallback,
} from "./fallbacks";
import type { ErrorBoundaryProps } from "./types";

// Re-exports
export { ErrorBoundary } from "./ErrorBoundary";
export {
  GlobalErrorFallback,
  RouteErrorFallback,
  FeatureErrorFallback,
} from "./fallbacks";
export type { ErrorBoundaryProps, FallbackProps } from "./types";

/**
 * GlobalErrorBoundary - Wraps the entire application
 *
 * Use at the root level in main.tsx to catch any unhandled errors.
 * Displays a full-page fallback with reload options.
 *
 * @example
 * ```tsx
 * <GlobalErrorBoundary>
 *   <App />
 * </GlobalErrorBoundary>
 * ```
 */
interface GlobalErrorBoundaryProps {
  children: ReactNode;
  onError?: ErrorBoundaryProps["onError"];
}

export function GlobalErrorBoundary({
  children,
  onError,
}: GlobalErrorBoundaryProps) {
  return (
    <ErrorBoundary
      boundaryId="Global"
      onError={onError}
      fallback={(props) => <GlobalErrorFallback {...props} />}
    >
      {children}
    </ErrorBoundary>
  );
}

/**
 * RouteErrorBoundary - Wraps individual routes/pages
 *
 * Use in route definitions to isolate page-level errors.
 * Automatically resets when the route changes.
 *
 * @example
 * ```tsx
 * <RouteErrorBoundary routeId="Dashboard">
 *   <DashboardPage />
 * </RouteErrorBoundary>
 * ```
 */
interface RouteErrorBoundaryProps {
  children: ReactNode;
  routeId: string;
  onError?: ErrorBoundaryProps["onError"];
}

export function RouteErrorBoundary({
  children,
  routeId,
  onError,
}: RouteErrorBoundaryProps) {
  const location = useLocation();

  return (
    <ErrorBoundary
      boundaryId={routeId}
      onError={onError}
      resetKeys={[location.pathname]}
      fallback={(props) => <RouteErrorFallback {...props} />}
    >
      {children}
    </ErrorBoundary>
  );
}

/**
 * FeatureErrorBoundary - Wraps individual components/widgets
 *
 * Use around critical or isolated components that might fail
 * without needing to crash the entire page.
 *
 * @example
 * ```tsx
 * <FeatureErrorBoundary featureId="MetricsChart" title="Metrics">
 *   <MetricsChart data={data} />
 * </FeatureErrorBoundary>
 * ```
 */
interface FeatureErrorBoundaryProps {
  children: ReactNode;
  featureId: string;
  title?: string;
  minHeight?: number | string;
  showCard?: boolean;
  onError?: ErrorBoundaryProps["onError"];
  resetKeys?: unknown[];
}

export function FeatureErrorBoundary({
  children,
  featureId,
  title,
  minHeight,
  showCard,
  onError,
  resetKeys,
}: FeatureErrorBoundaryProps) {
  return (
    <ErrorBoundary
      boundaryId={featureId}
      onError={onError}
      resetKeys={resetKeys}
      fallback={(props) => (
        <FeatureErrorFallback
          {...props}
          title={title}
          minHeight={minHeight}
          showCard={showCard}
        />
      )}
    >
      {children}
    </ErrorBoundary>
  );
}
