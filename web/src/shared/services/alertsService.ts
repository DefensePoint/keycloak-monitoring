import { apiClient } from "@/shared/lib/apiClient";
import type {
  ConfigurationAlert,
  AlertStatus,
} from "@/shared/types/keycloak.types";

export type { ConfigurationAlert, AlertStatus };

export interface AlertsResponse {
  alerts: ConfigurationAlert[];
  total: number;
  limit: number;
  offset: number;
}

export interface AlertStats {
  total_active: number;
  by_severity: {
    info: number;
    warning: number;
    error: number;
    critical: number;
  };
  by_type: {
    configuration: number;
    identity_provider: number;
  };
  recent_count: number;
}

class AlertsService {
  async getAlerts(
    tenantId: string,
    limit: number = 100,
    offset: number = 0,
    status?: string,
    severity?: string,
    type?: string,
    realm?: string,
    resourceType?: string,
  ): Promise<AlertsResponse> {
    const params: Record<string, string | number> = { limit, offset };
    if (status) params.status = status;
    if (severity) params.severity = severity;
    if (type) params.type = type;
    if (realm) params.realm = realm;
    if (resourceType) params.resource_type = resourceType;

    return apiClient.get<AlertsResponse>(`/tenants/${tenantId}/alerts`, params);
  }

  async getAlert(
    tenantId: string,
    alertId: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.get<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/get`,
      { id: alertId },
    );
  }

  async getAlertStats(tenantId: string): Promise<AlertStats> {
    return apiClient.get<AlertStats>(`/tenants/${tenantId}/alerts/stats`);
  }

  async getAmfaCheckerStatus(tenantId: string): Promise<{ enabled: boolean }> {
    return apiClient.get<{ enabled: boolean }>(
      `/tenants/${tenantId}/amfa-checker`,
    );
  }

  async reloadAmfaAlerts(tenantId: string): Promise<{ status: string }> {
    return apiClient.post<{ status: string }>(
      `/tenants/${tenantId}/amfa-checker/run`,
      {},
    );
  }

  async updateAlertStatus(
    tenantId: string,
    alertId: string,
    status: AlertStatus,
    acknowledgedBy?: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.post<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/update-status?id=${encodeURIComponent(alertId)}`,
      { status, acknowledged_by: acknowledgedBy },
    );
  }

  async resolveAlert(
    tenantId: string,
    alertId: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.post<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/resolve?id=${encodeURIComponent(alertId)}`,
      {},
    );
  }
}

export const alertsService = new AlertsService();
