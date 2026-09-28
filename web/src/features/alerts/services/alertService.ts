import { apiClient } from "@/shared/lib/apiClient";
import type {
  ConfigurationAlert,
  AlertsResponse,
  AlertStats,
  AlertRule,
  AlertRulesResponse,
  CreateAlertRuleRequest,
  UpdateAlertRuleRequest,
  AlertStatus,
} from "../types";

class AlertService {
  async getAlerts(
    tenantId: string,
    limit: number = 100,
    offset: number = 0,
    status?: string,
    severity?: string,
    type?: string,
    realm?: string,
  ): Promise<AlertsResponse> {
    const params: Record<string, string | number> = { limit, offset };
    if (status) params.status = status;
    if (severity) params.severity = severity;
    if (type) params.type = type;
    if (realm) params.realm = realm;

    return apiClient.get<AlertsResponse>(`/tenants/${tenantId}/alerts`, params);
  }

  async getAlert(
    tenantId: string,
    alertId: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.get<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/${alertId}`,
    );
  }

  async getAlertStats(tenantId: string): Promise<AlertStats> {
    return apiClient.get<AlertStats>(`/tenants/${tenantId}/alerts/stats`);
  }

  async updateAlertStatus(
    tenantId: string,
    alertId: string,
    status: AlertStatus,
    acknowledgedBy?: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.put<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/${alertId}/status`,
      { status, acknowledged_by: acknowledgedBy },
    );
  }

  async resolveAlert(
    tenantId: string,
    alertId: string,
  ): Promise<ConfigurationAlert> {
    return apiClient.post<ConfigurationAlert>(
      `/tenants/${tenantId}/alerts/${alertId}/resolve`,
      {},
    );
  }

  async getAlertRules(tenantId: string): Promise<AlertRulesResponse> {
    return apiClient.get<AlertRulesResponse>(
      `/tenants/${tenantId}/alerts/rules`,
    );
  }

  async getAlertRule(tenantId: string, ruleId: string): Promise<AlertRule> {
    return apiClient.get<AlertRule>(
      `/tenants/${tenantId}/alerts/rules/${ruleId}`,
    );
  }

  async createAlertRule(
    tenantId: string,
    rule: CreateAlertRuleRequest,
  ): Promise<AlertRule> {
    return apiClient.post<AlertRule>(`/tenants/${tenantId}/alerts/rules`, rule);
  }

  async updateAlertRule(
    tenantId: string,
    ruleId: string,
    rule: UpdateAlertRuleRequest,
  ): Promise<AlertRule> {
    return apiClient.put<AlertRule>(
      `/tenants/${tenantId}/alerts/rules/${ruleId}`,
      rule,
    );
  }

  async deleteAlertRule(tenantId: string, ruleId: string): Promise<void> {
    return apiClient.delete<void>(
      `/tenants/${tenantId}/alerts/rules/${ruleId}`,
    );
  }
}

export const alertService = new AlertService();
