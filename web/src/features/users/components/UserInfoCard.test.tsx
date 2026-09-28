import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material";
import { UserInfoCard } from "./UserInfoCard";

const theme = createTheme();

vi.mock("@/shared/components", () => ({
  SectionCard: ({
    title,
    children,
  }: {
    title: string;
    children: React.ReactNode;
  }) => (
    <div>
      <h2>{title}</h2>
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
  LoadingSkeleton: () => <div>Loading user details...</div>,
}));

const renderWithTheme = (component: React.ReactElement) => {
  return render(<ThemeProvider theme={theme}>{component}</ThemeProvider>);
};

describe("UserInfoCard", () => {
  const mockUserDetails = {
    id: "user-123",
    username: "testuser",
    email: "test@example.com",
    firstName: "Test",
    lastName: "User",
    enabled: true,
    emailVerified: true,
    createdTimestamp: 1704110400000,
  };

  const mockUserGroups = [
    { id: "group-1", name: "Admins" },
    { id: "group-2", name: "Users" },
  ];

  const mockRoleMappings = {
    realmMappings: [
      { id: "role-1", name: "admin", description: "Admin role" },
      { id: "role-2", name: "user", description: "User role" },
    ],
    clientMappings: {
      "test-client": {
        client: "test-client",
        mappings: [
          { id: "client-role-1", name: "client-admin", description: "" },
        ],
      },
    },
  };

  const defaultProps = {
    userDetails: mockUserDetails,
    userGroups: mockUserGroups,
    roleMappings: mockRoleMappings,
    loading: false,
  };

  it("should show loading state when loading is true", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} loading={true} />);

    expect(screen.getByText("Loading user details...")).toBeInTheDocument();
  });

  it("should show error message when user details are not available", () => {
    renderWithTheme(
      <UserInfoCard {...defaultProps} userDetails={null} loading={false} />,
    );

    expect(
      screen.getByText(/User not found or unable to load user details/i),
    ).toBeInTheDocument();
  });

  it("should render user information section title", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("User Information")).toBeInTheDocument();
  });

  it("should display username", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Username:")).toBeInTheDocument();
    expect(screen.getByText("testuser")).toBeInTheDocument();
  });

  it("should display email", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Email:")).toBeInTheDocument();
    expect(screen.getByText("test@example.com")).toBeInTheDocument();
  });

  it("should display full name", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Full Name:")).toBeInTheDocument();
    expect(screen.getByText("Test User")).toBeInTheDocument();
  });

  it("should display N/A when full name is not available", () => {
    const userWithoutName = {
      ...mockUserDetails,
      firstName: undefined,
      lastName: undefined,
    };
    renderWithTheme(
      <UserInfoCard {...defaultProps} userDetails={userWithoutName} />,
    );

    expect(screen.getByText("Full Name:")).toBeInTheDocument();
    expect(screen.getAllByText("N/A")).toHaveLength(1);
  });

  it("should display enabled status", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("should display disabled status when user is disabled", () => {
    const disabledUser = { ...mockUserDetails, enabled: false };
    renderWithTheme(
      <UserInfoCard {...defaultProps} userDetails={disabledUser} />,
    );

    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("should display email verified status", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Email Verified")).toBeInTheDocument();
    expect(screen.getByText("Verified")).toBeInTheDocument();
  });

  it("should display not verified status when email is not verified", () => {
    const unverifiedUser = { ...mockUserDetails, emailVerified: false };
    renderWithTheme(
      <UserInfoCard {...defaultProps} userDetails={unverifiedUser} />,
    );

    expect(screen.getByText("Not Verified")).toBeInTheDocument();
  });

  it("should display user ID", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("User ID")).toBeInTheDocument();
    expect(screen.getByText("user-123")).toBeInTheDocument();
  });

  it("should display groups section", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Groups")).toBeInTheDocument();
    expect(screen.getByText("Admins")).toBeInTheDocument();
    expect(screen.getByText("Users")).toBeInTheDocument();
  });

  it("should display no groups message when user has no groups", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} userGroups={[]} />);

    expect(screen.getByText("No groups assigned")).toBeInTheDocument();
  });

  it("should display roles section", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Roles")).toBeInTheDocument();
  });

  it("should display realm roles", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Realm Roles")).toBeInTheDocument();
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("user")).toBeInTheDocument();
  });

  it("should display client roles", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Client Roles")).toBeInTheDocument();
    expect(screen.getByText("test-client")).toBeInTheDocument();
    expect(screen.getByText("client-admin")).toBeInTheDocument();
  });

  it("should display no roles message when user has no roles", () => {
    const noRolesMapping = {
      realmMappings: [],
      clientMappings: {},
    };
    renderWithTheme(
      <UserInfoCard {...defaultProps} roleMappings={noRolesMapping} />,
    );

    expect(screen.getByText("No roles assigned")).toBeInTheDocument();
  });

  it("should display created timestamp", () => {
    renderWithTheme(<UserInfoCard {...defaultProps} />);

    expect(screen.getByText("Created:")).toBeInTheDocument();
    const expectedDate = new Date(1704110400000).toLocaleString();
    expect(screen.getByText(expectedDate)).toBeInTheDocument();
  });

  it("should display N/A for missing created timestamp", () => {
    const userWithoutTimestamp = {
      ...mockUserDetails,
      createdTimestamp: undefined,
    };
    renderWithTheme(
      <UserInfoCard {...defaultProps} userDetails={userWithoutTimestamp} />,
    );

    expect(screen.getByText("Created:")).toBeInTheDocument();
  });
});
