import React from "react";
import { Box, Button, Typography } from "@mui/material";
import { CheckCircle } from "@mui/icons-material";
import { SectionCard } from "@/shared/components";

interface AlertActionsProps {
  status: string;
  onAcknowledge: () => void;
  onIgnore: () => void;
  onResolve: () => void;
}

export const AlertActions: React.FC<AlertActionsProps> = ({
  status,
  onAcknowledge,
  onIgnore,
  onResolve,
}) => {
  const isResolved = status === "resolved" || status === "ignored";

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
      ) : (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2 }}>
          {status === "active" && (
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
          {status !== "ignored" && (
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
        </Box>
      )}
    </SectionCard>
  );
};
