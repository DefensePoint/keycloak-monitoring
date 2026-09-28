import { Box, Typography, SxProps, Theme } from "@mui/material";
import { InboxOutlined } from "@mui/icons-material";
import { ReactNode } from "react";

export interface EmptyStateProps {
  /**
   * Message to display
   */
  message: string;
  /**
   * Optional subtitle/description
   */
  description?: string;
  /**
   * Icon to display above the message
   * @default <InboxOutlined />
   */
  icon?: ReactNode;
  /**
   * Optional action button or element
   */
  action?: ReactNode;
  /**
   * Minimum height of the container
   * @default 200
   */
  minHeight?: number | string;
  /**
   * Additional sx props for the container
   */
  sx?: SxProps<Theme>;
}

/**
 * EmptyState - Display when there's no data to show
 *
 * Shows a centered message with optional icon and action button.
 * Used for empty lists, no search results, etc.
 *
 * @example
 * ```tsx
 * <EmptyState message="No events found" />
 * <EmptyState
 *   message="No data"
 *   description="Get started by adding your first item"
 *   icon={<DatasetIcon />}
 *   action={<Button>Add Item</Button>}
 * />
 * ```
 */
export function EmptyState({
  message,
  description,
  icon,
  action,
  minHeight = 200,
  sx,
}: EmptyStateProps) {
  // Use default icon if none provided
  const displayIcon = icon !== undefined ? icon : <InboxOutlined />;

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight,
        py: 4,
        textAlign: "center",
        ...sx,
      }}
    >
      {displayIcon && (
        <Box
          sx={{
            fontSize: 48,
            color: "text.secondary",
            mb: 2,
            opacity: 0.5,
          }}
        >
          {displayIcon}
        </Box>
      )}

      <Typography
        variant="body1"
        color="text.secondary"
        sx={{ fontWeight: 500, mb: description ? 1 : 0 }}
      >
        {message}
      </Typography>

      {description && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {description}
        </Typography>
      )}

      {action && <Box sx={{ mt: 2 }}>{action}</Box>}
    </Box>
  );
}
