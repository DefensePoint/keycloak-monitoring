import type { Tenant } from "@/shared/types";

// Values that mean "the platform could not reach/authenticate this tenant's
// Keycloak". "healthy" and "unknown" (not yet checked) are NOT broken, so a
// freshly-created tenant does not flash a false connection error.
const BROKEN_STATUSES = new Set(["unhealthy", "degraded", "down", "error"]);

/**
 * True when the tenant's Keycloak connection is broken (unreachable or auth
 * failed), as opposed to healthy-but-empty.
 */
export function isTenantConnectionBroken(tenant?: Tenant | null): boolean {
  if (!tenant) return false;
  return BROKEN_STATUSES.has((tenant.health_status ?? "").toLowerCase());
}

/**
 * The human-readable connection error detail for a broken tenant, if any.
 */
export function tenantConnectionError(tenant?: Tenant | null): string | undefined {
  if (!tenant) return undefined;
  return tenant.last_error || tenant.health_message || undefined;
}
