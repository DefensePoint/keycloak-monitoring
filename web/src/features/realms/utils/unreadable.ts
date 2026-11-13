import { DEFAULT_BRANDING } from "@/shared/lib/branding";

/**
 * Explains a realm the monitoring client can no longer read: Keycloak still
 * answers for it, but without its settings, so what the product (named by
 * its short name) shows is what it last read. Returns undefined for a realm
 * it can read.
 */
export function unreadableRealmText(
  unreadableSince?: string | null,
  lastChecked?: string,
  shortName: string = DEFAULT_BRANDING.short_name,
): string | undefined {
  if (!unreadableSince) return undefined;
  const since = new Date(unreadableSince).toLocaleString();
  const read = lastChecked ? new Date(lastChecked).toLocaleString() : undefined;
  return (
    `${shortName} cannot read this realm's settings since ${since}: the monitoring client lacks access to it in Keycloak. ` +
    (read
      ? `What is shown was last read at ${read}.`
      : "What is shown was read earlier.")
  );
}
