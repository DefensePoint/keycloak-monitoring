import { useEffect, useRef, useState } from "react";
import { Box, Tooltip, Typography } from "@mui/material";
import { httpStatusOf } from "@/shared/lib";

/** One query a page shows data from. */
export interface LiveSource {
  /** What the data is, in the user's words: "events", "alert counts". */
  label: string;
  /** False while the query cannot run, as before a tenant is selected. */
  enabled: boolean;
  /** The query's error. TanStack Query clears it, and its isError, while it
   * refetches a query that has no data, so it can be null after a failure. */
  error: unknown;
  /** When the data shown was fetched, in ms since the epoch; 0 if never. */
  dataUpdatedAt: number;
  /** When the query last failed, in ms since the epoch; 0 if never. Unlike
   * the error, it survives a refetch. */
  errorUpdatedAt: number;
}

/**
 * Data older than this is stale. Every dashboard query refreshes every 5 to
 * 10 seconds, so this is several missed refreshes, not one slow request.
 */
export const STALE_AFTER_MS = 30_000;

export type LiveState = "loading" | "live" | "failing" | "stale";

export interface LiveStatus {
  state: LiveState;
  /** When the oldest data shown was fetched; 0 if nothing is shown yet. */
  oldest: number;
  /** The sources whose last refresh failed. */
  failing: string[];
  /** The sources still without a first answer after STALE_AFTER_MS. */
  waiting: string[];
}

/** lastFailed reports whether the source's last finished request failed. */
function lastFailed(s: LiveSource): boolean {
  return s.errorUpdatedAt > s.dataUpdatedAt;
}

/** unanswered reports whether the source has not finished a request yet. */
function unanswered(s: LiveSource): boolean {
  return s.dataUpdatedAt === 0 && s.errorUpdatedAt === 0;
}

/**
 * liveStatus judges the data a page shows. A source whose last request was
 * refused with 403 is left out: the user may not see that data at all, which
 * no refresh will change, and the page says so where the data would be.
 * Stale data outranks a source still loading, so a request that hangs
 * cannot hide behind one that has not answered yet. A first request that
 * hangs has neither data nor an error, so a source still unanswered
 * STALE_AFTER_MS after waitingSince says it began waiting is stale too.
 */
export function liveStatus(
  sources: LiveSource[],
  now: number,
  waitingSince: ReadonlyMap<string, number> = new Map(),
): LiveStatus {
  const judged = sources.filter(
    (s) => s.enabled && !(lastFailed(s) && httpStatusOf(s.error) === 403),
  );
  const failing = judged.filter(lastFailed).map((s) => s.label);
  const shown = judged.filter((s) => s.dataUpdatedAt > 0);
  const oldest = shown.length
    ? Math.min(...shown.map((s) => s.dataUpdatedAt))
    : 0;
  const pending = judged.filter(unanswered);
  const waiting = pending
    .filter((s) => now - (waitingSince.get(s.label) ?? now) > STALE_AFTER_MS)
    .map((s) => s.label);

  let state: LiveState = "live";
  if (failing.length > 0) {
    state = "failing";
  } else if (
    (shown.length > 0 && now - oldest > STALE_AFTER_MS) ||
    waiting.length > 0
  ) {
    state = "stale";
  } else if (judged.length === 0 || pending.length > 0) {
    // Nothing to judge yet, or a first answer still to come.
    state = "loading";
  }
  return { state, oldest, failing, waiting };
}

/**
 * useLastErrors fills in the error TanStack Query clears while it refetches,
 * with the last one each source had, so a refused source stays refused.
 */
function useLastErrors(sources: LiveSource[]): LiveSource[] {
  const last = useRef(new Map<string, unknown>());
  return sources.map((s) => {
    if (s.error != null) {
      last.current.set(s.label, s.error);
      return s;
    }
    return lastFailed(s) ? { ...s, error: last.current.get(s.label) } : s;
  });
}

/**
 * useWaitingSince records when each source began waiting for its first
 * answer, and forgets it once the source answers, so a source that waits
 * again (a tenant switch starts its queries afresh) gets the full limit.
 */
function useWaitingSince(sources: LiveSource[]): ReadonlyMap<string, number> {
  const since = useRef(new Map<string, number>());
  for (const s of sources) {
    if (s.enabled && unanswered(s)) {
      if (!since.current.has(s.label)) since.current.set(s.label, Date.now());
    } else {
      since.current.delete(s.label);
    }
  }
  return since.current;
}

/** useNow returns the current time, updated every intervalMs. */
function useNow(intervalMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

const LABEL: Record<LiveState, string> = {
  loading: "Loading",
  live: "Live",
  failing: "Not refreshing",
  stale: "Stale",
};

function explain({ state, failing, waiting }: LiveStatus): string {
  switch (state) {
    case "failing":
      return `Could not refresh: ${failing.join(", ")}. "Updated" is the time of the oldest data shown.`;
    case "stale":
      if (waiting.length > 0) {
        return `No answer for over ${STALE_AFTER_MS / 1000} seconds: ${waiting.join(", ")}. "Updated" is the time of the oldest data shown.`;
      }
      return `No new data for over ${STALE_AFTER_MS / 1000} seconds. "Updated" is the time of the oldest data shown.`;
    case "loading":
      return "Loading the dashboard data.";
    default:
      return `All data was refreshed within the last ${STALE_AFTER_MS / 1000} seconds.`;
  }
}

/**
 * LiveIndicator says whether the data on a page is current: a pulsing green
 * "Live" only while every source refreshes, amber when one fails or goes
 * stale. "Updated" is the time of the oldest data shown, so a failing source
 * is not hidden behind one that still refreshes.
 */
export function LiveIndicator({ sources }: { sources: LiveSource[] }) {
  const now = useNow(5000);
  const status = liveStatus(
    useLastErrors(sources),
    now,
    useWaitingSince(sources),
  );
  const { state, oldest } = status;
  const updated = oldest ? new Date(oldest).toLocaleTimeString() : "—";
  const warning = state === "failing" || state === "stale";

  return (
    <Tooltip title={explain(status)}>
      <Box
        role="status"
        data-state={state}
        sx={{ display: "flex", alignItems: "center", gap: 1 }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Box
            sx={{
              width: 8,
              height: 8,
              borderRadius: "50%",
              bgcolor:
                state === "live"
                  ? "success.main"
                  : warning
                    ? "warning.main"
                    : "text.disabled",
              ...(state === "live" && {
                animation: "pulse 2s cubic-bezier(0.4, 0, 0.6, 1) infinite",
                "@keyframes pulse": {
                  "0%, 100%": { opacity: 1 },
                  "50%": { opacity: 0.5 },
                },
              }),
            }}
          />
          <Typography
            variant="caption"
            color={warning ? "warning.main" : "text.secondary"}
          >
            {LABEL[state]}
          </Typography>
        </Box>
        <Typography variant="caption" color="text.secondary">
          •
        </Typography>
        <Typography variant="caption" color="text.secondary">
          Updated {updated}
        </Typography>
      </Box>
    </Tooltip>
  );
}
