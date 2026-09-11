import { Box, Typography, Button, SxProps, Theme } from "@mui/material";
import {
  ErrorOutline as ErrorIcon,
  Refresh as RefreshIcon,
} from "@mui/icons-material";
import { getErrorMessage } from "@/shared/utils/errors";
import { API_ERRORS } from "@/shared/constants/errors";

export interface ErrorStateProps {
  /** The error object or message */
  error: Error | string | null;
  /** Custom title for the error */
  title?: string;
  /** Callback when retry button is clicked */
  onRetry?: () => void;
  /** Show error details (for development) */
  showDetails?: boolean;
  /** Minimum height of the container */
  minHeight?: number | string;
  /** Additional sx props */
  sx?: SxProps<Theme>;
}

/**
 * ErrorState - Display error state with retry option
 *
 * Shows a user-friendly error message with an optional retry button.
 * Error messages are sanitized to prevent leaking sensitive information.
 *
 * @example
 * ```tsx
 * <ErrorState
 *   error={error}
 *   title="Failed to load data"
 *   onRetry={() => refetch()}
 * />
 * ```
 */
export function ErrorState({
  error,
  title = "Something went wrong",
  onRetry,
  showDetails = false,
  minHeight = 200,
  sx,
}: ErrorStateProps) {
  // Extract and sanitize error message
  const errorMessage =
    typeof error === "string"
      ? error
      : error
        ? getErrorMessage(error, API_ERRORS.LOAD_DATA)
        : API_ERRORS.LOAD_DATA;

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight,
        py: 4,
        px: 2,
        textAlign: "center",
        ...sx,
      }}
    >
      {/* Error Icon */}
      <Box
        sx={{
          width: 64,
          height: 64,
          borderRadius: "50%",
          bgcolor: "rgba(219, 40, 51, 0.1)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          mb: 2,
        }}
      >
        <ErrorIcon sx={{ fontSize: 32, color: "error.main" }} />
      </Box>

      {/* Title */}
      <Typography
        variant="h6"
        sx={{
          fontWeight: 500,
          mb: 1,
        }}
      >
        {title}
      </Typography>

      {/* Error Message */}
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ mb: 3, maxWidth: 400 }}
      >
        {errorMessage}
      </Typography>

      {/* Error Details (development only) */}
      {showDetails && error instanceof Error && (
        <Box
          sx={{
            bgcolor: "rgba(219, 40, 51, 0.05)",
            border: "1px solid",
            borderColor: "error.main",
            borderRadius: 1,
            p: 2,
            mb: 3,
            maxWidth: 500,
            width: "100%",
            overflow: "auto",
          }}
        >
          <Typography
            variant="caption"
            component="pre"
            sx={{
              fontFamily: "monospace",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
              m: 0,
              textAlign: "left",
            }}
          >
            {error.stack || error.message}
          </Typography>
        </Box>
      )}

      {/* Retry Button */}
      {onRetry && (
        <Button
          variant="contained"
          startIcon={<RefreshIcon />}
          onClick={onRetry}
          sx={{ textTransform: "uppercase" }}
        >
          Try Again
        </Button>
      )}
    </Box>
  );
}
