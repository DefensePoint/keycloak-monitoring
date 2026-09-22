import React from "react";
import { Snackbar, Alert, AlertColor } from "@mui/material";

interface ToastProps {
  readonly message: string;
  readonly type: "success" | "error" | "info";
  readonly onClose: () => void;
  readonly duration?: number;
}

export const Toast: React.FC<ToastProps> = ({
  message,
  type,
  onClose,
  duration = 3000,
}) => {
  return (
    <Snackbar
      open={true}
      autoHideDuration={duration}
      onClose={onClose}
      anchorOrigin={{ vertical: "top", horizontal: "right" }}
      sx={{ marginTop: 2 }}
    >
      <Alert
        onClose={onClose}
        severity={type as AlertColor}
        variant="standard"
        sx={{ minWidth: 320, maxWidth: 448 }}
      >
        {message}
      </Alert>
    </Snackbar>
  );
};
