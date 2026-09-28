import React from "react";
import {
  ThemeProvider as MuiThemeProvider,
  StyledEngineProvider,
} from "@mui/material/styles";
import CssBaseline from "@mui/material/CssBaseline";
import { theme } from "./index";

interface DefensePointThemeProviderProps {
  readonly children: React.ReactNode;
}

/**
 * DefensePoint Theme Provider
 *
 * Wraps the application with MUI theme configuration.
 * StyledEngineProvider with injectFirst ensures Tailwind CSS classes
 * take precedence over MUI's default styles when both are applied.
 */
export const DefensePointThemeProvider: React.FC<
  DefensePointThemeProviderProps
> = ({ children }) => {
  return (
    <StyledEngineProvider injectFirst>
      <MuiThemeProvider theme={theme}>
        <CssBaseline />
        {children}
      </MuiThemeProvider>
    </StyledEngineProvider>
  );
};
