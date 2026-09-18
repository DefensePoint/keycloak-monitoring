import {
  useQuery,
  useMutation,
  useQueryClient,
  keepPreviousData,
} from "@tanstack/react-query";
import { alertsService, type AlertStatus } from "@/shared/services";

interface UseAlertsParams {
  tenantId: string | undefined;
  status?: string;
  severity?: string;
  type?: string;
  realm?: string;
  resourceType?: string;
  limit?: number;
  offset?: number;
  refetchInterval?: number;
}

export function useAlerts({
  tenantId,
  status,
  severity,
  type,
  realm,
  resourceType,
  limit = 100,
  offset = 0,
  refetchInterval,
}: UseAlertsParams) {
  return useQuery({
    queryKey: [
      "alerts",
      tenantId,
      status,
      severity,
      type,
      realm,
      resourceType,
      limit,
      offset,
    ],
    queryFn: () =>
      alertsService.getAlerts(
        tenantId!,
        limit,
        offset,
        status,
        severity,
        type,
        realm,
        resourceType,
      ),
    enabled: !!tenantId,
    refetchInterval,
    placeholderData: keepPreviousData,
  });
}

export function useAlert(
  tenantId: string | undefined,
  alertId: string | undefined,
) {
  return useQuery({
    queryKey: ["alert", tenantId, alertId],
    queryFn: () => alertsService.getAlert(tenantId!, alertId!),
    enabled: !!tenantId && !!alertId,
  });
}

export function useAlertStats(tenantId: string | undefined) {
  return useQuery({
    queryKey: ["alert-stats", tenantId],
    queryFn: () => alertsService.getAlertStats(tenantId!),
    enabled: !!tenantId,
  });
}

export function useAmfaCheckerStatus(tenantId: string | undefined) {
  return useQuery({
    queryKey: ["amfa-checker-status", tenantId],
    queryFn: () => alertsService.getAmfaCheckerStatus(tenantId!),
    enabled: !!tenantId,
  });
}

export function useReloadAmfaAlerts() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (tenantId: string) =>
      alertsService.reloadAmfaAlerts(tenantId),
    onSuccess: (_, tenantId) => {
      queryClient.invalidateQueries({
        queryKey: ["alerts", tenantId],
      });
      queryClient.invalidateQueries({
        queryKey: ["alert-stats", tenantId],
      });
    },
  });
}

export function useUpdateAlertStatus() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      tenantId,
      alertId,
      status,
      acknowledgedBy,
    }: {
      tenantId: string;
      alertId: string;
      status: AlertStatus;
      acknowledgedBy?: string;
    }) =>
      alertsService.updateAlertStatus(
        tenantId,
        alertId,
        status,
        acknowledgedBy,
      ),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["alerts", variables.tenantId],
      });
      queryClient.invalidateQueries({
        queryKey: ["alert", variables.tenantId, variables.alertId],
      });
      queryClient.invalidateQueries({
        queryKey: ["alert-stats", variables.tenantId],
      });
    },
  });
}

export function useResolveAlert() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      tenantId,
      alertId,
    }: {
      tenantId: string;
      alertId: string;
    }) => alertsService.resolveAlert(tenantId, alertId),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["alerts", variables.tenantId],
      });
      queryClient.invalidateQueries({
        queryKey: ["alert", variables.tenantId, variables.alertId],
      });
      queryClient.invalidateQueries({
        queryKey: ["alert-stats", variables.tenantId],
      });
    },
  });
}
