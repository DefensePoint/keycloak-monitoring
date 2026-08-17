import React from "react";
import { Box, Typography, Button } from "@mui/material";
import { Add } from "@mui/icons-material";
import { PermissionGate, PageHeader } from "@/shared/components";
import { PERMISSIONS } from "@/shared/constants";
import type { AdminUsersPageHeaderProps } from "../types";

export const AdminUsersPageHeader: React.FC<AdminUsersPageHeaderProps> = ({
  usersCount,
  onCreateClick,
}) => {
  return (
    <Box>
      <PageHeader
        title="User Management"
        subtitle="Manage platform users and their role assignments"
      >
        <PermissionGate permission={PERMISSIONS.PLATFORM_USERS.WRITE}>
          <Button
            variant="contained"
            color="error"
            startIcon={<Add />}
            onClick={onCreateClick}
          >
            Create User
          </Button>
        </PermissionGate>
      </PageHeader>

      <Box sx={{ mb: 3 }}>
        <Typography variant="body2" color="text.secondary">
          Total Users:{" "}
          <Typography component="span" color="text.primary">
            {usersCount}
          </Typography>
        </Typography>
      </Box>
    </Box>
  );
};
