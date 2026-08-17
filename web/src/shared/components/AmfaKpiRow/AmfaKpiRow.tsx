import { Box, useTheme } from "@mui/material";
import { Login, Warning, People, ReportProblem } from "@mui/icons-material";
import {
  StatsCardGrid,
  type StatsCardItem,
} from "@/shared/components/StatsCardGrid";
import type { AmfaStats } from "@/shared/types";

interface Props {
  stats: AmfaStats | undefined;
}

function fmt(n: number | undefined | null): string {
  if (n === undefined || n === null) return "—";
  return n.toLocaleString();
}

/** Four-card AMFA KPI strip (Total logins, Risky, Unique users, Flagged IPs). */
export function AmfaKpiRow({ stats }: Props) {
  const theme = useTheme();

  const items: StatsCardItem[] = [
    {
      icon: Login,
      label: "Total Login Events",
      value: fmt(stats?.total),
      color: theme.palette.info.main,
    },
    {
      icon: Warning,
      label: "Risky (Risk 3+)",
      value: fmt(stats?.risky),
      color: theme.palette.warning.main,
    },
    {
      icon: People,
      label: "Unique Users",
      value: fmt(stats?.unique_users),
      color: theme.palette.success.main,
    },
    {
      icon: ReportProblem,
      label: "Flagged IPs",
      value: fmt(stats?.flagged_ips),
      color: theme.palette.error.main,
    },
  ];

  return (
    <Box sx={{ mb: 3 }}>
      <StatsCardGrid items={items} columns={{ xs: 1, sm: 2, lg: 4 }} size="md" />
    </Box>
  );
}
