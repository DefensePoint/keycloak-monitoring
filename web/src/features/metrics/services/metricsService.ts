import { apiClient } from "@/shared/lib/apiClient";
import type { OperatorMetricsSummary } from "../types";

interface OperatorMetricsParams {
  tenantId: string;
  startDate: string;
  endDate: string;
}

export const metricsService = {
  getOperatorMetrics: ({
    tenantId,
    startDate,
    endDate,
  }: OperatorMetricsParams): Promise<OperatorMetricsSummary[]> =>
    apiClient.get<OperatorMetricsSummary[]>(
      `/tenants/${tenantId}/metrics/operators?start_date=${startDate}&end_date=${endDate}`,
    ),
};
