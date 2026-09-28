import { useQuery } from "@tanstack/react-query";
import { healthService } from "@/shared/services";

export function useVersion(tenantId: string | undefined) {
  return useQuery({
    queryKey: ["version", tenantId],
    queryFn: () => healthService.getVersion(tenantId!),
    enabled: !!tenantId,
    staleTime: Infinity,
  });
}

export function useInfinispanMetrics(tenantId: string | undefined) {
  return useQuery({
    queryKey: ["infinispan-metrics", tenantId],
    queryFn: () => healthService.getInfinispanMetrics(tenantId!),
    enabled: !!tenantId,
    refetchInterval: 10000,
  });
}
