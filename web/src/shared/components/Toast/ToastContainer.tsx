import {
  Box,
  Snackbar,
  Alert,
  AlertColor,
  Slide,
  SlideProps,
} from "@mui/material";
import { useToast, type ToastMessage, type ToastType } from "@/shared/context";

/**
 * Slide transition for toasts
 */
function SlideTransition(props: SlideProps) {
  return <Slide {...props} direction="left" />;
}

/**
 * Map toast type to MUI Alert severity
 */
function getSeverity(type: ToastType): AlertColor {
  switch (type) {
    case "success":
      return "success";
    case "error":
      return "error";
    case "warning":
      return "warning";
    case "info":
    default:
      return "info";
  }
}

interface ToastItemProps {
  toast: ToastMessage;
  index: number;
  onClose: (id: string) => void;
}

/**
 * Individual toast item
 */
function ToastItem({ toast, index, onClose }: ToastItemProps) {
  return (
    <Snackbar
      open
      TransitionComponent={SlideTransition}
      anchorOrigin={{ vertical: "top", horizontal: "right" }}
      sx={{
        position: "relative",
        mt: index > 0 ? 1 : 0,
      }}
    >
      <Alert
        onClose={() => onClose(toast.id)}
        severity={getSeverity(toast.type)}
        variant="filled"
        sx={{
          minWidth: 320,
          maxWidth: 448,
          boxShadow: 3,
        }}
      >
        {toast.message}
      </Alert>
    </Snackbar>
  );
}

/**
 * ToastContainer - Renders the toast notification stack
 *
 * Place this component at the root of your app (after ToastProvider).
 * It will render all active toasts in a stacked layout.
 *
 * @example
 * ```tsx
 * <ToastProvider>
 *   <App />
 *   <ToastContainer />
 * </ToastProvider>
 * ```
 */
export function ToastContainer() {
  const { toasts, removeToast } = useToast();

  if (toasts.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        position: "fixed",
        top: 16,
        right: 16,
        zIndex: 9999,
        display: "flex",
        flexDirection: "column",
        gap: 1,
      }}
    >
      {toasts.map((toast, index) => (
        <ToastItem
          key={toast.id}
          toast={toast}
          index={index}
          onClose={removeToast}
        />
      ))}
    </Box>
  );
}
