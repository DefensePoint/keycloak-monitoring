// Re-export shared Keycloak types
export type {
  KeycloakHealth,
  KeycloakMetrics,
  KeycloakRealmInfo,
  KeycloakVersionInfo,
  KeycloakDashboard,
  KeycloakDashboardAll,
  KeycloakEvent,
  EventsResponse,
  AlertSeverity,
  AlertStatus,
  ConfigurationAlert,
  KeycloakUser,
  KeycloakClient,
} from "@/shared/types";

// Alias for backward compatibility
export type { KeycloakEvent as Event } from "@/shared/types";
