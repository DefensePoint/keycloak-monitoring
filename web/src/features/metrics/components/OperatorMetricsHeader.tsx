import React from "react";
import { PageHeader } from "@/shared/components";

export const OperatorMetricsHeader: React.FC = () => {
  return (
    <PageHeader
      title="Metrics"
      subtitle="Track operator performance, response times, and workload distribution"
    />
  );
};
