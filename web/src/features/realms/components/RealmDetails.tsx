import { useTenant } from "@/shared/context";
import { useRealmDashboard } from "../hooks";
import { formatBytes } from "@/shared/utils";
import {
  DialogHeader,
  LoadingSkeleton,
  StatusBadge,
} from "@/shared/components";
import {
  Dialog,
  DialogContent,
  Box,
  Typography,
  Card,
  CardContent,
  Chip,
  Divider,
} from "@mui/material";
import { Circle } from "@mui/icons-material";

interface RealmDetailsProps {
  realmName: string;
  onClose: () => void;
}

export function RealmDetails({ realmName, onClose }: RealmDetailsProps) {
  const { selectedTenant } = useTenant();

  const { data: realmDashboard, isLoading } = useRealmDashboard(
    selectedTenant?.tenant_id,
    realmName,
  );

  return (
    <Dialog
      open={true}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: {
          maxHeight: "90vh",
        },
      }}
    >
      <DialogHeader
        title={realmName}
        subtitle="Realm Details"
        onClose={onClose}
      />

      <DialogContent dividers>
        {isLoading ? (
          <LoadingSkeleton variant="text" lines={8} />
        ) : realmDashboard ? (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {/* Realm Info */}
            {realmDashboard.realm_info && (
              <Card>
                <CardContent>
                  <Typography
                    variant="h6"
                    sx={{
                      mb: 3,
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      fontWeight: 600,
                    }}
                  >
                    Realm Configuration
                  </Typography>
                  <Box
                    sx={{
                      display: "grid",
                      gridTemplateColumns: "repeat(2, 1fr)",
                      gap: 2,
                    }}
                  >
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Realm ID
                      </Typography>
                      <Typography
                        variant="body2"
                        sx={{ fontFamily: "monospace", mt: 0.5 }}
                      >
                        {realmDashboard.realm_info.realm_id}
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Display Name
                      </Typography>
                      <Typography variant="body2" sx={{ mt: 0.5 }}>
                        {realmDashboard.realm_info.display_name || "N/A"}
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Status
                      </Typography>
                      <Box sx={{ mt: 0.5 }}>
                        <StatusBadge
                          status={
                            realmDashboard.realm_info.enabled
                              ? "success"
                              : "error"
                          }
                          label={
                            realmDashboard.realm_info.enabled
                              ? "Enabled"
                              : "Disabled"
                          }
                          size="small"
                        />
                      </Box>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        SSL Required
                      </Typography>
                      <Typography variant="body2" sx={{ mt: 0.5 }}>
                        {realmDashboard.realm_info.ssl_required || "N/A"}
                      </Typography>
                    </Box>
                  </Box>

                  {/* Event Configuration */}
                  <Box sx={{ mt: 4, pt: 3 }}>
                    <Divider sx={{ mb: 3 }} />
                    <Typography
                      variant="subtitle1"
                      sx={{
                        mb: 2,
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        fontWeight: 600,
                      }}
                    >
                      Event Configuration
                    </Typography>
                    <Box
                      sx={{
                        display: "grid",
                        gridTemplateColumns: "repeat(2, 1fr)",
                        gap: 2,
                      }}
                    >
                      <Box>
                        <Typography
                          variant="overline"
                          color="text.secondary"
                          sx={{ display: "block", fontSize: "0.75rem" }}
                        >
                          User Events
                        </Typography>
                        <Box sx={{ mt: 0.5 }}>
                          <StatusBadge
                            status={
                              realmDashboard.realm_info.events_enabled
                                ? "success"
                                : "error"
                            }
                            label={
                              realmDashboard.realm_info.events_enabled
                                ? "Enabled"
                                : "Disabled"
                            }
                            size="small"
                          />
                        </Box>
                      </Box>
                      <Box>
                        <Typography
                          variant="overline"
                          color="text.secondary"
                          sx={{ display: "block", fontSize: "0.75rem" }}
                        >
                          Admin Events
                        </Typography>
                        <Box sx={{ mt: 0.5 }}>
                          <StatusBadge
                            status={
                              realmDashboard.realm_info.admin_events_enabled
                                ? "success"
                                : "error"
                            }
                            label={
                              realmDashboard.realm_info.admin_events_enabled
                                ? "Enabled"
                                : "Disabled"
                            }
                            size="small"
                          />
                        </Box>
                      </Box>
                      {realmDashboard.realm_info.events_listeners &&
                        realmDashboard.realm_info.events_listeners.length >
                          0 && (
                          <Box sx={{ gridColumn: "1 / -1" }}>
                            <Typography
                              variant="overline"
                              color="text.secondary"
                              sx={{ display: "block", fontSize: "0.75rem" }}
                            >
                              Event Listeners
                            </Typography>
                            <Box
                              sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 1,
                                mt: 1,
                              }}
                            >
                              {realmDashboard.realm_info.events_listeners.map(
                                (listener) => (
                                  <Chip
                                    key={listener}
                                    label={listener}
                                    size="small"
                                    variant="outlined"
                                    sx={{ fontFamily: "monospace" }}
                                  />
                                ),
                              )}
                            </Box>
                          </Box>
                        )}
                      {realmDashboard.realm_info.enabled_event_types &&
                        realmDashboard.realm_info.enabled_event_types.length >
                          0 && (
                          <Box sx={{ gridColumn: "1 / -1" }}>
                            <Typography
                              variant="overline"
                              color="text.secondary"
                              sx={{ display: "block", fontSize: "0.75rem" }}
                            >
                              Enabled Event Types
                            </Typography>
                            <Box
                              sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 1,
                                mt: 1,
                              }}
                            >
                              {realmDashboard.realm_info.enabled_event_types
                                .slice(0, 10)
                                .map((type) => (
                                  <Chip
                                    key={type}
                                    label={type}
                                    size="small"
                                    variant="outlined"
                                    sx={{ fontFamily: "monospace" }}
                                  />
                                ))}
                              {realmDashboard.realm_info.enabled_event_types
                                .length > 10 && (
                                <Chip
                                  label={`+${realmDashboard.realm_info.enabled_event_types.length - 10} more`}
                                  size="small"
                                  variant="outlined"
                                  color="secondary"
                                />
                              )}
                            </Box>
                          </Box>
                        )}
                    </Box>
                  </Box>

                  {/* Security Settings */}
                  <Box sx={{ mt: 4, pt: 3 }}>
                    <Divider sx={{ mb: 3 }} />
                    <Typography
                      variant="subtitle1"
                      sx={{
                        mb: 2,
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        fontWeight: 600,
                      }}
                    >
                      Security Settings
                    </Typography>
                    <Box
                      sx={{
                        display: "grid",
                        gridTemplateColumns: "repeat(3, 1fr)",
                        gap: 2,
                      }}
                    >
                      <Box>
                        <Box
                          sx={{ display: "flex", alignItems: "center", gap: 1 }}
                        >
                          <Circle
                            sx={{
                              fontSize: 12,
                              color: realmDashboard.realm_info
                                .brute_force_protected
                                ? "success.main"
                                : "error.main",
                            }}
                          />
                          <Typography variant="caption">
                            Brute Force Protection
                          </Typography>
                        </Box>
                      </Box>
                      <Box>
                        <Box
                          sx={{ display: "flex", alignItems: "center", gap: 1 }}
                        >
                          <Circle
                            sx={{
                              fontSize: 12,
                              color: realmDashboard.realm_info.verify_email
                                ? "success.main"
                                : "text.secondary",
                            }}
                          />
                          <Typography variant="caption">
                            Email Verification
                          </Typography>
                        </Box>
                      </Box>
                      <Box>
                        <Box
                          sx={{ display: "flex", alignItems: "center", gap: 1 }}
                        >
                          <Circle
                            sx={{
                              fontSize: 12,
                              color: realmDashboard.realm_info
                                .reset_password_allowed
                                ? "success.main"
                                : "text.secondary",
                            }}
                          />
                          <Typography variant="caption">
                            Password Reset
                          </Typography>
                        </Box>
                      </Box>
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            )}

            {/* Metrics */}
            {realmDashboard.metrics && (
              <Card>
                <CardContent>
                  <Typography
                    variant="h6"
                    sx={{
                      mb: 3,
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      fontWeight: 600,
                    }}
                  >
                    Current Metrics
                  </Typography>
                  <Box
                    sx={{
                      display: "grid",
                      gridTemplateColumns: "repeat(3, 1fr)",
                      gap: 3,
                    }}
                  >
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Total Users
                      </Typography>
                      <Typography variant="h3" sx={{ mt: 1 }}>
                        {realmDashboard.metrics.total_users}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {realmDashboard.metrics.enabled_users} enabled,{" "}
                        {realmDashboard.metrics.disabled_users} disabled
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Active Sessions
                      </Typography>
                      <Typography variant="h3" sx={{ mt: 1 }}>
                        {realmDashboard.metrics.active_sessions}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {realmDashboard.metrics.offline_sessions} offline
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Total Clients
                      </Typography>
                      <Typography variant="h3" sx={{ mt: 1 }}>
                        {realmDashboard.metrics.total_clients}
                      </Typography>
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            )}

            {/* Recent Events */}
            {realmDashboard.recent_events && (
              <Card>
                <CardContent>
                  <Typography
                    variant="h6"
                    sx={{
                      mb: 3,
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      fontWeight: 600,
                    }}
                  >
                    Recent Events ({realmDashboard.recent_events.time_period})
                  </Typography>
                  <Box
                    sx={{
                      display: "grid",
                      gridTemplateColumns: "repeat(2, 1fr)",
                      gap: 2,
                    }}
                  >
                    <Box>
                      <Card variant="outlined">
                        <CardContent
                          sx={{
                            display: "flex",
                            justifyContent: "space-between",
                            alignItems: "center",
                          }}
                        >
                          <Typography
                            variant="subtitle2"
                            color="text.secondary"
                            sx={{ textTransform: "uppercase" }}
                          >
                            Successful Logins
                          </Typography>
                          <Typography variant="h4" color="success.main">
                            {realmDashboard.recent_events.logins}
                          </Typography>
                        </CardContent>
                      </Card>
                    </Box>
                    <Box>
                      <Card variant="outlined">
                        <CardContent
                          sx={{
                            display: "flex",
                            justifyContent: "space-between",
                            alignItems: "center",
                          }}
                        >
                          <Typography
                            variant="subtitle2"
                            color="text.secondary"
                            sx={{ textTransform: "uppercase" }}
                          >
                            Failed Logins
                          </Typography>
                          <Typography variant="h4" color="error.main">
                            {realmDashboard.recent_events.login_errors}
                          </Typography>
                        </CardContent>
                      </Card>
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            )}

            {/* Health Status */}
            {realmDashboard.health && (
              <Card>
                <CardContent>
                  <Typography
                    variant="h6"
                    sx={{
                      mb: 3,
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      fontWeight: 600,
                    }}
                  >
                    Server Health
                  </Typography>
                  <Box
                    sx={{
                      display: "grid",
                      gridTemplateColumns: "repeat(4, 1fr)",
                      gap: 2,
                    }}
                  >
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Status
                      </Typography>
                      <Box sx={{ mt: 1 }}>
                        <StatusBadge
                          status={
                            realmDashboard.health.status === "UP"
                              ? "success"
                              : "error"
                          }
                          label={realmDashboard.health.status}
                          size="small"
                        />
                      </Box>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Response Time
                      </Typography>
                      <Typography variant="body2" sx={{ mt: 1 }}>
                        {realmDashboard.health.response_time_ms}ms
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Memory Used
                      </Typography>
                      <Typography variant="body2" sx={{ mt: 1 }}>
                        {formatBytes(realmDashboard.health.memory_used_bytes)}
                      </Typography>
                    </Box>
                    <Box>
                      <Typography
                        variant="overline"
                        color="text.secondary"
                        sx={{ display: "block", fontSize: "0.75rem" }}
                      >
                        Uptime
                      </Typography>
                      <Typography variant="body2" sx={{ mt: 1 }}>
                        {Math.floor(
                          realmDashboard.health.uptime_millis / 1000 / 60 / 60,
                        )}
                        h
                      </Typography>
                    </Box>
                  </Box>
                </CardContent>
              </Card>
            )}
          </Box>
        ) : (
          <Box
            sx={{
              display: "flex",
              justifyContent: "center",
              py: 8,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No data available for this realm
            </Typography>
          </Box>
        )}
      </DialogContent>
    </Dialog>
  );
}
