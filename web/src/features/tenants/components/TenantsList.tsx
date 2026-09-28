import React from "react";
import { Box, Button } from "@mui/material";
import { Add, Storage } from "@mui/icons-material";
import { EmptyState } from "@/shared/components";
import { TenantCard } from "./TenantCard";
import type { TenantsListProps } from "../types";

export const TenantsList: React.FC<TenantsListProps> = ({
  tenants,
  onEdit,
  onDelete,
  onToggleEnabled,
  onCreateClick,
}) => {
  if (tenants.length === 0) {
    return (
      <EmptyState
        icon={<Storage sx={{ fontSize: 64 }} />}
        message="No tenants configured"
        action={
          <Button
            variant="contained"
            color="error"
            startIcon={<Add />}
            onClick={onCreateClick}
            sx={{
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              fontWeight: 600,
            }}
          >
            Create Your First Tenant
          </Button>
        }
      />
    );
  }

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          md: "repeat(2, 1fr)",
          lg: "repeat(3, 1fr)",
        },
        gap: 3,
      }}
    >
      {tenants.map((tenant) => (
        <TenantCard
          key={tenant.tenant_id}
          tenant={tenant}
          onEdit={onEdit}
          onDelete={onDelete}
          onToggleEnabled={onToggleEnabled}
        />
      ))}
    </Box>
  );
};
