import React from "react";
import { PageHeader } from "@/shared/components";

export const AlertsHeader: React.FC = () => {
  return (
    <PageHeader
      title="Configuration Alerts"
      subtitle="Monitor and manage Keycloak configuration issues"
    />
  );
};
