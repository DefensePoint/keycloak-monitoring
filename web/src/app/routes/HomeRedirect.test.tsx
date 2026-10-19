import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@/test-utils";
import { HomeRedirect } from "./HomeRedirect";

const mockNavigate = vi.fn();
const mockUseTenant = vi.fn();
const mockHasAccessToTenant = vi.fn();

vi.mock("react-router-dom", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => mockUseTenant(),
  useAuth: () => ({ hasAccessToTenant: mockHasAccessToTenant }),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => <div>Redirecting...</div>,
}));

const tenants = [{ tenant_id: "tenant-1" }, { tenant_id: "tenant-2" }];

// The landing page sends a user to the first tenant the same check that
// guards each tenant's pages lets them into, so a user with a global role
// lands on a tenant rather than on the admin-only tenant list.
describe("HomeRedirect", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseTenant.mockReturnValue({ tenants, isLoading: false, error: null });
  });

  it("opens the first tenant the user may access", async () => {
    mockHasAccessToTenant.mockImplementation((id: string) => id === "tenant-2");
    render(<HomeRedirect />);
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/tenant-2", { replace: true }),
    );
  });

  it("opens the first tenant for a user who may access them all", async () => {
    mockHasAccessToTenant.mockReturnValue(true);
    render(<HomeRedirect />);
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/tenant-1", { replace: true }),
    );
  });

  it("shows the tenant list when the user may access none", async () => {
    mockHasAccessToTenant.mockReturnValue(false);
    render(<HomeRedirect />);
    await waitFor(() =>
      expect(mockNavigate).toHaveBeenCalledWith("/tenants", { replace: true }),
    );
  });
});
