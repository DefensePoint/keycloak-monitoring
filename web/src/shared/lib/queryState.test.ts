import { describe, expect, it } from "vitest";
import { requestFailed } from "./queryState";

describe("requestFailed", () => {
  it("is true while the query is in error", () => {
    expect(requestFailed({ isError: true })).toBe(true);
  });

  // A refetch of a query that never loaded clears isError; errorUpdatedAt
  // still says the last finished request failed.
  it("is true while a failed first load is retried", () => {
    expect(
      requestFailed({
        isError: false,
        dataUpdatedAt: 0,
        errorUpdatedAt: 1_000,
      }),
    ).toBe(true);
  });

  it("is false once a request succeeds after a failure", () => {
    expect(
      requestFailed({
        isError: false,
        dataUpdatedAt: 2_000,
        errorUpdatedAt: 1_000,
      }),
    ).toBe(false);
  });

  it("is false for a query that has not finished a request yet", () => {
    expect(
      requestFailed({ isError: false, dataUpdatedAt: 0, errorUpdatedAt: 0 }),
    ).toBe(false);
  });
});
