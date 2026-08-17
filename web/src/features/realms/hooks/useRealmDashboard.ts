import { useQuery } from "@tanstack/react-query";
import { realmService } from "../services";
import type { KeycloakDashboard } from "../types";

interface UseRealmDashboardOptions {
  enabled?: boolean;
  refetchInterval?: number;
}

export function useRealmDashboard(
  tenantId: string | undefined,
  realmName: string,
  options: UseRealmDashboardOptions = {},
) {
  const { enabled = true, refetchInterval = 10000 } = options;

  return useQuery({
    queryKey: ["keycloak-realm", tenantId, realmName],
    queryFn: () =>
      realmService.getKeycloakDashboard(
        tenantId!,
        realmName,
      ) as Promise<KeycloakDashboard>,
    enabled: !!tenantId && enabled,
    refetchInterval,
  });
}
