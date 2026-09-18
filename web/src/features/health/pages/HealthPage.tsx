import {
  Box,
  Typography,
  LinearProgress,
  Chip,
  Link,
  Tooltip,
} from "@mui/material";
import {
  Warning as WarningIcon,
  OpenInNew as OpenInNewIcon,
  SentimentDissatisfied as SadIcon,
  Error as ErrorIcon,
} from "@mui/icons-material";
import { useKeycloakDashboard } from "@/shared/hooks";
import { useTenant } from "@/shared/context";
import { HighAvailability } from "../components/HighAvailability";
import { InfinispanMetrics } from "../components/InfinispanMetrics";
import { formatBytes } from "@/shared/utils";
import {
  LoadingSkeleton,
  EmptyState,
  SectionCard,
  PageHeader,
  FeatureErrorBoundary,
} from "@/shared/components";

export function HealthPage() {
  const { selectedTenant } = useTenant();

  const { data: keycloakDashboard, isLoading } = useKeycloakDashboard({
    tenantId: selectedTenant?.tenant_id,
    refetchInterval: 5000,
  });

  const health =
    keycloakDashboard && "health" in keycloakDashboard
      ? keycloakDashboard.health
      : null;
  const versionInfo =
    keycloakDashboard && "version_info" in keycloakDashboard
      ? keycloakDashboard.version_info
      : null;
  const latestVersion = versionInfo ? versionInfo.latest_keycloak_version : "";
  const releaseUrl = versionInfo ? versionInfo.keycloak_release_url : "";
  const releaseLabel = "View Keycloak Release Notes";
  const isOutdated = versionInfo ? versionInfo.is_keycloak_outdated : false;
  const latestVersionDisplay = latestVersion?.trim()
    ? latestVersion
    : "Unknown";
  const highlightSecurity = versionInfo?.has_security_updates ?? false;
  const updateTitle = highlightSecurity
    ? "Security Update Required"
    : "Update Available";

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <PageHeader
        title="System Health"
        subtitle="Monitor system health and performance metrics"
      />

      {isLoading ? (
        <LoadingSkeleton variant="card" count={3} />
      ) : health ? (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "1fr",
              md: "repeat(2, 1fr)",
              lg: "repeat(3, 1fr)",
            },
            gap: 3,
          }}
        >
          {/* Overall Status */}
          <SectionCard title="Keycloak Server Status">
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" },
                gap: 3,
              }}
            >
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Status
                </Typography>
                <Chip
                  label={health.status}
                  color={
                    health.status === "UP"
                      ? "success"
                      : health.status === "DEGRADED"
                        ? "warning"
                        : "error"
                  }
                  sx={{ fontWeight: 600 }}
                />
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Response Time
                </Typography>
                <Typography variant="h4">
                  {health.response_time_ms}ms
                </Typography>
              </Box>
              {health.server_version && (
                <Box>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{
                      display: "block",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      mb: 1,
                    }}
                  >
                    Server Version
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Typography variant="h4">
                      {health.server_version}
                    </Typography>
                    {versionInfo && isOutdated && (
                      <Tooltip
                        title={`${updateTitle}: ${latestVersionDisplay} available`}
                      >
                        <WarningIcon
                          sx={{
                            fontSize: 20,
                            color: highlightSecurity
                              ? "error.main"
                              : "warning.main",
                          }}
                        />
                      </Tooltip>
                    )}
                  </Box>
                </Box>
              )}
            </Box>
            {versionInfo && isOutdated && (
              <Box sx={{ mt: 3 }}>
                <Typography variant="body2" color="text.secondary">
                  {updateTitle}: {latestVersionDisplay}
                  {releaseUrl && (
                    <>
                      {" — "}
                      <Link
                        href={releaseUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        sx={{ fontSize: "inherit" }}
                      >
                        {releaseLabel}
                        <OpenInNewIcon
                          sx={{
                            fontSize: 12,
                            ml: 0.5,
                            verticalAlign: "middle",
                          }}
                        />
                      </Link>
                    </>
                  )}
                </Typography>
              </Box>
            )}
          </SectionCard>

          {/* Memory Metrics */}
          <SectionCard title="Memory Usage">
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" },
                gap: 3,
              }}
            >
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Memory Used
                </Typography>
                <Typography variant="h4">
                  {formatBytes(health.memory_used_bytes)}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Memory Max
                </Typography>
                <Typography variant="h4">
                  {formatBytes(health.memory_max_bytes)}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Memory Free
                </Typography>
                <Typography variant="h4">
                  {formatBytes(health.memory_free_bytes)}
                </Typography>
              </Box>
            </Box>

            {/* Memory Usage Bar */}
            <Box sx={{ mt: 3 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  mb: 1,
                }}
              >
                <Typography variant="caption" color="text.secondary">
                  Usage
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {(
                    (health.memory_used_bytes / health.memory_max_bytes) *
                    100
                  ).toFixed(1)}
                  %
                </Typography>
              </Box>
              <LinearProgress
                variant="determinate"
                value={
                  (health.memory_used_bytes / health.memory_max_bytes) * 100
                }
                color={
                  health.memory_used_bytes / health.memory_max_bytes > 0.9
                    ? "error"
                    : health.memory_used_bytes / health.memory_max_bytes > 0.7
                      ? "warning"
                      : "success"
                }
                sx={{ height: 12 }}
              />
            </Box>
          </SectionCard>

          {/* Uptime */}
          <SectionCard title="Uptime">
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: {
                  xs: "repeat(2, 1fr)",
                  sm: "repeat(4, 1fr)",
                },
                gap: 3,
              }}
            >
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Days
                </Typography>
                <Typography variant="h4">
                  {Math.floor(health.uptime_millis / 1000 / 60 / 60 / 24)}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Hours
                </Typography>
                <Typography variant="h4">
                  {Math.floor((health.uptime_millis / 1000 / 60 / 60) % 24)}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Minutes
                </Typography>
                <Typography variant="h4">
                  {Math.floor((health.uptime_millis / 1000 / 60) % 60)}
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    display: "block",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    mb: 1,
                  }}
                >
                  Seconds
                </Typography>
                <Typography variant="h4">
                  {Math.floor((health.uptime_millis / 1000) % 60)}
                </Typography>
              </Box>
            </Box>
          </SectionCard>

          {/* High Availability Section */}
          {selectedTenant && (
            <FeatureErrorBoundary
              featureId="HighAvailability"
              title="High Availability"
              resetKeys={[selectedTenant.tenant_id]}
            >
              <HighAvailability
                tenantId={selectedTenant.tenant_id}
                infinispanEnabled={selectedTenant.infinispan_enabled}
              />
            </FeatureErrorBoundary>
          )}

          {/* Infinispan Cache Metrics (detailed view) */}
          {selectedTenant && selectedTenant.infinispan_enabled && (
            <FeatureErrorBoundary
              featureId="InfinispanMetrics"
              title="Infinispan Cache Metrics"
              resetKeys={[selectedTenant.tenant_id]}
            >
              <InfinispanMetrics tenantId={selectedTenant.tenant_id} />
            </FeatureErrorBoundary>
          )}

          {/* Error Message (if any) */}
          {health.error_message && (
            <SectionCard title="Error">
              <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5 }}>
                <ErrorIcon
                  sx={{
                    fontSize: 24,
                    color: "error.main",
                    flexShrink: 0,
                  }}
                />
                <Typography variant="body2" color="error.main">
                  {health.error_message}
                </Typography>
              </Box>
            </SectionCard>
          )}
        </Box>
      ) : (
        <EmptyState
          icon={<SadIcon sx={{ fontSize: 64 }} />}
          message="No health data available"
        />
      )}
    </Box>
  );
}
