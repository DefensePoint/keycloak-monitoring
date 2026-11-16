import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test-utils";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "./AuthContext";
import {
  authService,
  NotAuthenticatedError,
  ServerUnavailableError,
} from "@/shared/services/authService";
import { ServerUnavailable } from "@/app/routes/ServerUnavailable";
import { LoginPage } from "@/features/auth/pages/LoginPage";
import { useBranding } from "@/shared/hooks/useBranding";

vi.mock("@/shared/services/authService", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/shared/services/authService")>()),
  authService: {
    getAuthConfig: vi.fn(),
    getUserInfo: vi.fn(),
    loginSimple: vi.fn(),
    loginOAuth2: vi.fn(),
    logout: vi.fn(),
  },
}));

const acme = {
  name: "Acme Monitoring Platform",
  short_name: "ACME",
  logo_text: "ACME",
};

function BrandingProbe() {
  const b = useBranding();
  return (
    <span data-testid="branding">{`${b.name}|${b.short_name}|${b.logo_text}`}</span>
  );
}

function renderWithAuth(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <AuthProvider>{ui}</AuthProvider>
    </QueryClientProvider>,
  );
}

describe("branding from the server", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.mocked(authService.getUserInfo).mockRejectedValue(
      new NotAuthenticatedError(),
    );
  });

  it("uses the server's names, remembers them and titles the page", async () => {
    vi.mocked(authService.getAuthConfig).mockResolvedValue({
      simple_enabled: true,
      oauth2_enabled: false,
      auth_required: true,
      branding: acme,
    });
    renderWithAuth(<BrandingProbe />);

    await waitFor(() =>
      expect(screen.getByTestId("branding")).toHaveTextContent(
        "Acme Monitoring Platform|ACME|ACME",
      ),
    );
    expect(document.title).toBe(
      "Acme Monitoring Platform - Security Monitoring Dashboard",
    );
    expect(JSON.parse(localStorage.getItem("kmt:branding") ?? "null")).toEqual(
      acme,
    );
  });

  it("keeps the default names for a server that sends none", async () => {
    vi.mocked(authService.getAuthConfig).mockResolvedValue({
      simple_enabled: true,
      oauth2_enabled: false,
      auth_required: true,
    });
    renderWithAuth(<BrandingProbe />);

    await waitFor(() =>
      expect(document.title).toBe(
        "Keycloak Monitoring Tool - Security Monitoring Dashboard",
      ),
    );
    expect(screen.getByTestId("branding")).toHaveTextContent(
      "Keycloak Monitoring Tool|KMT|Keycloak",
    );
  });

  it("shows the remembered names while the server cannot be reached", async () => {
    localStorage.setItem("kmt:branding", JSON.stringify(acme));
    vi.mocked(authService.getAuthConfig).mockRejectedValue(
      new ServerUnavailableError(),
    );
    renderWithAuth(
      <ServerUnavailable nextRetryAt={null} onRetryNow={() => {}} />,
    );

    expect(
      await screen.findByText("Can't reach the ACME server."),
    ).toBeInTheDocument();
    expect(screen.getByText("ACME")).toBeInTheDocument();
    expect(screen.getByText("MONITORING PLATFORM")).toBeInTheDocument();
  });

  it("leaves the page title alone until the server names the product", async () => {
    document.title = "Security Monitoring Dashboard";
    let answer: (
      config: Awaited<ReturnType<typeof authService.getAuthConfig>>,
    ) => void = () => {};
    vi.mocked(authService.getAuthConfig).mockReturnValue(
      new Promise((resolve) => {
        answer = resolve;
      }),
    );
    renderWithAuth(<BrandingProbe />);

    await new Promise((r) => setTimeout(r, 20));
    expect(document.title).toBe("Security Monitoring Dashboard");

    answer({
      simple_enabled: true,
      oauth2_enabled: false,
      auth_required: true,
      branding: acme,
    });
    await waitFor(() =>
      expect(document.title).toBe(
        "Acme Monitoring Platform - Security Monitoring Dashboard",
      ),
    );
  });

  it("names no product when the server cannot be reached on a first visit", async () => {
    vi.mocked(authService.getAuthConfig).mockRejectedValue(
      new ServerUnavailableError(),
    );
    renderWithAuth(
      <ServerUnavailable nextRetryAt={null} onRetryNow={() => {}} />,
    );

    expect(
      await screen.findByText("Can't reach the server."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Keycloak")).not.toBeInTheDocument();
    expect(screen.queryByText(/KMT|MONITORING TOOL/)).not.toBeInTheDocument();
  });

  it("brands the sign-in page", async () => {
    vi.mocked(authService.getAuthConfig).mockResolvedValue({
      simple_enabled: true,
      oauth2_enabled: false,
      auth_required: true,
      branding: acme,
    });
    renderWithAuth(<LoginPage />);

    expect(await screen.findByText("ACME")).toBeInTheDocument();
    expect(screen.getByText("MONITORING PLATFORM")).toBeInTheDocument();
    expect(screen.queryByText("Keycloak")).not.toBeInTheDocument();
  });
});
