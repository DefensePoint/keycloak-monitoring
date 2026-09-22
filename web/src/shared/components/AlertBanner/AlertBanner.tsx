import { Alert, AlertTitle, IconButton, SxProps, Theme } from "@mui/material";
import { Close } from "@mui/icons-material";
import { ReactNode } from "react";

export interface AlertBannerProps {
  /**
   * Severity level
   */
  severity: "error" | "warning" | "info" | "success";
  /**
   * Alert title (optional)
   */
  title?: string;
  /**
   * Alert message
   */
  message: string | ReactNode;
  /**
   * Show close button
   * @default false
   */
  closable?: boolean;
  /**
   * Callback when close button is clicked
   */
  onClose?: () => void;
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
}

/**
 * AlertBanner - Display alerts, warnings, errors, and success messages
 *
 * Based on MUI Alert with consistent styling.
 * Used for system messages, validation errors, success confirmations, etc.
 *
 * @example
 * ```tsx
 * <AlertBanner severity="error" message="Something went wrong" />
 *
 * <AlertBanner
 *   severity="warning"
 *   title="Warning"
 *   message="This action cannot be undone"
 * />
 *
 * <AlertBanner
 *   severity="success"
 *   message="Operation completed successfully"
 *   closable
 *   onClose={() => setShowAlert(false)}
 * />
 * ```
 */
export function AlertBanner({
  severity,
  title,
  message,
  closable = false,
  onClose,
  sx,
}: AlertBannerProps) {
  return (
    <Alert
      severity={severity}
      action={
        closable && onClose ? (
          <IconButton
            aria-label="close"
            color="inherit"
            size="small"
            onClick={onClose}
          >
            <Close fontSize="inherit" />
          </IconButton>
        ) : undefined
      }
      sx={{
        alignItems: "center",
        ...sx,
      }}
    >
      {title && (
        <AlertTitle sx={{ fontWeight: 600, mb: 0.5 }}>{title}</AlertTitle>
      )}
      {typeof message === "string" ? message : message}
    </Alert>
  );
}
