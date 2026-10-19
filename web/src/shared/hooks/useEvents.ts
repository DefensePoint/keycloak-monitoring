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
  type?: string;
  severity?: string;
  minRisk?: number;
  refetchInterval?: number;
}

/**
 * The error the single-event endpoint answers with for an id that is missing
 * or outside the caller's realms (handleEvent in system_handlers.go).
 */
export const EVENT_NOT_FOUND = "Event not found";

/** True when a useEvent error is the server's 404, not a failure to answer. */
export function isEventNotFoundError(error: unknown): boolean {
  return error instanceof Error && error.message === EVENT_NOT_FOUND;
}

/**
 * One event by id, for links that name an event the loaded list may not hold.
 * Not retried: a miss is a 404 that a retry would only repeat.
 */
export function useEvent(
  tenantId: string | undefined,
  eventId: string | undefined,
  enabled = true,
) {
  return useQuery({
    queryKey: ["event", tenantId, eventId],
    queryFn: () => eventsService.getEvent(tenantId!, eventId!),
    enabled: enabled && !!tenantId && !!eventId,
    retry: false,
  });
}

export function useEvents({
  tenantId,
  limit = 100,
  offset = 0,
  startTime,
  endTime,
  source,
  realm,
  type,
  severity,
  minRisk,
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
      type,
      severity,
      minRisk,
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
        { type, severity, minRisk },
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
