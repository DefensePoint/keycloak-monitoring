import React, { useMemo, useCallback } from "react";
import { Box, Chip, IconButton, alpha, useTheme } from "@mui/material";
import { ChevronRight } from "@mui/icons-material";
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
} from "material-react-table";
import { LoadingSkeleton, EmptyState } from "@/shared/components";
import { MRT_OPTIONS_WITH_TOOLBAR } from "@/shared/constants";
import type { UserEventsTableProps, UserEvent } from "../types";

export const UserEventsTable: React.FC<UserEventsTableProps> = ({
  events,
  loading,
  onEventClick,
}) => {
  const theme = useTheme();

  const getSeverityColor = useCallback(
    (severity: string): { bgcolor: string; color: string } => {
      switch (severity.toLowerCase()) {
        case "error":
          return {
            bgcolor: alpha(theme.palette.error.main, 0.2),
            color: theme.palette.error.main,
          };
        case "warning":
          return {
            bgcolor: alpha(theme.palette.warning.main, 0.2),
            color: theme.palette.warning.main,
          };
        case "info":
        default:
          return {
            bgcolor: alpha(theme.palette.info.main, 0.2),
            color: theme.palette.info.main,
          };
      }
    },
    [theme.palette],
  );

  const columns = useMemo<MRT_ColumnDef<UserEvent>[]>(
    () => [
      {
        accessorKey: "timestamp",
        header: "Timestamp",
        size: 180,
        Cell: ({ cell }: { cell: MRT_Cell<UserEvent, unknown> }) => (
          <Box sx={{ fontFamily: "monospace", fontSize: "0.875rem" }}>
            {new Date(cell.getValue() as string).toLocaleString()}
          </Box>
        ),
      },
      {
        accessorKey: "type",
        header: "Type",
        size: 150,
        Cell: ({ cell }: { cell: MRT_Cell<UserEvent, unknown> }) => (
          <Box sx={{ fontWeight: 500 }}>{cell.getValue() as string}</Box>
        ),
      },
      {
        accessorKey: "source",
        header: "Source",
        size: 150,
      },
      {
        accessorKey: "description",
        header: "Description",
        size: 300,
        Cell: ({ cell }: { cell: MRT_Cell<UserEvent, unknown> }) => (
          <Box
            sx={{
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              maxWidth: 300,
            }}
          >
            {cell.getValue() as string}
          </Box>
        ),
      },
      {
        accessorKey: "severity",
        header: "Severity",
        size: 100,
        Cell: ({ cell }: { cell: MRT_Cell<UserEvent, unknown> }) => {
          const severity = cell.getValue() as string;
          const { bgcolor, color } = getSeverityColor(severity);
          return (
            <Chip
              label={severity}
              sx={{
                bgcolor,
                color,
                fontSize: "0.75rem",
              }}
            />
          );
        },
      },
      {
        id: "actions",
        header: "",
        size: 60,
        enableSorting: false,
        enableColumnFilter: false,
        Cell: ({ row }: { row: MRT_Row<UserEvent> }) => (
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation();
              onEventClick(row.original);
            }}
            sx={{
              color: "text.secondary",
              "&:hover": {
                color: "text.primary",
              },
            }}
          >
            <ChevronRight />
          </IconButton>
        ),
      },
    ],
    [onEventClick, getSeverityColor],
  );

  if (loading) {
    return <LoadingSkeleton variant="table" rows={5} />;
  }

  if (events.length === 0) {
    return <EmptyState message="No events found for this user" />;
  }

  return (
    <Box sx={{ width: "100%" }}>
      <MaterialReactTable
        {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
        columns={columns}
        data={events}
        enablePagination
        enableSorting
        enableColumnFilters
        initialState={{
          density: "compact",
          pagination: { pageSize: 10, pageIndex: 0 },
        }}
        muiTableBodyRowProps={({ row }: { row: MRT_Row<UserEvent> }) => ({
          onClick: () => onEventClick(row.original),
          sx: { cursor: "pointer" },
        })}
        renderTopToolbarCustomActions={() => (
          <Box sx={{ p: 2 }}>
            <Box
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              User Events
            </Box>
            <Box sx={{ fontSize: "0.75rem", color: "text.secondary" }}>
              Showing {events.length} events
            </Box>
          </Box>
        )}
      />
    </Box>
  );
};
