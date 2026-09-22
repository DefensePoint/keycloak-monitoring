import React from "react";
import { PageHeader } from "@/shared/components";

export const RealmsPageHeader: React.FC = () => {
  return (
    <PageHeader
      title="Realms"
      subtitle="Manage and monitor Keycloak realm configurations"
    />
  );
};
