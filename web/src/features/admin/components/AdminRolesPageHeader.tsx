import React from "react";
import { Box, Typography, Button } from "@mui/material";
import { Add } from "@mui/icons-material";
import { PermissionGate, PageHeader } from "@/shared/components";
import { PERMISSIONS } from "@/shared/constants";
import type { AdminRolesPageHeaderProps } from "../types";

export const AdminRolesPageHeader: React.FC<AdminRolesPageHeaderProps> = ({
  rolesCount,
  onCreateClick,
}) => {
  return (
    <Box>
      <PageHeader
        title="Role Management"
        subtitle="Manage roles and their permissions"
      >
        <PermissionGate permission={PERMISSIONS.ROLES.WRITE}>
          <Button
            variant="contained"
            color="error"
            startIcon={<Add />}
            onClick={onCreateClick}
          >
            Create Role
          </Button>
        </PermissionGate>
      </PageHeader>

      <Box sx={{ mb: 3 }}>
        <Typography variant="body2" color="text.secondary">
          Total Roles:{" "}
          <Typography component="span" color="text.primary">
            {rolesCount}
          </Typography>
        </Typography>
      </Box>
    </Box>
  );
};
