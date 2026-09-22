import React from "react";
import {
  Box,
  Typography,
  Chip,
  Paper,
  IconButton,
  Tooltip,
  alpha,
  useTheme,
} from "@mui/material";
import { Edit, Delete, PowerSettingsNew } from "@mui/icons-material";
import type { TenantCardProps } from "../types";

export const TenantCard: React.FC<TenantCardProps> = ({
  tenant,
  onEdit,
  onDelete,
  onToggleEnabled,
}) => {
  const theme = useTheme();

  const isHealthy = tenant.health_status?.toLowerCase() === "healthy";
  const isConfigDefined = tenant.is_config_defined;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 3,
        border: "1px solid",
        borderColor: "divider",
        cursor: isConfigDefined ? "default" : "pointer",
        transition: "border-color 0.2s",
        "&:hover": {
          borderColor: isConfigDefined ? "divider" : theme.palette.error.main,
        },
      }}
      onClick={isConfigDefined ? undefined : () => onEdit(tenant)}
    >
      {/* Header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          mb: 2,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            {tenant.name}
          </Typography>
          {tenant.is_default && (
            <Chip
              label="Default"
              size="small"
              sx={{
                bgcolor: alpha(theme.palette.error.main, 0.2),
                color: theme.palette.error.main,
                fontWeight: 700,
                textTransform: "uppercase",
                fontSize: "0.75rem",
              }}
            />
          )}
          {isConfigDefined && (
            <Tooltip title="Managed via config.yaml — edit the config file and restart to change this tenant">
              <Chip
                label="Read-only"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.info.main, 0.2),
                  color: theme.palette.info.main,
                  fontWeight: 700,
                  textTransform: "uppercase",
                  fontSize: "0.75rem",
                }}
              />
            </Tooltip>
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          {tenant.last_error && (
            <Tooltip title={tenant.last_error}>
              <Chip
                label="Error"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.warning.main, 0.2),
                  color: theme.palette.warning.main,
                  fontWeight: 700,
                  textTransform: "uppercase",
                  fontSize: "0.75rem",
                }}
              />
            </Tooltip>
          )}
          <Chip
            label={
              isHealthy ? "Healthy" : tenant.enabled ? "Unhealthy" : "Disabled"
            }
            size="small"
            sx={{
              bgcolor: isHealthy
                ? alpha(theme.palette.success.main, 0.2)
                : !tenant.enabled
                  ? alpha(theme.palette.text.secondary, 0.2)
                  : alpha(theme.palette.error.main, 0.2),
              color: isHealthy
                ? theme.palette.success.main
                : !tenant.enabled
                  ? theme.palette.text.secondary
                  : theme.palette.error.main,
              fontWeight: 700,
              textTransform: "uppercase",
              fontSize: "0.75rem",
            }}
          />
        </Box>
      </Box>

      {/* Info Rows */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <Typography
            variant="caption"
            sx={{
              color: "text.secondary",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            Server URL
          </Typography>
          <Typography
            variant="body2"
            sx={{ fontWeight: 600, maxWidth: "60%", textAlign: "right" }}
            noWrap
          >
            {tenant.server_url}
          </Typography>
        </Box>

        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <Typography
            variant="caption"
            sx={{
              color: "text.secondary",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            Default Realm
          </Typography>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            {tenant.default_realm || "-"}
          </Typography>
        </Box>

        {tenant.last_health_check && (
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
            }}
          >
            <Typography
              variant="caption"
              sx={{
                color: "text.secondary",
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Last Check
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {new Date(tenant.last_health_check).toLocaleString()}
            </Typography>
          </Box>
        )}

        {/* Action Icons */}
        <Box
          sx={{
            pt: 1.5,
            borderTop: "1px solid",
            borderColor: "divider",
            display: "flex",
            justifyContent: "flex-end",
            gap: 0.5,
          }}
        >
          <Tooltip
            title={
              isConfigDefined
                ? "Managed via config.yaml — edit the config file and restart to change this tenant"
                : tenant.enabled
                  ? "Disable"
                  : "Enable"
            }
          >
            <span>
              <IconButton
                size="small"
                disabled={isConfigDefined}
                aria-label={tenant.enabled ? "Disable" : "Enable"}
                onClick={(e) => {
                  e.stopPropagation();
                  onToggleEnabled(tenant);
                }}
                sx={{
                  color: tenant.enabled
                    ? theme.palette.warning.main
                    : theme.palette.success.main,
                }}
              >
                <PowerSettingsNew fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip
            title={
              isConfigDefined
                ? "Managed via config.yaml — edit the config file and restart to change this tenant"
                : "Edit"
            }
          >
            <span>
              <IconButton
                size="small"
                disabled={isConfigDefined}
                aria-label="Edit"
                onClick={(e) => {
                  e.stopPropagation();
                  onEdit(tenant);
                }}
                sx={{ color: theme.palette.primary.main }}
              >
                <Edit fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
          <Tooltip
            title={
              isConfigDefined
                ? "Managed via config.yaml — edit the config file and restart to change this tenant"
                : "Delete"
            }
          >
            <span>
              <IconButton
                size="small"
                disabled={isConfigDefined}
                aria-label="Delete"
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete(tenant.tenant_id);
                }}
                sx={{ color: theme.palette.error.main }}
              >
                <Delete fontSize="small" />
              </IconButton>
            </span>
          </Tooltip>
        </Box>
      </Box>
    </Paper>
  );
};
