import { Card, CardContent, Box, SxProps, Theme } from "@mui/material";
import { ReactNode } from "react";

export interface FilterCardProps {
  /**
   * Filter controls to render inside the card
   */
  children: ReactNode;
  /**
   * Optional footer content (e.g., result count, clear button)
   */
  footer?: ReactNode;
  /**
   * Additional sx props for the card
   */
  sx?: SxProps<Theme>;
}

/**
 * FilterCard - Standardized container for filter controls
 *
 * Provides consistent styling for filter sections across pages
 * (Alerts, Metrics, Events, etc.)
 *
 * @example
 * ```tsx
 * <FilterCard
 *   footer={<Typography>Showing 10 of 50 items</Typography>}
 * >
 *   <TextField label="Search" />
 *   <Select label="Status" />
 * </FilterCard>
 * ```
 */
export function FilterCard({ children, footer, sx }: FilterCardProps) {
  return (
    <Card sx={sx}>
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        {children}
        {footer && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              flexWrap: "wrap",
              gap: 1,
              mt: 2,
            }}
          >
            {footer}
          </Box>
        )}
      </CardContent>
    </Card>
  );
}
