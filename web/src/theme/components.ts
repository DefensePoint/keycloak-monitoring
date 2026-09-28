import { Components, Theme } from "@mui/material/styles";
import { defensePointColors } from "./palette";

export const components: Components<Omit<Theme, "components">> = {
  MuiCssBaseline: {
    styleOverrides: {
      body: {
        scrollbarColor: `${defensePointColors.border.elevated} ${defensePointColors.background.primary}`,
        "&::-webkit-scrollbar, & *::-webkit-scrollbar": {
          width: 8,
          height: 8,
        },
        "&::-webkit-scrollbar-track, & *::-webkit-scrollbar-track": {
          background: defensePointColors.background.primary,
        },
        "&::-webkit-scrollbar-thumb, & *::-webkit-scrollbar-thumb": {
          backgroundColor: defensePointColors.border.elevated,
          borderRadius: 4,
        },
      },
    },
  },
  MuiButton: {
    defaultProps: {
      disableElevation: true,
    },
    styleOverrides: {
      root: {
        borderRadius: 4,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        fontWeight: 600,
        padding: "12px 16px",
      },
      containedPrimary: {
        backgroundColor: defensePointColors.red.primary,
        "&:hover": {
          backgroundColor: defensePointColors.red.dark,
        },
      },
      outlined: {
        borderColor: defensePointColors.border.primary,
        "&:hover": {
          borderColor: defensePointColors.border.elevated,
          backgroundColor: defensePointColors.background.secondary,
        },
      },
      text: {
        "&:hover": {
          backgroundColor: "rgba(219, 40, 51, 0.08)",
        },
      },
    },
  },
  MuiTextField: {
    defaultProps: {
      variant: "outlined",
      size: "small",
      InputLabelProps: {
        sx: {
          textTransform: "uppercase",
          letterSpacing: "0.05em",
          fontSize: "0.875rem",
          fontWeight: 500,
        },
      },
    },
    styleOverrides: {
      root: {
        "& .MuiOutlinedInput-root": {
          backgroundColor: defensePointColors.background.secondary,
          "& fieldset": {
            borderColor: defensePointColors.border.primary,
          },
          "&:hover fieldset": {
            borderColor: defensePointColors.border.elevated,
          },
          "&.Mui-focused fieldset": {
            borderColor: defensePointColors.red.primary,
          },
          // Date picker input styling
          '& input[type="date"]': {
            colorScheme: "dark",
            "&::-webkit-calendar-picker-indicator": {
              filter: "invert(1)",
              cursor: "pointer",
              opacity: 0.7,
              "&:hover": {
                opacity: 1,
              },
            },
          },
        },
      },
    },
  },
  MuiCard: {
    styleOverrides: {
      root: {
        backgroundColor: defensePointColors.background.elevated,
        border: `1px solid ${defensePointColors.border.primary}`,
        borderRadius: 8,
      },
    },
  },
  MuiPaper: {
    styleOverrides: {
      root: {
        backgroundImage: "none",
      },
      elevation1: {
        boxShadow: "0 0 30px rgba(0, 0, 0, 0.8)",
      },
    },
  },
  MuiDialog: {
    styleOverrides: {
      paper: {
        backgroundColor: defensePointColors.background.elevated,
        border: `1px solid ${defensePointColors.border.primary}`,
      },
    },
  },
  MuiTableCell: {
    styleOverrides: {
      root: {
        borderBottom: `1px solid ${defensePointColors.border.primary}`,
      },
      head: {
        backgroundColor: defensePointColors.background.secondary,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        fontSize: "0.75rem",
      },
    },
  },
  MuiTableRow: {
    styleOverrides: {
      root: {
        "&:hover": {
          backgroundColor: defensePointColors.background.secondary,
        },
      },
    },
  },
  MuiChip: {
    styleOverrides: {
      root: {
        borderRadius: 4,
        fontWeight: 700,
        letterSpacing: "0.05em",
        textTransform: "uppercase",
      },
    },
  },
  MuiTooltip: {
    styleOverrides: {
      tooltip: {
        backgroundColor: defensePointColors.background.elevated,
        border: `1px solid ${defensePointColors.border.primary}`,
        fontSize: "0.75rem",
      },
    },
  },
  MuiMenu: {
    styleOverrides: {
      paper: {
        backgroundColor: defensePointColors.background.elevated,
        border: `1px solid ${defensePointColors.border.primary}`,
      },
    },
  },
  MuiMenuItem: {
    styleOverrides: {
      root: {
        "&:hover": {
          backgroundColor: defensePointColors.background.secondary,
        },
        "&.Mui-selected": {
          backgroundColor: "rgba(219, 40, 51, 0.16)",
          "&:hover": {
            backgroundColor: "rgba(219, 40, 51, 0.24)",
          },
        },
      },
    },
  },
  MuiSwitch: {
    styleOverrides: {
      root: {
        "& .MuiSwitch-switchBase.Mui-checked": {
          color: defensePointColors.red.primary,
          "& + .MuiSwitch-track": {
            backgroundColor: defensePointColors.red.primary,
          },
        },
      },
    },
  },
  MuiTabs: {
    styleOverrides: {
      indicator: {
        backgroundColor: defensePointColors.red.primary,
      },
    },
  },
  MuiTab: {
    styleOverrides: {
      root: {
        textTransform: "uppercase",
        letterSpacing: "0.1em",
        fontWeight: 500,
        "&.Mui-selected": {
          color: defensePointColors.red.primary,
        },
      },
    },
  },
  MuiAlert: {
    styleOverrides: {
      root: {
        borderRadius: 4,
      },
      standardError: {
        backgroundColor: "rgba(219, 40, 51, 0.2)",
        border: "1px solid rgba(219, 40, 51, 0.5)",
      },
      standardSuccess: {
        backgroundColor: "rgba(16, 185, 129, 0.2)",
        border: "1px solid rgba(16, 185, 129, 0.5)",
      },
      standardWarning: {
        backgroundColor: "rgba(245, 158, 11, 0.2)",
        border: "1px solid rgba(245, 158, 11, 0.5)",
      },
      standardInfo: {
        backgroundColor: "rgba(96, 165, 250, 0.2)",
        border: "1px solid rgba(96, 165, 250, 0.5)",
      },
    },
  },
  MuiLinearProgress: {
    styleOverrides: {
      root: {
        backgroundColor: defensePointColors.background.secondary,
        borderRadius: 4,
      },
    },
  },
  MuiSkeleton: {
    styleOverrides: {
      root: {
        backgroundColor: defensePointColors.background.secondary,
      },
    },
  },
  MuiPopover: {
    styleOverrides: {
      paper: {
        backgroundColor: defensePointColors.background.elevated,
        border: `1px solid ${defensePointColors.border.primary}`,
        boxShadow: "0 0 30px rgba(0, 0, 0, 0.8)",
      },
    },
  },
  MuiList: {
    styleOverrides: {
      root: {
        padding: 8,
      },
    },
  },
  MuiListItemButton: {
    styleOverrides: {
      root: {
        borderRadius: 4,
        marginBottom: 4,
        "&:hover": {
          backgroundColor: defensePointColors.background.secondary,
        },
        "&.Mui-selected": {
          backgroundColor: defensePointColors.background.secondary,
          color: defensePointColors.red.primary,
          fontWeight: 500,
          borderLeft: `2px solid ${defensePointColors.red.primary}`,
          "&:hover": {
            backgroundColor: defensePointColors.border.primary,
          },
        },
      },
    },
  },
  MuiListItemIcon: {
    styleOverrides: {
      root: {
        minWidth: 40,
      },
    },
  },
  MuiListItemText: {
    styleOverrides: {
      primary: {
        fontWeight: 500,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
      },
    },
  },
  MuiAccordion: {
    styleOverrides: {
      root: {
        backgroundColor: "transparent",
        boxShadow: "none",
        "&:before": {
          display: "none",
        },
        "&.Mui-expanded": {
          margin: 0,
        },
      },
    },
  },
  MuiAccordionSummary: {
    styleOverrides: {
      root: {
        padding: 0,
        minHeight: 40,
        "&.Mui-expanded": {
          minHeight: 40,
        },
      },
      content: {
        margin: "8px 0",
        "&.Mui-expanded": {
          margin: "8px 0",
        },
      },
    },
  },
  MuiAccordionDetails: {
    styleOverrides: {
      root: {
        padding: 0,
        paddingTop: 8,
      },
    },
  },
  MuiIconButton: {
    styleOverrides: {
      root: {
        color: defensePointColors.text.secondary,
        "&:hover": {
          backgroundColor: defensePointColors.background.secondary,
          color: defensePointColors.text.primary,
        },
      },
    },
  },
  MuiCircularProgress: {
    styleOverrides: {
      root: {
        color: defensePointColors.red.primary,
      },
    },
  },
  MuiDivider: {
    styleOverrides: {
      root: {
        borderColor: defensePointColors.border.primary,
      },
    },
  },
  MuiTypography: {
    styleOverrides: {
      h3: {
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "-0.01em",
      },
      h4: {
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
      },
      caption: {
        textTransform: "uppercase",
        letterSpacing: "0.05em",
        fontWeight: 500,
      },
    },
  },
  MuiDialogTitle: {
    styleOverrides: {
      root: {
        fontWeight: 700,
        textTransform: "uppercase",
        letterSpacing: "0.05em",
      },
    },
  },
};
