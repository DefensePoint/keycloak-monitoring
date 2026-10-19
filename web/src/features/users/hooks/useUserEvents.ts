import { useQuery } from "@tanstack/react-query";
import { eventService } from "../services";

// The most events one request returns: the API's page cap.
export const USER_EVENTS_LIMIT = 1000;

interface UseUserEventsParams {
  tenantId: string | undefined;
  realmName: string | undefined;
  userId: string | undefined;
  startTime?: string;
  endTime?: string;
  severity?: string;
}

export function useUserEvents({
  tenantId,
  realmName,
  userId,
  startTime,
  endTime,
  severity,
}: UseUserEventsParams) {
  return useQuery({
    queryKey: [
      "user-events",
      tenantId,
      realmName,
      userId,
      startTime,
      endTime,
      severity,
    ],
    // Realm, user and severity are filtered server-side, so the 1000-row
    // window (the API maximum) holds this user's matching events only. The
    // realm matters even though Keycloak user IDs are per realm: without it
    // the page listed the user's events from every realm the caller can see.
    queryFn: () =>
      eventService.getEvents(
        tenantId!,
        USER_EVENTS_LIMIT,
        0,
        startTime,
        endTime,
        {
          severity,
          userId,
          realm: realmName,
        },
      ),
    enabled: !!tenantId,
    refetchInterval: 10000,
  });
}
