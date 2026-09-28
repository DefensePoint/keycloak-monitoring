import type {
  Event,
  ConfigurationAlert,
  KeycloakDashboard,
  KeycloakUser,
  KeycloakClient,
} from "./keycloak.types";

export interface RealmPageHeaderProps {
  realmName: string;
  tenantId: string;
  selectedRealm: string;
  allRealms: { realm_name: string; enabled: boolean }[];
  defaultRealm?: string;
  userName?: string;
  userEmail?: string;
  onRealmChange: (realm: string) => void;
  onTimeRangeChange: (start: Date | null, end: Date | null) => void;
  onLogout: () => void;
}

export interface RealmOverviewMetricsProps {
  dashboard: KeycloakDashboard;
}

export interface RealmConfigurationAlertsProps {
  alerts: ConfigurationAlert[];
  onClick?: (alert: ConfigurationAlert) => void;
}

export interface RealmRecentEventsProps {
  timePeriod: string;
  events: Event[];
  onEventClick: (event: Event) => void;
}

export interface RealmUsersListProps {
  users: KeycloakUser[];
  searchTerm: string;
  showAll: boolean;
  onSearchChange: (term: string) => void;
  onShowAllToggle: () => void;
  onUserClick: (user: KeycloakUser) => void;
}

export interface RealmClientsListProps {
  clients: KeycloakClient[];
  searchTerm: string;
  showAll: boolean;
  onSearchChange: (term: string) => void;
  onShowAllToggle: () => void;
  onClientClick: (client: KeycloakClient) => void;
}

export interface EventDetailModalProps {
  open: boolean;
  event: Event | null;
  tenantId: string | undefined;
  realmName: string | undefined;
  onClose: () => void;
}

export interface UserDetailModalProps {
  open: boolean;
  user: KeycloakUser | null;
  onClose: () => void;
}

export interface ClientDetailModalProps {
  open: boolean;
  client: KeycloakClient | null;
  onClose: () => void;
}
