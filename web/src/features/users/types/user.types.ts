import type {
  KeycloakGroup,
  KeycloakRole,
  KeycloakEvent,
  KeycloakUser,
  RoleMappings,
  UserDetailsResponse as SharedUserDetailsResponse,
  EventsResponse,
} from "@/shared/types";

// Alias for backward compatibility
export type UserEvent = KeycloakEvent;
export type UserDetails = KeycloakUser;

// Re-export shared types
export type { KeycloakGroup, KeycloakRole, RoleMappings };

// Use shared UserDetailsResponse
export type UserDetailsResponse = SharedUserDetailsResponse;

// Component Props
export interface UserInfoCardProps {
  userDetails: UserDetails | undefined;
  userGroups: KeycloakGroup[];
  roleMappings: RoleMappings | undefined;
  loading: boolean;
}

export interface UserEventsFiltersProps {
  searchTerm: string;
  severityFilter: string;
  sourceFilter: string;
  limit: number;
  onSearchChange: (value: string) => void;
  onSeverityChange: (value: string) => void;
  onSourceChange: (value: string) => void;
  onLimitChange: (value: number) => void;
  onReset: () => void;
  onTimeRangeChange: (start: Date | null, end: Date | null) => void;
}

export interface UserEventsTableProps {
  events: UserEvent[];
  loading: boolean;
  onEventClick: (event: UserEvent) => void;
}

// Re-export EventsResponse from shared types
export type { EventsResponse };
