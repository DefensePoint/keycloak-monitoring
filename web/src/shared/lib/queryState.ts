/** The parts of a TanStack Query result requestFailed reads. */
export interface QueryOutcome {
  isError?: boolean;
  dataUpdatedAt?: number;
  errorUpdatedAt?: number;
}

/**
 * requestFailed reports whether a query's data should be shown as
 * unavailable: it is in error, or its last finished request failed.
 *
 * isError alone is not enough. TanStack Query clears error and isError each
 * time it refetches a query that has never loaded, so after a failed first
 * load isError is false for as long as every retry is in flight, and a
 * display reading it alone falls back to its empty value, such as a 0 behind
 * a green dot. errorUpdatedAt survives the refetch.
 */
export function requestFailed(q: QueryOutcome): boolean {
  return !!q.isError || (q.errorUpdatedAt ?? 0) > (q.dataUpdatedAt ?? 0);
}
