import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  MenuItem,
  Paper,
  Typography,
  Chip,
  alpha,
  useTheme,
} from "@mui/material";
import type { AssignRoleModalProps } from "../types";

export const AssignRoleModal: React.FC<AssignRoleModalProps> = ({
  open,
  user,
  roles,
  tenants,
  selectedRoleId,
  selectedTenantId,
  onRoleChange,
  onTenantChange,
  onSubmit,
  onClose,
  isSubmitting = false,
}) => {
  const theme = useTheme();

  if (!user) return null;

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
          bgcolor: alpha(theme.palette.text.secondary, 0.2),
          color: theme.palette.text.secondary,
        };
    }
  };

  const handleClose = () => {
    onRoleChange(0);
    onTenantChange(null);
    onClose();
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "divider",
        },
      }}
    >
      <DialogTitle>Assign Role to {user.name}</DialogTitle>
      <DialogContent>
        <Box
          component="form"
          onSubmit={onSubmit}
          id="assign-role-form"
          sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2 }}
        >
          <TextField
            required
            select
            fullWidth
            label="Select Role"
            value={selectedRoleId || ""}
            onChange={(e) => onRoleChange(Number(e.target.value))}
          >
            <MenuItem value="">Select a role...</MenuItem>
            {roles.map((role) => (
              <MenuItem key={role.id} value={role.id}>
                {role.display_name} ({role.name})
              </MenuItem>
            ))}
          </TextField>

          <TextField
            select
            fullWidth
            label="Tenant Scope"
            value={selectedTenantId || ""}
            onChange={(e) => onTenantChange(e.target.value || null)}
            helperText="Select a tenant to restrict this role assignment to a specific tenant, or leave as 'Global' for all tenants."
          >
            <MenuItem value="">Global (All Tenants)</MenuItem>
            {tenants.map((tenant) => (
              <MenuItem key={tenant.tenant_id} value={tenant.tenant_id}>
                {tenant.name} ({tenant.tenant_id})
              </MenuItem>
            ))}
          </TextField>

          <Paper
            elevation={0}
            sx={{
              p: 2,
              border: "1px solid",
              borderColor: "divider",
            }}
          >
            <Typography variant="caption" color="text.secondary">
              Current roles for this user:
            </Typography>
            <Box sx={{ mt: 1.5, display: "flex", flexWrap: "wrap", gap: 1 }}>
              {user.roleAssignments && user.roleAssignments.length > 0 ? (
                user.roleAssignments.map((assignment) => {
                  const badgeColors = getRoleBadgeColor(
                    assignment.role?.name || "",
                  );
                  return (
                    <Chip
                      key={assignment.id}
                      label={
                        <>
                          {assignment.role?.display_name ||
                            assignment.role?.name}
                          {assignment.tenant_id && (
                            <Typography
                              component="span"
                              variant="caption"
                              sx={{
                                ml: 0.5,
                                opacity: 0.7,
                                fontSize: "0.65rem",
                              }}
                            >
                              @{" "}
                              {tenants.find(
                                (t) => t.tenant_id === assignment.tenant_id,
                              )?.name || assignment.tenant_id}
                            </Typography>
                          )}
                        </>
                      }
                      size="small"
                      sx={{
                        bgcolor: badgeColors.bgcolor,
                        color: badgeColors.color,
                      }}
                      title={
                        assignment.tenant_id
                          ? `Scoped to tenant: ${assignment.tenant_id}`
                          : "Global role (all tenants)"
                      }
                    />
                  );
                })
              ) : (
                <Typography
                  variant="caption"
                  color="text.secondary"
                  fontStyle="italic"
                >
                  No roles assigned
                </Typography>
              )}
            </Box>
          </Paper>
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button
          onClick={handleClose}
          variant="outlined"
          disabled={isSubmitting}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          form="assign-role-form"
          variant="contained"
          color="error"
          disabled={isSubmitting}
        >
          {isSubmitting ? "Assigning..." : "Assign Role"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
