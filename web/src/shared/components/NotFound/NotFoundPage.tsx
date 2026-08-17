import { Box, Typography, Button, SxProps, Theme } from "@mui/material";
import { SearchOff as SearchOffIcon } from "@mui/icons-material";
import { useNavigate } from "react-router-dom";

export interface NotFoundPageProps {
  /** Custom title for the error */
  title?: string;
  /** Custom message explaining what wasn't found */
  message?: string;
  /** Whether to show the "Go Home" button */
  showHomeButton?: boolean;
  /** Additional sx props */
  sx?: SxProps<Theme>;
}

/**
 * NotFoundPage - Display 404 Not Found state
 *
 * Shows a user-friendly not found message when the user
 * navigates to a page that doesn't exist.
 *
 * @example
 * ```tsx
 * <NotFoundPage />
 * <NotFoundPage
 *   title="Page Not Found"
 *   message="The page you're looking for doesn't exist."
 * />
 * ```
 */
export function NotFoundPage({
  title = "Page Not Found",
  message = "The page you're looking for doesn't exist or has been moved.",
  showHomeButton = true,
  sx,
}: NotFoundPageProps) {
  const navigate = useNavigate();

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
      {/* Not Found Icon */}
      <Box
        sx={{
          width: 80,
          height: 80,
          borderRadius: "50%",
          bgcolor: "rgba(255, 152, 0, 0.1)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          mb: 3,
        }}
      >
        <SearchOffIcon sx={{ fontSize: 40, color: "warning.main" }} />
      </Box>

      {/* 404 Code */}
      <Typography
        variant="h2"
        sx={{
          fontWeight: 700,
          color: "warning.main",
          mb: 1,
          letterSpacing: "-0.02em",
        }}
      >
        404
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
      <Typography
        variant="body1"
        color="text.secondary"
        sx={{ maxWidth: 400, mb: showHomeButton ? 3 : 0 }}
      >
        {message}
      </Typography>

      {/* Home Button */}
      {showHomeButton && (
        <Button
          variant="contained"
          color="primary"
          onClick={() => navigate("/")}
        >
          Return Home
        </Button>
      )}
    </Box>
  );
}
