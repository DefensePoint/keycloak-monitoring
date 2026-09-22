import React, { useState } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Box,
  Typography,
  IconButton,
  Chip,
  Button,
  Divider,
} from "@mui/material";
import { Close as CloseIcon } from "@mui/icons-material";
import type { ConfigurationAlert } from "../types";
import { Toast } from "@/shared/components/Toast";
import { ConfirmDialog } from "@/shared/components/ConfirmDialog";
import { useUpdateAlertStatus, useResolveAlert } from "@/shared/hooks";
import { useTenant, useAuth } from "@/shared/context";

interface AlertDetailModalProps {
  open: boolean;
  alert: ConfigurationAlert | null;
  onClose: () => void;
}

type SeverityColor = "error" | "warning" | "info" | "default";
type StatusColor = "error" | "warning" | "success" | "default";

function getSeverityColor(severity: string): SeverityColor {
  switch (severity) {
    case "critical":
      return "error";
    case "error":
      return "error";
    case "warning":
      return "warning";
    case "info":
      return "info";
    default:
      return "default";
  }
}

function getStatusColor(status: string): StatusColor {
  switch (status) {
    case "active":
      return "error";
    case "acknowledged":
      return "warning";
    case "resolved":
      return "success";
    default:
      return "default";
  }
}

export const AlertDetailModal: React.FC<AlertDetailModalProps> = ({
  open,
  alert,
  onClose,
}) => {
  const { selectedTenant } = useTenant();
  const { user } = useAuth();
  const { mutate: updateAlertStatus } = useUpdateAlertStatus();
  const { mutate: resolveAlert } = useResolveAlert();

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

  if (!alert) return null;

  // Parse metadata safely
  let metadata: Record<string, string> = {};
  if (alert.metadata) {
    try {
      metadata = JSON.parse(alert.metadata);
    } catch {
      // Ignore parsing errors
    }
  }

  const handleAcknowledgeAlert = () => {
    if (!selectedTenant) return;

    updateAlertStatus(
      {
        tenantId: selectedTenant.tenant_id,
        alertId: alert.alert_id,
        status: "acknowledged",
        acknowledgedBy: user?.preferred_username || user?.email || "unknown",
      },
      {
        onSuccess: () => {
          setToast({
            message: "Alert acknowledged successfully",
            type: "success",
          });
          onClose();
        },
        onError: () => {
          setToast({ message: "Failed to acknowledge alert", type: "error" });
        },
      },
    );
  };

  const handleIgnoreAlert = () => {
    if (!selectedTenant) return;

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
              onClose();
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
    if (!selectedTenant) return;

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
              onClose();
            },
            onError: () => {
              setToast({ message: "Failed to resolve alert", type: "error" });
            },
          },
        );
      },
    });
  };

  const labelStyle = {
    textTransform: "uppercase",
    letterSpacing: "0.05em",
    color: "text.secondary",
  } as const;

  return (
    <>
      <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
        <DialogTitle
          sx={{
            bgcolor: "background.paper",
            borderBottom: 1,
            borderColor: "divider",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.02em",
            }}
          >
            Alert Details
          </Typography>
          <IconButton
            onClick={onClose}
            size="small"
            sx={{ color: "text.secondary" }}
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>

        <DialogContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {/* Alert ID */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Alert ID
              </Typography>
              <Typography
                variant="body2"
                sx={{ fontFamily: "monospace", mt: 0.5 }}
              >
                {alert.alert_id}
              </Typography>
            </Box>

            {/* Title */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Title
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5, fontWeight: 500 }}>
                {alert.title}
              </Typography>
            </Box>

            {/* Severity & Status */}
            <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Severity
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={alert.severity.toUpperCase()}
                    color={getSeverityColor(alert.severity)}
                    size="small"
                    sx={{ fontWeight: 700, fontSize: "0.75rem" }}
                  />
                </Box>
              </Box>
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Status
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={alert.status.toUpperCase()}
                    color={getStatusColor(alert.status)}
                    size="small"
                    sx={{ fontWeight: 700, fontSize: "0.75rem" }}
                  />
                </Box>
              </Box>
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Type
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={alert.type.toUpperCase()}
                    size="small"
                    sx={{ fontWeight: 700, fontSize: "0.75rem" }}
                  />
                </Box>
              </Box>
            </Box>

            {/* Check Type */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Check Type
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {alert.check_type}
              </Typography>
            </Box>

            {/* Description */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Description
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {alert.description}
              </Typography>
            </Box>

            {/* Recommendation */}
            {alert.recommendation && (
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Recommendation
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {alert.recommendation}
                </Typography>
              </Box>
            )}

            <Divider sx={{ my: 1 }} />

            {/* Resource Information */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Source
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {alert.source}
              </Typography>
            </Box>

            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Realm
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {alert.realm_name}
              </Typography>
            </Box>

            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Resource
              </Typography>
              <Box sx={{ mt: 0.5 }}>
                <Typography variant="body2">{alert.resource_name}</Typography>
                <Typography variant="caption" color="text.secondary">
                  {alert.resource_type}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    fontFamily: "monospace",
                    display: "block",
                    mt: 0.5,
                  }}
                >
                  {alert.resource_id}
                </Typography>
              </Box>
            </Box>

            <Divider sx={{ my: 1 }} />

            {/* Timeline */}
            <Box>
              <Typography variant="caption" sx={labelStyle}>
                First Detected
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {new Date(alert.first_detected).toLocaleString()}
              </Typography>
            </Box>

            <Box>
              <Typography variant="caption" sx={labelStyle}>
                Last Seen
              </Typography>
              <Typography variant="body2" sx={{ mt: 0.5 }}>
                {new Date(alert.last_seen).toLocaleString()}
              </Typography>
            </Box>

            {alert.acknowledged_at && (
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Acknowledged At
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {new Date(alert.acknowledged_at).toLocaleString()}
                  {alert.acknowledged_by && (
                    <Typography
                      component="span"
                      variant="caption"
                      color="text.secondary"
                      sx={{ ml: 1 }}
                    >
                      by {alert.acknowledged_by}
                    </Typography>
                  )}
                </Typography>
              </Box>
            )}

            {alert.resolved_at && (
              <Box>
                <Typography variant="caption" sx={labelStyle}>
                  Resolved At
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {new Date(alert.resolved_at).toLocaleString()}
                </Typography>
              </Box>
            )}

            {/* Metadata */}
            {Object.keys(metadata).length > 0 && (
              <>
                <Divider sx={{ my: 1 }} />
                {Object.entries(metadata).map(([key, value]) => (
                  <Box key={key}>
                    <Typography variant="caption" sx={labelStyle}>
                      {key.replace(/_/g, " ")}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: "monospace",
                        mt: 0.5,
                        wordBreak: "break-all",
                      }}
                    >
                      {String(value)}
                    </Typography>
                  </Box>
                ))}
              </>
            )}
          </Box>
        </DialogContent>

        {/* Actions */}
        {alert.status === "active" && (
          <DialogActions sx={{ px: 3, pb: 3, gap: 1 }}>
            <Button variant="outlined" onClick={handleIgnoreAlert}>
              Ignore
            </Button>
            <Button
              variant="outlined"
              color="warning"
              onClick={handleAcknowledgeAlert}
            >
              Acknowledge
            </Button>
            <Button
              variant="contained"
              color="success"
              onClick={handleResolveAlert}
            >
              Resolve
            </Button>
          </DialogActions>
        )}

        {alert.status === "acknowledged" && (
          <DialogActions sx={{ px: 3, pb: 3, gap: 1 }}>
            <Button variant="outlined" onClick={handleIgnoreAlert}>
              Ignore
            </Button>
            <Button
              variant="contained"
              color="success"
              onClick={handleResolveAlert}
            >
              Resolve
            </Button>
          </DialogActions>
        )}
      </Dialog>

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
    </>
  );
};
