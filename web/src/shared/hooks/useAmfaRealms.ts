import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/shared/lib/apiClient";

interface AmfaRealmsResponse {
  realms: string[];
  count: number;
}

/**
 * Names of realms whose Keycloak browser login flow uses an Adaptive MFA
 * (AMFA) authenticator. Used to show/hide AMFA-only UI sections (KPI cards, geo
 * map, Risk column) per realm. Backed by GET /keycloak/amfa-realms, which the
 * backend caches; a realm's AMFA config changes rarely, so this is polled slowly.
 */
export const useAmfaRealms = (tenantId?: string) =>
  useQuery({
    queryKey: ["amfa", "realms", tenantId],
    queryFn: async () => {
      const res = await apiClient.get<AmfaRealmsResponse>(
        `/tenants/${tenantId}/keycloak/amfa-realms`,
      );
      return res.realms ?? [];
    },
    enabled: Boolean(tenantId),
    staleTime: 5 * 60_000,
    refetchInterval: 5 * 60_000,
  });
