import React, { useEffect } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Alert,
  Typography,
} from "@mui/material";
import { Info } from "@mui/icons-material";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormTextField, FormCheckbox } from "@/shared/components";
import {
  updateUserSchema,
  type UpdateUserFormData,
} from "@/shared/lib/zodFormSchemas";
import type { UserWithRoles } from "../types";

interface EditUserModalProps {
  open: boolean;
  user: UserWithRoles | null;
  onSubmit: (data: UpdateUserFormData) => void;
  onClose: () => void;
  isSubmitting?: boolean;
}

export const EditUserModal: React.FC<EditUserModalProps> = ({
  open,
  user,
  onSubmit,
  onClose,
  isSubmitting = false,
}) => {
  const { control, handleSubmit, reset } = useForm<UpdateUserFormData>({
    resolver: zodResolver(updateUserSchema),
    mode: "onBlur",
    defaultValues: {
      username: user?.preferred_username ?? "",
      email: user?.email ?? "",
      name: user?.name ?? "",
      password: "",
      is_active: user?.is_active ?? false,
      is_blocked: user?.is_blocked ?? false,
    },
  });

  const isActive = useWatch({ control, name: "is_active" });
  const isBlocked = useWatch({ control, name: "is_blocked" });

  useEffect(() => {
    if (user) {
      reset({
        username: user.preferred_username,
        email: user.email,
        name: user.name,
        password: "",
        is_active: user.is_active,
        is_blocked: user.is_blocked,
      });
    }
  }, [user, reset]);

  if (!user) return null;

  const isOAuthUser = user.auth_method === "oauth";
  // Any change of status revokes the user's API tokens: cutting the user off
  // ends them for good, and restoring the user ends any still held. Say so
  // before the admin saves.
  const isDeactivating = user.is_active && !isActive;
  const isBlocking = !user.is_blocked && isBlocked;
  const revocationWarning = revocationWarningFor(isDeactivating, isBlocking);
  const restoreNote = restoreNoteFor(
    !user.is_active && isActive,
    user.is_blocked && !isBlocked,
  );

  const handleFormSubmit = (data: UpdateUserFormData) => {
    onSubmit(data);
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "divider",
        },
      }}
    >
      <DialogTitle>Edit User: {user.preferred_username}</DialogTitle>
      <DialogContent>
        <Box
          component="form"
          onSubmit={handleSubmit(handleFormSubmit)}
          id="edit-user-form"
          sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2 }}
        >
          {isOAuthUser ? (
            <>
              <Alert severity="info" icon={<Info />}>
                <Typography variant="body2" sx={{ mb: 0.5 }}>
                  OAuth Authenticated User
                </Typography>
                <Typography variant="caption">
                  Identity fields (username, email, name, password) are managed
                  by the OAuth provider and cannot be modified here. You can
                  only change the user's status.
                </Typography>
              </Alert>
              <FormTextField
                name="username"
                control={control}
                label="Username"
                disabled
                fullWidth
              />
              <FormTextField
                name="email"
                control={control}
                label="Email"
                disabled
                fullWidth
              />
              <FormTextField
                name="name"
                control={control}
                label="Full Name"
                disabled
                fullWidth
              />
              <Box
                sx={{ pt: 2, borderTop: "1px solid", borderColor: "divider" }}
              >
                <Typography variant="body2" sx={{ mb: 2 }}>
                  User Status
                </Typography>
                <Box sx={{ display: "flex", gap: 3 }}>
                  <FormCheckbox
                    name="is_active"
                    control={control}
                    label="Active"
                  />
                  <FormCheckbox
                    name="is_blocked"
                    control={control}
                    label="Blocked"
                  />
                </Box>
              </Box>
            </>
          ) : (
            <>
              <FormTextField
                name="username"
                control={control}
                label="Username"
                required
                fullWidth
              />
              <FormTextField
                name="email"
                control={control}
                label="Email"
                type="email"
                required
                fullWidth
              />
              <FormTextField
                name="name"
                control={control}
                label="Full Name"
                required
                fullWidth
              />
              <FormTextField
                name="password"
                control={control}
                label="Password"
                type="password"
                placeholder="Leave empty to keep current password"
                fullWidth
                helperText="Leave empty to keep current password. If changing, minimum 12 characters required."
              />
              <Box sx={{ display: "flex", gap: 3 }}>
                <FormCheckbox
                  name="is_active"
                  control={control}
                  label="Active"
                />
                <FormCheckbox
                  name="is_blocked"
                  control={control}
                  label="Blocked"
                />
              </Box>
            </>
          )}
          {revocationWarning && (
            <Alert severity="warning">{revocationWarning}</Alert>
          )}
          {restoreNote && <Alert severity="info">{restoreNote}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose} variant="outlined" disabled={isSubmitting}>
          Cancel
        </Button>
        <Button
          type="submit"
          form="edit-user-form"
          variant="contained"
          color="error"
          disabled={isSubmitting}
        >
          {isSubmitting ? "Saving..." : "Save Changes"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

// revocationWarningFor says what saving does to the user's API tokens, or
// returns null when the save revokes none.
function revocationWarningFor(
  isDeactivating: boolean,
  isBlocking: boolean,
): string | null {
  if (isDeactivating && isBlocking) {
    return "Saving deactivates and blocks this user and revokes their API tokens. Reactivating or unblocking them later does not restore the tokens.";
  }
  if (isDeactivating) {
    return "Saving deactivates this user and revokes their API tokens. Reactivating them later does not restore the tokens.";
  }
  if (isBlocking) {
    return "Saving blocks this user and revokes their API tokens. Unblocking them later does not restore the tokens.";
  }
  return null;
}

// restoreNoteFor says what saving does to the API tokens of a user being
// restored, or returns null when the save restores nothing.
function restoreNoteFor(
  isReactivating: boolean,
  isUnblocking: boolean,
): string | null {
  const action =
    isReactivating && isUnblocking
      ? "reactivates and unblocks"
      : isReactivating
        ? "reactivates"
        : isUnblocking
          ? "unblocks"
          : null;
  if (!action) return null;
  return `Saving ${action} this user. Any API tokens they still hold are revoked, so they need to issue new ones.`;
}
