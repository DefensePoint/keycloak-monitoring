export interface OperatorMetricsSummary {
  operator_email: string;
  operator_name: string;
  total_alerts_handled: number;
  alerts_acknowledged: number;
  alerts_resolved: number;
  alerts_ignored: number;
  critical_alerts_handled: number;
  high_alerts_handled: number;
  avg_response_time: number;
  avg_time_to_acknowledge: number;
  avg_time_to_resolve: number;
  avg_time_to_ignore: number;
  avg_acknowledge_to_resolve_time: number;
  avg_acknowledge_to_ignore_time: number;
  avg_active_to_acknowledge_time: number;
  total_work_time_hours: number;
}
