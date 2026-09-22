import { useQuery } from "@tanstack/react-query";
import { eventService } from "../services";

interface UseUserEventsParams {
  tenantId: string | undefined;
  userId: string | undefined;
  startTime?: string;
  endTime?: string;
}

export function useUserEvents({
  tenantId,
  userId,
  startTime,
  endTime,
}: UseUserEventsParams) {
  return useQuery({
    queryKey: ["user-events", tenantId, userId, startTime, endTime],
    queryFn: () =>
      eventService.getEvents(tenantId!, 1000, 0, startTime, endTime),
    enabled: !!tenantId,
    refetchInterval: 10000,
  });
}
