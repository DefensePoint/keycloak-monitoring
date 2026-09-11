import { useState, useEffect, useMemo, useCallback } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import {
  Box,
  Typography,
  Paper,
  Chip,
  Button,
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  IconButton,
  Pagination,
  alpha,
  useTheme,
} from "@mui/material";
import {
  Close,
  FiberManualRecord,
  CheckCircle,
  Cancel,
  ArrowForward,
} from "@mui/icons-material";
import { useTenant } from "@/shared/context";
import {
  useAlerts,
  useRealmSelector,
  useKeycloakDashboard,
  useRealmDashboard,
  useRealmEvents,
  useRealmUsers,
  useRealmClients,
} from "@/shared/hooks";
import {
  LoadingSkeleton,
  PageHeader,
  FeatureErrorBoundary,
} from "@/shared/components";
import { formatBytes } from "@/shared/utils";
import { RealmSelector } from "../components";
import type {
  Event,
  ConfigurationAlert,
  KeycloakUser,
  KeycloakClient,
} from "../types";
import { useRealmNavigation } from "../hooks";

type EventWithFormattedDate = Event & {
  formattedDate: string;
};

export function RealmPage() {
  const { realmName, tenantId } = useParams<{
    realmName: string;
    tenantId: string;
  }>();
  const navigate = useNavigate();
  const { selectedTenant } = useTenant();
  const theme = useTheme();

  const [selectedEvent, setSelectedEvent] = useState<Event | null>(null);
  const [selectedUser, setSelectedUser] = useState<KeycloakUser | null>(null);
  const [selectedClient, setSelectedClient] = useState<KeycloakClient | null>(
    null,
  );
  const [userSearchTerm, setUserSearchTerm] = useState<string>("");
  const [clientSearchTerm, setClientSearchTerm] = useState<string>("");
  const [showAllUsers, setShowAllUsers] = useState<boolean>(false);
  const [showAllClients, setShowAllClients] = useState<boolean>(false);
  const [eventsPage, setEventsPage] = useState(1);
  const eventsPerPage = 10;

  const { data: realmDashboard, isLoading } = useRealmDashboard({
    tenantId: selectedTenant?.tenant_id,
    realm: realmName,
  });

  const { data: allRealmsDashboard } = useKeycloakDashboard({
    tenantId: selectedTenant?.tenant_id,
  });

  const allRealms =
    allRealmsDashboard && "realms" in allRealmsDashboard
      ? allRealmsDashboard.realms
      : [];

  const { selectedRealm, handleRealmChange } = useRealmSelector({
    tenantId,
    defaultRealm: selectedTenant?.default_realm,
    availableRealms: allRealms,
  });

  const { navigateToRealm } = useRealmNavigation(tenantId);

  useEffect(() => {
    if (realmName && selectedTenant?.default_realm && allRealms.length > 0) {
      if (
        selectedRealm === selectedTenant.default_realm &&
        realmName !== selectedTenant.default_realm
      ) {
        navigate(`/${tenantId}/realm/${selectedTenant.default_realm}`, {
          replace: true,
        });
      }
    }
  }, [
    realmName,
    selectedRealm,
    selectedTenant?.default_realm,
    allRealms.length,
    tenantId,
    navigate,
  ]);

  const handleRealmSelectorChange = useCallback(
    (realm: string) => {
      handleRealmChange(realm);
      navigateToRealm(realm);
    },
    [handleRealmChange, navigateToRealm],
  );

  const { data: eventsResponse } = useRealmEvents({
    tenantId: selectedTenant?.tenant_id,
    realmName,
  });

  const { data: usersResponse } = useRealmUsers({
    tenantId: selectedTenant?.tenant_id,
    realmName,
  });

  const { data: clientsResponse } = useRealmClients({
    tenantId: selectedTenant?.tenant_id,
    realmName,
  });

  const { data: alertsResponse } = useAlerts({
    tenantId: selectedTenant?.tenant_id,
    status: "active",
    realm: realmName,
    refetchInterval: 30000,
  });

  const allEvents = useMemo(
    () => eventsResponse?.events || [],
    [eventsResponse?.events],
  );
  const allUsers = useMemo(
    () => usersResponse?.users || [],
    [usersResponse?.users],
  );
  const allClients = useMemo(
    () => clientsResponse?.clients || [],
    [clientsResponse?.clients],
  );
  const alerts = useMemo(
    () => alertsResponse?.alerts || [],
    [alertsResponse?.alerts],
  );

  // Memoize helper functions
  const getSeverityColor = useCallback(
    (severity: string): { bgcolor: string; color: string } => {
      switch (severity) {
        case "critical":
        case "error":
          return {
            bgcolor: alpha(theme.palette.error.main, 0.2),
            color: theme.palette.error.light,
          };
        case "warning":
          return {
            bgcolor: alpha(theme.palette.warning.main, 0.2),
            color: theme.palette.warning.light,
          };
        default:
          return {
            bgcolor: alpha(theme.palette.info.main, 0.2),
            color: theme.palette.info.light,
          };
      }
    },
    [theme],
  );

  const getClientStatusColor = useCallback(
    (client: {
      enabled?: boolean;
      publicClient?: boolean;
      bearerOnly?: boolean;
    }): string => {
      if (!client.enabled) return theme.palette.text.disabled;
      if (client.publicClient) return theme.palette.info.main;
      if (client.bearerOnly) return theme.palette.warning.main;
      return theme.palette.success.main;
    },
    [theme],
  );

  // Memoize filtered users
  const users = useMemo(() => {
    return allUsers.filter((user: KeycloakUser) => {
      if (!userSearchTerm) return true;
      const term = userSearchTerm.toLowerCase();
      return (
        user.username?.toLowerCase().includes(term) ||
        user.email?.toLowerCase().includes(term) ||
        user.firstName?.toLowerCase().includes(term) ||
        user.lastName?.toLowerCase().includes(term)
      );
    });
  }, [allUsers, userSearchTerm]);

  // Memoize filtered clients
  const clients = useMemo(() => {
    return allClients.filter((client: KeycloakClient) => {
      if (!clientSearchTerm) return true;
      const term = clientSearchTerm.toLowerCase();
      return (
        client.clientId?.toLowerCase().includes(term) ||
        client.name?.toLowerCase().includes(term) ||
        client.description?.toLowerCase().includes(term)
      );
    });
  }, [allClients, clientSearchTerm]);

  // Memoize paginated events with formatted dates
  const { totalEventsPages, paginatedEvents } = useMemo(() => {
    const totalPages = Math.ceil(allEvents.length / eventsPerPage);
    const eventsWithDates: EventWithFormattedDate[] = allEvents
      .slice((eventsPage - 1) * eventsPerPage, eventsPage * eventsPerPage)
      .map((event) => ({
        ...event,
        formattedDate: new Date(event.timestamp).toLocaleString(),
      }));

    return {
      totalEventsPages: totalPages,
      paginatedEvents: eventsWithDates,
    };
  }, [allEvents, eventsPage]);

  if (!realmName) {
    return null;
  }

  if (isLoading) {
    return <LoadingSkeleton variant="card" count={4} />;
  }

  if (!realmDashboard) {
    return (
      <Box sx={{ p: 4, textAlign: "center" }}>
        <Typography color="text.secondary">No realm data available</Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header */}
      <PageHeader
        title="Realms"
        subtitle="Manage and monitor Keycloak realm configurations"
      >
        <RealmSelector
          selectedRealm={realmName || "all"}
          onRealmChange={handleRealmSelectorChange}
          realms={allRealms}
          defaultRealm={selectedTenant?.default_realm}
        />
      </PageHeader>

      {/* Realm Title */}
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 3 }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          {realmName}
        </Typography>
        {realmName === selectedTenant?.default_realm && (
          <Chip label="Default" size="small" color="error" />
        )}
      </Box>

      <Box>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
          {/* Realm Configuration */}
          {realmDashboard.realm_info && (
            <Paper
              elevation={0}
              sx={{ p: 3, border: "1px solid", borderColor: "divider" }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 3,
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
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
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
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Display Name
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {realmDashboard.realm_info.display_name || "N/A"}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Status
                  </Typography>
                  <Box sx={{ mt: 0.5 }}>
                    <Chip
                      label={
                        realmDashboard.realm_info.enabled
                          ? "Enabled"
                          : "Disabled"
                      }
                      size="small"
                      sx={{
                        bgcolor: realmDashboard.realm_info.enabled
                          ? alpha(theme.palette.success.main, 0.2)
                          : alpha(theme.palette.error.main, 0.2),
                        color: realmDashboard.realm_info.enabled
                          ? theme.palette.success.light
                          : theme.palette.error.light,
                        fontWeight: 700,
                        textTransform: "uppercase",
                        fontSize: "0.7rem",
                      }}
                    />
                  </Box>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    SSL Required
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {realmDashboard.realm_info.ssl_required || "N/A"}
                  </Typography>
                </Box>
              </Box>

              {/* Event Configuration */}
              <Box
                sx={{
                  mt: 4,
                  pt: 3,
                  borderTop: "1px solid",
                  borderColor: "divider",
                }}
              >
                <Typography
                  variant="subtitle1"
                  sx={{
                    fontWeight: 700,
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 2,
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
                      variant="caption"
                      color="text.secondary"
                      sx={{ textTransform: "uppercase" }}
                    >
                      User Events
                    </Typography>
                    <Box sx={{ mt: 0.5 }}>
                      <Chip
                        label={
                          realmDashboard.realm_info.events_enabled
                            ? "Enabled"
                            : "Disabled"
                        }
                        size="small"
                        sx={{
                          bgcolor: realmDashboard.realm_info.events_enabled
                            ? alpha(theme.palette.success.main, 0.2)
                            : alpha(theme.palette.error.main, 0.2),
                          color: realmDashboard.realm_info.events_enabled
                            ? theme.palette.success.light
                            : theme.palette.error.light,
                          fontWeight: 700,
                          textTransform: "uppercase",
                          fontSize: "0.7rem",
                        }}
                      />
                    </Box>
                  </Box>
                  <Box>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ textTransform: "uppercase" }}
                    >
                      Admin Events
                    </Typography>
                    <Box sx={{ mt: 0.5 }}>
                      <Chip
                        label={
                          realmDashboard.realm_info.admin_events_enabled
                            ? "Enabled"
                            : "Disabled"
                        }
                        size="small"
                        sx={{
                          bgcolor: realmDashboard.realm_info
                            .admin_events_enabled
                            ? alpha(theme.palette.success.main, 0.2)
                            : alpha(theme.palette.error.main, 0.2),
                          color: realmDashboard.realm_info.admin_events_enabled
                            ? theme.palette.success.light
                            : theme.palette.error.light,
                          fontWeight: 700,
                          textTransform: "uppercase",
                          fontSize: "0.7rem",
                        }}
                      />
                    </Box>
                  </Box>
                </Box>
              </Box>

              {/* Security Settings */}
              <Box
                sx={{
                  mt: 4,
                  pt: 3,
                  borderTop: "1px solid",
                  borderColor: "divider",
                }}
              >
                <Typography
                  variant="subtitle1"
                  sx={{
                    fontWeight: 700,
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 2,
                  }}
                >
                  Security Settings
                </Typography>
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: "repeat(2, 1fr)",
                    gap: 2,
                  }}
                >
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      {realmDashboard.realm_info.brute_force_protected ? (
                        <CheckCircle
                          sx={{
                            fontSize: 16,
                            color: theme.palette.success.main,
                          }}
                        />
                      ) : (
                        <Cancel
                          sx={{ fontSize: 16, color: theme.palette.error.main }}
                        />
                      )}
                      <Typography variant="caption">
                        Brute Force Protection
                      </Typography>
                    </Box>
                  </Box>
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      {realmDashboard.realm_info.verify_email ? (
                        <CheckCircle
                          sx={{
                            fontSize: 16,
                            color: theme.palette.success.main,
                          }}
                        />
                      ) : (
                        <FiberManualRecord
                          sx={{
                            fontSize: 12,
                            color: theme.palette.text.disabled,
                          }}
                        />
                      )}
                      <Typography variant="caption">
                        Email Verification
                      </Typography>
                    </Box>
                  </Box>
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      {realmDashboard.realm_info.reset_password_allowed ? (
                        <CheckCircle
                          sx={{
                            fontSize: 16,
                            color: theme.palette.success.main,
                          }}
                        />
                      ) : (
                        <FiberManualRecord
                          sx={{
                            fontSize: 12,
                            color: theme.palette.text.disabled,
                          }}
                        />
                      )}
                      <Typography variant="caption">Password Reset</Typography>
                    </Box>
                  </Box>
                </Box>
              </Box>
            </Paper>
          )}

          {/* Configuration Alerts */}
          <FeatureErrorBoundary
            featureId="RealmAlerts"
            title="Configuration Alerts"
            resetKeys={[selectedTenant?.tenant_id, realmName]}
          >
            <Paper
              elevation={0}
              sx={{
                border: "1px solid",
                borderColor: "divider",
                overflow: "hidden",
              }}
            >
              <Box
                sx={{
                  px: 3,
                  py: 2,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
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
                <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                  {alerts.length > 0 && (
                    <Chip
                      label={`${alerts.length} Active`}
                      size="small"
                      sx={{
                        bgcolor: alpha(theme.palette.error.main, 0.2),
                        color: theme.palette.error.light,
                        border: "1px solid",
                        borderColor: alpha(theme.palette.error.main, 0.3),
                        fontWeight: 700,
                        textTransform: "uppercase",
                        fontSize: "0.7rem",
                      }}
                    />
                  )}
                  <Link
                    to={`/${tenantId}/alerts?realm=${realmName}`}
                    style={{
                      textDecoration: "none",
                      color: theme.palette.error.main,
                      fontSize: "0.875rem",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      fontWeight: 600,
                    }}
                  >
                    View All →
                  </Link>
                </Box>
              </Box>
              <Box sx={{ p: 3 }}>
                {alerts.length > 0 ? (
                  <Box
                    sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}
                  >
                    {alerts.slice(0, 5).map((alert: ConfigurationAlert) => (
                      <Box
                        key={alert.alert_id}
                        sx={{
                          display: "flex",
                          alignItems: "flex-start",
                          gap: 2,
                          p: 2,
                          bgcolor: "background.paper",
                          borderRadius: 1,
                          border: "1px solid",
                          borderColor: "divider",
                          "&:hover": {
                            borderColor: alpha(theme.palette.error.main, 0.5),
                          },
                          transition: "border-color 0.2s",
                        }}
                      >
                        <Box sx={{ flexShrink: 0 }}>
                          <Chip
                            label={alert.severity}
                            size="small"
                            sx={{
                              ...getSeverityColor(alert.severity),
                              fontWeight: 600,
                              textTransform: "uppercase",
                              fontSize: "0.7rem",
                              border: "1px solid",
                              borderColor: getSeverityColor(alert.severity)
                                .color,
                            }}
                          />
                        </Box>
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                          <Typography
                            variant="body2"
                            fontWeight={600}
                            sx={{ mb: 0.5 }}
                          >
                            {alert.title}
                          </Typography>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{
                              display: "-webkit-box",
                              WebkitLineClamp: 2,
                              WebkitBoxOrient: "vertical",
                              overflow: "hidden",
                            }}
                          >
                            {alert.description}
                          </Typography>
                        </Box>
                      </Box>
                    ))}
                  </Box>
                ) : (
                  <Box sx={{ textAlign: "center", py: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                      No active alerts
                    </Typography>
                  </Box>
                )}
              </Box>
            </Paper>
          </FeatureErrorBoundary>

          {/* Metrics */}
          {realmDashboard.metrics && (
            <Paper
              elevation={0}
              sx={{ p: 3, border: "1px solid", borderColor: "divider" }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 3,
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
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Total Users
                  </Typography>
                  <Typography variant="h3" sx={{ mt: 1, mb: 0.5 }}>
                    {realmDashboard.metrics.total_users}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {realmDashboard.metrics.enabled_users} enabled,{" "}
                    {realmDashboard.metrics.disabled_users} disabled
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Active Sessions
                  </Typography>
                  <Typography variant="h3" sx={{ mt: 1, mb: 0.5 }}>
                    {realmDashboard.metrics.active_sessions}
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {realmDashboard.metrics.offline_sessions} offline
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Total Clients
                  </Typography>
                  <Typography variant="h3" sx={{ mt: 1 }}>
                    {realmDashboard.metrics.total_clients}
                  </Typography>
                </Box>
              </Box>
            </Paper>
          )}

          {/* Users & Clients Grid */}
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", lg: "repeat(2, 1fr)" },
              gap: 3,
            }}
          >
            {/* Users Section */}
            <Paper
              elevation={0}
              sx={{
                border: "1px solid",
                borderColor: "divider",
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
              }}
            >
              <Box
                sx={{
                  px: 3,
                  py: 2,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
                <Typography
                  variant="h6"
                  sx={{
                    fontWeight: 700,
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Users
                </Typography>
                {realmDashboard.metrics && (
                  <Typography variant="body2" color="text.secondary">
                    {realmDashboard.metrics.total_users} total
                  </Typography>
                )}
              </Box>
              <Box sx={{ p: 3 }}>
                <TextField
                  fullWidth
                  size="small"
                  placeholder="Search users..."
                  value={userSearchTerm}
                  onChange={(e) => setUserSearchTerm(e.target.value)}
                  sx={{ mb: 1.5 }}
                />
                {users.length > 0 ? (
                  <>
                    <Box
                      sx={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 1,
                        maxHeight: 320,
                        overflowY: "auto",
                      }}
                    >
                      {(showAllUsers ? users : users.slice(0, 10)).map(
                        (user: KeycloakUser) => (
                          <Box
                            key={user.id}
                            component="button"
                            onClick={() =>
                              navigate(
                                `/${tenantId}/realm/${realmName}/user/${user.id}`,
                              )
                            }
                            sx={{
                              width: "100%",
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "space-between",
                              p: 1,
                              bgcolor: "background.default",
                              borderRadius: 1,
                              border: "none",
                              cursor: "pointer",
                              textAlign: "left",
                              "&:hover": {
                                bgcolor: alpha(theme.palette.primary.main, 0.1),
                              },
                              transition: "background-color 0.2s",
                            }}
                          >
                            <Box
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                gap: 1,
                                flex: 1,
                                minWidth: 0,
                              }}
                            >
                              <FiberManualRecord
                                sx={{
                                  fontSize: 8,
                                  color: user.enabled
                                    ? theme.palette.success.main
                                    : theme.palette.text.disabled,
                                }}
                              />
                              <Box sx={{ minWidth: 0, flex: 1 }}>
                                <Typography
                                  variant="body2"
                                  fontWeight={500}
                                  noWrap
                                >
                                  {user.username}
                                </Typography>
                                {user.email && (
                                  <Typography
                                    variant="caption"
                                    color="text.secondary"
                                    noWrap
                                  >
                                    {user.email}
                                  </Typography>
                                )}
                              </Box>
                            </Box>
                          </Box>
                        ),
                      )}
                    </Box>
                    {users.length > 10 && (
                      <Box sx={{ textAlign: "center", pt: 1.5 }}>
                        <Button
                          size="small"
                          onClick={() => setShowAllUsers(!showAllUsers)}
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            fontWeight: 600,
                            fontSize: "0.75rem",
                          }}
                        >
                          {showAllUsers
                            ? "Show Less"
                            : `Show All (${users.length} users)`}
                        </Button>
                      </Box>
                    )}
                  </>
                ) : (
                  <Box sx={{ textAlign: "center", py: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                      {userSearchTerm
                        ? "No users match your search"
                        : "No users found"}
                    </Typography>
                  </Box>
                )}
              </Box>
            </Paper>

            {/* Clients Section */}
            <Paper
              elevation={0}
              sx={{
                border: "1px solid",
                borderColor: "divider",
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
              }}
            >
              <Box
                sx={{
                  px: 3,
                  py: 2,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
                <Typography
                  variant="h6"
                  sx={{
                    fontWeight: 700,
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Clients
                </Typography>
                {realmDashboard.metrics && (
                  <Typography variant="body2" color="text.secondary">
                    {realmDashboard.metrics.total_clients} total
                  </Typography>
                )}
              </Box>
              <Box sx={{ p: 3 }}>
                <TextField
                  fullWidth
                  size="small"
                  placeholder="Search clients..."
                  value={clientSearchTerm}
                  onChange={(e) => setClientSearchTerm(e.target.value)}
                  sx={{ mb: 1.5 }}
                />
                {clients.length > 0 ? (
                  <>
                    <Box
                      sx={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 1,
                        maxHeight: 320,
                        overflowY: "auto",
                      }}
                    >
                      {(showAllClients ? clients : clients.slice(0, 10)).map(
                        (client: KeycloakClient) => (
                          <Box
                            key={client.id}
                            component="button"
                            onClick={() => setSelectedClient(client)}
                            sx={{
                              width: "100%",
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "space-between",
                              p: 1,
                              bgcolor: "background.default",
                              borderRadius: 1,
                              border: "none",
                              cursor: "pointer",
                              textAlign: "left",
                              "&:hover": {
                                bgcolor: alpha(theme.palette.primary.main, 0.1),
                              },
                              transition: "background-color 0.2s",
                            }}
                          >
                            <Box
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                gap: 1,
                                flex: 1,
                                minWidth: 0,
                              }}
                            >
                              <FiberManualRecord
                                sx={{
                                  fontSize: 8,
                                  color: getClientStatusColor(client),
                                }}
                              />
                              <Box sx={{ minWidth: 0, flex: 1 }}>
                                <Typography
                                  variant="body2"
                                  fontWeight={500}
                                  noWrap
                                >
                                  {client.clientId}
                                </Typography>
                                {client.name &&
                                  client.name !== client.clientId && (
                                    <Typography
                                      variant="caption"
                                      color="text.secondary"
                                      noWrap
                                    >
                                      {client.name}
                                    </Typography>
                                  )}
                              </Box>
                            </Box>
                            <Box
                              sx={{
                                display: "flex",
                                gap: 1,
                                flexShrink: 0,
                                ml: 1,
                              }}
                            >
                              {client.publicClient && (
                                <Chip
                                  label="Public"
                                  size="small"
                                  sx={{
                                    bgcolor: alpha(
                                      theme.palette.info.main,
                                      0.2,
                                    ),
                                    color: theme.palette.info.light,
                                    fontSize: "0.65rem",
                                    height: 20,
                                  }}
                                />
                              )}
                              {client.bearerOnly && (
                                <Chip
                                  label="Bearer"
                                  size="small"
                                  sx={{
                                    bgcolor: alpha(
                                      theme.palette.warning.main,
                                      0.2,
                                    ),
                                    color: theme.palette.warning.light,
                                    fontSize: "0.65rem",
                                    height: 20,
                                  }}
                                />
                              )}
                            </Box>
                          </Box>
                        ),
                      )}
                    </Box>
                    {clients.length > 10 && (
                      <Box sx={{ textAlign: "center", pt: 1.5 }}>
                        <Button
                          size="small"
                          onClick={() => setShowAllClients(!showAllClients)}
                          sx={{
                            textTransform: "uppercase",
                            letterSpacing: "0.05em",
                            fontWeight: 600,
                            fontSize: "0.75rem",
                          }}
                        >
                          {showAllClients
                            ? "Show Less"
                            : `Show All (${clients.length} clients)`}
                        </Button>
                      </Box>
                    )}
                  </>
                ) : (
                  <Box sx={{ textAlign: "center", py: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                      {clientSearchTerm
                        ? "No clients match your search"
                        : "No clients found"}
                    </Typography>
                  </Box>
                )}
              </Box>
            </Paper>
          </Box>

          {/* Recent Events */}
          {realmDashboard.recent_events && (
            <Paper
              elevation={0}
              sx={{ p: 3, border: "1px solid", borderColor: "divider" }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 3,
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
                  <Box
                    sx={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      p: 1.5,
                      bgcolor: "background.default",
                      borderRadius: 1,
                    }}
                  >
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                      }}
                    >
                      Successful Logins
                    </Typography>
                    <Typography
                      variant="h4"
                      fontWeight={700}
                      color="success.light"
                    >
                      {realmDashboard.recent_events.logins}
                    </Typography>
                  </Box>
                </Box>
                <Box>
                  <Box
                    sx={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      p: 1.5,
                      bgcolor: "background.default",
                      borderRadius: 1,
                    }}
                  >
                    <Typography
                      variant="body2"
                      color="text.secondary"
                      sx={{
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                      }}
                    >
                      Failed Logins
                    </Typography>
                    <Typography variant="h4" color="error.light">
                      {realmDashboard.recent_events.login_errors}
                    </Typography>
                  </Box>
                </Box>
              </Box>
            </Paper>
          )}

          {/* Realm Events Table */}
          <Paper
            elevation={0}
            sx={{
              border: "1px solid",
              borderColor: "divider",
              overflow: "hidden",
            }}
          >
            <Box
              sx={{
                px: 3,
                py: 2,
                borderBottom: "1px solid",
                borderColor: "divider",
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
              }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Realm Events
              </Typography>
              {allEvents.length > 0 && (
                <Typography variant="body2" color="text.secondary">
                  {allEvents.length} total
                </Typography>
              )}
            </Box>
            <Box sx={{ width: "100%", overflowX: "auto" }}>
              <Box
                component="table"
                sx={{ width: "100%", borderCollapse: "collapse" }}
              >
                <Box component="thead">
                  <Box component="tr">
                    <Box
                      component="th"
                      sx={{
                        textAlign: "left",
                        px: 3,
                        py: 1.5,
                        fontSize: "0.75rem",
                        fontWeight: 500,
                        color: "text.secondary",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        borderBottom: "1px solid",
                        borderColor: "divider",
                      }}
                    >
                      Timestamp
                    </Box>
                    <Box
                      component="th"
                      sx={{
                        textAlign: "left",
                        px: 3,
                        py: 1.5,
                        fontSize: "0.75rem",
                        fontWeight: 500,
                        color: "text.secondary",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        borderBottom: "1px solid",
                        borderColor: "divider",
                      }}
                    >
                      Type
                    </Box>
                    <Box
                      component="th"
                      sx={{
                        textAlign: "left",
                        px: 3,
                        py: 1.5,
                        fontSize: "0.75rem",
                        fontWeight: 500,
                        color: "text.secondary",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        borderBottom: "1px solid",
                        borderColor: "divider",
                      }}
                    >
                      Description
                    </Box>
                    <Box
                      component="th"
                      sx={{
                        textAlign: "left",
                        px: 3,
                        py: 1.5,
                        fontSize: "0.75rem",
                        fontWeight: 500,
                        color: "text.secondary",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        borderBottom: "1px solid",
                        borderColor: "divider",
                      }}
                    >
                      Severity
                    </Box>
                    <Box
                      component="th"
                      sx={{
                        textAlign: "right",
                        px: 3,
                        py: 1.5,
                        fontSize: "0.75rem",
                        fontWeight: 500,
                        color: "text.secondary",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        borderBottom: "1px solid",
                        borderColor: "divider",
                      }}
                    />
                  </Box>
                </Box>
                <Box component="tbody">
                  {allEvents.length === 0 ? (
                    <Box component="tr">
                      <Box
                        component="td"
                        colSpan={5}
                        sx={{ px: 3, py: 4, textAlign: "center" }}
                      >
                        <Typography variant="body2" color="text.secondary">
                          No events to display
                        </Typography>
                      </Box>
                    </Box>
                  ) : (
                    paginatedEvents.map((event: EventWithFormattedDate) => (
                      <Box
                        component="tr"
                        key={event.event_id}
                        onClick={() => setSelectedEvent(event)}
                        sx={{
                          borderBottom: "1px solid",
                          borderColor: "divider",
                          cursor: "pointer",
                          "&:hover": {
                            bgcolor: "background.paper",
                          },
                          transition: "background-color 0.2s",
                        }}
                      >
                        <Box component="td" sx={{ px: 3, py: 2 }}>
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            sx={{ fontFamily: "monospace" }}
                          >
                            {event.formattedDate}
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ px: 3, py: 2 }}>
                          <Typography variant="body2" fontWeight={500}>
                            {event.type}
                          </Typography>
                        </Box>
                        <Box
                          component="td"
                          sx={{ px: 3, py: 2, maxWidth: 400 }}
                        >
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            noWrap
                          >
                            {event.description}
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ px: 3, py: 2 }}>
                          <Chip
                            label={event.severity}
                            size="small"
                            sx={{
                              ...getSeverityColor(event.severity),
                              fontWeight: 700,
                              textTransform: "uppercase",
                              fontSize: "0.7rem",
                            }}
                          />
                        </Box>
                        <Box
                          component="td"
                          sx={{ px: 3, py: 2, textAlign: "right" }}
                        >
                          <ArrowForward
                            sx={{ fontSize: 20, color: "text.secondary" }}
                          />
                        </Box>
                      </Box>
                    ))
                  )}
                </Box>
              </Box>
            </Box>
            {totalEventsPages > 1 && (
              <Box
                sx={{
                  px: 3,
                  py: 2,
                  borderTop: "1px solid",
                  borderColor: "divider",
                  display: "flex",
                  justifyContent: "center",
                }}
              >
                <Pagination
                  count={totalEventsPages}
                  page={eventsPage}
                  onChange={(_, page) => setEventsPage(page)}
                  color="primary"
                  size="small"
                />
              </Box>
            )}
          </Paper>

          {/* Health Status */}
          {realmDashboard.health && (
            <Paper
              elevation={0}
              sx={{ p: 3, border: "1px solid", borderColor: "divider" }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 3,
                }}
              >
                Server Health
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
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Status
                  </Typography>
                  <Box sx={{ mt: 1 }}>
                    <Chip
                      label={realmDashboard.health.status}
                      size="small"
                      sx={{
                        bgcolor:
                          realmDashboard.health.status === "UP"
                            ? alpha(theme.palette.success.main, 0.2)
                            : alpha(theme.palette.error.main, 0.2),
                        color:
                          realmDashboard.health.status === "UP"
                            ? theme.palette.success.light
                            : theme.palette.error.light,
                        fontWeight: 700,
                        textTransform: "uppercase",
                        fontSize: "0.7rem",
                      }}
                    />
                  </Box>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Response Time
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    {realmDashboard.health.response_time_ms}ms
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Memory Used
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    {formatBytes(realmDashboard.health.memory_used_bytes)}
                  </Typography>
                </Box>
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
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
            </Paper>
          )}
        </Box>
      </Box>

      {/* Event Details Modal */}
      <Dialog
        open={!!selectedEvent}
        onClose={() => setSelectedEvent(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            Event Details
          </Typography>
          <IconButton onClick={() => setSelectedEvent(null)} size="small">
            <Close />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ mt: 2 }}>
          {selectedEvent && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Event ID
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ fontFamily: "monospace", mt: 0.5 }}
                >
                  {selectedEvent.event_id}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Timestamp
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {new Date(selectedEvent.timestamp).toLocaleString()}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Type
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {selectedEvent.type}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Description
                </Typography>
                <Typography variant="body2" sx={{ mt: 0.5 }}>
                  {selectedEvent.description}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Severity
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={selectedEvent.severity}
                    size="small"
                    sx={{
                      ...getSeverityColor(selectedEvent.severity),
                      fontWeight: 700,
                      textTransform: "uppercase",
                      fontSize: "0.7rem",
                    }}
                  />
                </Box>
              </Box>
            </Box>
          )}
        </DialogContent>
      </Dialog>

      {/* User Details Modal */}
      <Dialog
        open={!!selectedUser}
        onClose={() => setSelectedUser(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            User Details
          </Typography>
          <IconButton onClick={() => setSelectedUser(null)} size="small">
            <Close />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ mt: 2 }}>
          {selectedUser && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Username
                </Typography>
                <Typography variant="body2" fontWeight={500} sx={{ mt: 0.5 }}>
                  {selectedUser.username}
                </Typography>
              </Box>
              {selectedUser.email && (
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Email
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedUser.email}
                  </Typography>
                </Box>
              )}
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Status
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={selectedUser.enabled ? "Enabled" : "Disabled"}
                    size="small"
                    sx={{
                      bgcolor: selectedUser.enabled
                        ? alpha(theme.palette.success.main, 0.2)
                        : alpha(theme.palette.error.main, 0.2),
                      color: selectedUser.enabled
                        ? theme.palette.success.light
                        : theme.palette.error.light,
                      fontWeight: 700,
                      textTransform: "uppercase",
                      fontSize: "0.7rem",
                    }}
                  />
                </Box>
              </Box>
            </Box>
          )}
        </DialogContent>
      </Dialog>

      {/* Client Details Modal */}
      <Dialog
        open={!!selectedClient}
        onClose={() => setSelectedClient(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            Client Details
          </Typography>
          <IconButton onClick={() => setSelectedClient(null)} size="small">
            <Close />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ mt: 2 }}>
          {selectedClient && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Client ID
                </Typography>
                <Typography variant="body2" fontWeight={500} sx={{ mt: 0.5 }}>
                  {selectedClient.clientId}
                </Typography>
              </Box>
              {selectedClient.name && (
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ textTransform: "uppercase" }}
                  >
                    Display Name
                  </Typography>
                  <Typography variant="body2" sx={{ mt: 0.5 }}>
                    {selectedClient.name}
                  </Typography>
                </Box>
              )}
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Status
                </Typography>
                <Box sx={{ mt: 0.5 }}>
                  <Chip
                    label={selectedClient.enabled ? "Enabled" : "Disabled"}
                    size="small"
                    sx={{
                      bgcolor: selectedClient.enabled
                        ? alpha(theme.palette.success.main, 0.2)
                        : alpha(theme.palette.error.main, 0.2),
                      color: selectedClient.enabled
                        ? theme.palette.success.light
                        : theme.palette.error.light,
                      fontWeight: 700,
                      textTransform: "uppercase",
                      fontSize: "0.7rem",
                    }}
                  />
                </Box>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ textTransform: "uppercase" }}
                >
                  Client Type
                </Typography>
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 1 }}>
                  {selectedClient.publicClient && (
                    <Chip
                      label="Public"
                      size="small"
                      sx={{
                        bgcolor: alpha(theme.palette.info.main, 0.2),
                        color: theme.palette.info.light,
                        fontWeight: 700,
                        textTransform: "uppercase",
                        fontSize: "0.7rem",
                      }}
                    />
                  )}
                  {selectedClient.bearerOnly && (
                    <Chip
                      label="Bearer-only"
                      size="small"
                      sx={{
                        bgcolor: alpha(theme.palette.warning.main, 0.2),
                        color: theme.palette.warning.light,
                        fontWeight: 700,
                        textTransform: "uppercase",
                        fontSize: "0.7rem",
                      }}
                    />
                  )}
                  {!selectedClient.publicClient &&
                    !selectedClient.bearerOnly && (
                      <Chip
                        label="Confidential"
                        size="small"
                        sx={{
                          bgcolor: alpha(theme.palette.success.main, 0.2),
                          color: theme.palette.success.light,
                          fontWeight: 700,
                          textTransform: "uppercase",
                          fontSize: "0.7rem",
                        }}
                      />
                    )}
                </Box>
              </Box>
            </Box>
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
