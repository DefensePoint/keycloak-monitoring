import React from "react";
import { Box } from "@mui/material";
import { CloudOff } from "@mui/icons-material";
import { LoadingSkeleton, EmptyState } from "@/shared/components";
import { RealmCard } from "./RealmCard";
import type { RealmsGridProps } from "../types";

export const RealmsGrid: React.FC<RealmsGridProps> = ({
  realms,
  onRealmClick,
  loading,
  connectionBroken = false,
  connectionError,
}) => {
  if (loading) {
    return <LoadingSkeleton variant="card" count={6} />;
  }

  if (connectionBroken) {
    return (
      <EmptyState
        icon={<CloudOff color="error" />}
        message="Can't connect to Keycloak"
        description={
          connectionError ??
          "The monitoring service could not reach or authenticate to this tenant's Keycloak. Realms cannot be loaded until the connection is restored."
        }
      />
    );
  }

  if (realms.length === 0) {
    return <EmptyState message="No realms match your filters" />;
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
      {realms.map((realm) => (
        <RealmCard
          key={realm.realm_name}
          realm={realm}
          onClick={() => onRealmClick(realm.realm_name)}
        />
      ))}
    </Box>
  );
};
