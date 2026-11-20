import type { KeycloakHealth } from "@/shared/types";

export const HEALTH_METRICS_HINT =
  "Keycloak 26.7 and later show these details only to a master realm administrator. For the update check, a Keycloak version can be entered in the tenant settings.";

export function getHealthMetricsHint(
  health: KeycloakHealth | null | undefined,
  missing: boolean,
) {
  return health?.status === "UP" && missing ? HEALTH_METRICS_HINT : null;
}

// The API stores missing server-info numbers as zero. In particular, a zero
// memory maximum cannot be used as a measurement or a percentage denominator.
export function getHealthAvailability(health?: KeycloakHealth | null) {
  return {
    memoryFree: Boolean(
      health &&
      Number.isFinite(health.memory_free_bytes) &&
      health.memory_free_bytes >= 0,
    ),
    memory: Boolean(
      health &&
      Number.isFinite(health.memory_max_bytes) &&
      health.memory_max_bytes > 0 &&
      Number.isFinite(health.memory_used_bytes) &&
      health.memory_used_bytes >= 0 &&
      health.memory_used_bytes <= health.memory_max_bytes,
    ),
    uptime: Boolean(
      health &&
      Number.isFinite(health.uptime_millis) &&
      health.uptime_millis > 0,
    ),
    version: Boolean(
      health?.server_version?.trim() &&
      health.server_version.trim().toLowerCase() !== "unknown",
    ),
  };
}
