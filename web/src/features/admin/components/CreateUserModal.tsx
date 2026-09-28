import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
} from "@mui/material";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormTextField } from "@/shared/components";
import {
  createUserSchema,
  type CreateUserFormData,
} from "@/shared/lib/zodFormSchemas";

interface CreateUserModalProps {
  open: boolean;
  onSubmit: (data: CreateUserFormData) => void;
  onClose: () => void;
  isSubmitting?: boolean;
}

export const CreateUserModal: React.FC<CreateUserModalProps> = ({
  open,
  onSubmit,
  onClose,
  isSubmitting = false,
}) => {
  const { control, handleSubmit, reset } = useForm<CreateUserFormData>({
    resolver: zodResolver(createUserSchema),
    mode: "onBlur",
    defaultValues: {
      username: "",
      email: "",
      name: "",
      password: "",
    },
  });

  const handleClose = () => {
    reset();
    onClose();
  };

  const handleFormSubmit = (data: CreateUserFormData) => {
    onSubmit(data);
    reset();
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "divider",
        },
      }}
    >
      <DialogTitle>Create New User</DialogTitle>
      <DialogContent>
        <Box
          component="form"
          onSubmit={handleSubmit(handleFormSubmit)}
          id="create-user-form"
          sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2 }}
        >
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
            required
            fullWidth
            helperText="Minimum 12 characters with uppercase, lowercase, number, and special character"
          />
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button
          onClick={handleClose}
          variant="outlined"
          disabled={isSubmitting}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          form="create-user-form"
          variant="contained"
          color="error"
          disabled={isSubmitting}
        >
          {isSubmitting ? "Creating..." : "Create User"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
