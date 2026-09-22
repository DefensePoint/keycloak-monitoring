import type { ConfigurationAlert, AlertSeverity } from "@/shared/types";

// Backward compatibility alias
export type Alert = ConfigurationAlert;

// Alert-specific types (not in shared)
export type AlertSource = "configuration" | "event" | "log" | "metric";
export type AlertType =
  | "configuration"
  | "security"
  | "compliance"
  | "performance"
  | "identity_provider"
  | "realm"
  | "client"
  | "event";

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

// Alert Rules
export interface RuleConditions {
  event_type?: string;
  event_category?: string;
  field_matches?: Record<string, string>;
  threshold_count?: number;
  threshold_window?: string;
  realm_name?: string;
  client_id?: string;
  user_pattern?: string;
}

export interface AlertRule {
  id: string;
  tenant_id?: string;
  name: string;
  description: string;
  enabled: boolean;
  source: AlertSource;
  severity: AlertSeverity;
  conditions: RuleConditions;
  title_template: string;
  description_template: string;
  recommendation_template: string;
  notify_slack: boolean;
  slack_channel?: string;
  created_at: string;
  updated_at: string;
  created_by: string;
}

export interface AlertRulesResponse {
  rules: AlertRule[];
  count: number;
}

export interface CreateAlertRuleRequest {
  name: string;
  description: string;
  enabled: boolean;
  source: AlertSource;
  severity: AlertSeverity;
  conditions: RuleConditions;
  title_template: string;
  description_template: string;
  recommendation_template: string;
  notify_slack: boolean;
  slack_channel?: string;
}

export interface UpdateAlertRuleRequest {
  name?: string;
  description?: string;
  enabled?: boolean;
  severity?: AlertSeverity;
  conditions?: RuleConditions;
  title_template?: string;
  description_template?: string;
  recommendation_template?: string;
  notify_slack?: boolean;
  slack_channel?: string;
}
