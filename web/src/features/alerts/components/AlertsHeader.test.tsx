import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@/test-utils";
import { AlertsHeader } from "./AlertsHeader";

vi.mock("react-router-dom", () => ({
  useLocation: () => ({ pathname: "/tenant-1/alerts" }),
  useParams: () => ({ tenantId: "tenant-1" }),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

vi.mock("@/shared/context", () => ({
  useTenant: () => ({
    selectedTenant: { tenant_id: "tenant-1", name: "Test Tenant" },
  }),
}));

describe("AlertsHeader", () => {
  it("should render header with title and description", () => {
    render(<AlertsHeader />);

    expect(screen.getByText("Configuration Alerts")).toBeInTheDocument();
    expect(
      screen.getByText("Monitor and manage Keycloak configuration issues"),
    ).toBeInTheDocument();
  });

  it("should render as a heading element", () => {
    render(<AlertsHeader />);

    const heading = screen.getByRole("heading", {
      name: /configuration alerts/i,
    });
    expect(heading).toBeInTheDocument();
  });
});
