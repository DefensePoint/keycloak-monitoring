import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
  Box,
} from "@mui/material";
import { Warning } from "@mui/icons-material";

interface ConfirmDialogProps {
  readonly open: boolean;
  readonly title: string;
  readonly message: string;
  readonly confirmText?: string;
  readonly cancelText?: string;
  readonly confirmColor?: "error" | "success" | "warning" | "secondary";
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
  /** Whether an action is in progress (disables buttons) */
  readonly isLoading?: boolean;
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  open,
  title,
  message,
  confirmText = "Confirm",
  cancelText = "Cancel",
  confirmColor = "error",
  onConfirm,
  onCancel,
  isLoading = false,
}) => {
  return (
    <Dialog
      open={open}
      onClose={onCancel}
      maxWidth="sm"
      fullWidth
      data-testid="confirm-dialog"
      PaperProps={{
        sx: {
          maxWidth: 448,
          mx: 2,
        },
      }}
    >
      {/* Icon */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          width: 48,
          height: 48,
          mx: "auto",
          mt: 3,
          mb: 2,
          borderRadius: "50%",
          bgcolor: (theme) =>
            theme.palette.mode === "dark"
              ? "rgba(244, 67, 54, 0.2)"
              : "rgba(244, 67, 54, 0.1)",
          border: 1,
          borderColor: "error.main",
        }}
      >
        <Warning sx={{ color: "error.main", fontSize: 24 }} />
      </Box>

      <DialogTitle
        sx={{
          textAlign: "center",
          pb: 1,
        }}
      >
        {title}
      </DialogTitle>

      <DialogContent>
        <Typography variant="body2" color="text.secondary" align="center">
          {message}
        </Typography>
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 3, gap: 1.5 }}>
        <Button
          onClick={onCancel}
          variant="outlined"
          fullWidth
          disabled={isLoading}
        >
          {cancelText}
        </Button>
        <Button
          onClick={onConfirm}
          variant="contained"
          color={confirmColor}
          fullWidth
          disabled={isLoading}
        >
          {isLoading ? "Processing..." : confirmText}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
