import React from "react";
import {
  Box,
  Typography,
  Chip,
  IconButton,
  Paper,
  alpha,
  useTheme,
} from "@mui/material";
import { Visibility, Delete } from "@mui/icons-material";
import { PermissionGate } from "@/shared/components";
import { PERMISSIONS } from "@/shared/constants";
import type { RoleCardProps } from "../types";

export const RoleCard: React.FC<RoleCardProps> = ({
  role,
  onView,
  onDelete,
}) => {
  const theme = useTheme();

  const getRoleBadgeColor = (
    roleName: string,
  ): { bgcolor: string; color: string } => {
    switch (roleName) {
      case "admin":
        return {
          bgcolor: alpha(theme.palette.error.main, 0.2),
          color: theme.palette.error.light,
        };
      case "operator":
        return {
          bgcolor: alpha(theme.palette.warning.main, 0.2),
          color: theme.palette.warning.light,
        };
      case "viewer":
        return {
          bgcolor: alpha(theme.palette.info.main, 0.2),
          color: theme.palette.info.light,
        };
      default:
        return {
          bgcolor: alpha(theme.palette.secondary.main, 0.2),
          color: theme.palette.secondary.light,
        };
    }
  };

  const badgeColors = getRoleBadgeColor(role.name);

  return (
    <Paper
      elevation={0}
      sx={{
        p: 3,
        border: "1px solid",
        borderColor: "divider",
        transition: "border-color 0.2s",
        "&:hover": {
          borderColor: alpha(theme.palette.error.main, 0.3),
        },
      }}
    >
      <Box
        sx={{
          display: "flex",
          alignItems: "start",
          justifyContent: "space-between",
          mb: 2,
        }}
      >
        <Box sx={{ flex: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Chip
              label={role.display_name}
              size="small"
              sx={{
                bgcolor: badgeColors.bgcolor,
                color: badgeColors.color,
              }}
            />
            {role.is_system && (
              <Chip
                label="System"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.text.secondary, 0.2),
                  color: theme.palette.text.secondary,
                }}
              />
            )}
          </Box>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <IconButton
            size="small"
            onClick={() => onView(role)}
            sx={{ color: theme.palette.error.main }}
            title="View details"
          >
            <Visibility fontSize="small" />
          </IconButton>
          <PermissionGate permission={PERMISSIONS.ROLES.DELETE}>
            {!role.is_system && (
              <IconButton
                size="small"
                onClick={() => onDelete(role.id, role.is_system)}
                sx={{ color: theme.palette.error.light }}
                title="Delete role"
              >
                <Delete fontSize="small" />
              </IconButton>
            )}
          </PermissionGate>
        </Box>
      </Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        {role.description}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        Role name:{" "}
        <Typography component="span" variant="caption" color="text.primary">
          {role.name}
        </Typography>
      </Typography>
    </Paper>
  );
};
