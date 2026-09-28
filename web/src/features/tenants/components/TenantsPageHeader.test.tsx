import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@/test-utils";
import userEvent from "@testing-library/user-event";
import { TenantsPageHeader } from "./TenantsPageHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenants" }),
  useParams: () => ({}),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => ({
    selectedTenant: null,
  }),
}));

describe("TenantsPageHeader", () => {
  const mockOnAddClick = vi.fn();

  const defaultProps = {
    onAddClick: mockOnAddClick,
    tenantsCount: 5,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render header title", () => {
    render(<TenantsPageHeader {...defaultProps} />);

    expect(screen.getByText("Tenant Management")).toBeInTheDocument();
  });

  it("should render add tenant button", () => {
    render(<TenantsPageHeader {...defaultProps} />);

    expect(
      screen.getByRole("button", { name: /add tenant/i }),
    ).toBeInTheDocument();
  });

  it("should call onAddClick when add button is clicked", async () => {
    const user = userEvent.setup();
    render(<TenantsPageHeader {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /add tenant/i }));

    expect(mockOnAddClick).toHaveBeenCalledTimes(1);
  });

  it("should render title with correct styling", () => {
    render(<TenantsPageHeader {...defaultProps} />);

    const title = screen.getByText("Tenant Management");
    expect(title).toHaveClass("MuiTypography-h3");
  });
});
