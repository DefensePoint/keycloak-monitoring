import React from "react";
import {
  Box,
  Typography,
  Chip,
  Paper,
  alpha,
  useTheme,
  Tooltip,
} from "@mui/material";
import type { RealmCardProps } from "../types";

export const RealmCard: React.FC<RealmCardProps> = ({ realm, onClick }) => {
  const theme = useTheme();

  const hasEventWarning =
    realm.enabled &&
    (!realm.events_enabled ||
      !realm.events_listeners ||
      realm.events_listeners.length === 0);

  const isHealthy = realm.enabled && realm.is_healthy;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 3,
        border: "1px solid",
        borderColor: "divider",
        cursor: "pointer",
        transition: "border-color 0.2s",
        "&:hover": {
          borderColor: theme.palette.error.main,
        },
      }}
      onClick={onClick}
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
        <Typography variant="h6" sx={{ fontWeight: 600 }}>
          {realm.realm_name}
        </Typography>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          {hasEventWarning && (
            <Tooltip title="Event collection is not properly configured">
              <Chip
                label="Warning"
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
            label={isHealthy ? "Healthy" : "Unhealthy"}
            size="small"
            sx={{
              bgcolor: isHealthy
                ? alpha(theme.palette.success.main, 0.2)
                : alpha(theme.palette.error.main, 0.2),
              color: isHealthy
                ? theme.palette.success.main
                : theme.palette.error.main,
              fontWeight: 700,
              textTransform: "uppercase",
              fontSize: "0.75rem",
            }}
          />
        </Box>
      </Box>

      {/* Metrics */}
      {realm.metrics && (
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
              Users
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {realm.metrics.total_users} ({realm.metrics.enabled_users}{" "}
              enabled)
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
              Sessions
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {realm.metrics.active_sessions} active
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
              Clients
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {realm.metrics.total_clients}
            </Typography>
          </Box>

          <Box sx={{ pt: 1.5, borderTop: "1px solid", borderColor: "divider" }}>
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                mb: 0.5,
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
                Logins
              </Typography>
              <Typography
                variant="body2"
                sx={{ fontWeight: 600, color: theme.palette.success.main }}
              >
                {realm.metrics.login_events}
              </Typography>
            </Box>

            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                mb: 0.5,
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
                Failed Logins
              </Typography>
              <Typography
                variant="body2"
                sx={{ fontWeight: 600, color: theme.palette.error.main }}
              >
                {realm.metrics.failed_login_events}
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
                Logouts
              </Typography>
              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                {realm.metrics.logout_events}
              </Typography>
            </Box>
          </Box>
        </Box>
      )}
    </Paper>
  );
};
