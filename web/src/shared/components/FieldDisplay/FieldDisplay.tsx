import { Box, Typography, SxProps, Theme } from "@mui/material";
import { ReactNode } from "react";

export interface FieldDisplayProps {
  /**
   * Field label
   */
  label: string;
  /**
   * Field value (can be string, number, or ReactNode for custom content)
   */
  value: string | number | ReactNode;
  /**
   * Value text size
   * @default "medium"
   */
  size?: "small" | "medium" | "large";
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
}

/**
 * FieldDisplay - Display a label and value in a consistent format
 *
 * Shows a small uppercase label with a value below it.
 * Commonly used to display structured information (metrics, details, etc.)
 *
 * @example
 * ```tsx
 * <FieldDisplay label="Total Users" value={1234} />
 * <FieldDisplay label="Memory Used" value="1.5 GB" size="large" />
 * <FieldDisplay label="Status" value={<Chip label="Active" />} />
 * ```
 */
export function FieldDisplay({
  label,
  value,
  size = "medium",
  sx,
}: FieldDisplayProps) {
  // Size mapping for value typography
  const valueVariant = {
    small: "body2",
    medium: "h6",
    large: "h4",
  }[size] as "body2" | "h6" | "h4";

  return (
    <Box sx={sx}>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{
          display: "block",
          fontSize: "0.75rem",
          fontWeight: 600,
          letterSpacing: "0.1em",
          mb: 0.5,
        }}
      >
        {label}
      </Typography>
      {typeof value === "string" || typeof value === "number" ? (
        <Typography variant={valueVariant} sx={{ fontWeight: 600 }}>
          {typeof value === "number" ? value.toLocaleString() : value}
        </Typography>
      ) : (
        value
      )}
    </Box>
  );
}
