import { useQuery } from "@tanstack/react-query";
import { userService } from "../services";
import type { UserDetailsResponse } from "../types";

interface UseUserDetailsParams {
  tenantId: string | undefined;
  realmName: string | undefined;
  userId: string | undefined;
}

export function useUserDetails({
  tenantId,
  realmName,
  userId,
}: UseUserDetailsParams) {
  return useQuery<UserDetailsResponse>({
    queryKey: ["user-details", tenantId, realmName, userId],
    queryFn: () => userService.getUserDetails(tenantId!, realmName!, userId!),
    enabled: !!tenantId && !!realmName && !!userId,
  });
}
