import { Box, Typography, Card, CardContent, Skeleton } from "@mui/material";
import {
  Security as ShieldIcon,
  Warning as AlertTriangleIcon,
  CheckCircle as CheckCircleIcon,
  Dns as ServerIcon,
  ShowChart as ActivityIcon,
  Hub as NetworkIcon,
  Schedule as ClockIcon,
} from "@mui/icons-material";
import { useInfinispanMetrics } from "../hooks";
import { formatNumber } from "@/shared/utils";

interface HighAvailabilityProps {
  tenantId: string;
  infinispanEnabled?: boolean;
}

export function HighAvailability({
  tenantId,
  infinispanEnabled = false,
}: HighAvailabilityProps) {
  const {
    data: metrics,
    isLoading,
    error,
  } = useInfinispanMetrics(tenantId, { enabled: infinispanEnabled });

  // If tenant doesn't have InfiniSpan configured, show informational message
  if (!infinispanEnabled) {
    return (
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
            <ShieldIcon sx={{ fontSize: 20, color: "info.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              High Availability
            </Typography>
          </Box>

          {/* Status Card */}
          <Card variant="outlined" sx={{ mb: 2 }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5 }}>
                <ServerIcon
                  sx={{ fontSize: 20, color: "text.secondary", flexShrink: 0 }}
                />
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 1 }}>
                    High Availability features are not configured for this
                    tenant.
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    This tenant is running in single-node mode without
                    clustering.
                  </Typography>
                </Box>
              </Box>
            </CardContent>
          </Card>

          {/* Benefits Section */}
          <Box
            sx={{
              bgcolor: "rgba(96, 165, 250, 0.05)",
              border: "1px solid rgba(96, 165, 250, 0.3)",
              borderRadius: 1,
              p: 2,
            }}
          >
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5 }}>
              <Box
                sx={{
                  bgcolor: "rgba(96, 165, 250, 0.1)",
                  borderRadius: "50%",
                  p: 1,
                  flexShrink: 0,
                }}
              >
                <ShieldIcon sx={{ fontSize: 16, color: "info.main" }} />
              </Box>
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                  InfiniSpan Clustering: Recommended for High Availability
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ mb: 1.5, display: "block" }}
                >
                  InfiniSpan provides distributed caching and session
                  replication across multiple Keycloak nodes, offering several
                  critical benefits for production environments:
                </Typography>
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  <Box
                    sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}
                  >
                    <CheckCircleIcon
                      sx={{
                        fontSize: 12,
                        color: "success.main",
                        flexShrink: 0,
                        mt: 0.25,
                      }}
                    />
                    <Typography variant="caption" color="text.secondary">
                      <Typography
                        component="span"
                        variant="caption"
                        sx={{ fontWeight: 500 }}
                      >
                        Zero Downtime:
                      </Typography>{" "}
                      Automatic failover if a node fails
                    </Typography>
                  </Box>
                  <Box
                    sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}
                  >
                    <CheckCircleIcon
                      sx={{
                        fontSize: 12,
                        color: "success.main",
                        flexShrink: 0,
                        mt: 0.25,
                      }}
                    />
                    <Typography variant="caption" color="text.secondary">
                      <Typography
                        component="span"
                        variant="caption"
                        sx={{ fontWeight: 500 }}
                      >
                        Session Continuity:
                      </Typography>{" "}
                      User sessions persist across node failures
                    </Typography>
                  </Box>
                  <Box
                    sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}
                  >
                    <CheckCircleIcon
                      sx={{
                        fontSize: 12,
                        color: "success.main",
                        flexShrink: 0,
                        mt: 0.25,
                      }}
                    />
                    <Typography variant="caption" color="text.secondary">
                      <Typography
                        component="span"
                        variant="caption"
                        sx={{ fontWeight: 500 }}
                      >
                        Load Distribution:
                      </Typography>{" "}
                      Spread authentication load across multiple servers
                    </Typography>
                  </Box>
                  <Box
                    sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}
                  >
                    <CheckCircleIcon
                      sx={{
                        fontSize: 12,
                        color: "success.main",
                        flexShrink: 0,
                        mt: 0.25,
                      }}
                    />
                    <Typography variant="caption" color="text.secondary">
                      <Typography
                        component="span"
                        variant="caption"
                        sx={{ fontWeight: 500 }}
                      >
                        Data Redundancy:
                      </Typography>{" "}
                      Cached data replicated for resilience
                    </Typography>
                  </Box>
                </Box>
                <Box
                  sx={{
                    mt: 1.5,
                    pt: 1.5,
                    borderTop: "1px solid",
                    borderColor: "divider",
                  }}
                >
                  <Typography variant="caption" sx={{ color: "info.main" }}>
                    Consider enabling InfiniSpan clustering for mission-critical
                    authentication services that require high availability and
                    cannot tolerate downtime.
                  </Typography>
                </Box>
              </Box>
            </Box>
          </Box>
        </CardContent>
      </Card>
    );
  }

  // At this point, infinispanEnabled is true
  if (isLoading) {
    return (
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
            <ShieldIcon sx={{ fontSize: 20, color: "primary.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              High Availability
            </Typography>
          </Box>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            <Skeleton
              variant="rectangular"
              height={96}
              sx={{ borderRadius: 1 }}
            />
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(3, 1fr)",
                gap: 2,
              }}
            >
              <Skeleton
                variant="rectangular"
                height={80}
                sx={{ borderRadius: 1 }}
              />
              <Skeleton
                variant="rectangular"
                height={80}
                sx={{ borderRadius: 1 }}
              />
              <Skeleton
                variant="rectangular"
                height={80}
                sx={{ borderRadius: 1 }}
              />
            </Box>
          </Box>
        </CardContent>
      </Card>
    );
  }

  // If we can't fetch metrics (InfiniSpan is enabled at this point), show error
  if (error || !metrics) {
    return (
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
            <ShieldIcon sx={{ fontSize: 20, color: "primary.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              High Availability
            </Typography>
          </Box>

          {/* Error Status */}
          <Box
            sx={{
              bgcolor: "rgba(219, 40, 51, 0.1)",
              border: "1px solid",
              borderColor: "primary.main",
              borderRadius: 1,
              p: 2,
              mb: 2,
            }}
          >
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5 }}>
              <AlertTriangleIcon
                sx={{ fontSize: 20, color: "primary.main", flexShrink: 0 }}
              />
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                  InfiniSpan Metrics Unavailable
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  InfiniSpan is configured for this tenant, but metrics cannot
                  be retrieved.
                </Typography>
              </Box>
            </Box>
          </Box>

          {/* Troubleshooting Section */}
          <Card variant="outlined">
            <CardContent sx={{ p: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 500, mb: 1.5 }}>
                Possible Issues:
              </Typography>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                  <Box
                    sx={{
                      width: 4,
                      height: 4,
                      borderRadius: "50%",
                      bgcolor: "text.secondary",
                      mt: 1,
                      flexShrink: 0,
                    }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    <Typography
                      component="span"
                      variant="caption"
                      sx={{ fontWeight: 500 }}
                    >
                      Metrics Endpoint Disabled:
                    </Typography>{" "}
                    The Keycloak /metrics endpoint may not be enabled. Ensure{" "}
                    <Typography
                      component="code"
                      variant="caption"
                      sx={{ color: "warning.main", fontFamily: "monospace" }}
                    >
                      --metrics-enabled=true
                    </Typography>{" "}
                    is set in Keycloak configuration.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                  <Box
                    sx={{
                      width: 4,
                      height: 4,
                      borderRadius: "50%",
                      bgcolor: "text.secondary",
                      mt: 1,
                      flexShrink: 0,
                    }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    <Typography
                      component="span"
                      variant="caption"
                      sx={{ fontWeight: 500 }}
                    >
                      InfiniSpan Not Running:
                    </Typography>{" "}
                    The InfiniSpan clustering service may not be active or
                    properly configured.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                  <Box
                    sx={{
                      width: 4,
                      height: 4,
                      borderRadius: "50%",
                      bgcolor: "text.secondary",
                      mt: 1,
                      flexShrink: 0,
                    }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    <Typography
                      component="span"
                      variant="caption"
                      sx={{ fontWeight: 500 }}
                    >
                      Network/Permission Issues:
                    </Typography>{" "}
                    The monitoring service may not have access to the metrics
                    endpoint.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                  <Box
                    sx={{
                      width: 4,
                      height: 4,
                      borderRadius: "50%",
                      bgcolor: "text.secondary",
                      mt: 1,
                      flexShrink: 0,
                    }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    <Typography
                      component="span"
                      variant="caption"
                      sx={{ fontWeight: 500 }}
                    >
                      Single Node Deployment:
                    </Typography>{" "}
                    If running a single Keycloak node, InfiniSpan may be in
                    local mode without clustering metrics.
                  </Typography>
                </Box>
              </Box>
              <Box
                sx={{
                  mt: 2,
                  pt: 1.5,
                  borderTop: "1px solid",
                  borderColor: "divider",
                }}
              >
                <Typography variant="caption" color="text.disabled">
                  Check Keycloak logs and ensure the instance is configured with
                  InfiniSpan clustering enabled.
                </Typography>
              </Box>
            </CardContent>
          </Card>
        </CardContent>
      </Card>
    );
  }

  // TypeScript safety check - metrics should exist at this point
  if (!metrics) {
    return null;
  }

  const getStatusColor = (isHealthy: boolean) => {
    return isHealthy ? "success.main" : "primary.main";
  };

  const getStatusBgColor = (isHealthy: boolean) => {
    return isHealthy ? "rgba(16, 185, 129, 0.1)" : "rgba(219, 40, 51, 0.1)";
  };

  const StatusIconComponent = metrics.is_healthy
    ? CheckCircleIcon
    : AlertTriangleIcon;
  const hasReplicationIssues = metrics.replication_failures > 0;
  const isClusterSizeAdequate =
    metrics.cluster_size >= metrics.required_minimum_nodes;

  return (
    <Card>
      <CardContent sx={{ p: 3 }}>
        {/* Header with Status */}
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            mb: 3,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <ShieldIcon sx={{ fontSize: 20, color: "primary.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              High Availability
            </Typography>
          </Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              px: 1.5,
              py: 0.5,
              borderRadius: 1,
              bgcolor: getStatusBgColor(metrics.is_healthy),
            }}
          >
            <StatusIconComponent
              sx={{
                fontSize: 20,
                color: getStatusColor(metrics.is_healthy),
              }}
            />
            <Typography
              variant="body2"
              sx={{
                fontWeight: 500,
                color: getStatusColor(metrics.is_healthy),
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              {metrics.is_healthy ? "Healthy" : "Degraded"}
            </Typography>
          </Box>
        </Box>

        {/* Cluster Overview */}
        <Card variant="outlined" sx={{ mb: 2 }}>
          <CardContent sx={{ p: 2 }}>
            <Box
              sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}
            >
              <NetworkIcon sx={{ fontSize: 16, color: "info.main" }} />
              <Typography
                variant="body2"
                sx={{
                  fontWeight: 600,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Cluster Overview
              </Typography>
            </Box>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: {
                  xs: "1fr",
                  md: "repeat(3, 1fr)",
                },
                gap: 2,
              }}
            >
              {/* Cluster Size */}
              <Box>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    mb: 0.5,
                  }}
                >
                  <Typography variant="caption" color="text.secondary">
                    Cluster Nodes
                  </Typography>
                  {!isClusterSizeAdequate && (
                    <AlertTriangleIcon
                      sx={{ fontSize: 12, color: "primary.main" }}
                    />
                  )}
                </Box>
                <Typography variant="h4">
                  {formatNumber(metrics.cluster_size || 1)}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.disabled"
                  sx={{ mt: 0.5, display: "block" }}
                >
                  Min required: {formatNumber(metrics.required_minimum_nodes)}
                </Typography>
                {!isClusterSizeAdequate && (
                  <Typography
                    variant="caption"
                    color="primary.main"
                    sx={{ mt: 0.5, display: "block" }}
                  >
                    Below minimum threshold
                  </Typography>
                )}
              </Box>

              {/* Replication Status */}
              <Box>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    mb: 0.5,
                  }}
                >
                  <Typography variant="caption" color="text.secondary">
                    Replication
                  </Typography>
                  {hasReplicationIssues && (
                    <AlertTriangleIcon
                      sx={{ fontSize: 12, color: "primary.main" }}
                    />
                  )}
                </Box>
                <Typography variant="h4">
                  {formatNumber(metrics.replication_count || 0)}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.disabled"
                  sx={{ mt: 0.5, display: "block" }}
                >
                  Total replications
                </Typography>
                {hasReplicationIssues && (
                  <Typography
                    variant="caption"
                    color="primary.main"
                    sx={{ mt: 0.5, display: "block" }}
                  >
                    {formatNumber(metrics.replication_failures)} failures
                  </Typography>
                )}
              </Box>

              {/* Replication Performance */}
              <Box>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{
                    mb: 0.5,
                    display: "block",
                  }}
                >
                  Avg Replication Time
                </Typography>
                <Typography variant="h4">
                  {metrics.average_replication_time_ms > 0
                    ? `${metrics.average_replication_time_ms.toFixed(1)}ms`
                    : "N/A"}
                </Typography>
                <Typography
                  variant="caption"
                  color="text.disabled"
                  sx={{ mt: 0.5, display: "block" }}
                >
                  Response time
                </Typography>
              </Box>
            </Box>
          </CardContent>
        </Card>

        {/* Key Features */}
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "1fr",
              md: "repeat(3, 1fr)",
            },
            gap: 2,
          }}
        >
          {/* Session Distribution */}
          <Card variant="outlined">
            <CardContent sx={{ p: 2 }}>
              <Box
                sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}
              >
                <ActivityIcon
                  sx={{ fontSize: 16, color: "rgba(168, 85, 247, 1)" }}
                />
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 500,
                    color: "text.secondary",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Session Distribution
                </Typography>
              </Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <CheckCircleIcon sx={{ fontSize: 16, color: "success.main" }} />
                <Typography variant="body2">Active</Typography>
              </Box>
              <Typography
                variant="caption"
                color="text.disabled"
                sx={{ mt: 0.5, display: "block" }}
              >
                Sessions replicated across nodes
              </Typography>
            </CardContent>
          </Card>

          {/* Cache Replication */}
          <Card variant="outlined">
            <CardContent sx={{ p: 2 }}>
              <Box
                sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}
              >
                <ServerIcon sx={{ fontSize: 16, color: "info.main" }} />
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 500,
                    color: "text.secondary",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Cache Replication
                </Typography>
              </Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <CheckCircleIcon sx={{ fontSize: 16, color: "success.main" }} />
                <Typography variant="body2">Distributed</Typography>
              </Box>
              <Typography
                variant="caption"
                color="text.disabled"
                sx={{ mt: 0.5, display: "block" }}
              >
                Data synchronized across cluster
              </Typography>
            </CardContent>
          </Card>

          {/* Failover Ready */}
          <Card variant="outlined">
            <CardContent sx={{ p: 2 }}>
              <Box
                sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}
              >
                <ClockIcon sx={{ fontSize: 16, color: "warning.main" }} />
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 500,
                    color: "text.secondary",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Failover Ready
                </Typography>
              </Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                {isClusterSizeAdequate ? (
                  <CheckCircleIcon
                    sx={{ fontSize: 16, color: "success.main" }}
                  />
                ) : (
                  <AlertTriangleIcon
                    sx={{ fontSize: 16, color: "primary.main" }}
                  />
                )}
                <Typography variant="body2">
                  {isClusterSizeAdequate ? "Ready" : "Limited"}
                </Typography>
              </Box>
              <Typography
                variant="caption"
                color="text.disabled"
                sx={{ mt: 0.5, display: "block" }}
              >
                Automatic node failover
              </Typography>
            </CardContent>
          </Card>
        </Box>

        {/* Warning Messages */}
        {(!metrics.is_healthy || hasReplicationIssues) && (
          <Box
            sx={{
              mt: 2,
              bgcolor: "rgba(219, 40, 51, 0.1)",
              border: "1px solid",
              borderColor: "primary.main",
              borderRadius: 1,
              p: 1.5,
            }}
          >
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
              <AlertTriangleIcon
                sx={{ fontSize: 16, color: "primary.main", flexShrink: 0 }}
              />
              <Box sx={{ color: "primary.main" }}>
                <Typography
                  variant="caption"
                  sx={{ fontWeight: 600, mb: 0.5, display: "block" }}
                >
                  High Availability Warning
                </Typography>
                {!isClusterSizeAdequate && (
                  <Typography variant="caption" sx={{ display: "block" }}>
                    • Cluster size is below the minimum required nodes for full
                    redundancy
                  </Typography>
                )}
                {hasReplicationIssues && (
                  <Typography variant="caption" sx={{ display: "block" }}>
                    • Replication failures detected - some data may not be fully
                    synchronized
                  </Typography>
                )}
              </Box>
            </Box>
          </Box>
        )}
      </CardContent>
    </Card>
  );
}
