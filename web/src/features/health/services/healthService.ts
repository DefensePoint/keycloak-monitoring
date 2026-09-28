import { apiClient } from "@/shared/lib/apiClient";
import type { InfinispanMetrics } from "@/shared/types/infinispan";
import type { VersionInfo } from "@/shared/types/version";

class HealthService {
  async getInfinispanMetrics(tenantId: string): Promise<InfinispanMetrics> {
    return apiClient.get<InfinispanMetrics>(
      `/tenants/${tenantId}/keycloak/infinispan/metrics`,
    );
  }

  async getVersion(tenantId: string): Promise<VersionInfo> {
    return apiClient.get<VersionInfo>(`/tenants/${tenantId}/version`);
  }
}

export const healthService = new HealthService();
