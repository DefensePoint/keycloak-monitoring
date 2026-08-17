import { Chip, ChipProps } from "@mui/material";

export interface StatusBadgeProps {
  /**
   * Status type that determines the color
   */
  status: "success" | "error" | "warning" | "info" | "default";
  /**
   * Label text to display
   */
  label: string;
  /**
   * Size of the badge
   * @default "small"
   */
  size?: "small" | "medium";
  /**
   * Variant of the badge
   * @default "filled"
   */
  variant?: "filled" | "outlined";
  /**
   * Additional MUI Chip props
   */
  chipProps?: Omit<ChipProps, "label" | "color" | "size" | "variant">;
}

/**
 * StatusBadge - A reusable badge component for displaying status with consistent styling
 *
 * @example
 * ```tsx
 * <StatusBadge status="success" label="Active" />
 * <StatusBadge status="error" label="Failed" />
 * <StatusBadge status="warning" label="Pending" variant="outlined" />
 * ```
 */
export function StatusBadge({
  status,
  label,
  size = "small",
  variant = "filled",
  chipProps,
}: StatusBadgeProps) {
  // Map status to MUI color
  const colorMap: Record<StatusBadgeProps["status"], ChipProps["color"]> = {
    success: "success",
    error: "error",
    warning: "warning",
    info: "info",
    default: "default",
  };

  return (
    <Chip
      label={label}
      color={colorMap[status]}
      size={size}
      variant={variant}
      sx={{
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        ...chipProps?.sx,
      }}
      {...chipProps}
    />
  );
}
