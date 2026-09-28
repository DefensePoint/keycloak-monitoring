import { apiClient } from "@/shared/lib/apiClient";
import type { EventsResponse, Stats } from "../types";

class EventService {
  async getEvents(
    tenantId: string,
    limit: number = 100,
    offset: number = 0,
    startTime?: string,
    endTime?: string,
    source?: string,
    realm?: string,
  ): Promise<EventsResponse> {
    const params: Record<string, string | number> = { limit, offset };
    if (startTime) params.start_time = startTime;
    if (endTime) params.end_time = endTime;
    if (source) params.source = source;
    // See getStats: a realm goes in `realm`, never encoded as a source.
    if (realm) params.realm = realm;

    return apiClient.get<EventsResponse>(`/tenants/${tenantId}/events`, params);
  }

  async getStats(
    tenantId: string,
    startTime?: string,
    endTime?: string,
    realm?: string,
  ): Promise<Stats> {
    const params: Record<string, string> = {};
    if (startTime) params.start_time = startTime;
    if (endTime) params.end_time = endTime;
    // A realm is sent as `realm`, not as source="keycloak:<realm>". The
    // latter excluded the realm's AMFA-mirrored rows, which are exactly the
    // events an AMFA realm is selected to look at.
    if (realm) params.realm = realm;

    return apiClient.get<Stats>(`/tenants/${tenantId}/stats`, params);
  }
}

export const eventService = new EventService();
