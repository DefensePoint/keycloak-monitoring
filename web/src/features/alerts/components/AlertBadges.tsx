import React from "react";
import { Box, Chip } from "@mui/material";
import { FieldDisplay } from "@/shared/components";

type ChipColor = "error" | "warning" | "info" | "default";

interface AlertBadgesProps {
  severity: string;
  status: string;
  type: string;
}

function getSeverityColor(severity: string): ChipColor {
  switch (severity) {
    case "critical":
      return "error";
    case "error":
      return "error";
    case "warning":
      return "warning";
    case "info":
      return "info";
    default:
      return "default";
  }
}

function getStatusColor(status: string): ChipColor {
  switch (status) {
    case "active":
      return "error";
    case "acknowledged":
      return "warning";
    case "resolved":
      return "default";
    case "ignored":
      return "default";
    default:
      return "default";
  }
}

export const AlertBadges: React.FC<AlertBadgesProps> = ({
  severity,
  status,
  type,
}) => {
  const safeSeverity = severity ?? "unknown";
  const safeStatus = status ?? "unknown";
  const safeType = type ?? "unknown";

  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: {
          xs: "1fr",
          sm: "repeat(3, auto)",
        },
        gap: 3,
        justifyContent: "start",
      }}
    >
      <FieldDisplay
        label="Severity"
        value={
          <Chip
            label={safeSeverity.toUpperCase()}
            color={getSeverityColor(safeSeverity)}
            size="small"
          />
        }
      />
      <FieldDisplay
        label="Status"
        value={
          <Chip
            label={safeStatus.toUpperCase()}
            color={getStatusColor(safeStatus)}
            size="small"
          />
        }
      />
      <FieldDisplay
        label="Type"
        value={
          <Chip
            label={safeType.toUpperCase()}
            size="small"
            sx={{
              bgcolor: "background.paper",
            }}
          />
        }
      />
    </Box>
  );
};
