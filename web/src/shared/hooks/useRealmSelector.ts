import { useState } from "react";

interface UseRealmSelectorProps {
  tenantId: string | undefined;
  defaultRealm: string | undefined;
  availableRealms: Array<{ realm_name: string }>;
}

interface RealmChoice {
  tenantId: string | undefined;
  realm: string;
}

function readSavedRealm(tenantId: string): string | null {
  try {
    return sessionStorage.getItem(`selectedRealm_${tenantId}`);
  } catch {
    return null;
  }
}

function wantedRealm(
  choice: RealmChoice | undefined,
  tenantId: string | undefined,
  defaultRealm: string | undefined,
): string | undefined {
  if (choice && choice.tenantId === tenantId) return choice.realm;
  if (!tenantId) return undefined;
  return readSavedRealm(tenantId) ?? defaultRealm;
}

export function useRealmSelector({
  tenantId,
  defaultRealm,
  availableRealms,
}: UseRealmSelectorProps) {
  const [choice, setChoice] = useState<RealmChoice>();

  const wanted = wantedRealm(choice, tenantId, defaultRealm);
  const selectedRealm =
    wanted && availableRealms.some((r) => r.realm_name === wanted)
      ? wanted
      : "all";

  const handleRealmChange = (realm: string) => {
    setChoice({ tenantId, realm });
    if (tenantId) {
      try {
        sessionStorage.setItem(`selectedRealm_${tenantId}`, realm);
      } catch {
        return;
      }
    }
  };

  return {
    selectedRealm,
    handleRealmChange,
  };
}
