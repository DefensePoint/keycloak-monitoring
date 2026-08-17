import { Box, Typography, Card, CardContent, Skeleton } from "@mui/material";
import {
  Storage as DatabaseIcon,
  Memory as HardDriveIcon,
  ShowChart as ActivityIcon,
  Warning as AlertTriangleIcon,
  CheckCircle as CheckCircleIcon,
  Dns as ServerIcon,
} from "@mui/icons-material";
import { useInfinispanMetrics } from "../hooks";
import { formatBytes, formatNumber } from "@/shared/utils";
import { AlertBanner } from "@/shared/components";
import type { CacheStats } from "@/shared/types/infinispan";

interface InfinispanMetricsProps {
  tenantId: string;
}

export function InfinispanMetrics({ tenantId }: InfinispanMetricsProps) {
  const { data: metrics, isLoading, error } = useInfinispanMetrics(tenantId);

  if (isLoading) {
    return (
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
            <DatabaseIcon sx={{ fontSize: 20, color: "primary.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Infinispan Cache
            </Typography>
          </Box>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
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
        </CardContent>
      </Card>
    );
  }

  if (error || !metrics) {
    return (
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2 }}>
            <DatabaseIcon sx={{ fontSize: 20, color: "primary.main" }} />
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Infinispan Cache
            </Typography>
          </Box>
          <AlertBanner
            severity="warning"
            message="Infinispan metrics not available. Check if the /metrics endpoint is enabled in Keycloak."
          />
        </CardContent>
      </Card>
    );
  }

  // Get important caches (sessions, users, realms)
  const importantCaches = ["sessions", "users", "realms", "clientSessions"];
  const cacheList = Object.values(metrics.cache_stats || {})
    .filter((cache) => importantCaches.includes(cache.cache_name))
    .sort((a, b) => b.approximate_entries - a.approximate_entries);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Header */}
      <Card>
        <CardContent sx={{ p: 3 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              mb: 2,
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <DatabaseIcon sx={{ fontSize: 20, color: "primary.main" }} />
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 600,
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                }}
              >
                Infinispan Cache
              </Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              {metrics.is_healthy ? (
                <>
                  <CheckCircleIcon
                    sx={{ fontSize: 20, color: "success.main" }}
                  />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "success.main",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    Healthy
                  </Typography>
                </>
              ) : (
                <>
                  <AlertTriangleIcon
                    sx={{ fontSize: 20, color: "primary.main" }}
                  />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "primary.main",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    Degraded
                  </Typography>
                </>
              )}
            </Box>
          </Box>

          {/* Cluster State */}
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "1fr",
                md: "repeat(4, 1fr)",
              },
              gap: 2,
            }}
          >
            <Card variant="outlined">
              <CardContent sx={{ p: 2 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mb: 0.5,
                  }}
                >
                  <ServerIcon sx={{ fontSize: 16, color: "info.main" }} />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "text.secondary",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    Cluster Size
                  </Typography>
                </Box>
                <Typography variant="h4">
                  {formatNumber(metrics.cluster_size || 1)}
                </Typography>
                {metrics.required_minimum_nodes > 0 && (
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ mt: 0.5 }}
                  >
                    Min required: {formatNumber(metrics.required_minimum_nodes)}
                  </Typography>
                )}
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent sx={{ p: 2 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mb: 0.5,
                  }}
                >
                  <ActivityIcon
                    sx={{ fontSize: 16, color: "rgba(168, 85, 247, 1)" }}
                  />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "text.secondary",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    Replication
                  </Typography>
                </Box>
                <Typography variant="h4">
                  {formatNumber(metrics.replication_count || 0)}
                </Typography>
                {metrics.replication_failures > 0 && (
                  <Typography
                    variant="caption"
                    color="primary.main"
                    sx={{ mt: 0.5 }}
                  >
                    {formatNumber(metrics.replication_failures)} failures
                  </Typography>
                )}
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent sx={{ p: 2 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mb: 0.5,
                  }}
                >
                  <HardDriveIcon sx={{ fontSize: 16, color: "success.main" }} />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "text.secondary",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    JVM Memory
                  </Typography>
                </Box>
                <Typography variant="h4">
                  {metrics.jvm_memory_used_percent.toFixed(1)}%
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ mt: 0.5 }}
                >
                  {formatBytes(metrics.jvm_memory_used_bytes)} /{" "}
                  {formatBytes(metrics.jvm_memory_committed_bytes)}
                </Typography>
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent sx={{ p: 2 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mb: 0.5,
                  }}
                >
                  <ActivityIcon sx={{ fontSize: 16, color: "warning.main" }} />
                  <Typography
                    variant="body2"
                    sx={{
                      fontWeight: 500,
                      color: "text.secondary",
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                    }}
                  >
                    CPU Usage
                  </Typography>
                </Box>
                <Typography variant="h4">
                  {(metrics.process_cpu_usage * 100).toFixed(1)}%
                </Typography>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ mt: 0.5 }}
                >
                  Process CPU
                </Typography>
              </CardContent>
            </Card>
          </Box>
        </CardContent>
      </Card>

      {/* Cache Statistics */}
      {cacheList.length > 0 && (
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                mb: 2,
              }}
            >
              Cache Statistics
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              {cacheList.map((cache) => (
                <CacheStatsCard key={cache.cache_name} cache={cache} />
              ))}
            </Box>
          </CardContent>
        </Card>
      )}
    </Box>
  );
}

function CacheStatsCard({ cache }: { cache: CacheStats }) {
  const getHitRatioColor = (ratio: number) => {
    if (ratio >= 80) return "success.main";
    if (ratio >= 50) return "warning.main";
    return "primary.main";
  };

  const getHitRatioBgColor = (ratio: number) => {
    if (ratio >= 80) return "rgba(16, 185, 129, 0.1)";
    if (ratio >= 50) return "rgba(245, 158, 11, 0.1)";
    return "rgba(219, 40, 51, 0.1)";
  };

  return (
    <Card
      variant="outlined"
      sx={{
        transition: "border-color 0.2s",
        "&:hover": {
          borderColor: "primary.main",
        },
      }}
    >
      <CardContent sx={{ p: 2 }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            mb: 1.5,
          }}
        >
          <Typography
            variant="subtitle1"
            sx={{ fontWeight: 500, textTransform: "capitalize" }}
          >
            {cache.cache_name}
          </Typography>
          <Box
            sx={{
              px: 1,
              py: 0.5,
              borderRadius: 1,
              bgcolor: getHitRatioBgColor(cache.hit_ratio),
            }}
          >
            <Typography
              variant="caption"
              sx={{
                fontWeight: 500,
                color: getHitRatioColor(cache.hit_ratio),
              }}
            >
              Hit Ratio: {cache.hit_ratio.toFixed(1)}%
            </Typography>
          </Box>
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "repeat(2, 1fr)",
              md: "repeat(5, 1fr)",
            },
            gap: 1.5,
          }}
        >
          <Box>
            <Typography variant="body2" color="text.secondary">
              Entries
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              {formatNumber(cache.approximate_entries)}
            </Typography>
          </Box>
          <Box>
            <Typography variant="body2" color="text.secondary">
              Hits
            </Typography>
            <Typography
              variant="body2"
              sx={{ fontWeight: 600, color: "success.main" }}
            >
              {formatNumber(cache.hits)}
            </Typography>
          </Box>
          <Box>
            <Typography variant="body2" color="text.secondary">
              Misses
            </Typography>
            <Typography
              variant="body2"
              sx={{ fontWeight: 600, color: "primary.main" }}
            >
              {formatNumber(cache.misses)}
            </Typography>
          </Box>
          <Box>
            <Typography variant="body2" color="text.secondary">
              Evictions
            </Typography>
            <Typography
              variant="body2"
              sx={{ fontWeight: 600, color: "warning.main" }}
            >
              {formatNumber(cache.evictions)}
            </Typography>
          </Box>
          <Box>
            <Typography variant="body2" color="text.secondary">
              Avg Hit Time
            </Typography>
            <Typography
              variant="body2"
              sx={{ fontWeight: 600, color: "info.main" }}
            >
              {cache.hit_time_ms.toFixed(2)}ms
            </Typography>
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
}
