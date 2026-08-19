import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ThemeProvider, createTheme } from "@mui/material";
import { BrowserRouter } from "react-router-dom";
import { RealmPageHeader } from "./RealmPageHeader";

const theme = createTheme();

vi.mock("./RealmSelector", () => ({
  RealmSelector: ({
    selectedRealm,
    onRealmChange,
  }: {
    selectedRealm: string;
    onRealmChange: (realm: string) => void;
  }) => (
    <div>
      <button onClick={() => onRealmChange("realm-2")}>
        Change Realm to {selectedRealm}
      </button>
    </div>
  ),
}));

vi.mock("@/shared/components/TimeSelector", () => ({
  TimeSelector: ({
    onTimeRangeChange,
  }: {
    onTimeRangeChange: (range: { start: number; end: number }) => void;
  }) => (
    <button onClick={() => onTimeRangeChange({ start: 0, end: 100 })}>
      Change Time Range
    </button>
  ),
}));

const renderWithProviders = (component: React.ReactElement) => {
  return render(
    <BrowserRouter>
      <ThemeProvider theme={theme}>{component}</ThemeProvider>
    </BrowserRouter>,
  );
};

describe("RealmPageHeader", () => {
  const mockOnRealmChange = vi.fn();
  const mockOnTimeRangeChange = vi.fn();
  const mockOnLogout = vi.fn();

  const defaultProps = {
    realmName: "test-realm",
    tenantId: "tenant-1",
    selectedRealm: "test-realm",
    allRealms: [{ realm_name: "test-realm" }, { realm_name: "realm-2" }],
    defaultRealm: "master",
    userName: "John Doe",
    userEmail: "john@example.com",
    onRealmChange: mockOnRealmChange,
    onTimeRangeChange: mockOnTimeRangeChange,
    onLogout: mockOnLogout,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render realm name", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByText("test-realm")).toBeInTheDocument();
  });

  it("should render Keycloak Realm label", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByText("Keycloak Realm")).toBeInTheDocument();
  });

  it("should display user name and email", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByText("John Doe")).toBeInTheDocument();
    expect(screen.getByText("john@example.com")).toBeInTheDocument();
  });

  it("should render logout button", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByRole("button", { name: /logout/i })).toBeInTheDocument();
  });

  it("should call onLogout when logout button is clicked", async () => {
    const user = userEvent.setup();
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: /logout/i }));

    expect(mockOnLogout).toHaveBeenCalledTimes(1);
  });

  it("should show default badge when realm is default", () => {
    const propsWithDefault = {
      ...defaultProps,
      realmName: "master",
      defaultRealm: "master",
    };
    renderWithProviders(<RealmPageHeader {...propsWithDefault} />);

    expect(screen.getByText("Default")).toBeInTheDocument();
  });

  it("should not show default badge when realm is not default", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.queryByText("Default")).not.toBeInTheDocument();
  });

  it("should render RealmSelector component", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByText(/Change Realm to/i)).toBeInTheDocument();
  });

  it("should render TimeSelector component", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    expect(screen.getByText("Change Time Range")).toBeInTheDocument();
  });

  it("should not render user info when userName is not provided", () => {
    const propsWithoutUser = {
      ...defaultProps,
      userName: undefined,
      userEmail: undefined,
    };
    renderWithProviders(<RealmPageHeader {...propsWithoutUser} />);

    expect(screen.queryByText("John Doe")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /logout/i }),
    ).not.toBeInTheDocument();
  });

  it("should render logo with link to tenant dashboard", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    const logoLink = screen.getAllByRole("link")[0];
    expect(logoLink).toHaveAttribute("href", "/tenant-1");
  });

  it("should render KMT link to tenant dashboard", () => {
    renderWithProviders(<RealmPageHeader {...defaultProps} />);

    const homeLink = screen.getByText("KMT").closest("a");
    expect(homeLink).toHaveAttribute("href", "/tenant-1");
  });
});
