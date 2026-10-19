import { apiClient } from "@/shared/lib/apiClient";
import type { EventsResponse } from "../types";

// UserEventFilters narrows the query server-side. userId scopes it to one
// user, so a busy tenant's other events cannot crowd this user's out of the
// page window.
export interface UserEventFilters {
  severity?: string;
  userId?: string;
  realm?: string;
}

export const eventService = {
  getEvents: async (
    tenantId: string,
    limit: number,
    offset: number,
    startTime?: string,
    endTime?: string,
    filters?: UserEventFilters,
  ): Promise<EventsResponse> => {
    const params = new URLSearchParams({
      limit: limit.toString(),
      offset: offset.toString(),
    });

    if (startTime) {
      params.append("start_time", startTime);
    }
    if (endTime) {
      params.append("end_time", endTime);
    }
    if (filters?.severity) {
      params.append("severity", filters.severity);
    }
    if (filters?.userId) {
      params.append("user_id", filters.userId);
    }
    if (filters?.realm) {
      params.append("realm", filters.realm);
    }

    return apiClient.get<EventsResponse>(
      `/tenants/${tenantId}/events?${params.toString()}`,
    );
  },
};
