import { describe, expect, it } from "vitest";
import { neverRead, unreadableRealmText } from "./unreadable";

describe("unreadableRealmText", () => {
  it("is undefined for a realm that can be read", () => {
    expect(
      unreadableRealmText(undefined, "2026-10-14T10:00:00Z"),
    ).toBeUndefined();
  });

  it("gives the last read time of a realm read before", () => {
    const text = unreadableRealmText(
      "2026-10-14T10:05:00Z",
      "2026-10-14T10:00:00Z",
      "KMT",
    );
    expect(text).toContain(
      `last read at ${new Date("2026-10-14T10:00:00Z").toLocaleString()}`,
    );
  });

  it("says a realm never read has not been read", () => {
    const text = unreadableRealmText("2026-10-14T10:05:00Z", undefined, "KMT");
    expect(text).toMatch(/^KMT cannot read this realm's settings since /);
    expect(text).toContain("It has not been read yet");
    expect(text).not.toContain("read earlier");
  });
});

describe("neverRead", () => {
  it("is true only for an unreadable realm with no read time", () => {
    expect(neverRead({ unreadable_since: "2026-10-14T10:05:00Z" })).toBe(true);
    expect(
      neverRead({
        unreadable_since: "2026-10-14T10:05:00Z",
        last_checked: "2026-10-14T10:00:00Z",
      }),
    ).toBe(false);
    expect(neverRead({ last_checked: "2026-10-14T10:00:00Z" })).toBe(false);
    expect(neverRead({})).toBe(false);
  });
});
