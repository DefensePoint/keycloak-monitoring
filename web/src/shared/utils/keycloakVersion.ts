import type {
  KeycloakHealth,
  KeycloakVersionInfo,
} from "@/shared/types/keycloak.types";
import type { Tenant } from "@/shared/types/tenant.types";
import { getHealthAvailability } from "./healthAvailability";

export interface ShownKeycloakVersion {
  /** The version to show, or "" when none is known. */
  version: string;
  /** True when it is the version an operator entered, not Keycloak's own. */
  entered: boolean;
  /** For an entered version, when it was set, as "Nov 20, 2026". */
  setOn?: string;
}

/**
 * The Keycloak version a page shows: the one Keycloak reported, or else the
 * one an operator entered in the tenant settings, which the update check then
 * uses. Keycloak 26.7 and later report theirs only to a master realm
 * administrator. The entered version comes from the update check's result
 * when there is one, and else from the tenant itself, so it shows with the
 * version check turned off and before the first check has run.
 */
export function shownKeycloakVersion(
  health: KeycloakHealth | null | undefined,
  versionInfo: KeycloakVersionInfo | null | undefined,
  tenant?: Pick<Tenant, "keycloak_version" | "keycloak_version_set_at"> | null,
): ShownKeycloakVersion {
  if (getHealthAvailability(health).version && health?.server_version) {
    return { version: health.server_version, entered: false };
  }
  const fromCheck = versionInfo?.version_source === "tenant_settings";
  const entered = (
    fromCheck ? versionInfo?.current_version : tenant?.keycloak_version
  )?.trim();
  if (!entered) {
    return { version: "", entered: false };
  }
  const setAtText = fromCheck
    ? versionInfo?.version_set_at
    : tenant?.keycloak_version_set_at;
  const setAt = setAtText ? new Date(setAtText) : null;
  return {
    version: entered,
    entered: true,
    setOn:
      setAt && !Number.isNaN(setAt.getTime())
        ? setAt.toLocaleDateString("en-US", {
            year: "numeric",
            month: "short",
            day: "numeric",
          })
        : undefined,
  };
}

/** The note shown under an entered version. */
export function enteredVersionNote(shown: ShownKeycloakVersion): string {
  if (!shown.entered) return "";
  return shown.setOn
    ? `Entered in the tenant settings on ${shown.setOn}`
    : "Entered in the tenant settings";
}
