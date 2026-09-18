// Event is an alias of the canonical shared KeycloakEvent so the feature and
// the shared services can never drift (they previously held duplicated,
// manually-synced definitions).
import type { KeycloakEvent } from "@/shared/types";

export type Event = KeycloakEvent;

export interface EventsResponse {
  events: Event[];
  count: number;
  total: number;
}

export interface Stats {
  total_events: number;
  by_severity?: Record<string, number>;
  by_type?: Record<string, number>;
  by_source?: Record<string, number>;
}
