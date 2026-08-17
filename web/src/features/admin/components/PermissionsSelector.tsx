import React from "react";
import {
  Box,
  Typography,
  FormControlLabel,
  Checkbox,
  Paper,
  alpha,
  useTheme,
} from "@mui/material";
import type { PermissionsSelectorProps } from "../types";
import type { Permission } from "../types";

export const PermissionsSelector: React.FC<PermissionsSelectorProps> = ({
  permissions,
  selectedPermissionIds,
  onTogglePermission,
}) => {
  const theme = useTheme();

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

  const groupedPermissions = groupPermissionsByResource(permissions);

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        Permissions * (Select at least one)
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
        {Object.entries(groupedPermissions).map(([resource, perms]) => (
          <Box key={resource} sx={{ mb: 3, "&:last-child": { mb: 0 } }}>
            <Typography
              variant="caption"
              sx={{
                mb: 1,
                display: "block",
              }}
            >
              {resource}
            </Typography>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", md: "repeat(2, 1fr)" },
                gap: 1,
              }}
            >
              {perms.map((perm) => (
                <FormControlLabel
                  key={perm.id}
                  control={
                    <Checkbox
                      checked={selectedPermissionIds.includes(perm.id)}
                      onChange={() => onTogglePermission(perm.id)}
                      size="small"
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2">
                        {perm.display_name}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {perm.description}
                      </Typography>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{
                          display: "block",
                          mt: 0.5,
                        }}
                      >
                        {perm.name}
                      </Typography>
                    </Box>
                  }
                  sx={{
                    p: 1,
                    m: 0,
                    borderRadius: 1,
                    "&:hover": {
                      bgcolor: alpha(theme.palette.action.hover, 0.05),
                    },
                    alignItems: "flex-start",
                  }}
                />
              ))}
            </Box>
          </Box>
        ))}
      </Paper>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ mt: 1, display: "block" }}
      >
        Selected: {selectedPermissionIds.length} permission(s)
      </Typography>
    </Box>
  );
};
