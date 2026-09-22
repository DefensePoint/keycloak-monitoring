import React from "react";
import { useTheme } from "@mui/material";
import {
  Notifications,
  Error as ErrorIcon,
  Warning,
  Info,
  ReportProblem,
} from "@mui/icons-material";
import { StatsCardGrid, type StatsCardItem } from "@/shared/components";
import type { AlertStats } from "../types";

interface AlertStatsCardsProps {
  stats: AlertStats | null;
}

export const AlertStatsCards: React.FC<AlertStatsCardsProps> = ({ stats }) => {
  const theme = useTheme();

  if (!stats) return null;

  const items: StatsCardItem[] = [
    {
      icon: Notifications,
      label: "Total Active",
      value: stats.total_active || 0,
      color: theme.palette.primary.main,
    },
    {
      icon: ReportProblem,
      label: "Critical",
      value: stats.by_severity?.critical || 0,
      color: theme.palette.error.main,
    },
    {
      icon: ErrorIcon,
      label: "Error",
      value: stats.by_severity?.error || 0,
      color: theme.palette.warning.main,
    },
    {
      icon: Warning,
      label: "Warning",
      value: stats.by_severity?.warning || 0,
      color: theme.palette.warning.light,
    },
    {
      icon: Info,
      label: "Info",
      value: stats.by_severity?.info || 0,
      color: theme.palette.info.main,
    },
  ];

  return (
    <StatsCardGrid
      items={items}
      columns={{ xs: 1, sm: 2, lg: 5 }}
      size="md"
      sx={{ mb: 3 }}
    />
  );
};
