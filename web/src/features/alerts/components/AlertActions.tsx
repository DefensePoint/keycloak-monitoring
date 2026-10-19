import React from "react";
import { Box, Button, Typography } from "@mui/material";
import { CheckCircle } from "@mui/icons-material";
import { SectionCard } from "@/shared/components";

interface AlertActionsProps {
  status: string;
  onAcknowledge: () => void;
  onIgnore: () => void;
  onResolve: () => void;
  /** alerts:acknowledge */
  canAcknowledge: boolean;
  /** alerts:resolve, which ignoring and resolving both need */
  canResolve: boolean;
}

export const AlertActions: React.FC<AlertActionsProps> = ({
  status,
  onAcknowledge,
  onIgnore,
  onResolve,
  canAcknowledge,
  canResolve,
}) => {
  const isResolved = status === "resolved" || status === "ignored";
  const showAcknowledge = status === "active" && canAcknowledge;
  const showIgnore = status !== "ignored" && canResolve;

  return (
    <SectionCard title="Actions">
      {isResolved ? (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <CheckCircle color="success" />
          <Typography variant="body2" color="text.secondary">
            This alert has been {status === "resolved" ? "resolved" : "ignored"}
            . No further actions available.
          </Typography>
        </Box>
      ) : !showAcknowledge && !canResolve ? (
        <Typography variant="body2" color="text.secondary">
          You don&apos;t have permission to change this alert.
        </Typography>
      ) : (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2 }}>
          {showAcknowledge && (
            <Button
              variant="contained"
              color="warning"
              onClick={onAcknowledge}
              sx={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                fontWeight: 600,
              }}
            >
              Acknowledge
            </Button>
          )}
          {showIgnore && (
            <Button
              variant="outlined"
              onClick={onIgnore}
              sx={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                fontWeight: 600,
              }}
            >
              Ignore
            </Button>
          )}
          {canResolve && (
            <Button
              variant="contained"
              color="success"
              onClick={onResolve}
              sx={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                fontWeight: 600,
              }}
            >
              Resolve
            </Button>
          )}
        </Box>
      )}
    </SectionCard>
  );
};
