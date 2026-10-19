/**
 * Explains a realm the monitoring client can no longer read: Keycloak still
 * answers for it, but without its settings, so what KMT shows is what it
 * last read. Returns undefined for a realm it can read.
 */
export function unreadableRealmText(
  unreadableSince?: string | null,
  lastChecked?: string,
): string | undefined {
  if (!unreadableSince) return undefined;
  const since = new Date(unreadableSince).toLocaleString();
  const read = lastChecked ? new Date(lastChecked).toLocaleString() : undefined;
  return (
    `KMT cannot read this realm's settings since ${since}: the monitoring client lacks access to it in Keycloak. ` +
    (read
      ? `What is shown was last read at ${read}.`
      : "What is shown was read earlier.")
  );
}
