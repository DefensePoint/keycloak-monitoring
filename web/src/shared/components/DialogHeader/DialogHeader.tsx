import { DialogTitle, Box, Typography, IconButton } from "@mui/material";
import { Close } from "@mui/icons-material";

export interface DialogHeaderProps {
  /**
   * Dialog title
   */
  title: string;
  /**
   * Optional subtitle
   */
  subtitle?: string;
  /**
   * Close button click handler
   */
  onClose: () => void;
}

/**
 * DialogHeader - Standardized header for dialogs/modals
 *
 * Provides consistent dialog headers with title, optional subtitle,
 * and close button.
 *
 * @example
 * ```tsx
 * <Dialog open={open} onClose={handleClose}>
 *   <DialogHeader
 *     title="Realm Details"
 *     subtitle="master"
 *     onClose={handleClose}
 *   />
 *   <DialogContent>
 *     ...
 *   </DialogContent>
 * </Dialog>
 * ```
 */
export function DialogHeader({ title, subtitle, onClose }: DialogHeaderProps) {
  return (
    <DialogTitle
      sx={{
        display: "flex",
        alignItems: "flex-start",
        justifyContent: "space-between",
        gap: 2,
        pb: 2,
      }}
    >
      <Box sx={{ flex: 1 }}>
        <Typography
          variant="h4"
          sx={{
            fontWeight: 700,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
          }}
        >
          {title}
        </Typography>
        {subtitle && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {subtitle}
          </Typography>
        )}
      </Box>
      <IconButton
        aria-label="close"
        onClick={onClose}
        sx={{
          color: "text.secondary",
          "&:hover": {
            color: "text.primary",
          },
        }}
      >
        <Close />
      </IconButton>
    </DialogTitle>
  );
}
