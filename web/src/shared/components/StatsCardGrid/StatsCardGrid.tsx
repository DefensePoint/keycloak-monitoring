import { Box, SxProps, Theme } from "@mui/material";
import { ReactNode } from "react";
import { MetricCard } from "../MetricCard/MetricCard";

export interface StatsCardItem {
  /**
   * MUI Icon component to display
   */
  icon: React.ElementType;
  /**
   * Card label/title
   */
  label: string;
  /**
   * Card value (number or string)
   */
  value: string | number;
  /**
   * Icon and background color
   */
  color: string;
  /**
   * Optional footer content
   */
  footer?: ReactNode;
  /**
   * Optional click handler
   */
  onClick?: () => void;
}

export interface StatsCardGridProps {
  /**
   * Array of card items to display
   */
  items: StatsCardItem[];
  /**
   * Responsive column configuration
   */
  columns?: {
    xs?: number;
    sm?: number;
    md?: number;
    lg?: number;
  };
  /**
   * Typography size for values: sm=h5, md=h4, lg=h3
   */
  size?: "sm" | "md" | "lg";
  /**
   * Grid gap spacing (MUI spacing units)
   */
  spacing?: number;
  /**
   * Additional sx props for the container
   */
  sx?: SxProps<Theme>;
}

/**
 * StatsCardGrid - Responsive grid of metric cards with icons
 *
 * Provides a consistent layout for displaying stats cards across
 * Dashboard, Alerts, and Metrics pages.
 *
 * @example
 * ```tsx
 * const items = [
 *   { icon: People, label: "Total Users", value: 1234, color: theme.palette.info.main },
 *   { icon: Notifications, label: "Alerts", value: 56, color: theme.palette.warning.main },
 * ];
 *
 * <StatsCardGrid items={items} columns={{ xs: 1, sm: 2, lg: 4 }} size="md" />
 * ```
 */
export function StatsCardGrid({
  items,
  columns = { xs: 1, sm: 2, lg: 4 },
  size = "md",
  spacing = 2,
  sx,
}: StatsCardGridProps) {
  const getGridTemplateColumns = () => {
    const cols: Record<string, string> = {};
    if (columns.xs) cols.xs = `repeat(${columns.xs}, 1fr)`;
    if (columns.sm) cols.sm = `repeat(${columns.sm}, 1fr)`;
    if (columns.md) cols.md = `repeat(${columns.md}, 1fr)`;
    if (columns.lg) cols.lg = `repeat(${columns.lg}, 1fr)`;
    return cols;
  };

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: getGridTemplateColumns(),
        gap: spacing,
        ...sx,
      }}
    >
      {items.map((item, index) => {
        const IconComponent = item.icon;
        return (
          <MetricCard
            key={index}
            label={item.label}
            value={item.value}
            icon={<IconComponent />}
            iconColor={item.color}
            footer={item.footer}
            onClick={item.onClick}
            size={size}
          />
        );
      })}
    </Box>
  );
}
