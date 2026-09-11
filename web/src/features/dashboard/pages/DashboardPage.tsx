import { useState, useMemo, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";
import {
  Box,
  Typography,
  Button,
  Card,
  CardContent,
  LinearProgress,
  Dialog,
  DialogContent,
  DialogActions,
  ToggleButtonGroup,
  ToggleButton,
  TextField,
  CircularProgress,
  Chip,
} from "@mui/material";
import {
  Description as DescriptionIcon,
  FiberManualRecord as DotIcon,
  ChevronRight as ChevronRightIcon,
  Dashboard as DashboardIcon,
  Notifications as NotificationsIcon,
  People as PeopleIcon,
  ErrorOutline as ErrorOutlineIcon,
  WarningAmber,
} from "@mui/icons-material";
import {
  useKeycloakDashboard,
  useRealmSelector,
  useEvents,
  useEventStats,
  useAlertStats,
} from "@/shared/hooks";
import { useTenant, useToast } from "@/shared/context";
import {
  RealmSelector,
  TimeSelector,
  EmptyState,
  DialogHeader,
  PageHeader,
  FeatureErrorBoundary,
} from "@/shared/components";
import { useGenerateReport } from "../hooks";
import {
  formatBytes,
  formatUptime,
  isTenantConnectionBroken,
} from "@/shared/utils";
import type { KeycloakEvent } from "@/shared/types";

type EventWithFormattedTime = KeycloakEvent & {
  formattedTime: string;
};

export function DashboardPage() {
  const navigate = useNavigate();
  const { tenantId } = useParams<{ tenantId: string }>();
  const { selectedTenant } = useTenant();
  const { showToast } = useToast();
  const [startTime, setStartTime] = useState<string | undefined>(undefined);
  const [endTime, setEndTime] = useState<string | undefined>(undefined);

  // Report generation state
  const [showReportModal, setShowReportModal] = useState(false);
  const [reportTimeRange, setReportTimeRange] = useState<
    "last_week" | "last_month" | "custom"
  >("last_month");
  const [reportStartDate, setReportStartDate] = useState<string>(() => {
    const date = new Date();
    date.setMonth(date.getMonth() - 1);
    return date.toISOString().split("T")[0];
  });
  const [reportEndDate, setReportEndDate] = useState<string>(() => {
    return new Date().toISOString().split("T")[0];
  });

  const { mutate: generateReport, isPending: isGeneratingReport } =
    useGenerateReport();

  // Memoize helper functions
  const getSeverityColor = useCallback(
    (severity: string): "error" | "warning" | "info" => {
      switch (severity) {
        case "error":
          return "error";
        case "warning":
          return "warning";
        default:
          return "info";
      }
    },
    [],
  );

  const getMemoryBarColor = useCallback(
    (memoryRatio: number): "error" | "warning" | "success" => {
      if (memoryRatio > 0.9) {
        return "error";
      }
      if (memoryRatio > 0.7) {
        return "warning";
      }
      return "success";
    },
    [],
  );

  const handleReportTimeRangeChange = useCallback(
    (range: "last_week" | "last_month" | "custom") => {
      setReportTimeRange(range);
      const endDate = new Date();
      const startDate = new Date();

      if (range === "last_week") {
        startDate.setDate(startDate.getDate() - 7);
      } else if (range === "last_month") {
        startDate.setMonth(startDate.getMonth() - 1);
      }

      if (range !== "custom") {
        setReportStartDate(startDate.toISOString().split("T")[0]);
        setReportEndDate(endDate.toISOString().split("T")[0]);
      }
    },
    [],
  );

  const handleTimeRangeChange = useCallback(
    (start: Date | null, end: Date | null) => {
      setStartTime(start ? start.toISOString() : undefined);
      setEndTime(end ? end.toISOString() : undefined);
    },
    [],
  );

  const handleGenerateReport = useCallback(() => {
    if (!selectedTenant) return;

    generateReport(
      {
        tenantId: selectedTenant.tenant_id,
        tenantName: selectedTenant.name,
        startDate: reportStartDate,
        endDate: reportEndDate,
      },
      {
        onSuccess: () => {
          setShowReportModal(false);
          showToast({
            message: "Report generated successfully",
            type: "success",
          });
        },
        onError: (err) => {
          console.error("Error generating report:", err);
          showToast({ message: "Failed to generate report", type: "error" });
        },
      },
    );
  }, [
    selectedTenant,
    generateReport,
    reportStartDate,
    reportEndDate,
    showToast,
  ]);

  const { data: keycloakDashboard, dataUpdatedAt: keycloakUpdatedAt } =
    useKeycloakDashboard({
      tenantId: selectedTenant?.tenant_id,
    });

  const realms = useMemo(
    () => keycloakDashboard?.realms ?? [],
    [keycloakDashboard?.realms],
  );

  const realmsConnectionBroken = isTenantConnectionBroken(selectedTenant);

  const { selectedRealm, handleRealmChange } = useRealmSelector({
    tenantId,
    defaultRealm: selectedTenant?.default_realm,
    availableRealms: realms,
  });

  const { data: eventsResponse } = useEvents({
    tenantId: selectedTenant?.tenant_id,
    limit: 10,
    startTime,
    endTime,
    // Matches the stats call below, which already filters by realm. Encoding
    // the realm as a Keycloak source here dropped the realm's AMFA events, so
    // the counts and the list disagreed on the same page.
    realm: selectedRealm !== "all" ? selectedRealm : undefined,
  });

  const { data: stats, dataUpdatedAt: statsUpdatedAt } = useEventStats({
    tenantId: selectedTenant?.tenant_id,
    startTime,
    endTime,
    realm: selectedRealm !== "all" ? selectedRealm : undefined,
  });

  const { data: alertStats } = useAlertStats(selectedTenant?.tenant_id);

  // Pre-format events with timestamps to avoid repeated formatting in render
  const events = useMemo<EventWithFormattedTime[]>(() => {
    const eventsList = eventsResponse?.events || [];
    return eventsList.map((event) => ({
      ...event,
      formattedTime: new Date(event.timestamp).toLocaleTimeString(),
    }));
  }, [eventsResponse?.events]);

  // Memoize formatted last update time
  const lastUpdateTime = useMemo(() => {
    return new Date(
      Math.max(statsUpdatedAt || 0, keycloakUpdatedAt || 0),
    ).toLocaleTimeString();
  }, [statsUpdatedAt, keycloakUpdatedAt]);

  // Memoize realm filtering
  const filteredRealms = useMemo(() => {
    return selectedRealm === "all"
      ? realms
      : realms.filter((r) => r.realm_name === selectedRealm);
  }, [realms, selectedRealm]);

  // Memoize aggregate metrics from filtered realms
  const metrics = useMemo(() => {
    return {
      totalUsers: filteredRealms.reduce(
        (sum, r) => sum + (r.metrics?.total_users || 0),
        0,
      ),
      totalSessions: filteredRealms.reduce(
        (sum, r) => sum + (r.metrics?.active_sessions || 0),
        0,
      ),
      totalLogins: filteredRealms.reduce(
        (sum, r) => sum + (r.metrics?.login_events || 0),
        0,
      ),
      totalFailedLogins: filteredRealms.reduce(
        (sum, r) => sum + (r.metrics?.failed_login_events || 0),
        0,
      ),
      healthyRealms: filteredRealms.filter((r) => r.enabled && r.is_healthy)
        .length,
    };
  }, [filteredRealms]);

  // Memoize memory calculations
  const memoryMetrics = useMemo(() => {
    if (!keycloakDashboard?.health) return null;

    const { memory_used_bytes, memory_max_bytes } = keycloakDashboard.health;
    const memoryRatio = memory_used_bytes / memory_max_bytes;
    const memoryPercentage = (memoryRatio * 100).toFixed(1);

    return {
      ratio: memoryRatio,
      percentage: memoryPercentage,
      used: formatBytes(memory_used_bytes),
      max: formatBytes(memory_max_bytes),
      color: getMemoryBarColor(memoryRatio),
    };
  }, [keycloakDashboard?.health, getMemoryBarColor]);

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      {/* Header with Auto-Refresh Indicator */}
      <PageHeader
        title="Dashboard"
        subtitle="Real-time monitoring overview"
        showBreadcrumb={false}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Box
              sx={{
                width: 8,
                height: 8,
                borderRadius: "50%",
                bgcolor: "success.main",
                animation: "pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite",
                "@keyframes pulse": {
                  "0%, 100%": { opacity: 1 },
                  "50%": { opacity: 0.5 },
                },
              }}
            />
            <Typography variant="caption" color="text.secondary">
              Live
            </Typography>
          </Box>
          <Typography variant="caption" color="text.secondary">
            •
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Updated {lastUpdateTime}
          </Typography>
        </Box>
        <RealmSelector
          selectedRealm={selectedRealm}
          onRealmChange={handleRealmChange}
          realms={realms}
          defaultRealm={selectedTenant?.default_realm}
        />
        <TimeSelector onTimeRangeChange={handleTimeRangeChange} />
        <Button
          onClick={() => setShowReportModal(true)}
          variant="contained"
          startIcon={<DescriptionIcon />}
          sx={{ textTransform: "uppercase" }}
        >
          Generate Report
        </Button>
      </PageHeader>

      {/* Key Metrics Grid */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            sm: "repeat(2, 1fr)",
            lg: "repeat(5, 1fr)",
          },
          gap: 3,
          mb: 4,
        }}
      >
        {/* Total Events */}
        <Card
          sx={{
            cursor: "pointer",
            transition: "all 0.2s",
            "&:hover": {
              borderColor: "primary.main",
              transform: "translateY(-2px)",
              boxShadow: 2,
            },
          }}
          onClick={() => navigate(`/${tenantId}/events`)}
        >
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 2,
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "rgba(96, 165, 250, 0.1)",
                  borderRadius: 2,
                }}
              >
                <DescriptionIcon sx={{ color: "info.main", fontSize: 32 }} />
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="subtitle2" color="text.secondary">
                  Total Events
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5, fontWeight: 700 }}>
                  {stats?.total_events?.toLocaleString() || 0}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DotIcon sx={{ fontSize: 12, color: "success.main" }} />
              <Typography variant="caption" color="text.secondary">
                View all events →
              </Typography>
            </Box>
          </CardContent>
        </Card>

        {/* Realms */}
        <Card
          sx={{
            cursor: "pointer",
            transition: "all 0.2s",
            "&:hover": {
              borderColor: "primary.main",
              transform: "translateY(-2px)",
              boxShadow: 2,
            },
          }}
          onClick={() => navigate(`/${tenantId}/realms`)}
        >
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 2,
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "rgba(219, 40, 51, 0.1)",
                  borderRadius: 2,
                }}
              >
                <DashboardIcon sx={{ color: "primary.main", fontSize: 32 }} />
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="subtitle2" color="text.secondary">
                  Realms
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5, fontWeight: 700 }}>
                  {realmsConnectionBroken ? (
                    <WarningAmber color="error" sx={{ fontSize: 32 }} />
                  ) : (
                    realms.length
                  )}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DotIcon
                sx={{
                  fontSize: 12,
                  color: realmsConnectionBroken
                    ? "error.main"
                    : metrics.healthyRealms === realms.length
                      ? "success.main"
                      : "warning.main",
                }}
              />
              <Typography variant="caption" color="text.secondary">
                {realmsConnectionBroken
                  ? "Connection error • View realms →"
                  : `${metrics.healthyRealms} healthy • View realms →`}
              </Typography>
            </Box>
          </CardContent>
        </Card>

        {/* Configuration Alerts */}
        <Card
          sx={{
            cursor: "pointer",
            transition: "all 0.2s",
            "&:hover": {
              borderColor: "primary.main",
              transform: "translateY(-2px)",
              boxShadow: 2,
            },
          }}
          onClick={() => navigate(`/${tenantId}/alerts`)}
        >
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 2,
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "rgba(245, 158, 11, 0.1)",
                  borderRadius: 2,
                }}
              >
                <NotificationsIcon
                  sx={{ color: "warning.main", fontSize: 32 }}
                />
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="subtitle2" color="text.secondary">
                  Active Alerts
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5, fontWeight: 700 }}>
                  {alertStats?.total_active?.toLocaleString() || 0}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DotIcon
                sx={{
                  fontSize: 12,
                  color: (() => {
                    if ((alertStats?.by_severity?.critical || 0) > 0)
                      return "error.main";
                    if ((alertStats?.total_active || 0) > 0)
                      return "warning.main";
                    return "success.main";
                  })(),
                }}
              />
              <Typography variant="caption" color="text.secondary">
                {(alertStats?.by_severity?.critical || 0) > 0
                  ? `${alertStats?.by_severity?.critical} critical`
                  : "View all alerts →"}
              </Typography>
            </Box>
          </CardContent>
        </Card>

        {/* Active Users */}
        <Card
          sx={{
            transition: "all 0.2s",
            "&:hover": {
              transform: "translateY(-2px)",
              boxShadow: 2,
            },
          }}
        >
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 2,
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "rgba(96, 165, 250, 0.1)",
                  borderRadius: 2,
                }}
              >
                <PeopleIcon sx={{ color: "info.main", fontSize: 32 }} />
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="subtitle2" color="text.secondary">
                  Total Users
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5, fontWeight: 700 }}>
                  {metrics.totalUsers.toLocaleString()}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DotIcon sx={{ fontSize: 12, color: "info.main" }} />
              <Typography variant="caption" color="text.secondary">
                {metrics.totalSessions} active sessions
              </Typography>
            </Box>
          </CardContent>
        </Card>

        {/* Failed Logins */}
        <Card
          sx={{
            transition: "all 0.2s",
            "&:hover": {
              transform: "translateY(-2px)",
              boxShadow: 2,
            },
          }}
        >
          <CardContent sx={{ p: 3 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: 2,
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "rgba(219, 40, 51, 0.1)",
                  borderRadius: 2,
                }}
              >
                <ErrorOutlineIcon sx={{ color: "error.main", fontSize: 32 }} />
              </Box>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="subtitle2" color="text.secondary">
                  Failed Logins
                </Typography>
                <Typography variant="h3" sx={{ mt: 0.5, fontWeight: 700 }}>
                  {metrics.totalFailedLogins.toLocaleString()}
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DotIcon
                sx={{
                  fontSize: 12,
                  color:
                    metrics.totalFailedLogins > 0
                      ? "error.main"
                      : "success.main",
                }}
              />
              <Typography variant="caption" color="text.secondary">
                {metrics.totalLogins} successful
              </Typography>
            </Box>
          </CardContent>
        </Card>
      </Box>

      {/* System Health */}
      {keycloakDashboard &&
        "health" in keycloakDashboard &&
        keycloakDashboard.health && (
          <FeatureErrorBoundary
            featureId="SystemHealth"
            title="System Health"
            resetKeys={[selectedTenant?.tenant_id]}
          >
            <Card sx={{ mb: 4 }}>
              <CardContent sx={{ p: 3 }}>
                <Typography
                  variant="h6"
                  sx={{ fontWeight: 600, textTransform: "uppercase", mb: 3 }}
                >
                  System Health
                </Typography>
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" },
                    gap: 4,
                  }}
                >
                  <Box>
                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        gap: 1.5,
                        mb: 1,
                      }}
                    >
                      <Box
                        sx={{
                          width: 12,
                          height: 12,
                          borderRadius: "50%",
                          bgcolor:
                            keycloakDashboard.health.status === "UP"
                              ? "success.main"
                              : "error.main",
                        }}
                      />
                      <Typography variant="caption" color="text.secondary">
                        Status
                      </Typography>
                    </Box>
                    <Typography
                      variant="h5"
                      sx={{
                        fontWeight: 700,
                        color:
                          keycloakDashboard.health.status === "UP"
                            ? "success.main"
                            : "error.main",
                      }}
                    >
                      {keycloakDashboard.health.status}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ mt: 0.5 }}
                    >
                      {keycloakDashboard.health.response_time_ms}ms response
                    </Typography>
                  </Box>
                  {memoryMetrics && (
                    <Box>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ mb: 1 }}
                      >
                        Memory
                      </Typography>
                      <Typography variant="h5" sx={{ fontWeight: 700 }}>
                        {memoryMetrics.percentage}%
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ mt: 0.5 }}
                      >
                        {memoryMetrics.used} / {memoryMetrics.max}
                      </Typography>
                      <LinearProgress
                        variant="determinate"
                        value={memoryMetrics.ratio * 100}
                        color={memoryMetrics.color}
                        sx={{ mt: 1, height: 8, borderRadius: 4 }}
                      />
                    </Box>
                  )}
                  <Box>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ mb: 1 }}
                    >
                      Uptime
                    </Typography>
                    <Typography variant="h5" sx={{ fontWeight: 700 }}>
                      {formatUptime(keycloakDashboard.health.uptime_millis)}
                    </Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ mt: 0.5 }}
                    >
                      {keycloakDashboard.health.server_version ||
                        "Unknown version"}
                    </Typography>
                  </Box>
                </Box>
              </CardContent>
            </Card>
          </FeatureErrorBoundary>
        )}

      {/* Quick Actions & Recent Activity */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", lg: "repeat(2, 1fr)" },
          gap: 3,
          mb: 4,
        }}
      >
        {/* Quick Actions */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{ fontWeight: 600, textTransform: "uppercase", mb: 3 }}
            >
              Quick Actions
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
              <Button
                onClick={() => navigate(`/${tenantId}/events`)}
                variant="outlined"
                endIcon={<ChevronRightIcon />}
                sx={{
                  justifyContent: "space-between",
                  textTransform: "uppercase",
                }}
              >
                View All Events
              </Button>
              <Button
                onClick={() => navigate(`/${tenantId}/realms`)}
                variant="outlined"
                endIcon={<ChevronRightIcon />}
                sx={{
                  justifyContent: "space-between",
                  textTransform: "uppercase",
                }}
              >
                Manage Realms
              </Button>
              <Button
                onClick={() => navigate(`/${tenantId}/health`)}
                variant="outlined"
                endIcon={<ChevronRightIcon />}
                sx={{
                  justifyContent: "space-between",
                  textTransform: "uppercase",
                }}
              >
                System Health
              </Button>
            </Box>
          </CardContent>
        </Card>

        {/* Recent Events */}
        <FeatureErrorBoundary
          featureId="RecentEvents"
          title="Recent Activity"
          resetKeys={[selectedTenant?.tenant_id]}
        >
          <Card>
            <CardContent sx={{ p: 3 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  mb: 3,
                }}
              >
                <Typography
                  variant="h6"
                  sx={{ fontWeight: 600, textTransform: "uppercase" }}
                >
                  Recent Activity
                </Typography>
                <Button
                  onClick={() => navigate(`/${tenantId}/events`)}
                  size="small"
                  sx={{ textTransform: "uppercase" }}
                >
                  View All →
                </Button>
              </Box>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {events.length === 0 ? (
                  <EmptyState message="No recent events" minHeight={128} />
                ) : (
                  events.slice(0, 3).map((event) => (
                    <Card
                      key={event.event_id}
                      variant="outlined"
                      sx={{
                        cursor: "pointer",
                        transition: "all 0.2s",
                        "&:hover": {
                          bgcolor: "action.hover",
                        },
                      }}
                      onClick={() =>
                        navigate(`/${tenantId}/events?event=${event.event_id}`)
                      }
                    >
                      <CardContent
                        sx={{
                          display: "flex",
                          gap: 1.5,
                          p: 2,
                          "&:last-child": { pb: 2 },
                        }}
                      >
                        <Box
                          sx={{
                            width: 8,
                            height: 8,
                            borderRadius: "50%",
                            bgcolor: `${getSeverityColor(event.severity)}.main`,
                            mt: 0.75,
                            flexShrink: 0,
                          }}
                        />
                        <Box sx={{ flex: 1, minWidth: 0 }}>
                          <Typography
                            variant="body2"
                            sx={{ fontWeight: 500 }}
                            noWrap
                          >
                            {event.type}
                          </Typography>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            noWrap
                            sx={{ display: "block", mt: 0.5 }}
                          >
                            {event.description}
                          </Typography>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{ mt: 0.5 }}
                          >
                            {event.formattedTime} • {event.source}
                          </Typography>
                        </Box>
                      </CardContent>
                    </Card>
                  ))
                )}
              </Box>
            </CardContent>
          </Card>
        </FeatureErrorBoundary>
      </Box>

      {/* Top Realms */}
      {realms.length > 0 && (
        <FeatureErrorBoundary
          featureId="TopRealms"
          title="Top Realms"
          resetKeys={[selectedTenant?.tenant_id]}
        >
          <Card sx={{ mb: 4 }}>
            <CardContent sx={{ p: 3 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  mb: 3,
                }}
              >
                <Typography
                  variant="h6"
                  sx={{ fontWeight: 600, textTransform: "uppercase" }}
                >
                  Top Realms
                </Typography>
                <Button
                  onClick={() => navigate(`/${tenantId}/realms`)}
                  size="small"
                  sx={{ textTransform: "uppercase" }}
                >
                  View All →
                </Button>
              </Box>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {realms.slice(0, 5).map((realm) => {
                  const hasWarning =
                    realm.enabled &&
                    (!realm.events_enabled ||
                      !realm.events_listeners ||
                      realm.events_listeners.length === 0);
                  return (
                    <Card
                      key={realm.realm_name}
                      variant="outlined"
                      sx={{
                        cursor: "pointer",
                        transition: "all 0.2s",
                        "&:hover": {
                          bgcolor: "action.hover",
                        },
                      }}
                      onClick={() =>
                        navigate(`/${tenantId}/realm/${realm.realm_name}`)
                      }
                    >
                      <CardContent
                        sx={{
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                          p: 2,
                          "&:last-child": { pb: 2 },
                        }}
                      >
                        <Box
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            gap: 1.5,
                          }}
                        >
                          <Box
                            sx={{
                              width: 8,
                              height: 8,
                              borderRadius: "50%",
                              bgcolor:
                                realm.enabled && realm.is_healthy
                                  ? "success.main"
                                  : "error.main",
                            }}
                          />
                          <Box>
                            <Box
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                gap: 1,
                              }}
                            >
                              <Typography
                                variant="body2"
                                sx={{ fontWeight: 500 }}
                              >
                                {realm.realm_name}
                              </Typography>
                              {hasWarning && (
                                <Chip
                                  label="Warning"
                                  size="small"
                                  color="warning"
                                  sx={{
                                    height: 20,
                                    fontSize: "0.65rem",
                                    fontWeight: 600,
                                  }}
                                />
                              )}
                            </Box>
                            {realm.metrics && (
                              <Typography
                                variant="caption"
                                color="text.secondary"
                              >
                                {realm.metrics.total_users} users •{" "}
                                {realm.metrics.active_sessions} sessions
                              </Typography>
                            )}
                          </Box>
                        </Box>
                        {realm.metrics && (
                          <Box sx={{ textAlign: "right" }}>
                            <Typography
                              variant="body2"
                              sx={{ fontWeight: 600, color: "success.main" }}
                            >
                              {realm.metrics.login_events}
                            </Typography>
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              logins
                            </Typography>
                          </Box>
                        )}
                      </CardContent>
                    </Card>
                  );
                })}
              </Box>
            </CardContent>
          </Card>
        </FeatureErrorBoundary>
      )}

      {/* Report Generation Modal */}
      <Dialog
        open={showReportModal}
        onClose={() => setShowReportModal(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogHeader
          title="Generate Report"
          subtitle="Select the date range for the report"
          onClose={() => setShowReportModal(false)}
        />
        <DialogContent>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3, pt: 2 }}>
            <Box>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ textTransform: "uppercase", mb: 1.5, fontWeight: 500 }}
              >
                Time Range
              </Typography>
              <ToggleButtonGroup
                value={reportTimeRange}
                exclusive
                onChange={(_, value) =>
                  value && handleReportTimeRangeChange(value)
                }
                fullWidth
                sx={{ gap: 1 }}
              >
                <ToggleButton
                  value="last_week"
                  sx={{ textTransform: "uppercase", flex: 1 }}
                >
                  Last Week
                </ToggleButton>
                <ToggleButton
                  value="last_month"
                  sx={{ textTransform: "uppercase", flex: 1 }}
                >
                  Last Month
                </ToggleButton>
                <ToggleButton
                  value="custom"
                  sx={{ textTransform: "uppercase", flex: 1 }}
                >
                  Custom
                </ToggleButton>
              </ToggleButtonGroup>
            </Box>
            {reportTimeRange === "custom" && (
              <>
                <TextField
                  label="Start Date"
                  type="date"
                  value={reportStartDate}
                  onChange={(e) => setReportStartDate(e.target.value)}
                  InputLabelProps={{ shrink: true }}
                  inputProps={{ max: reportEndDate }}
                  fullWidth
                />
                <TextField
                  label="End Date"
                  type="date"
                  value={reportEndDate}
                  onChange={(e) => setReportEndDate(e.target.value)}
                  InputLabelProps={{ shrink: true }}
                  inputProps={{
                    min: reportStartDate,
                    max: new Date().toISOString().split("T")[0],
                  }}
                  fullWidth
                />
              </>
            )}
            {reportTimeRange !== "custom" && (
              <Typography variant="body2" color="text.secondary">
                Report period:{" "}
                <Typography component="span" color="text.primary">
                  {reportStartDate}
                </Typography>{" "}
                to{" "}
                <Typography component="span" color="text.primary">
                  {reportEndDate}
                </Typography>
              </Typography>
            )}
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 3 }}>
          <Button
            onClick={() => setShowReportModal(false)}
            disabled={isGeneratingReport}
            variant="outlined"
            sx={{ textTransform: "uppercase" }}
          >
            Cancel
          </Button>
          <Button
            onClick={handleGenerateReport}
            disabled={isGeneratingReport}
            variant="contained"
            sx={{ textTransform: "uppercase" }}
            startIcon={
              isGeneratingReport ? (
                <CircularProgress size={16} color="inherit" />
              ) : null
            }
          >
            {isGeneratingReport ? "Generating..." : "Generate Report"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
