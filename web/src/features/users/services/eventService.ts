import { apiClient } from "@/shared/lib/apiClient";
import type { EventsResponse } from "../types";

export const eventService = {
  getEvents: async (
    tenantId: string,
    limit: number,
    offset: number,
    startTime?: string,
    endTime?: string,
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

    return apiClient.get<EventsResponse>(
      `/tenants/${tenantId}/events?${params.toString()}`,
    );
  },
};
