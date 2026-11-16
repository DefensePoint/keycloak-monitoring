import { ContextType, ReactElement } from "react";
import { render, RenderOptions } from "@testing-library/react";
import { ThemeProvider } from "@mui/material";
import { theme } from "./theme";
import { AuthContext } from "@/shared/context/AuthContext";
import type { Branding } from "@/shared/types/auth.types";

/** Names no part of the UI shows by default, for tests of custom branding. */
export const acmeBranding: Branding = {
  name: "Acme Monitoring Platform",
  short_name: "ACME",
  logo_text: "Acme",
};

/**
 * ui inside an AuthContext that carries only the given branding, which is
 * all useBranding reads. A test that mocks useAuth keeps its mock for the
 * rest.
 */
export function withBranding(
  ui: React.ReactNode,
  branding: Branding = acmeBranding,
): ReactElement {
  const value = { branding, brandingKnown: true } as ContextType<
    typeof AuthContext
  >;
  return <AuthContext.Provider value={value}>{ui}</AuthContext.Provider>;
}

const AllTheProviders = ({ children }: { children: React.ReactNode }) => {
  return <ThemeProvider theme={theme}>{children}</ThemeProvider>;
};

const customRender = (
  ui: ReactElement,
  options?: Omit<RenderOptions, "wrapper">,
) => render(ui, { wrapper: AllTheProviders, ...options });

export * from "@testing-library/react";
export { customRender as render };
