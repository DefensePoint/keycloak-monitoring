import { useQuery } from "@tanstack/react-query";
import { healthService } from "../services";

interface UseInfinispanMetricsOptions {
  enabled?: boolean;
  refetchInterval?: number;
}

export function useInfinispanMetrics(
  tenantId: string | undefined,
  options: UseInfinispanMetricsOptions = {},
) {
  const { enabled = true, refetchInterval = 10000 } = options;

  return useQuery({
    queryKey: ["infinispan-metrics", tenantId],
    queryFn: () => {
      if (!tenantId) {
        throw new Error("tenantId is required for Infinispan metrics");
      }
      return healthService.getInfinispanMetrics(tenantId);
    },
    refetchInterval,
    retry: false,
    enabled: enabled && !!tenantId,
  });
}
