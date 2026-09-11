import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { eventsService } from "@/shared/services";

interface UseEventsParams {
  tenantId: string | undefined;
  limit?: number;
  offset?: number;
  startTime?: string;
  endTime?: string;
  source?: string;
  realm?: string;
  refetchInterval?: number;
}

export function useEvents({
  tenantId,
  limit = 100,
  offset = 0,
  startTime,
  endTime,
  source,
  realm,
  refetchInterval = 10000,
}: UseEventsParams) {
  return useQuery({
    queryKey: [
      "events",
      tenantId,
      source,
      realm,
      startTime,
      endTime,
      limit,
      offset,
    ],
    queryFn: () =>
      eventsService.getEvents(
        tenantId!,
        limit,
        offset,
        startTime,
        endTime,
        source,
        realm,
      ),
    enabled: !!tenantId,
    refetchInterval,
    placeholderData: keepPreviousData,
  });
}

interface UseEventStatsParams {
  tenantId: string | undefined;
  startTime?: string;
  endTime?: string;
  realm?: string;
  refetchInterval?: number;
}

export function useEventStats({
  tenantId,
  startTime,
  endTime,
  realm,
  refetchInterval = 5000,
}: UseEventStatsParams) {
  return useQuery({
    queryKey: ["stats", tenantId, realm, startTime, endTime],
    queryFn: () => eventsService.getStats(tenantId!, startTime, endTime, realm),
    enabled: !!tenantId,
    refetchInterval,
  });
}
