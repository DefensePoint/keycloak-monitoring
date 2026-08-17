import { Box, Typography, Button } from "@mui/material";
import {
  ErrorOutline as ErrorIcon,
  Refresh as RefreshIcon,
  ArrowBack as BackIcon,
} from "@mui/icons-material";
import { useNavigate } from "react-router-dom";
import { AlertBanner } from "@/shared/components/AlertBanner";
import type { FallbackProps } from "../types";
import { getErrorMessage } from "@/shared/utils/errors";
import { BOUNDARY_ERRORS } from "@/shared/constants/errors";

/**
 * RouteErrorFallback - Page-level fallback for route errors
 *
 * Displayed when a page fails to render.
 * Allows users to retry, go back, or navigate home.
 */
export function RouteErrorFallback({
  error,
  resetError,
  boundaryId,
}: FallbackProps) {
  const navigate = useNavigate();

  const handleGoBack = () => {
    navigate(-1);
  };

  const handleGoHome = () => {
    navigate("/tenants");
  };

  // Derive page name from boundaryId if available
  const pageName = boundaryId
    ? boundaryId.replace(/([A-Z])/g, " $1").trim()
    : "This page";

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header matching page style */}
      <Box sx={{ mb: 4 }}>
        <Typography variant="h3" sx={{ mb: 1 }}>
          Error Loading Page
        </Typography>
        <Typography variant="body1" color="text.secondary">
          {pageName} encountered an unexpected error
        </Typography>
      </Box>

      {/* Error Alert */}
      <AlertBanner
        severity="error"
        title="Page Error"
        message={getErrorMessage(error, BOUNDARY_ERRORS.ROUTE_GENERIC)}
        sx={{ mb: 3 }}
      />

      {/* Error Details Card */}
      <Box
        sx={{
          bgcolor: "background.paper",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
          p: 4,
          textAlign: "center",
        }}
      >
        {/* Icon */}
        <Box
          sx={{
            width: 64,
            height: 64,
            borderRadius: "50%",
            bgcolor: "rgba(219, 40, 51, 0.1)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            mx: "auto",
            mb: 3,
          }}
        >
          <ErrorIcon sx={{ fontSize: 32, color: "error.main" }} />
        </Box>

        <Typography
          variant="body1"
          color="text.secondary"
          sx={{ mb: 3, maxWidth: 400, mx: "auto" }}
        >
          {BOUNDARY_ERRORS.ROUTE_DESCRIPTION}
        </Typography>

        {/* Actions */}
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            gap: 2,
            justifyContent: "center",
          }}
        >
          <Button
            variant="contained"
            startIcon={<RefreshIcon />}
            onClick={resetError}
            sx={{ textTransform: "uppercase" }}
          >
            Try Again
          </Button>
          <Button
            variant="outlined"
            startIcon={<BackIcon />}
            onClick={handleGoBack}
            sx={{ textTransform: "uppercase" }}
          >
            Go Back
          </Button>
          <Button
            variant="text"
            onClick={handleGoHome}
            sx={{ textTransform: "uppercase" }}
          >
            Home
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
