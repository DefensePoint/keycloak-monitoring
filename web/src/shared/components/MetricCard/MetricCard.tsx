import {
  Card,
  CardActionArea,
  CardContent,
  Box,
  Typography,
  SxProps,
  Theme,
  alpha,
} from "@mui/material";
import React, { ReactNode } from "react";

export interface MetricCardProps {
  /**
   * Metric label/title
   */
  label: string;
  /**
   * Metric value (number or string)
   */
  value: string | number;
  /**
   * Optional icon to display
   */
  icon?: ReactNode;
  /**
   * Optional subtitle/description
   */
  subtitle?: string;
  /**
   * Make the card clickable
   */
  onClick?: () => void;
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
  /**
   * Icon color - when provided, adds colored background to icon
   */
  iconColor?: string;
  /**
   * Optional footer content (e.g., status indicator)
   */
  footer?: ReactNode;
  /**
   * Typography size for value: sm=h5, md=h4, lg=h3 (default: lg)
   */
  size?: "sm" | "md" | "lg";
}

/**
 * MetricCard - Simple, composable card for displaying metrics
 *
 * Generic component that can be composed by features to create
 * specific metric displays (Dashboard metrics, Health metrics, etc.)
 *
 * @example
 * ```tsx
 * // Simple usage
 * <MetricCard label="Total Users" value={1234} />
 *
 * // With icon and subtitle
 * <MetricCard
 *   label="Active Sessions"
 *   value={567}
 *   icon={<PeopleIcon />}
 *   subtitle="Last 24 hours"
 * />
 *
 * // Clickable
 * <MetricCard
 *   label="Events"
 *   value={1234}
 *   onClick={() => navigate('/events')}
 * />
 * ```
 */
export function MetricCard({
  label,
  value,
  icon,
  subtitle,
  onClick,
  sx,
  iconColor,
  footer,
  size = "lg",
}: MetricCardProps) {
  const valueVariant = {
    sm: "h5",
    md: "h4",
    lg: "h3",
  }[size] as "h5" | "h4" | "h3";

  const content = (
    <CardContent sx={{ p: 3, "&:last-child": { pb: 3 } }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 2,
        }}
      >
        {/* Icon (if provided) */}
        {icon && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              ...(iconColor && {
                p: 1.5,
                borderRadius: 1,
                bgcolor: alpha(iconColor, 0.1),
              }),
            }}
          >
            {iconColor && React.isValidElement(icon)
              ? React.cloneElement(icon as React.ReactElement, {
                  sx: { fontSize: 28, color: iconColor },
                })
              : icon}
          </Box>
        )}

        {/* Label and Value */}
        <Box sx={{ flex: 1, textAlign: icon ? "right" : "left" }}>
          <Typography
            variant="overline"
            color="text.secondary"
            sx={{
              display: "block",
              fontSize: "0.75rem",
              fontWeight: 600,
              letterSpacing: "0.1em",
              mb: 1,
            }}
          >
            {label}
          </Typography>
          <Typography
            variant={valueVariant}
            sx={{
              fontWeight: 700,
              lineHeight: 1.2,
            }}
          >
            {typeof value === "number" ? value.toLocaleString() : value}
          </Typography>

          {/* Subtitle (if provided) */}
          {subtitle && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ mt: 1, display: "block" }}
            >
              {subtitle}
            </Typography>
          )}
        </Box>
      </Box>

      {/* Footer (if provided) */}
      {footer && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 2 }}>
          {footer}
        </Box>
      )}
    </CardContent>
  );

  return (
    <Card
      sx={{
        height: "100%",
        transition: "all 0.2s",
        ...(onClick && {
          cursor: "pointer",
          "&:hover": {
            borderColor: "primary.main",
            transform: "translateY(-2px)",
            boxShadow: 2,
          },
        }),
        ...sx,
      }}
    >
      {onClick ? (
        <CardActionArea onClick={onClick} sx={{ height: "100%" }}>
          {content}
        </CardActionArea>
      ) : (
        content
      )}
    </Card>
  );
}
