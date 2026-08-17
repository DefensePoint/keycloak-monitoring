import { Box, Typography, Button } from "@mui/material";
import {
  ErrorOutline as ErrorIcon,
  Refresh as RefreshIcon,
  Home as HomeIcon,
} from "@mui/icons-material";
import type { FallbackProps } from "../types";
import { getErrorMessage } from "@/shared/utils/errors";
import { BOUNDARY_ERRORS } from "@/shared/constants/errors";

/**
 * DefensePoint theme colors (hardcoded for GlobalErrorFallback)
 * This component renders outside ThemeProvider, so we need inline styles
 */
const colors = {
  background: {
    primary: "#0E0E0E",
    elevated: "#212121",
  },
  border: {
    primary: "#2a2a2a",
  },
  text: {
    primary: "#ffffff",
    secondary: "#a0a0a0",
    muted: "#707070",
  },
  red: {
    primary: "#DB2833",
  },
};

/**
 * GlobalErrorFallback - Full-page fallback for catastrophic errors
 *
 * Displayed when the entire application crashes.
 * Provides options to reload or navigate home.
 *
 * NOTE: Uses hardcoded colors because this component renders
 * outside of ThemeProvider when the app crashes at the root level.
 */
export function GlobalErrorFallback({ error, resetError }: FallbackProps) {
  const handleReload = () => {
    window.location.reload();
  };

  const handleNavigateHome = () => {
    window.location.href = "/tenants";
  };

  return (
    <Box
      sx={{
        minHeight: "100vh",
        bgcolor: colors.background.primary,
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        p: 4,
      }}
    >
      <Box
        sx={{
          maxWidth: 500,
          width: "100%",
          bgcolor: colors.background.elevated,
          border: "1px solid",
          borderColor: colors.border.primary,
          borderRadius: 1,
          p: 4,
          textAlign: "center",
        }}
      >
        {/* Error Icon */}
        <Box
          sx={{
            width: 80,
            height: 80,
            borderRadius: "50%",
            bgcolor: "rgba(219, 40, 51, 0.1)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            mx: "auto",
            mb: 3,
          }}
        >
          <ErrorIcon sx={{ fontSize: 40, color: colors.red.primary }} />
        </Box>

        {/* Title */}
        <Typography
          variant="h5"
          sx={{
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            mb: 2,
            color: colors.text.primary,
          }}
        >
          Application Error
        </Typography>

        {/* Description */}
        <Typography
          variant="body1"
          sx={{ mb: 1, color: colors.text.secondary }}
        >
          {BOUNDARY_ERRORS.GLOBAL_DESCRIPTION}
        </Typography>

        {/* Error Message (sanitized) */}
        <Box
          sx={{
            bgcolor: "rgba(219, 40, 51, 0.1)",
            border: "1px solid",
            borderColor: colors.red.primary,
            borderRadius: 1,
            p: 2,
            mb: 3,
            mt: 2,
          }}
        >
          <Typography
            variant="body2"
            sx={{
              fontFamily: "monospace",
              wordBreak: "break-word",
              color: colors.text.primary,
            }}
          >
            {getErrorMessage(error, BOUNDARY_ERRORS.GENERIC_ERROR)}
          </Typography>
        </Box>

        {/* Actions */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <Button
            variant="contained"
            startIcon={<RefreshIcon />}
            onClick={handleReload}
            fullWidth
            sx={{
              textTransform: "uppercase",
              bgcolor: colors.red.primary,
              "&:hover": {
                bgcolor: "#b01f28",
              },
            }}
          >
            Reload Application
          </Button>
          <Button
            variant="outlined"
            startIcon={<HomeIcon />}
            onClick={handleNavigateHome}
            fullWidth
            sx={{
              textTransform: "uppercase",
              borderColor: colors.border.primary,
              color: colors.text.primary,
              "&:hover": {
                borderColor: colors.red.primary,
                bgcolor: "rgba(219, 40, 51, 0.08)",
              },
            }}
          >
            Go to Home
          </Button>
          <Button
            variant="text"
            onClick={resetError}
            fullWidth
            sx={{
              textTransform: "uppercase",
              color: colors.text.muted,
              "&:hover": {
                bgcolor: "rgba(255, 255, 255, 0.05)",
              },
            }}
          >
            Try to Continue
          </Button>
        </Box>
      </Box>

      {/* Footer */}
      <Typography
        variant="caption"
        sx={{ mt: 3, color: colors.text.secondary }}
      >
        If this problem persists, please contact support.
      </Typography>
    </Box>
  );
}
