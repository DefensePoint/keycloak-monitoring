import { apiClient } from "@/shared/lib/apiClient";
import type {
  Tenant,
  TenantsResponse,
  TenantCreate,
  TenantUpdate,
  TenantHealth,
  RealmListItem,
} from "@/shared/types";

export interface KeycloakDashboardResponse {
  realms: RealmListItem[];
  total_realms: number;
}

class TenantService {
  async getTenants(): Promise<TenantsResponse> {
    return apiClient.get<TenantsResponse>("/tenants");
  }

  async getTenant(tenantId: string): Promise<Tenant> {
    return apiClient.get<Tenant>(`/tenants/${tenantId}`);
  }

  async createTenant(tenant: TenantCreate): Promise<Tenant> {
    return apiClient.post<Tenant>("/tenants", tenant);
  }

  async updateTenant(tenantId: string, tenant: TenantUpdate): Promise<Tenant> {
    return apiClient.put<Tenant>(`/tenants/${tenantId}`, tenant);
  }

  async deleteTenant(tenantId: string): Promise<void> {
    return apiClient.delete<void>(`/tenants/${tenantId}`);
  }

  async getTenantHealth(tenantId: string): Promise<TenantHealth> {
    return apiClient.get<TenantHealth>(`/tenants/${tenantId}/health`);
  }

  async getKeycloakDashboard(
    tenantId: string,
  ): Promise<KeycloakDashboardResponse> {
    return apiClient.get<KeycloakDashboardResponse>(
      `/tenants/${tenantId}/keycloak/dashboard`,
    );
  }
}

export const tenantService = new TenantService();
