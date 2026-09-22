import React from "react";
import { Box } from "@mui/material";
import { RoleCard } from "./RoleCard";
import type { RolesGridProps } from "../types";

export const RolesGrid: React.FC<RolesGridProps> = ({
  roles,
  onView,
  onDelete,
}) => {
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
      {roles.map((role) => (
        <RoleCard
          key={role.id}
          role={role}
          onView={onView}
          onDelete={onDelete}
        />
      ))}
    </Box>
  );
};
