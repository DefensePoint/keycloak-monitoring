import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/shared/lib/apiClient";
import type { AmfaGeoBucket } from "@/shared/types";

interface UseAmfaGeoParams {
  tenantId?: string;
  realm?: string;
  startTime?: string;
  endTime?: string;
}

/**
 * AMFA geo buckets for the login map. Gated on tenant + realm: the backend
 * `/amfa/geo` endpoint is per-realm, so the query stays idle until a specific
 * realm is selected.
 */
export const useAmfaGeo = (params: UseAmfaGeoParams) =>
  useQuery({
    queryKey: ["amfa", "geo", params],
    queryFn: () => {
      const query: Record<string, string> = { realm_id: params.realm! };
      if (params.startTime) query.start_time = params.startTime;
      if (params.endTime) query.end_time = params.endTime;
      return apiClient.get<AmfaGeoBucket[]>(
        `/tenants/${params.tenantId}/amfa/geo`,
        query,
      );
    },
    enabled: Boolean(params.tenantId && params.realm),
    refetchInterval: 60_000,
  });
