import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Typography,
} from "@mui/material";

interface ServerUnavailableProps {
  nextRetryAt: number | null;
  onRetryNow: () => void;
}

// secondsUntil is the whole seconds left until at, never below zero.
function secondsUntil(at: number | null): number {
  return at === null ? 0 : Math.max(0, Math.ceil((at - Date.now()) / 1000));
}

// ServerUnavailable is shown while startup cannot reach the server. It counts
// down to the next automatic attempt; the app continues on its own once the
// server answers.
export function ServerUnavailable({
  nextRetryAt,
  onRetryNow,
}: ServerUnavailableProps) {
  const [seconds, setSeconds] = useState(() => secondsUntil(nextRetryAt));

  useEffect(() => {
    setSeconds(secondsUntil(nextRetryAt));
    const tick = setInterval(() => setSeconds(secondsUntil(nextRetryAt)), 1000);
    return () => clearInterval(tick);
  }, [nextRetryAt]);

  return (
    <Box
      sx={{
        minHeight: "100vh",
        bgcolor: "background.default",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        p: 2,
      }}
    >
      <Card
        sx={{
          maxWidth: 480,
          width: "100%",
          bgcolor: "background.paper",
          border: 1,
          borderColor: "divider",
        }}
      >
        <CardContent sx={{ p: 4 }}>
          <Box sx={{ textAlign: "center", mb: 4 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 1.5,
                mb: 1,
              }}
            >
              <Box
                component="img"
                src="/defensepoint-logo.svg"
                alt="DefensePoint"
                sx={{ width: 48, height: 48 }}
              />
              <Typography variant="h3" sx={{ color: "text.primary" }}>
                Keycloak
              </Typography>
            </Box>
            <Typography variant="body2" sx={{ color: "text.secondary" }}>
              MONITORING TOOL
            </Typography>
          </Box>

          <Alert severity="warning" sx={{ mb: 3 }}>
            Can't reach the KMT server.
          </Alert>
          <Typography
            variant="body2"
            sx={{ color: "text.secondary", mb: 2, textAlign: "center" }}
          >
            {seconds > 0 ? `Retrying in ${seconds}s…` : "Retrying…"}
          </Typography>
          <Button
            variant="contained"
            color="primary"
            fullWidth
            onClick={onRetryNow}
          >
            Retry now
          </Button>
        </CardContent>
      </Card>
    </Box>
  );
}
