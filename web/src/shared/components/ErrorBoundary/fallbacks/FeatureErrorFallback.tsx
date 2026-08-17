import { Box, Typography, Button } from "@mui/material";
import {
  WarningAmber as WarningIcon,
  Refresh as RefreshIcon,
} from "@mui/icons-material";
import type { FallbackProps } from "../types";
import { getErrorMessage } from "@/shared/utils/errors";
import { BOUNDARY_ERRORS } from "@/shared/constants/errors";

interface FeatureErrorFallbackProps extends FallbackProps {
  /** Custom title for the error section */
  title?: string;
  /** Minimum height of the fallback container */
  minHeight?: number | string;
  /** Show card styling (border, background) */
  showCard?: boolean;
}

/**
 * FeatureErrorFallback - Component-level fallback for widget errors
 *
 * Displayed when an individual component/widget fails.
 * Designed to be compact and not disrupt the rest of the page.
 */
export function FeatureErrorFallback({
  error,
  resetError,
  boundaryId,
  title,
  minHeight = 200,
  showCard = true,
}: FeatureErrorFallbackProps) {
  const displayTitle =
    title || boundaryId?.replace(/([A-Z])/g, " $1").trim() || "Component";

  const content = (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight,
        py: 3,
        px: 2,
        textAlign: "center",
      }}
    >
      {/* Warning Icon */}
      <Box
        sx={{
          width: 48,
          height: 48,
          borderRadius: "50%",
          bgcolor: "rgba(245, 158, 11, 0.1)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          mb: 2,
        }}
      >
        <WarningIcon sx={{ fontSize: 24, color: "warning.main" }} />
      </Box>

      {/* Title */}
      <Typography variant="body1" sx={{ fontWeight: 500, mb: 1 }}>
        {displayTitle} unavailable
      </Typography>

      {/* Error Message */}
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ mb: 2, maxWidth: 300 }}
      >
        {getErrorMessage(error, BOUNDARY_ERRORS.FEATURE_GENERIC)}
      </Typography>

      {/* Retry Button */}
      <Button
        variant="outlined"
        size="small"
        startIcon={<RefreshIcon />}
        onClick={resetError}
        sx={{ textTransform: "uppercase" }}
      >
        Retry
      </Button>
    </Box>
  );

  if (showCard) {
    return (
      <Box
        sx={{
          bgcolor: "background.paper",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
        }}
      >
        {content}
      </Box>
    );
  }

  return content;
}
