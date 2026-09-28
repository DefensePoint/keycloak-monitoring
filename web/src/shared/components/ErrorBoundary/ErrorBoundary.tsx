import { Component, ErrorInfo, ReactNode } from "react";
import { Box, Typography, Button } from "@mui/material";
import type {
  ErrorBoundaryProps,
  ErrorBoundaryState,
  FallbackProps,
} from "./types";

/**
 * Default fallback component when no custom fallback is provided
 */
function DefaultFallback({ resetError }: FallbackProps) {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: 200,
        p: 4,
        textAlign: "center",
      }}
    >
      <Typography variant="h6" sx={{ mb: 2 }}>
        Something went wrong
      </Typography>
      <Button variant="contained" onClick={resetError}>
        Try Again
      </Button>
    </Box>
  );
}

/**
 * ErrorBoundary - Base class component for catching React errors
 *
 * React requires Error Boundaries to be class components.
 * This provides the core functionality with customizable fallback UI.
 *
 * @example
 * ```tsx
 * <ErrorBoundary
 *   boundaryId="Dashboard"
 *   fallback={(props) => <CustomFallback {...props} />}
 *   onError={(error) => console.error(error)}
 * >
 *   {children}
 * </ErrorBoundary>
 * ```
 */
export class ErrorBoundary extends Component<
  ErrorBoundaryProps,
  ErrorBoundaryState
> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    this.setState({ errorInfo });

    // Call onError callback for logging/reporting
    this.props.onError?.(error, errorInfo);

    // Log to console in development
    if (import.meta.env.DEV) {
      console.error(
        `[ErrorBoundary${this.props.boundaryId ? `:${this.props.boundaryId}` : ""}]`,
        error,
      );
      console.error("Component Stack:", errorInfo.componentStack);
    }
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps): void {
    // Reset when resetKeys change
    if (this.state.hasError && this.props.resetKeys) {
      const hasKeyChanged = this.props.resetKeys.some(
        (key, index) => key !== prevProps.resetKeys?.[index],
      );
      if (hasKeyChanged) {
        this.resetError();
      }
    }
  }

  resetError = (): void => {
    this.props.onReset?.();
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
  };

  render(): ReactNode {
    const { hasError, error, errorInfo } = this.state;
    const { children, fallback, boundaryId } = this.props;

    if (hasError && error) {
      const fallbackProps: FallbackProps = {
        error,
        errorInfo,
        resetError: this.resetError,
        boundaryId,
      };

      // Handle render prop pattern
      if (typeof fallback === "function") {
        return fallback(fallbackProps);
      }

      // Handle ReactNode fallback
      if (fallback) {
        return fallback;
      }

      // Default fallback
      return <DefaultFallback {...fallbackProps} />;
    }

    return children;
  }
}
