import { Box, Typography, SxProps, Theme } from "@mui/material";
import { Block as BlockIcon } from "@mui/icons-material";

export interface AccessDeniedPageProps {
  /** Custom title for the error */
  title?: string;
  /** Custom message explaining why access is denied */
  message?: string;
  /** Additional sx props */
  sx?: SxProps<Theme>;
}

/**
 * AccessDeniedPage - Display 403 Forbidden state
 *
 * Shows a user-friendly access denied message when the user
 * doesn't have permission to access a resource.
 *
 * @example
 * ```tsx
 * <AccessDeniedPage />
 * <AccessDeniedPage
 *   title="Admin Access Required"
 *   message="You need administrator privileges to access this page."
 * />
 * ```
 */
export function AccessDeniedPage({
  title = "Access Denied",
  message = "You don't have permission to access this resource.",
  sx,
}: AccessDeniedPageProps) {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "60vh",
        py: 4,
        px: 2,
        textAlign: "center",
        ...sx,
      }}
    >
      {/* Forbidden Icon */}
      <Box
        sx={{
          width: 80,
          height: 80,
          borderRadius: "50%",
          bgcolor: "rgba(219, 40, 51, 0.1)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          mb: 3,
        }}
      >
        <BlockIcon sx={{ fontSize: 40, color: "error.main" }} />
      </Box>

      {/* 403 Code */}
      <Typography
        variant="h2"
        sx={{
          fontWeight: 700,
          color: "error.main",
          mb: 1,
          letterSpacing: "-0.02em",
        }}
      >
        403
      </Typography>

      {/* Title */}
      <Typography
        variant="h5"
        sx={{
          fontWeight: 600,
          mb: 2,
        }}
      >
        {title}
      </Typography>

      {/* Message */}
      <Typography variant="body1" color="text.secondary" sx={{ maxWidth: 400 }}>
        {message}
      </Typography>
    </Box>
  );
}
