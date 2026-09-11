import { PaletteOptions } from "@mui/material/styles";

export const defensePointColors = {
  // Primary Red (Brand)
  red: {
    primary: "#DB2833",
    dark: "#b01f28",
    light: "#e63946",
  },
  // Backgrounds
  background: {
    primary: "#0E0E0E",
    secondary: "#1a1a1a",
    elevated: "#212121",
    card: "#1a1a1a",
  },
  // Borders
  border: {
    primary: "#2a2a2a",
    elevated: "#333333",
    accent: "#DB2833",
  },
  // Text
  text: {
    primary: "#ffffff",
    secondary: "#a0a0a0",
    muted: "#707070",
  },
  // Accent/Status colors
  accent: {
    success: "#10b981",
    warning: "#f59e0b",
    error: "#DB2833",
    info: "#60a5fa",
  },
  // Severity levels
  severity: {
    critical: "#7f1d1d",
    high: "#c2410c",
    medium: "#854d0e",
    low: "#1e40af",
    info: "#0e7490",
  },
} as const;

export const palette: PaletteOptions = {
  mode: "dark",
  primary: {
    main: defensePointColors.red.primary,
    dark: defensePointColors.red.dark,
    light: defensePointColors.red.light,
    contrastText: "#ffffff",
  },
  secondary: {
    main: defensePointColors.background.elevated,
    dark: defensePointColors.background.secondary,
    light: defensePointColors.border.elevated,
    contrastText: "#ffffff",
  },
  error: {
    main: defensePointColors.accent.error,
    light: defensePointColors.red.light,
    dark: defensePointColors.red.dark,
  },
  warning: {
    main: defensePointColors.accent.warning,
  },
  info: {
    main: defensePointColors.accent.info,
  },
  success: {
    main: defensePointColors.accent.success,
  },
  background: {
    default: defensePointColors.background.primary,
    paper: defensePointColors.background.elevated,
  },
  text: {
    primary: defensePointColors.text.primary,
    secondary: defensePointColors.text.secondary,
    disabled: defensePointColors.text.muted,
  },
  divider: defensePointColors.border.primary,
  action: {
    active: defensePointColors.text.primary,
    hover: "rgba(219, 40, 51, 0.08)",
    selected: "rgba(219, 40, 51, 0.16)",
    disabled: defensePointColors.text.muted,
    disabledBackground: defensePointColors.background.secondary,
  },
};
