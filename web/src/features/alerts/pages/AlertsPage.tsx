import { useState, useMemo, useCallback } from "react";
import {
  Box,
  Typography,
  Button,
  Chip,
  IconButton,
  Alert,
  alpha,
  useTheme,
} from "@mui/material";
import {
  MoreVert as MoreVertIcon,
  Visibility as VisibilityIcon,
} from "@mui/icons-material";
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
  type MRT_Cell,
  type MRT_PaginationState,
} from "material-react-table";
import { useTenant, useAuth } from "@/shared/context";
import type { ConfigurationAlert } from "../types";
import { Toast } from "@/shared/components/Toast";
import { ConfirmDialog } from "@/shared/components/ConfirmDialog";
import { LoadingSkeleton, FeatureErrorBoundary } from "@/shared/components";
import {
  AlertsHeader,
  AlertStatsCards,
  AlertsFilters,
  AlertActionsMenu,
  AlertDetailModal,
} from "../components";
import {
  useAlerts,
  useAlertStats,
  useAmfaCheckerStatus,
  useReloadAmfaAlerts,
  useUpdateAlertStatus,
  useResolveAlert,
} from "@/shared/hooks";
import { MRT_OPTIONS_WITH_TOOLBAR, PERMISSIONS } from "@/shared/constants";

type SeverityColor = "error" | "warning" | "info" | "success";

type AlertWithFormattedDate = ConfigurationAlert & {
  formattedFirstDetected: string;
};

function getSeverityColor(severity: string): SeverityColor {
  switch (severity) {
    case "critical":
      return "error";
    case "error":
      return "warning";
    case "warning":
      return "warning";
    case "info":
      return "info";
    default:
      return "info";
  }
}

export function AlertsPage() {
  const theme = useTheme();
  const { selectedTenant } = useTenant();
  const { hasPermission } = useAuth();

  const canAcknowledge = hasPermission(PERMISSIONS.ALERTS.ACKNOWLEDGE);
  const canResolve = hasPermission(PERMISSIONS.ALERTS.RESOLVE);

  const [statusFilter, setStatusFilter] = useState<string>("active");

  // Additional filters
  const [severityFilter, setSeverityFilter] = useState<string>("all");
  const [realmFilter, setRealmFilter] = useState<string>("all");
  const [resourceFilter, setResourceFilter] = useState<string>("all");

  // Server-side pagination
  const [pagination, setPagination] = useState<MRT_PaginationState>({
    pageIndex: 0,
    pageSize: 25,
  });
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [selectedAlertId, setSelectedAlertId] = useState<string | null>(null);
  const [detailModalAlert, setDetailModalAlert] =
    useState<ConfigurationAlert | null>(null);

  // Toast and dialog state
  const [toast, setToast] = useState<{
    message: string;
    type: "success" | "error" | "info";
  } | null>(null);
  const [confirmDialog, setConfirmDialog] = useState<{
    open: boolean;
    title: string;
    message: string;
    confirmText: string;
    confirmColor: "error" | "success" | "warning" | "secondary";
    onConfirm: () => void;
  }>({
    open: false,
    title: "",
    message: "",
    confirmText: "Confirm",
    confirmColor: "error",
    onConfirm: () => {},
  });

  // React Query hooks
  const {
    data: alertsData,
    isLoading,
    error,
    refetch: refetchAlerts,
  } = useAlerts({
    tenantId: selectedTenant?.tenant_id,
    status: statusFilter,
    severity: severityFilter === "all" ? undefined : severityFilter,
    realm: realmFilter === "all" ? undefined : realmFilter,
    resourceType: resourceFilter === "all" ? undefined : resourceFilter,
    limit: pagination.pageSize,
    offset: pagination.pageIndex * pagination.pageSize,
  });

  const totalCount = alertsData?.total ?? 0;

  const { data: stats } = useAlertStats(selectedTenant?.tenant_id);

  const { data: amfaCheckerStatus } = useAmfaCheckerStatus(
    selectedTenant?.tenant_id,
  );
  const { mutate: reloadAmfa, isPending: reloadingAmfa } =
    useReloadAmfaAlerts();

  const handleReloadAmfa = useCallback(() => {
    if (!selectedTenant) return;
    reloadAmfa(selectedTenant.tenant_id, {
      onSuccess: () => {
        setToast({ message: "AMFA alerts reloaded", type: "success" });
      },
      onError: (e) => {
        setToast({
          message:
            e instanceof Error && e.message.includes("amfa_unavailable")
              ? "AMFA is currently unavailable - try again"
              : "Couldn't reload AMFA alerts, try again",
          type: "error",
        });
      },
    });
  }, [selectedTenant, reloadAmfa]);

  const { mutate: updateAlertStatus, isPending: isUpdating } =
    useUpdateAlertStatus();
  const { mutate: resolveAlert, isPending: isResolving } = useResolveAlert();
  const isActionPending = isUpdating || isResolving;

  const alerts = useMemo<AlertWithFormattedDate[]>(() => {
    const alertsList = alertsData?.alerts || [];
    // Pre-format dates to avoid repeated toLocaleDateString() calls during rendering
    return alertsList.map((alert) => ({
      ...alert,
      formattedFirstDetected: new Date(
        alert.first_detected,
      ).toLocaleDateString(),
    }));
  }, [alertsData?.alerts]);

  const handleOpenMenu = useCallback(
    (event: React.MouseEvent<HTMLElement>, alertId: string) => {
      event.stopPropagation();
      setAnchorEl(event.currentTarget);
      setSelectedAlertId(alertId);
    },
    [],
  );

  const handleCloseMenu = () => {
    setAnchorEl(null);
    setSelectedAlertId(null);
  };

  const handleAcknowledgeAlert = (alertId: string) => {
    if (!selectedTenant) return;

    updateAlertStatus(
      {
        tenantId: selectedTenant.tenant_id,
        alertId,
        status: "acknowledged",
      },
      {
        onSuccess: () => {
          setToast({
            message: "Alert acknowledged successfully",
            type: "success",
          });
        },
        onError: () => {
          setToast({ message: "Failed to acknowledge alert", type: "error" });
        },
      },
    );
    handleCloseMenu();
  };

  const handleIgnoreAlert = (alertId: string) => {
    if (!selectedTenant) return;

    setConfirmDialog({
      open: true,
      title: "Ignore Alert",
      message:
        "Are you sure you want to ignore this alert? Future occurrences of this specific issue will not trigger new alerts.",
      confirmText: "Ignore",
      confirmColor: "secondary",
      onConfirm: () => {
        setConfirmDialog((prev) => ({ ...prev, open: false }));
        updateAlertStatus(
          {
            tenantId: selectedTenant.tenant_id,
            alertId,
            status: "ignored",
          },
          {
            onSuccess: () => {
              setToast({
                message: "Alert ignored successfully",
                type: "success",
              });
            },
            onError: () => {
              setToast({ message: "Failed to ignore alert", type: "error" });
            },
          },
        );
      },
    });
    handleCloseMenu();
  };

  const handleResolveAlert = (alertId: string) => {
    if (!selectedTenant) return;

    setConfirmDialog({
      open: true,
      title: "Resolve Alert",
      message:
        "Mark this alert as resolved? The configuration should already be fixed. The alert will be removed on the next check cycle if the issue no longer exists.",
      confirmText: "Resolve",
      confirmColor: "success",
      onConfirm: () => {
        setConfirmDialog((prev) => ({ ...prev, open: false }));
        resolveAlert(
          {
            tenantId: selectedTenant.tenant_id,
            alertId,
          },
          {
            onSuccess: () => {
              setToast({
                message: "Alert marked as resolved",
                type: "success",
              });
            },
            onError: () => {
              setToast({ message: "Failed to resolve alert", type: "error" });
            },
          },
        );
      },
    });
    handleCloseMenu();
  };

  // Get unique realms and resource types for filter dropdowns.
  // Filtering is server-side, so options are derived from the current page.
  // The currently-selected value is always kept present so an active filter
  // never disappears when its rows are not on the page being viewed.
  const uniqueRealms = useMemo(() => {
    const realms = new Set(alerts.map((alert) => alert.realm_name).filter(Boolean));
    if (realmFilter !== "all") realms.add(realmFilter);
    return Array.from(realms).sort((a, b) => a.localeCompare(b));
  }, [alerts, realmFilter]);

  const uniqueResourceTypes = useMemo(() => {
    const types = new Set(
      alerts.map((alert) => alert.resource_type).filter(Boolean),
    );
    if (resourceFilter !== "all") types.add(resourceFilter);
    return Array.from(types).sort((a, b) => a.localeCompare(b));
  }, [alerts, resourceFilter]);

  // Reset to first page whenever a filter changes so server pagination stays valid.
  const handleStatusFilterChange = useCallback((newStatus: string) => {
    setStatusFilter(newStatus);
    setSeverityFilter("all");
    setRealmFilter("all");
    setResourceFilter("all");
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const handleSeverityChange = useCallback((value: string) => {
    setSeverityFilter(value);
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const handleRealmChange = useCallback((value: string) => {
    setRealmFilter(value);
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const handleResourceChange = useCallback((value: string) => {
    setResourceFilter(value);
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const handleClearFilters = useCallback(() => {
    setSeverityFilter("all");
    setRealmFilter("all");
    setResourceFilter("all");
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  // Define columns
  const columns = useMemo<MRT_ColumnDef<AlertWithFormattedDate>[]>(
    () => [
      {
        accessorKey: "severity",
        header: "Severity",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<AlertWithFormattedDate, unknown>;
        }) => (
          <Chip
            label={(cell.getValue() as string).toUpperCase()}
            color={getSeverityColor(cell.getValue() as string)}
            size="small"
          />
        ),
      },
      {
        accessorKey: "source",
        header: "Source",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<AlertWithFormattedDate, unknown>;
        }) => (
          <Chip
            label={cell.getValue() as string}
            size="small"
            sx={{
              bgcolor: alpha(theme.palette.text.secondary, 0.2),
              color: theme.palette.text.secondary,
            }}
          />
        ),
      },
      {
        accessorKey: "title",
        header: "Title",
        grow: true,
        Cell: ({ row }: { row: MRT_Row<AlertWithFormattedDate> }) => (
          <Box>
            <Typography variant="body2" sx={{ fontWeight: 500 }}>
              {row.original.title}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {row.original.check_type}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: "realm_name",
        header: "Realm",
        grow: true,
      },
      {
        accessorKey: "resource_name",
        header: "Resource",
        grow: true,
        Cell: ({ row }: { row: MRT_Row<AlertWithFormattedDate> }) => (
          <Box>
            <Typography variant="body2">
              {row.original.resource_name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {row.original.resource_type}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: "formattedFirstDetected",
        header: "Detected",
        grow: true,
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<AlertWithFormattedDate, unknown>;
        }) => (
          <Typography variant="body2" color="text.secondary">
            {cell.getValue() as string}
          </Typography>
        ),
      },
      {
        id: "actions",
        header: "Actions",
        size: 100,
        Cell: ({ row }: { row: MRT_Row<AlertWithFormattedDate> }) => (
          <Box sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
            <IconButton
              size="small"
              onClick={() => setDetailModalAlert(row.original)}
              sx={{ color: "primary.main" }}
            >
              <VisibilityIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              onClick={(e) => handleOpenMenu(e, row.original.alert_id)}
            >
              <MoreVertIcon fontSize="small" />
            </IconButton>
          </Box>
        ),
      },
    ],
    [theme, handleOpenMenu],
  );

  if (isLoading) {
    return <LoadingSkeleton variant="table" rows={10} />;
  }

  if (error) {
    return (
      <Box sx={{ p: { xs: 2, sm: 4 } }}>
        <Alert
          severity="error"
          action={
            <Button
              color="inherit"
              size="small"
              onClick={() => refetchAlerts()}
            >
              Retry
            </Button>
          }
        >
          {error instanceof Error ? error.message : "Failed to load alerts"}
        </Alert>
      </Box>
    );
  }

  const selectedAlert = alerts.find((a) => a.alert_id === selectedAlertId);

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <AlertsHeader />

      {/* Statistics Cards */}
      <FeatureErrorBoundary
        featureId="AlertStatsCards"
        title="Alert Statistics"
        resetKeys={[selectedTenant?.tenant_id]}
      >
        <AlertStatsCards stats={stats ?? null} />
      </FeatureErrorBoundary>

      {/* Filters */}
      <AlertsFilters
        statusFilter={statusFilter}
        severityFilter={severityFilter}
        realmFilter={realmFilter}
        resourceFilter={resourceFilter}
        uniqueRealms={uniqueRealms}
        uniqueResourceTypes={uniqueResourceTypes}
        filteredCount={alerts.length}
        totalCount={totalCount}
        onStatusChange={handleStatusFilterChange}
        onSeverityChange={handleSeverityChange}
        onRealmChange={handleRealmChange}
        onResourceChange={handleResourceChange}
        onClearFilters={handleClearFilters}
        onRefresh={() => refetchAlerts()}
        showReloadAmfa={!!amfaCheckerStatus?.enabled}
        onReloadAmfa={handleReloadAmfa}
        reloadingAmfa={reloadingAmfa}
      />

      {/* Alerts Table */}
      <FeatureErrorBoundary
        featureId="AlertsTable"
        title="Alerts Table"
        resetKeys={[selectedTenant?.tenant_id, statusFilter]}
      >
        <MaterialReactTable
          {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
          columns={columns}
          data={alerts}
          enablePagination
          enableSorting
          enableColumnFilters
          enableGlobalFilter
          manualPagination
          rowCount={totalCount}
          state={{ pagination, isLoading }}
          onPaginationChange={setPagination}
          initialState={{
            density: "compact",
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
                Configuration Alerts
              </Typography>
            </Box>
          )}
        />
      </FeatureErrorBoundary>

      {/* Actions Menu */}
      <AlertActionsMenu
        anchorEl={anchorEl}
        selectedAlert={selectedAlert}
        onClose={handleCloseMenu}
        onAcknowledge={() => handleAcknowledgeAlert(selectedAlertId!)}
        onIgnore={() => handleIgnoreAlert(selectedAlertId!)}
        onResolve={() => handleResolveAlert(selectedAlertId!)}
        canAcknowledge={canAcknowledge}
        canResolve={canResolve}
        isActionPending={isActionPending}
      />

      {/* Toast Notification */}
      {toast && (
        <Toast
          message={toast.message}
          type={toast.type}
          onClose={() => setToast(null)}
        />
      )}

      {/* Confirm Dialog */}
      <ConfirmDialog
        open={confirmDialog.open}
        title={confirmDialog.title}
        message={confirmDialog.message}
        confirmText={confirmDialog.confirmText}
        confirmColor={confirmDialog.confirmColor}
        onConfirm={confirmDialog.onConfirm}
        onCancel={() => setConfirmDialog({ ...confirmDialog, open: false })}
      />

      {/* Alert Detail Modal */}
      <AlertDetailModal
        open={!!detailModalAlert}
        alert={detailModalAlert}
        onClose={() => setDetailModalAlert(null)}
      />
    </Box>
  );
}
