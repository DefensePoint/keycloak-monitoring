import type { ThemeOptions } from "@mui/material/styles";

export const typography: ThemeOptions["typography"] = {
  fontFamily: "'Inter', system-ui, -apple-system, sans-serif",
  fontWeightLight: 300,
  fontWeightRegular: 400,
  fontWeightMedium: 500,
  fontWeightBold: 700,

  h1: {
    fontSize: "2.5rem",
    fontWeight: 700,
    letterSpacing: "-0.01562em",
    lineHeight: 1.2,
  },
  h2: {
    fontSize: "2rem",
    fontWeight: 700,
    letterSpacing: "-0.00833em",
    lineHeight: 1.3,
  },
  h3: {
    fontSize: "1.75rem",
    fontWeight: 600,
    letterSpacing: "0em",
    lineHeight: 1.4,
  },
  h4: {
    fontSize: "1.5rem",
    fontWeight: 600,
    letterSpacing: "0.00735em",
    lineHeight: 1.4,
  },
  h5: {
    fontSize: "1.25rem",
    fontWeight: 600,
    letterSpacing: "0em",
    lineHeight: 1.5,
  },
  h6: {
    fontSize: "1rem",
    fontWeight: 600,
    letterSpacing: "0.0075em",
    lineHeight: 1.5,
    textTransform: "uppercase",
  },
  subtitle1: {
    fontSize: "1rem",
    fontWeight: 500,
    letterSpacing: "0.00938em",
    lineHeight: 1.75,
  },
  subtitle2: {
    fontSize: "0.875rem",
    fontWeight: 500,
    letterSpacing: "0.00714em",
    lineHeight: 1.57,
    textTransform: "uppercase",
  },
  body1: {
    fontSize: "1rem",
    fontWeight: 400,
    letterSpacing: "0.00938em",
    lineHeight: 1.5,
  },
  body2: {
    fontSize: "0.875rem",
    fontWeight: 400,
    letterSpacing: "0.01071em",
    lineHeight: 1.43,
  },
  button: {
    fontSize: "0.875rem",
    fontWeight: 600,
    letterSpacing: "0.1em",
    textTransform: "uppercase",
  },
  caption: {
    fontSize: "0.75rem",
    fontWeight: 400,
    letterSpacing: "0.03333em",
    lineHeight: 1.66,
  },
  overline: {
    fontSize: "0.75rem",
    fontWeight: 500,
    letterSpacing: "0.15em",
    textTransform: "uppercase",
    lineHeight: 2.66,
  },
};
