import React from "react";
import { useTheme } from "@mui/material";
import {
  People,
  Notifications,
  AccessTime,
  FlashOn,
} from "@mui/icons-material";
import { StatsCardGrid, type StatsCardItem } from "@/shared/components";

interface OperatorSummaryCardsProps {
  totalOperators: number;
  totalAlertsHandled: number;
  totalHours: number;
  avgResponseTime: string;
}

export const OperatorSummaryCards: React.FC<OperatorSummaryCardsProps> = ({
  totalOperators,
  totalAlertsHandled,
  totalHours,
  avgResponseTime,
}) => {
  const theme = useTheme();

  const items: StatsCardItem[] = [
    {
      icon: People,
      label: "Total Operators",
      value: totalOperators,
      color: theme.palette.info.main,
    },
    {
      icon: Notifications,
      label: "Total Alerts",
      value: totalAlertsHandled,
      color: theme.palette.warning.main,
    },
    {
      icon: AccessTime,
      label: "Total Hours",
      value: totalHours.toFixed(1),
      color: theme.palette.success.main,
    },
    {
      icon: FlashOn,
      label: "Avg Response",
      value: avgResponseTime,
      color: theme.palette.error.main,
    },
  ];

  return (
    <StatsCardGrid items={items} columns={{ xs: 1, sm: 2, lg: 4 }} size="md" />
  );
};
