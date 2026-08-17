import { apiClient } from "@/shared/lib/apiClient";
import type { UserDetailsResponse } from "../types";

export const userService = {
  getUserDetails: async (
    tenantId: string,
    realmName: string,
    userId: string,
  ): Promise<UserDetailsResponse> => {
    return apiClient.get<UserDetailsResponse>(
      `/tenants/${tenantId}/realms/${realmName}/users/${userId}`,
    );
  },
};
