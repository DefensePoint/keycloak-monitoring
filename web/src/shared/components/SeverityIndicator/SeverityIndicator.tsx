import { Box, SxProps, Theme } from "@mui/material";
import { Circle } from "@mui/icons-material";

export interface SeverityIndicatorProps {
  /**
   * Severity level that determines the color
   */
  severity: "critical" | "error" | "warning" | "info" | "success";
  /**
   * Size of the indicator in pixels
   * @default 8
   */
  size?: number;
  /**
   * Display style
   * @default "dot"
   */
  variant?: "dot" | "icon";
  /**
   * Show label text next to indicator
   */
  label?: string;
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
}

/**
 * SeverityIndicator - A colored circle/dot to indicate severity levels
 *
 * Uses the design system severity colors:
 * - critical: red (critical issues)
 * - error: orange/red (errors)
 * - warning: yellow (warnings)
 * - info: blue (informational)
 * - success: green (success/healthy)
 *
 * @example
 * ```tsx
 * <SeverityIndicator severity="error" />
 * <SeverityIndicator severity="warning" size={12} />
 * <SeverityIndicator severity="success" label="Healthy" variant="icon" />
 * ```
 */
export function SeverityIndicator({
  severity,
  size = 8,
  variant = "dot",
  label,
  sx,
}: SeverityIndicatorProps) {
  // Map severity to theme colors
  const getColor = (): string => {
    switch (severity) {
      case "critical":
        return "error.dark"; // Darker red for critical
      case "error":
        return "error.main";
      case "warning":
        return "warning.main";
      case "info":
        return "info.main";
      case "success":
        return "success.main";
      default:
        return "text.secondary";
    }
  };

  const color = getColor();

  if (variant === "icon") {
    return (
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: label ? 1 : 0,
          ...sx,
        }}
      >
        <Circle sx={{ fontSize: size, color }} />
        {label && (
          <Box component="span" sx={{ fontSize: "0.875rem", color }}>
            {label}
          </Box>
        )}
      </Box>
    );
  }

  // Dot variant - pure CSS circle
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        gap: label ? 1 : 0,
        ...sx,
      }}
    >
      <Box
        sx={{
          width: size,
          height: size,
          borderRadius: "50%",
          backgroundColor: color,
          flexShrink: 0,
        }}
      />
      {label && (
        <Box component="span" sx={{ fontSize: "0.875rem", color }}>
          {label}
        </Box>
      )}
    </Box>
  );
}
