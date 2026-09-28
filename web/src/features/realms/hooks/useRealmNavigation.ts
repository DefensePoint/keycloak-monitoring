import { useNavigate } from "react-router-dom";

export function useRealmNavigation(tenantId: string | undefined) {
  const navigate = useNavigate();

  const navigateToRealm = (realm: string) => {
    if (tenantId) {
      try {
        sessionStorage.setItem(`selectedRealm_${tenantId}`, realm);
      } catch {
        // Silently fail if sessionStorage is unavailable
      }
    }

    if (realm === "all") {
      navigate(`/${tenantId}/realm`);
    } else {
      navigate(`/${tenantId}/realm/${realm}`);
    }
  };

  return { navigateToRealm };
}
