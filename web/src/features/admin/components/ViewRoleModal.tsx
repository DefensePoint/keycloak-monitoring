import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  IconButton,
  Box,
  Typography,
  Chip,
  Paper,
  alpha,
  useTheme,
} from "@mui/material";
import { Close } from "@mui/icons-material";
import type { ViewRoleModalProps } from "../types";
import type { Permission } from "../types";

export const ViewRoleModal: React.FC<ViewRoleModalProps> = ({
  open,
  role,
  onClose,
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

  const groupPermissionsByResource = (
    perms: Permission[],
  ): Record<string, Permission[]> => {
    const grouped: Record<string, Permission[]> = {};
    perms.forEach((perm) => {
      if (!grouped[perm.resource]) {
        grouped[perm.resource] = [];
      }
      grouped[perm.resource].push(perm);
    });
    return grouped;
  };

  if (!role) return null;

  const badgeColors = getRoleBadgeColor(role.name);
  const groupedPermissions = role.permissions
    ? groupPermissionsByResource(role.permissions)
    : {};

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "divider",
        },
      }}
    >
      <DialogTitle
        sx={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "start",
        }}
      >
        <Box>
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              mb: 1,
            }}
          >
            {role.display_name}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {role.description}
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <Chip
              label={role.name}
              size="small"
              sx={{
                bgcolor: badgeColors.bgcolor,
                color: badgeColors.color,
              }}
            />
            {role.is_system && (
              <Chip
                label="System Role"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.text.secondary, 0.2),
                  color: theme.palette.text.secondary,
                }}
              />
            )}
          </Box>
        </Box>
        <IconButton
          onClick={onClose}
          size="small"
          sx={{ color: "text.secondary" }}
        >
          <Close />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              mb: 2,
              display: "block",
            }}
          >
            Permissions ({role.permissions?.length || 0})
          </Typography>
          <Paper
            elevation={0}
            sx={{
              p: 2,
              border: "1px solid",
              borderColor: "divider",
              maxHeight: 400,
              overflowY: "auto",
            }}
          >
            {role.permissions && role.permissions.length > 0 ? (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
                {Object.entries(groupedPermissions).map(([resource, perms]) => (
                  <Box key={resource}>
                    <Typography
                      variant="caption"
                      sx={{
                        fontWeight: 700,
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                        mb: 1,
                        display: "block",
                      }}
                    >
                      {resource}
                    </Typography>
                    <Box
                      sx={{
                        display: "grid",
                        gridTemplateColumns: {
                          xs: "1fr",
                          md: "repeat(2, 1fr)",
                        },
                        gap: 1,
                      }}
                    >
                      {perms.map((perm) => (
                        <Paper
                          key={perm.id}
                          elevation={0}
                          sx={{
                            p: 1.5,
                            border: "1px solid",
                            borderColor: "divider",
                          }}
                        >
                          <Typography variant="body2">
                            {perm.display_name}
                          </Typography>
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{ fontFamily: "monospace" }}
                          >
                            {perm.name}
                          </Typography>
                        </Paper>
                      ))}
                    </Box>
                  </Box>
                ))}
              </Box>
            ) : (
              <Typography
                variant="body2"
                color="text.secondary"
                fontStyle="italic"
              >
                No permissions assigned to this role
              </Typography>
            )}
          </Paper>
        </Box>
      </DialogContent>
    </Dialog>
  );
};
