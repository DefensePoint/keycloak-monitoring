import type { KeycloakMetrics } from "./keycloak.types";

// Realm type used in realms grid
export interface RealmGridItem {
  realm_name: string;
  enabled: boolean;
  is_healthy: boolean;
  events_enabled: boolean;
  events_listeners?: string[];
  metrics?: KeycloakMetrics;
}

// Component Props
export interface RealmsFiltersProps {
  searchTerm: string;
  statusFilter: string;
  healthFilter: string;
  eventWarningFilter: boolean | null;
  onSearchChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onHealthChange: (value: string) => void;
  onEventWarningChange: (value: boolean | null) => void;
  onReset: () => void;
}

export interface RealmCardProps {
  realm: RealmGridItem;
  onClick: () => void;
}

export interface RealmsGridProps {
  realms: RealmGridItem[];
  onRealmClick: (realmName: string) => void;
  loading: boolean;
  connectionBroken?: boolean;
  connectionError?: string;
}
