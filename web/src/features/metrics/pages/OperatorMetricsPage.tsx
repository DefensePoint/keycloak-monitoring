import { useState, useMemo } from "react";
import { Box, Typography, Alert } from "@mui/material";
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
} from "material-react-table";
import { useTenant } from "@/shared/context";
import { LoadingSkeleton, AlertBanner } from "@/shared/components";
import {
  OperatorMetricsHeader,
  OperatorDateRangeFilter,
  OperatorSummaryCards,
} from "../components";
import { useOperatorMetrics } from "../hooks";
import { MRT_OPTIONS_WITH_TOOLBAR } from "@/shared/constants";
import type { OperatorMetricsSummary } from "../types";

export function OperatorMetricsPage() {
  const { selectedTenant } = useTenant();

  // Date range state (default to last 30 days)
  const [startDate, setStartDate] = useState<string>(() => {
    const date = new Date();
    date.setDate(date.getDate() - 30);
    return date.toISOString().split("T")[0];
  });
  const [endDate, setEndDate] = useState<string>(() => {
    return new Date().toISOString().split("T")[0];
  });

  const {
    data: metrics = [],
    isLoading,
    error,
    refetch,
  } = useOperatorMetrics({
    tenantId: selectedTenant?.tenant_id,
    startDate,
    endDate,
  });

  const formatTime = (seconds: number): string => {
    if (
      !seconds ||
      seconds === 0 ||
      !Number.isFinite(seconds) ||
      Number.isNaN(seconds)
    )
      return "-";
    const minutes = seconds / 60;
    if (minutes < 60) {
      return `${minutes.toFixed(1)}m`;
    }
    const hours = minutes / 60;
    return `${hours.toFixed(1)}h`;
  };

  const formatHours = (hours: number): string => {
    if (!hours || hours === 0 || !Number.isFinite(hours) || Number.isNaN(hours))
      return "-";
    return `${hours.toFixed(1)}h`;
  };

  // Calculate totals
  const totalOperators = metrics.length;
  const totalAlertsHandled = metrics.reduce(
    (sum, m) => sum + (m.total_alerts_handled || 0),
    0,
  );
  const totalHours = metrics.reduce(
    (sum, m) => sum + (m.total_work_time_hours || 0),
    0,
  );
  const avgResponseTime =
    totalOperators > 0
      ? metrics.reduce((sum, m) => sum + (m.avg_response_time || 0), 0) /
        totalOperators
      : 0;

  // Define columns for performance table
  const performanceColumns = useMemo<MRT_ColumnDef<OperatorMetricsSummary>[]>(
    () => [
      {
        accessorKey: "operator_name",
        header: "Operator",
        grow: true,
        Cell: ({ row }: { row: MRT_Row<OperatorMetricsSummary> }) => (
          <Box>
            <Box sx={{ fontWeight: 500 }}>
              {row.original.operator_name || row.original.operator_email}
            </Box>
            <Typography variant="caption" color="text.secondary">
              {row.original.operator_email}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: "total_alerts_handled",
        header: "Handled",
        grow: true,
      },
      {
        accessorKey: "alerts_acknowledged",
        header: "Ack",
        grow: true,
      },
      {
        accessorKey: "alerts_resolved",
        header: "Resolved",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<OperatorMetricsSummary, unknown>;
        }) => (
          <Box sx={{ color: "success.main", fontWeight: 500 }}>
            {cell.getValue() as number}
          </Box>
        ),
      },
      {
        accessorKey: "alerts_ignored",
        header: "Ignored",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<OperatorMetricsSummary, unknown>;
        }) => (
          <Box sx={{ color: "text.secondary" }}>
            {cell.getValue() as number}
          </Box>
        ),
      },
      {
        accessorKey: "critical_alerts_handled",
        header: "Critical",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<OperatorMetricsSummary, unknown>;
        }) => (
          <Box sx={{ color: "error.main", fontWeight: 500 }}>
            {cell.getValue() as number}
          </Box>
        ),
      },
      {
        accessorKey: "avg_response_time",
        header: "Avg Response",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
      {
        accessorKey: "total_work_time_hours",
        header: "Hours",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatHours(cell.getValue() as number),
      },
    ],
    [],
  );

  // Define columns for timing breakdown table
  const timingColumns = useMemo<MRT_ColumnDef<OperatorMetricsSummary>[]>(
    () => [
      {
        accessorKey: "operator_name",
        header: "Operator",
        grow: true,
        Cell: ({ row }: { row: MRT_Row<OperatorMetricsSummary> }) => (
          <Box sx={{ fontWeight: 500 }}>
            {row.original.operator_name || row.original.operator_email}
          </Box>
        ),
      },
      {
        accessorKey: "avg_time_to_acknowledge",
        header: "Time to Ack",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
      {
        accessorKey: "avg_time_to_resolve",
        header: "Time to Resolve",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
      {
        accessorKey: "avg_time_to_ignore",
        header: "Time to Ignore",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
      {
        accessorKey: "avg_acknowledge_to_resolve_time",
        header: "Ack → Resolve",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
      {
        accessorKey: "avg_acknowledge_to_ignore_time",
        header: "Ack → Ignore",
        grow: true,
        Cell: ({ cell }: { cell: MRT_Cell<OperatorMetricsSummary, unknown> }) =>
          formatTime(cell.getValue() as number),
      },
    ],
    [],
  );

  if (isLoading) {
    return <LoadingSkeleton variant="card" count={6} />;
  }

  if (error) {
    return (
      <Box sx={{ p: { xs: 2, sm: 4 } }}>
        <AlertBanner
          severity="error"
          message={
            error instanceof Error
              ? error.message
              : "Failed to load operator metrics"
          }
        />
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <Box sx={{ mb: 3 }}>
        <OperatorMetricsHeader />
      </Box>

      {/* Info Banner */}
      <Alert
        severity="warning"
        sx={{ mb: 3, "& .MuiAlert-icon": { alignItems: "center" } }}
      >
        <Box component="ul" sx={{ m: 0, pl: 2 }}>
          <li>
            <Typography variant="body2" component="span">
              <strong>"Time to X"</strong> — Average time from alert detection
              to action
            </Typography>
          </li>
          <li>
            <Typography variant="body2" component="span">
              <strong>"Ack → Resolve"</strong> — Time spent on acknowledged
              alerts before resolving
            </Typography>
          </li>
        </Box>
      </Alert>

      {/* Summary Cards */}
      {metrics.length > 0 && (
        <Box sx={{ mb: 3 }}>
          <OperatorSummaryCards
            totalOperators={totalOperators}
            totalAlertsHandled={totalAlertsHandled}
            totalHours={totalHours}
            avgResponseTime={formatTime(avgResponseTime)}
          />
        </Box>
      )}

      {/* Date Range Filter */}
      <Box sx={{ mb: 3 }}>
        <OperatorDateRangeFilter
          startDate={startDate}
          endDate={endDate}
          onStartDateChange={setStartDate}
          onEndDateChange={setEndDate}
          onRefresh={() => refetch()}
        />
      </Box>

      {/* Operator Performance Table */}
      <Box sx={{ mb: 3, width: "100%" }}>
        <MaterialReactTable
          {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
          columns={performanceColumns}
          data={metrics}
          enablePagination
          enableSorting
          enableColumnFilters
          initialState={{
            density: "compact",
            pagination: { pageSize: 10, pageIndex: 0 },
          }}
          renderTopToolbarCustomActions={() => (
            <Box sx={{ px: 1, py: 0.5 }}>
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Operator Performance
              </Typography>
            </Box>
          )}
        />
      </Box>

      {/* Action Timing Breakdown Table */}
      <Box sx={{ mb: 3, width: "100%" }}>
        <MaterialReactTable
          {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
          columns={timingColumns}
          data={metrics}
          enablePagination
          enableSorting
          enableColumnFilters
          initialState={{
            density: "compact",
            pagination: { pageSize: 10, pageIndex: 0 },
          }}
          renderTopToolbarCustomActions={() => (
            <Box sx={{ px: 1, py: 0.5 }}>
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Action Timing Breakdown
              </Typography>
            </Box>
          )}
        />
      </Box>
    </Box>
  );
}
