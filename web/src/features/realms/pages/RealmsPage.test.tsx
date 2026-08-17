import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { RealmsPage } from "./RealmsPage";
import { useTenant } from "@/shared/context";

const mockUseKeycloakDashboard = vi.fn();
const mockUseNavigate = vi.fn();

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/realms" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  useNavigate: () => mockUseNavigate,
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("@/shared/hooks", () => ({
  useRealmSelector: () => ({
    selectedRealm: "all",
    handleRealmChange: vi.fn(),
  }),
  useKeycloakDashboard: () => mockUseKeycloakDashboard(),
}));

vi.mock("../components", () => ({
  RealmsPageHeader: ({
    onRealmChange,
  }: {
    onRealmChange: (realm: string) => void;
  }) => (
    <div data-testid="realms-header">
      <button onClick={() => onRealmChange("master")}>Select Master</button>
    </div>
  ),
  RealmsFilters: ({ onReset }: { onReset: () => void }) => (
    <div data-testid="realms-filters">
      <button onClick={onReset}>Reset Filters</button>
    </div>
  ),
  RealmsGrid: ({
    realms,
    onRealmClick,
    loading,
  }: {
    realms: unknown[];
    onRealmClick: (realmName: string) => void;
    loading: boolean;
  }) => (
    <div data-testid="realms-grid">
      {loading ? (
        <div>Loading...</div>
      ) : (
        realms.map((realm: { realm_name: string }) => (
          <div
            key={realm.realm_name}
            onClick={() => onRealmClick(realm.realm_name)}
          >
            {realm.realm_name}
          </div>
        ))
      )}
    </div>
  ),
}));

const mockRealms = [
  {
    realm_name: "master",
    enabled: true,
    is_healthy: true,
    events_enabled: true,
    events_listeners: ["event-listener"],
  },
  {
    realm_name: "test",
    enabled: true,
    is_healthy: false,
    events_enabled: false,
    events_listeners: [],
  },
  {
    realm_name: "disabled",
    enabled: false,
    is_healthy: false,
    events_enabled: true,
    events_listeners: ["event-listener"],
  },
];

describe("RealmsPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
    default_realm: "master",
  };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      selectTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoading: false,
      error: null,
    });

    mockUseKeycloakDashboard.mockReturnValue({
      data: { realms: mockRealms },
      isLoading: false,
      isError: false,
    });
  });

  it("should load and display realms", async () => {
    render(<RealmsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("realms-grid")).toBeInTheDocument();
    });

    expect(screen.getByText("master")).toBeInTheDocument();
    expect(screen.getByText("test")).toBeInTheDocument();
    expect(screen.getByText("disabled")).toBeInTheDocument();
  });

  it("should display realms header", async () => {
    render(<RealmsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("realms-header")).toBeInTheDocument();
    });
  });

  it("should display results count", async () => {
    render(<RealmsPage />);

    await waitFor(() => {
      expect(screen.getByText(/3 realms/)).toBeInTheDocument();
    });
  });

  it("should navigate to realm page when realm is clicked", async () => {
    const user = userEvent.setup();
    render(<RealmsPage />);

    await waitFor(() => {
      expect(screen.getByText("master")).toBeInTheDocument();
    });

    const masterRealm = screen.getByText("master");
    await user.click(masterRealm);

    expect(mockUseNavigate).toHaveBeenCalledWith("/tenant-1/realm/master");
  });

  it("should show loading state in grid while loading", () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: null,
      isLoading: true,
      isError: false,
    });

    render(<RealmsPage />);

    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("should handle empty realms list", () => {
    mockUseKeycloakDashboard.mockReturnValue({
      data: { realms: [] },
      isLoading: false,
      isError: false,
    });

    render(<RealmsPage />);

    // When there are no realms, the results count is not shown
    expect(screen.queryByText(/realms/)).not.toBeInTheDocument();
    expect(screen.getByTestId("realms-grid")).toBeInTheDocument();
  });
});
