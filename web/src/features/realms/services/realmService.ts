import { apiClient } from "@/shared/lib/apiClient";
import type {
  KeycloakDashboard,
  KeycloakDashboardAll,
  UsersResponse,
  ClientsResponse,
  UserDetailsResponse,
  EventsResponse,
} from "@/shared/types";

class RealmService {
  async getKeycloakDashboard(
    tenantId: string,
    realm?: string,
  ): Promise<KeycloakDashboard | KeycloakDashboardAll> {
    const params = realm ? { realm } : undefined;
    return apiClient.get<KeycloakDashboard | KeycloakDashboardAll>(
      `/tenants/${tenantId}/keycloak/dashboard`,
      params,
    );
  }

  async getRealmUsers(
    tenantId: string,
    realm: string,
    first: number = 0,
    max: number = 100,
  ): Promise<UsersResponse> {
    return apiClient.get<UsersResponse>(
      `/tenants/${tenantId}/keycloak/realms/users`,
      { realm, first, max },
    );
  }

  async getRealmClients(
    tenantId: string,
    realm: string,
  ): Promise<ClientsResponse> {
    return apiClient.get<ClientsResponse>(
      `/tenants/${tenantId}/keycloak/realms/clients`,
      { realm },
    );
  }

  async getUserDetails(
    tenantId: string,
    realm: string,
    userId: string,
  ): Promise<UserDetailsResponse> {
    return apiClient.get<UserDetailsResponse>(
      `/tenants/${tenantId}/keycloak/realms/users/details`,
      { realm, userId },
    );
  }

  async getRealmEvents(
    tenantId: string,
    realm: string,
    limit: number = 100,
    startTime?: string,
    endTime?: string,
  ): Promise<EventsResponse> {
    const params: Record<string, string | number> = {
      limit,
      // `realm`, not source="keycloak:<realm>": the older encoding matched
      // only the realm's Keycloak rows and dropped its AMFA-mirrored events.
      realm,
    };
    if (startTime) params.start_time = startTime;
    if (endTime) params.end_time = endTime;

    return apiClient.get<EventsResponse>(`/tenants/${tenantId}/events`, params);
  }
}

export const realmService = new RealmService();
