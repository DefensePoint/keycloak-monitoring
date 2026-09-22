import { useQuery } from "@tanstack/react-query";
import { metricsService } from "../services";

interface UseOperatorMetricsParams {
  tenantId: string | undefined;
  startDate: string;
  endDate: string;
}

export function useOperatorMetrics({
  tenantId,
  startDate,
  endDate,
}: UseOperatorMetricsParams) {
  return useQuery({
    queryKey: ["operator-metrics", tenantId, startDate, endDate],
    queryFn: () =>
      metricsService.getOperatorMetrics({
        tenantId: tenantId!,
        startDate,
        endDate,
      }),
    enabled: !!tenantId,
  });
}
