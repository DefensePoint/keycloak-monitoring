import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/shared/lib/apiClient";
import type { AmfaStats } from "@/shared/types";

interface UseAmfaStatsParams {
  tenantId?: string;
  realm?: string;
  startTime?: string;
  endTime?: string;
}

/**
 * Error code the backend returns (HTTP 501) when a tenant's AMFA transport
 * can't aggregate stats across all realms (see amfa.ErrAllRealmsUnsupported
 * server-side) - e.g. the HTTP-API repository, whose credentials are scoped
 * to a single realm.
 */
export const AMFA_ALL_REALMS_UNSUPPORTED = "amfa_all_realms_unsupported";

export function isAllRealmsUnsupportedError(error: unknown): boolean {
  return (
    error instanceof Error && error.message === AMFA_ALL_REALMS_UNSUPPORTED
  );
}

/**
 * AMFA KPI counters. Gated on tenant only: when `realm` is omitted the backend
 * aggregates across every realm in the tenant's AMFA DB ("all realms"), matching
 * the "All Realms" selection on the Events page. That aggregation isn't
 * supported by every AMFA transport, so once the backend reports it's
 * unsupported for this tenant, stop retrying/polling - the answer won't
 * change until a specific realm is selected instead.
 */
export const useAmfaStats = (params: UseAmfaStatsParams) =>
  useQuery({
    queryKey: ["amfa", "stats", params],
    queryFn: () => {
      const query: Record<string, string> = {};
      if (params.realm) query.realm_id = params.realm;
      if (params.startTime) query.start_time = params.startTime;
      if (params.endTime) query.end_time = params.endTime;
      return apiClient.get<AmfaStats>(
        `/tenants/${params.tenantId}/amfa/stats`,
        query,
      );
    },
    enabled: Boolean(params.tenantId),
    retry: (failureCount, error) =>
      !isAllRealmsUnsupportedError(error) && failureCount < 1,
    refetchInterval: (query) =>
      isAllRealmsUnsupportedError(query.state.error) ? false : 30_000,
  });
