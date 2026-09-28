import { useState } from "react";
import { useParams } from "react-router-dom";
import { Box, Typography, Button } from "@mui/material";
import { useTenant } from "@/shared/context";
import { Toast } from "@/shared/components/Toast";
import { ConfirmDialog } from "@/shared/components/ConfirmDialog";
import {
  LoadingSkeleton,
  AlertBanner,
  SectionCard,
  FieldDisplay,
} from "@/shared/components";
import { AlertDetailHeader, AlertBadges, AlertActions } from "../components";
import { formatDateTime } from "@/shared/utils";
import {
  useAlert,
  useUpdateAlertStatus,
  useResolveAlert,
} from "@/shared/hooks";

export function AlertDetailPage() {
  const { alertId, tenantId } = useParams<{
    alertId: string;
    tenantId: string;
  }>();
  const { selectedTenant } = useTenant();

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
    data: alert,
    isLoading,
    error,
  } = useAlert(selectedTenant?.tenant_id, alertId);

  const { mutate: updateAlertStatus } = useUpdateAlertStatus();
  const { mutate: resolveAlert } = useResolveAlert();

  const handleAcknowledgeAlert = () => {
    if (!alert || !selectedTenant) return;

    updateAlertStatus(
      {
        tenantId: selectedTenant.tenant_id,
        alertId: alert.alert_id,
        status: "acknowledged",
        acknowledgedBy: "admin",
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
  };

  const handleIgnoreAlert = () => {
    if (!alert || !selectedTenant) return;

    setConfirmDialog({
      open: true,
      title: "Ignore Alert",
      message:
        "Are you sure you want to ignore this alert? Future occurrences of this specific issue will not trigger new alerts.",
      confirmText: "Ignore",
      confirmColor: "secondary",
      onConfirm: () => {
        setConfirmDialog({ ...confirmDialog, open: false });
        updateAlertStatus(
          {
            tenantId: selectedTenant.tenant_id,
            alertId: alert.alert_id,
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
  };

  const handleResolveAlert = () => {
    if (!alert || !selectedTenant) return;

    setConfirmDialog({
      open: true,
      title: "Resolve Alert",
      message:
        "Mark this alert as resolved? The configuration should already be fixed. The alert will be removed on the next check cycle if the issue no longer exists.",
      confirmText: "Resolve",
      confirmColor: "success",
      onConfirm: () => {
        setConfirmDialog({ ...confirmDialog, open: false });
        resolveAlert(
          {
            tenantId: selectedTenant.tenant_id,
            alertId: alert.alert_id,
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
  };

  if (isLoading) {
    return <LoadingSkeleton variant="text" lines={10} />;
  }

  if (error || !alert) {
    return (
      <Box sx={{ p: { xs: 2, sm: 4 } }}>
        <AlertBanner
          severity="error"
          message={error instanceof Error ? error.message : "Alert not found"}
        />
        <Button
          variant="outlined"
          onClick={() => window.history.back()}
          sx={{ mt: 2 }}
        >
          Go Back
        </Button>
      </Box>
    );
  }

  // Parse metadata safely
  let metadata: Record<string, string> = {};
  if (alert.metadata) {
    try {
      metadata = JSON.parse(alert.metadata);
    } catch {
      // Ignore parsing errors
    }
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <AlertDetailHeader
        title={alert.title}
        checkType={alert.check_type}
        tenantId={tenantId || ""}
      />

      {/* Badges */}
      <Box sx={{ mt: 4 }}>
        <AlertBadges
          severity={alert.severity}
          status={alert.status}
          type={alert.type}
        />
      </Box>

      {/* Main Content */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            lg: "repeat(2, 1fr)",
          },
          gap: 3,
          mt: 4,
        }}
      >
        {/* Description */}
        <SectionCard title="Description">
          <Typography variant="body1">{alert.description}</Typography>
        </SectionCard>

        {/* Recommendation */}
        <SectionCard title="Recommendation">
          <Typography variant="body1">{alert.recommendation}</Typography>
        </SectionCard>

        {/* Resource Information */}
        <SectionCard title="Resource Information">
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: "1fr",
              gap: 2,
            }}
          >
            <FieldDisplay label="Source" value={alert.source} size="small" />
            <FieldDisplay label="Realm" value={alert.realm_name} size="small" />
            <FieldDisplay
              label="Resource Type"
              value={alert.resource_type}
              size="small"
            />
            <FieldDisplay
              label="Resource Name"
              value={alert.resource_name}
              size="small"
            />
            <FieldDisplay
              label="Resource ID"
              value={
                <Typography
                  variant="body2"
                  sx={{
                    fontFamily: "monospace",
                    fontSize: "0.75rem",
                    wordBreak: "break-all",
                  }}
                >
                  {alert.resource_id}
                </Typography>
              }
              size="small"
            />
          </Box>
        </SectionCard>

        {/* Timeline */}
        <SectionCard title="Timeline">
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: "1fr",
              gap: 2,
            }}
          >
            <FieldDisplay
              label="First Detected"
              value={formatDateTime(new Date(alert.first_detected))}
              size="small"
            />
            <FieldDisplay
              label="Last Seen"
              value={formatDateTime(new Date(alert.last_seen))}
              size="small"
            />
            {alert.acknowledged_at && (
              <Box>
                <FieldDisplay
                  label="Acknowledged At"
                  value={formatDateTime(new Date(alert.acknowledged_at))}
                  size="small"
                />
                {alert.acknowledged_by && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ mt: 0.5, display: "block" }}
                  >
                    by {alert.acknowledged_by}
                  </Typography>
                )}
              </Box>
            )}
            {alert.resolved_at && (
              <FieldDisplay
                label="Resolved At"
                value={formatDateTime(new Date(alert.resolved_at))}
                size="small"
              />
            )}
          </Box>
        </SectionCard>
      </Box>

      {/* Additional Details - Metadata */}
      {Object.keys(metadata).length > 0 && (
        <Box sx={{ mt: 3 }}>
          <SectionCard title="Additional Details">
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: {
                  xs: "1fr",
                  sm: "repeat(2, 1fr)",
                  lg: "repeat(3, 1fr)",
                },
                gap: 2,
              }}
            >
              {Object.entries(metadata).map(([key, value]) => (
                <FieldDisplay
                  key={key}
                  label={key.replace(/_/g, " ")}
                  value={
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: "monospace",
                        wordBreak: "break-all",
                      }}
                    >
                      {String(value)}
                    </Typography>
                  }
                  size="small"
                />
              ))}
            </Box>
          </SectionCard>
        </Box>
      )}

      {/* Actions */}
      <Box sx={{ mt: 3 }}>
        <AlertActions
          status={alert.status}
          onAcknowledge={handleAcknowledgeAlert}
          onIgnore={handleIgnoreAlert}
          onResolve={handleResolveAlert}
        />
      </Box>

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
    </Box>
  );
}
