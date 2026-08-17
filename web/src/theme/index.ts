import { createTheme, ThemeOptions } from "@mui/material/styles";
import { palette, defensePointColors } from "./palette";
import { typography } from "./typography";
import { components } from "./components";

const themeOptions: ThemeOptions = {
  palette,
  typography,
  components,
  shape: {
    borderRadius: 4,
  },
  spacing: 8,
};

export const theme = createTheme(themeOptions);

export { defensePointColors };
