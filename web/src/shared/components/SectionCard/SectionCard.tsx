import {
  Card,
  CardContent,
  Box,
  Typography,
  SxProps,
  Theme,
} from "@mui/material";
import { ReactNode } from "react";

export interface SectionCardProps {
  /**
   * Section title
   */
  title: string;
  /**
   * Optional subtitle
   */
  subtitle?: string;
  /**
   * Content to display in the card
   */
  children: ReactNode;
  /**
   * Optional action button/element in header
   */
  action?: ReactNode;
  /**
   * Additional sx props
   */
  sx?: SxProps<Theme>;
}

/**
 * SectionCard - Card with a header (title) and content area
 *
 * Generic component for displaying sections of content with a title.
 * Commonly used in pages to group related information.
 *
 * @example
 * ```tsx
 * // Simple usage
 * <SectionCard title="System Health">
 *   <Typography>Content goes here</Typography>
 * </SectionCard>
 *
 * // With subtitle and action
 * <SectionCard
 *   title="Recent Activity"
 *   subtitle="Last 24 hours"
 *   action={<Button size="small">View All</Button>}
 * >
 *   <List>...</List>
 * </SectionCard>
 * ```
 */
export function SectionCard({
  title,
  subtitle,
  children,
  action,
  sx,
}: SectionCardProps) {
  return (
    <Card sx={{ height: "100%", ...sx }}>
      {/* Header */}
      <Box
        sx={{
          px: 3,
          py: 2,
          borderBottom: 1,
          borderColor: "divider",
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 2,
        }}
      >
        <Box sx={{ flex: 1 }}>
          <Typography
            variant="h6"
            sx={{
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            {title}
          </Typography>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              {subtitle}
            </Typography>
          )}
        </Box>
        {action && <Box>{action}</Box>}
      </Box>

      {/* Content */}
      <CardContent sx={{ p: 3 }}>{children}</CardContent>
    </Card>
  );
}
