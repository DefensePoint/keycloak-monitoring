import { Box, SxProps, Theme } from "@mui/material";
import { ReactNode } from "react";

export interface ResponsiveGridProps {
  /**
   * Number of columns at different breakpoints
   * @example { xs: 1, sm: 2, md: 3, lg: 4 }
   */
  columns?: {
    xs?: number;
    sm?: number;
    md?: number;
    lg?: number;
    xl?: number;
  };
  /**
   * Gap between grid items (theme spacing units)
   * @default 2
   */
  spacing?: number;
  /**
   * Grid items
   */
  children: ReactNode;
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
}

/**
 * ResponsiveGrid - Responsive grid layout with breakpoint-based columns
 *
 * Simplifies creating responsive grids with different column counts
 * at different screen sizes.
 *
 * @example
 * ```tsx
 * // 1 column on mobile, 2 on tablet, 3 on desktop
 * <ResponsiveGrid columns={{ xs: 1, md: 2, lg: 3 }} spacing={2}>
 *   <MetricCard ... />
 *   <MetricCard ... />
 *   <MetricCard ... />
 * </ResponsiveGrid>
 *
 * // Equal columns at all sizes
 * <ResponsiveGrid columns={{ xs: 2 }} spacing={3}>
 *   {items}
 * </ResponsiveGrid>
 * ```
 */
export function ResponsiveGrid({
  columns = { xs: 1, sm: 2, md: 3 },
  spacing = 2,
  children,
  sx,
}: ResponsiveGridProps) {
  return (
    <Box
      sx={{
        display: "grid",
        gap: spacing,
        gridTemplateColumns: {
          xs: `repeat(${columns.xs || 1}, 1fr)`,
          sm: columns.sm ? `repeat(${columns.sm}, 1fr)` : undefined,
          md: columns.md ? `repeat(${columns.md}, 1fr)` : undefined,
          lg: columns.lg ? `repeat(${columns.lg}, 1fr)` : undefined,
          xl: columns.xl ? `repeat(${columns.xl}, 1fr)` : undefined,
        },
        ...sx,
      }}
    >
      {children}
    </Box>
  );
}
