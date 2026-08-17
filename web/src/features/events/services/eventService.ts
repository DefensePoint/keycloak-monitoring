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
  ): Promise<EventsResponse> {
    const params: Record<string, string | number> = { limit, offset };
    if (startTime) params.start_time = startTime;
    if (endTime) params.end_time = endTime;
    if (source) params.source = source;

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
    if (realm) params.source = `keycloak:${realm}`;

    return apiClient.get<Stats>(`/tenants/${tenantId}/stats`, params);
  }
}

export const eventService = new EventService();
