import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useGenerateReport } from "./useGenerateReport";
import { dashboardService } from "../services";
import { withBranding } from "@/test-utils";

vi.mock("../services", () => ({
  dashboardService: { generateMonthlyReport: vi.fn() },
}));

const params = {
  tenantId: "t1",
  tenantName: "tenant-a",
  startDate: "2026-01-01",
  endDate: "2026-01-31",
};

/** Runs the download and returns the name the file was saved under. */
async function downloadedFileName(branded: boolean): Promise<string> {
  let name = "";
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    name = this.download;
  });
  const client = new QueryClient();
  const { result } = renderHook(() => useGenerateReport(), {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        {branded ? withBranding(children) : children}
      </QueryClientProvider>
    ),
  });
  await act(() => result.current.mutateAsync(params));
  return name;
}

describe("useGenerateReport", () => {
  beforeEach(() => {
    vi.mocked(dashboardService.generateMonthlyReport).mockResolvedValue(
      new Blob(["%PDF-1.4"]),
    );
    window.URL.createObjectURL = vi.fn(() => "blob:report");
    window.URL.revokeObjectURL = vi.fn();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("names the file after the default short name", async () => {
    expect(await downloadedFileName(false)).toBe(
      "kmt-report-tenant-a-2026-01-01-to-2026-01-31.pdf",
    );
  });

  it("names the file after the deployment's own short name", async () => {
    expect(await downloadedFileName(true)).toBe(
      "acme-report-tenant-a-2026-01-01-to-2026-01-31.pdf",
    );
  });
});
