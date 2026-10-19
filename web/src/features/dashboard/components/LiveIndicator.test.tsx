import { describe, it, expect, vi, afterEach } from "vitest";
import { act, render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import {
  LiveIndicator,
  liveStatus,
  STALE_AFTER_MS,
  type LiveSource,
} from "./LiveIndicator";

const NOW = new Date("2026-10-14T09:00:00Z").getTime();

function source(label: string, over: Partial<LiveSource> = {}): LiveSource {
  return {
    label,
    enabled: true,
    error: null,
    dataUpdatedAt: NOW - 2_000,
    errorUpdatedAt: 0,
    ...over,
  };
}

/** The last request failed with error, after any data. */
function failed(error: unknown, over: Partial<LiveSource> = {}) {
  return { error, errorUpdatedAt: NOW - 1_000, ...over };
}

const forbidden = Object.assign(new Error("Insufficient permissions"), {
  status: 403,
});
const serverError = Object.assign(new Error("HTTP 500"), { status: 500 });

describe("liveStatus", () => {
  it("is live while every source has refreshed recently, and dates the oldest data", () => {
    const status = liveStatus(
      [source("a", { dataUpdatedAt: NOW - 9_000 }), source("b")],
      NOW,
    );
    expect(status).toEqual({
      state: "live",
      oldest: NOW - 9_000,
      failing: [],
      waiting: [],
    });
  });

  it("names a failing source, and its old data sets Updated even while others refresh", () => {
    const status = liveStatus(
      [
        source("events", failed(serverError, { dataUpdatedAt: NOW - 60_000 })),
        source("alert counts"),
      ],
      NOW,
    );
    expect(status.state).toBe("failing");
    expect(status.failing).toEqual(["events"]);
    expect(status.oldest).toBe(NOW - 60_000);
  });

  it("is failing when every source fails, as in a full outage", () => {
    const status = liveStatus(
      [
        source("a", failed(new Error("Failed to fetch"))),
        source("b", failed(serverError)),
      ],
      NOW,
    );
    expect(status.state).toBe("failing");
    expect(status.failing).toEqual(["a", "b"]);
  });

  it("is failing when a first load failed, with nothing shown for it", () => {
    const status = liveStatus(
      [source("a", failed(serverError, { dataUpdatedAt: 0 }))],
      NOW,
    );
    expect(status).toEqual({
      state: "failing",
      oldest: 0,
      failing: ["a"],
      waiting: [],
    });
  });

  it("is stale when no request failed but the data stopped arriving", () => {
    expect(
      liveStatus(
        [source("a", { dataUpdatedAt: NOW - STALE_AFTER_MS - 1 })],
        NOW,
      ).state,
    ).toBe("stale");
    expect(
      liveStatus([source("a", { dataUpdatedAt: NOW - STALE_AFTER_MS })], NOW)
        .state,
    ).toBe("live");
  });

  it("is loading while a source has not answered yet", () => {
    expect(
      liveStatus([source("a"), source("b", { dataUpdatedAt: 0 })], NOW).state,
    ).toBe("loading");
  });

  it("leaves out a source the user may not read, and its data", () => {
    const status = liveStatus(
      [
        source("alert counts", failed(forbidden, { dataUpdatedAt: 0 })),
        source("events"),
      ],
      NOW,
    );
    expect(status).toEqual({
      state: "live",
      oldest: NOW - 2_000,
      failing: [],
      waiting: [],
    });
  });

  // A first request that hangs has neither data nor an error, so it never
  // fails and never goes stale; past the stale limit it must stop reading as
  // a page that is still loading.
  it("is stale, naming it, when a source has waited too long for its first answer", () => {
    const waitingSince = new Map([
      ["alert counts", NOW - STALE_AFTER_MS - 1_000],
    ]);
    const status = liveStatus(
      [source("events"), source("alert counts", { dataUpdatedAt: 0 })],
      NOW,
      waitingSince,
    );
    expect(status.state).toBe("stale");
    expect(status.waiting).toEqual(["alert counts"]);
    expect(status.oldest).toBe(NOW - 2_000);
  });

  it("is still loading while a first answer is within the stale limit", () => {
    const waitingSince = new Map([["alert counts", NOW - 5_000]]);
    const status = liveStatus(
      [source("events"), source("alert counts", { dataUpdatedAt: 0 })],
      NOW,
      waitingSince,
    );
    expect(status.state).toBe("loading");
    expect(status.waiting).toEqual([]);
  });

  it("is loading, not live, with nothing to judge", () => {
    expect(liveStatus([source("a", { enabled: false })], NOW).state).toBe(
      "loading",
    );
    expect(
      liveStatus([source("a", failed(forbidden, { dataUpdatedAt: 0 }))], NOW)
        .state,
    ).toBe("loading");
  });
});

describe("LiveIndicator", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows Live with a pulsing green dot when all is fresh", () => {
    render(
      <LiveIndicator sources={[source("a", { dataUpdatedAt: Date.now() })]} />,
    );
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "live");
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("warns and names what failed, with Updated at the oldest data", async () => {
    const old = Date.now() - 60_000;
    render(
      <LiveIndicator
        sources={[
          source("recent events", {
            error: serverError,
            dataUpdatedAt: old,
            errorUpdatedAt: Date.now(),
          }),
          source("alert counts", { dataUpdatedAt: Date.now() }),
        ]}
      />,
    );
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "failing");
    expect(screen.getByText("Not refreshing")).toBeInTheDocument();
    expect(
      screen.getByText(`Updated ${new Date(old).toLocaleTimeString()}`),
    ).toBeInTheDocument();
    await userEvent.hover(screen.getByRole("status"));
    expect(
      await screen.findByText(/Could not refresh: recent events\./),
    ).toBeInTheDocument();
  });

  it("says which source it is still waiting for", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    vi.setSystemTime(NOW);
    render(
      <LiveIndicator
        sources={[
          source("events", { dataUpdatedAt: NOW }),
          source("alert counts", { dataUpdatedAt: 0 }),
        ]}
      />,
    );
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "loading");

    act(() => {
      vi.advanceTimersByTime(STALE_AFTER_MS + 5_000);
    });
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "stale");
    expect(screen.getByRole("status").getAttribute("aria-label") ?? "").toMatch(
      /alert counts/,
    );
  });

  // A tenant switch starts its queries afresh: a source that answered and is
  // waiting again gets the full limit, not the time it waited before.
  it("restarts the wait once a source has answered", () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    vi.setSystemTime(NOW);
    const waiting = [source("a", { dataUpdatedAt: 0 })];
    const { rerender } = render(<LiveIndicator sources={waiting} />);
    act(() => {
      vi.advanceTimersByTime(20_000);
    });
    rerender(
      <LiveIndicator sources={[source("a", { dataUpdatedAt: Date.now() })]} />,
    );
    rerender(<LiveIndicator sources={waiting} />);
    act(() => {
      vi.advanceTimersByTime(20_000);
    });
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "loading");
  });

  it("goes stale on its own when the data stops arriving", () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    vi.setSystemTime(NOW);
    render(<LiveIndicator sources={[source("a", { dataUpdatedAt: NOW })]} />);
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "live");

    act(() => {
      vi.advanceTimersByTime(STALE_AFTER_MS + 5_000);
    });
    expect(screen.getByRole("status")).toHaveAttribute("data-state", "stale");
    expect(screen.getByText("Stale")).toBeInTheDocument();
  });
});

describe("liveStatus across refetches", () => {
  it("keeps a failure while the query refetches, after TanStack cleared its error", () => {
    // A query with no data that refetches is pending again with no error,
    // but errorUpdatedAt still says its last request failed.
    expect(
      liveStatus(
        [
          source("a", {
            error: null,
            dataUpdatedAt: 0,
            errorUpdatedAt: NOW - 1_000,
          }),
        ],
        NOW,
      ).state,
    ).toBe("failing");
  });

  it("puts stale data ahead of a source still loading", () => {
    expect(
      liveStatus(
        [
          source("a", { dataUpdatedAt: NOW - STALE_AFTER_MS - 5_000 }),
          source("b", { dataUpdatedAt: 0 }),
        ],
        NOW,
      ).state,
    ).toBe("stale");
  });

  it("is live again once a failed source succeeds", () => {
    expect(
      liveStatus(
        [
          source("a", {
            error: null,
            dataUpdatedAt: NOW - 1_000,
            errorUpdatedAt: NOW - 9_000,
          }),
        ],
        NOW,
      ).state,
    ).toBe("live");
  });
});

// The same, through a real QueryClient: TanStack Query clears the error and
// isError of a query with no data while it refetches, which mocked query
// results do not show.
describe("LiveIndicator with real queries", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  function Harness({
    fetchA,
    fetchB,
  }: {
    fetchA: () => Promise<unknown>;
    fetchB: () => Promise<unknown>;
  }) {
    const a = useQuery({
      queryKey: ["a"],
      queryFn: fetchA,
      refetchInterval: 1_000,
      retry: false,
    });
    const b = useQuery({
      queryKey: ["b"],
      queryFn: fetchB,
      refetchInterval: 1_000,
      retry: false,
    });
    const sources: LiveSource[] = [
      ["a", a],
      ["b", b],
    ].map(([label, q]) => {
      const query = q as typeof a;
      return {
        label: label as string,
        enabled: true,
        error: query.error,
        dataUpdatedAt: query.dataUpdatedAt,
        errorUpdatedAt: query.errorUpdatedAt,
      };
    });
    return <LiveIndicator sources={sources} />;
  }

  function renderWith(
    fetchA: () => Promise<unknown>,
    fetchB: () => Promise<unknown>,
  ) {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    return render(
      <QueryClientProvider client={client}>
        <Harness fetchA={fetchA} fetchB={fetchB} />
      </QueryClientProvider>,
    );
  }

  const hang = () => new Promise<never>(() => {});
  const state = () => screen.getByRole("status").getAttribute("data-state");

  async function settle(ms: number) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(ms);
    });
  }

  it("leaves out a refused source while its refetch hangs", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: false });
    let calls = 0;
    renderWith(
      async () => ({ ok: true }),
      () => (++calls === 1 ? Promise.reject(forbidden) : hang()),
    );
    await settle(100);
    expect(state()).toBe("live");

    await settle(1_500); // b refetches and hangs: no error, no data
    expect(calls).toBeGreaterThan(1);
    expect(state()).toBe("live");
  });

  it("goes stale, not loading, when requests hang after good data", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: false });
    let calls = 0;
    renderWith(
      () => (++calls === 1 ? Promise.resolve({ ok: true }) : hang()),
      () => Promise.reject(forbidden),
    );
    await settle(100);
    expect(state()).toBe("live");

    await settle(STALE_AFTER_MS + 6_000);
    expect(state()).toBe("stale");
  });

  it("goes stale, not loading for ever, when a first request never answers", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: false });
    let calls = 0;
    renderWith(
      async () => ({ ok: true }),
      () => {
        calls++;
        return hang();
      },
    );
    await settle(100);
    expect(state()).toBe("loading");

    await settle(STALE_AFTER_MS + 6_000);
    // The first request is still in flight: TanStack starts no other.
    expect(calls).toBe(1);
    expect(state()).toBe("stale");
  });

  it("stays failing while a first load that failed refetches", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: false });
    let calls = 0;
    renderWith(
      async () => ({ ok: true }),
      () => (++calls === 1 ? Promise.reject(serverError) : hang()),
    );
    await settle(100);
    expect(state()).toBe("failing");

    await settle(1_500); // b refetches: TanStack clears its error
    expect(calls).toBeGreaterThan(1);
    expect(state()).toBe("failing");
  });
});
