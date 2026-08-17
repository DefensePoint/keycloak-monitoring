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
 * AMFA KPI counters. Gated on tenant only: when `realm` is omitted the backend
 * aggregates across every realm in the tenant's AMFA DB ("all realms"), matching
 * the "All Realms" selection on the Events page.
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
    refetchInterval: 30_000,
  });
