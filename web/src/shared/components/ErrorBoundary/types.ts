import { ReactNode, ErrorInfo } from "react";

export interface ErrorBoundaryProps {
  /** Content to render when no error */
  children: ReactNode;

  /** Custom fallback UI - receives error info and reset function */
  fallback?: ReactNode | ((props: FallbackProps) => ReactNode);

  /** Callback when error is caught - for logging/reporting */
  onError?: (error: Error, errorInfo: ErrorInfo) => void;

  /** Callback when error boundary resets */
  onReset?: () => void;

  /** Keys that trigger reset when changed (like route changes) */
  resetKeys?: unknown[];

  /** Boundary identifier for logging */
  boundaryId?: string;
}

export interface FallbackProps {
  /** The caught error */
  error: Error;

  /** React error info with component stack */
  errorInfo: ErrorInfo | null;

  /** Function to reset the error boundary */
  resetError: () => void;

  /** Boundary identifier */
  boundaryId?: string;
}

export interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}
