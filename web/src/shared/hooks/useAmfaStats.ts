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

/**
 * Error code the backend returns (HTTP 403) when the caller's realm scope does
 * not cover every realm. A property of the caller's permissions, not of the
 * integration, so it stays distinct from AMFA_ALL_REALMS_UNSUPPORTED.
 */
export const AMFA_REALM_SCOPE_REQUIRES_REALM =
  "amfa_realm_scope_requires_realm";

export function isAllRealmsUnsupportedError(error: unknown): boolean {
  return (
    error instanceof Error && error.message === AMFA_ALL_REALMS_UNSUPPORTED
  );
}

export function isRealmScopeRequiresRealmError(error: unknown): boolean {
  return (
    error instanceof Error && error.message === AMFA_REALM_SCOPE_REQUIRES_REALM
  );
}

/** Neither refusal can change until the user selects a single realm. */
function isSettledAllRealmsRefusal(error: unknown): boolean {
  return (
    isAllRealmsUnsupportedError(error) || isRealmScopeRequiresRealmError(error)
  );
}

/**
 * AMFA KPI counters. Gated on tenant only: when `realm` is omitted the backend
 * aggregates across every realm in the tenant's AMFA DB ("all realms"), matching
 * the "All Realms" selection on the Events page. Two things can refuse that -
 * the tenant's AMFA transport being unable to aggregate, and the caller's own
 * realm scope not covering every realm - and neither changes until a specific
 * realm is selected, so both stop the retry and the poll.
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
      !isSettledAllRealmsRefusal(error) && failureCount < 1,
    refetchInterval: (query) =>
      isSettledAllRealmsRefusal(query.state.error) ? false : 30_000,
  });
