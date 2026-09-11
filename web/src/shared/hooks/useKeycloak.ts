import { useQuery } from "@tanstack/react-query";
import { keycloakService } from "@/shared/services";
import type {
  KeycloakDashboard,
  KeycloakDashboardAll,
  UsersResponse,
  ClientsResponse,
} from "@/shared/types";

interface UseKeycloakDashboardOptions {
  tenantId: string | undefined;
  refetchInterval?: number;
}

interface UseRealmDashboardOptions extends UseKeycloakDashboardOptions {
  realm: string | undefined;
}

export function useKeycloakDashboard({
  tenantId,
  refetchInterval = 10000,
}: UseKeycloakDashboardOptions) {
  return useQuery<KeycloakDashboardAll>({
    queryKey: ["keycloak-dashboard", tenantId],
    queryFn: () =>
      keycloakService.getKeycloakDashboard(
        tenantId!,
      ) as Promise<KeycloakDashboardAll>,
    enabled: !!tenantId,
    refetchInterval,
    retry: false,
  });
}

export function useRealmDashboard({
  tenantId,
  realm,
  refetchInterval = 10000,
}: UseRealmDashboardOptions) {
  return useQuery<KeycloakDashboard>({
    queryKey: ["keycloak-realm", tenantId, realm],
    queryFn: () =>
      keycloakService.getKeycloakDashboard(
        tenantId!,
        realm!,
      ) as Promise<KeycloakDashboard>,
    enabled: !!tenantId && !!realm,
    refetchInterval,
    retry: false,
  });
}

interface UseRealmEventsOptions {
  tenantId: string | undefined;
  realmName: string | undefined;
  limit?: number;
  startTime?: string;
  endTime?: string;
  refetchInterval?: number;
}

export function useRealmEvents({
  tenantId,
  realmName,
  limit = 50,
  startTime,
  endTime,
  refetchInterval = 10000,
}: UseRealmEventsOptions) {
  return useQuery({
    queryKey: ["realm-events", tenantId, realmName, startTime, endTime],
    queryFn: () =>
      keycloakService.getRealmEvents(
        tenantId!,
        realmName!,
        limit,
        startTime,
        endTime,
      ),
    refetchInterval,
    enabled: !!realmName && !!tenantId,
  });
}

interface UseRealmUsersOptions {
  tenantId: string | undefined;
  realmName: string | undefined;
  first?: number;
  max?: number;
  refetchInterval?: number;
}

export function useRealmUsers({
  tenantId,
  realmName,
  first = 0,
  max = 100,
  refetchInterval = 30000,
}: UseRealmUsersOptions) {
  return useQuery<UsersResponse>({
    queryKey: ["realm-users", tenantId, realmName],
    queryFn: () =>
      keycloakService.getRealmUsers(tenantId!, realmName!, first, max),
    refetchInterval,
    enabled: !!realmName && !!tenantId,
  });
}

interface UseRealmClientsOptions {
  tenantId: string | undefined;
  realmName: string | undefined;
  refetchInterval?: number;
}

export function useRealmClients({
  tenantId,
  realmName,
  refetchInterval = 30000,
}: UseRealmClientsOptions) {
  return useQuery<ClientsResponse>({
    queryKey: ["realm-clients", tenantId, realmName],
    queryFn: () => keycloakService.getRealmClients(tenantId!, realmName!),
    refetchInterval,
    enabled: !!realmName && !!tenantId,
  });
}
