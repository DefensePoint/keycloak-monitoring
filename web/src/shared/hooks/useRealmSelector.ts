import { useState, useMemo } from "react";

interface UseRealmSelectorProps {
  tenantId: string | undefined;
  defaultRealm: string | undefined;
  availableRealms: Array<{ realm_name: string }>;
}

function getInitialRealm(
  tenantId: string | undefined,
  defaultRealm: string | undefined,
  availableRealms: Array<{ realm_name: string }>,
): string {
  if (!tenantId || availableRealms.length === 0) return "all";

  // Try sessionStorage first
  try {
    const sessionRealm = sessionStorage.getItem(`selectedRealm_${tenantId}`);
    if (sessionRealm === "all") return "all";
    if (
      sessionRealm &&
      availableRealms.some((r) => r.realm_name === sessionRealm)
    ) {
      return sessionRealm;
    }
  } catch {
    // Ignore sessionStorage errors
  }

  // Fallback to default realm
  if (
    defaultRealm &&
    availableRealms.some((r) => r.realm_name === defaultRealm)
  ) {
    return defaultRealm;
  }

  return "all";
}

export function useRealmSelector({
  tenantId,
  defaultRealm,
  availableRealms,
}: UseRealmSelectorProps) {
  const initialRealm = useMemo(
    () => getInitialRealm(tenantId, defaultRealm, availableRealms),
    [tenantId, defaultRealm, availableRealms],
  );

  const [selectedRealm, setSelectedRealm] = useState<string>(initialRealm);

  const handleRealmChange = (realm: string) => {
    setSelectedRealm(realm);
    if (tenantId) {
      try {
        sessionStorage.setItem(`selectedRealm_${tenantId}`, realm);
      } catch {
        // Silently fail if sessionStorage is unavailable
      }
    }
  };

  return {
    selectedRealm,
    handleRealmChange,
  };
}
