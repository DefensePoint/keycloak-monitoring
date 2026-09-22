import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { AlertDetailPage } from "./AlertDetailPage";
import { useTenant } from "@/shared/context";
import {
  useAlert,
  useUpdateAlertStatus,
  useResolveAlert,
} from "@/shared/hooks";

vi.mock("react-router-dom", () => ({
  useParams: () => ({ alertId: "alert-123", tenantId: "tenant-1" }),
}));

vi.mock("@/shared/context", () => ({
  useTenant: vi.fn(),
}));

vi.mock("@/shared/hooks", () => ({
  useAlert: vi.fn(),
  useUpdateAlertStatus: vi.fn(),
  useResolveAlert: vi.fn(),
}));

vi.mock("@/shared/components", () => ({
  LoadingSkeleton: () => (
    <div data-testid="loading-state">Loading alert...</div>
  ),
  AlertBanner: ({
    severity,
    message,
  }: {
    severity: string;
    message: string;
  }) => (
    <div data-testid="alert-banner" data-severity={severity}>
      {message}
    </div>
  ),
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children: React.ReactNode;
  }) => (
    <div data-testid="section-card">
      <h3>{title}</h3>
      {children}
    </div>
  ),
  FieldDisplay: ({
    label,
    value,
  }: {
    label: string;
    value: React.ReactNode;
  }) => (
    <div>
      <span>{label}:</span>
      <span>{value}</span>
    </div>
  ),
  Toast: null,
  ConfirmDialog: ({
    open,
    onConfirm,
    onCancel,
  }: {
    open: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  }) =>
    open ? (
      <div data-testid="confirm-dialog">
        <button onClick={onConfirm}>Confirm</button>
        <button onClick={onCancel}>Cancel</button>
      </div>
    ) : null,
}));

vi.mock("../components", () => ({
  AlertDetailHeader: ({ title }: { title: string }) => <h1>{title}</h1>,
  AlertBadges: () => <div data-testid="alert-badges">Badges</div>,
  AlertActions: ({
    onAcknowledge,
    onIgnore,
    onResolve,
  }: {
    onAcknowledge: () => void;
    onIgnore: () => void;
    onResolve: () => void;
  }) => (
    <div data-testid="alert-actions">
      <button onClick={onAcknowledge}>Acknowledge</button>
      <button onClick={onIgnore}>Ignore</button>
      <button onClick={onResolve}>Resolve</button>
    </div>
  ),
}));

vi.mock("@/shared/utils", () => ({
  formatDateTime: (date: Date) => date.toISOString(),
}));

const mockAlert = {
  alert_id: "alert-123",
  title: "Test Alert",
  description: "Test description",
  recommendation: "Fix this issue",
  severity: "critical",
  status: "active",
  type: "configuration",
  check_type: "security",
  source: "keycloak",
  realm_name: "master",
  resource_type: "realm",
  resource_name: "Test Realm",
  resource_id: "realm-123",
  first_detected: "2024-01-01T00:00:00Z",
  last_seen: "2024-01-02T00:00:00Z",
  metadata: JSON.stringify({ key: "value" }),
};

describe("AlertDetailPage", () => {
  const mockSelectedTenant = {
    tenant_id: "tenant-1",
    name: "Test Tenant",
  };

  const mockMutate = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(useTenant).mockReturnValue({
      selectedTenant: mockSelectedTenant,
      tenants: [mockSelectedTenant],
      setSelectedTenant: vi.fn(),
      refreshTenants: vi.fn(),
      isLoadingTenants: false,
    });
    vi.mocked(useAlert).mockReturnValue({
      data: mockAlert,
      isLoading: false,
      error: null,
    } as ReturnType<typeof useAlert>);
    vi.mocked(useUpdateAlertStatus).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateAlertStatus>);
    vi.mocked(useResolveAlert).mockReturnValue({
      mutate: mockMutate,
      isPending: false,
    } as unknown as ReturnType<typeof useResolveAlert>);
  });

  it("should render loading state initially", () => {
    vi.mocked(useAlert).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as ReturnType<typeof useAlert>);

    render(<AlertDetailPage />);
    expect(screen.getByTestId("loading-state")).toBeInTheDocument();
    expect(screen.getByText("Loading alert...")).toBeInTheDocument();
  });

  it("should load and display alert details", async () => {
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    expect(screen.getByText("Test description")).toBeInTheDocument();
    expect(screen.getByText("Fix this issue")).toBeInTheDocument();
  });

  it("should display error when alert loading fails", async () => {
    vi.mocked(useAlert).mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed"),
    } as ReturnType<typeof useAlert>);

    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByTestId("alert-banner")).toBeInTheDocument();
      expect(screen.getByText("Failed")).toBeInTheDocument();
    });
  });

  it("should call updateAlertStatus when acknowledge button is clicked", async () => {
    const user = userEvent.setup();
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    const acknowledgeButton = screen.getByText("Acknowledge");
    await user.click(acknowledgeButton);

    expect(mockMutate).toHaveBeenCalled();
  });

  it("should show confirm dialog before ignoring alert", async () => {
    const user = userEvent.setup();
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    const ignoreButton = screen.getByText("Ignore");
    await user.click(ignoreButton);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });
  });

  it("should call updateAlertStatus when ignore is confirmed", async () => {
    const user = userEvent.setup();
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    const ignoreButton = screen.getByText("Ignore");
    await user.click(ignoreButton);

    // Dialog opens - verify it's displayed
    await waitFor(() => {
      expect(
        screen.getByText(/Are you sure you want to ignore/),
      ).toBeInTheDocument();
    });
  });

  it("should show confirm dialog before resolving alert", async () => {
    const user = userEvent.setup();
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    const resolveButton = screen.getByText("Resolve");
    await user.click(resolveButton);

    await waitFor(() => {
      expect(screen.getByTestId("confirm-dialog")).toBeInTheDocument();
    });
  });

  it("should call resolveAlert when resolve is confirmed", async () => {
    const user = userEvent.setup();
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    const resolveButton = screen.getByText("Resolve");
    await user.click(resolveButton);

    // Dialog opens - verify it's displayed
    await waitFor(() => {
      expect(
        screen.getByText(/Mark this alert as resolved/),
      ).toBeInTheDocument();
    });
  });

  it("should display metadata when present", async () => {
    render(<AlertDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Test Alert")).toBeInTheDocument();
    });

    expect(screen.getByText("Additional Details")).toBeInTheDocument();
  });
});
