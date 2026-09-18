import type { MRT_TableOptions } from "material-react-table";
import { defensePointColors } from "@/theme/palette";

/**
 * Default configuration for MaterialReactTable instances.
 * Use the spread operator to apply: {...MRT_DEFAULT_OPTIONS}
 */
export const MRT_DEFAULT_OPTIONS: Partial<MRT_TableOptions<never>> = {
  enableColumnResizing: false,
  enableDensityToggle: false,
  enableFullScreenToggle: false,
  enableHiding: false,
  layoutMode: "grid",
  initialState: {
    density: "compact",
  },
  muiTablePaperProps: {
    elevation: 0,
    sx: {
      border: "1px solid",
      borderColor: "divider",
      borderRadius: 2,
      overflow: "hidden",
    },
  },
  muiTableProps: {
    sx: {
      "& .MuiTableCell-head": {
        bgcolor: "background.paper",
        color: "text.secondary",
        textTransform: "uppercase",
        fontWeight: 500,
        letterSpacing: "0.05em",
        fontSize: "0.75rem",
        whiteSpace: "nowrap",
        minWidth: "80px",
      },
    },
  },
  muiTableHeadCellProps: {
    sx: {
      "& .Mui-TableHeadCell-Content": {
        flexWrap: "nowrap",
        overflow: "hidden",
        gap: 1,
      },
      "& .Mui-TableHeadCell-Content-Labels": {
        flexWrap: "nowrap",
        gap: 0.5,
      },
      "& .Mui-TableHeadCell-Content-Actions": {
        flexWrap: "nowrap",
        gap: 0.25,
        ml: 0.5,
      },
    },
  },
  displayColumnDefOptions: {
    "mrt-row-actions": {
      header: "Actions",
      size: 100,
      minSize: 100,
      muiTableHeadCellProps: {
        align: "center",
      },
      muiTableBodyCellProps: {
        align: "center",
      },
    },
  },
  muiTableBodyRowProps: {
    sx: {
      "&:hover": {
        bgcolor: defensePointColors.background.secondary,
      },
    },
  },
};

/**
 * Default options with custom toolbar styling (for tables with section headers)
 */
export const MRT_OPTIONS_WITH_TOOLBAR: Partial<MRT_TableOptions<never>> = {
  ...MRT_DEFAULT_OPTIONS,
  muiTopToolbarProps: {
    sx: {
      borderBottom: "1px solid",
      borderColor: "divider",
    },
  },
};
