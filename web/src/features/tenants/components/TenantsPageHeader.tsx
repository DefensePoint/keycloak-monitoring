import React from "react";
import { Box, Button, Typography } from "@mui/material";
import { Add } from "@mui/icons-material";
import { PageHeader } from "@/shared/components";
import type { TenantsPageHeaderProps } from "../types";

export const TenantsPageHeader: React.FC<TenantsPageHeaderProps> = ({
  onAddClick,
  tenantsCount,
}) => {
  return (
    <Box>
      <PageHeader
        title="Tenant Management"
        subtitle="Configure and manage Keycloak tenant connections"
      >
        <Button variant="contained" startIcon={<Add />} onClick={onAddClick}>
          Add Tenant
        </Button>
      </PageHeader>
      <Box sx={{ mb: 3 }}>
        <Typography variant="body2" color="text.secondary">
          Total Tenants:{" "}
          <Typography component="span" color="text.primary">
            {tenantsCount}
          </Typography>
        </Typography>
      </Box>
    </Box>
  );
};
