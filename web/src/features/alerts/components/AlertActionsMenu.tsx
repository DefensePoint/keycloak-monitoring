import React from "react";
import { Menu, MenuItem, Typography, Box } from "@mui/material";
import { Block as BlockIcon } from "@mui/icons-material";
import type { ConfigurationAlert } from "../types";

interface AlertActionsMenuProps {
  anchorEl: HTMLElement | null;
  selectedAlert: ConfigurationAlert | undefined;
  onClose: () => void;
  onAcknowledge: () => void;
  onIgnore: () => void;
  onResolve: () => void;
  canAcknowledge: boolean;
  canResolve: boolean;
  isActionPending: boolean;
}

export const AlertActionsMenu: React.FC<AlertActionsMenuProps> = ({
  anchorEl,
  selectedAlert,
  onClose,
  onAcknowledge,
  onIgnore,
  onResolve,
  canAcknowledge,
  canResolve,
  isActionPending,
}) => {
  const hasAnyPermission = canAcknowledge || canResolve;

  if (!hasAnyPermission) {
    return (
      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={onClose}>
        <Box
          sx={{ px: 2, py: 1.5, display: "flex", alignItems: "center", gap: 1 }}
        >
          <BlockIcon fontSize="small" color="disabled" />
          <Typography variant="body2" color="text.secondary">
            No permission to manage alerts
          </Typography>
        </Box>
      </Menu>
    );
  }

  return (
    <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={onClose}>
      {selectedAlert?.status === "active" && canAcknowledge && (
        <MenuItem onClick={onAcknowledge} disabled={isActionPending}>
          {isActionPending ? "Processing..." : "Acknowledge"}
        </MenuItem>
      )}
      {canAcknowledge && (
        <MenuItem onClick={onIgnore} disabled={isActionPending}>
          {isActionPending ? "Processing..." : "Ignore"}
        </MenuItem>
      )}
      {selectedAlert?.status !== "resolved" && canResolve && (
        <MenuItem
          onClick={onResolve}
          disabled={isActionPending}
          sx={{ color: "success.main" }}
        >
          {isActionPending ? "Processing..." : "Resolve"}
        </MenuItem>
      )}
    </Menu>
  );
};
