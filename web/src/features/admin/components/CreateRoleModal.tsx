import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
} from "@mui/material";
import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormTextField } from "@/shared/components";
import {
  createRoleSchema,
  type CreateRoleFormData,
} from "@/shared/lib/zodFormSchemas";
import { PermissionsSelector } from "./PermissionsSelector";
import type { Permission } from "../types";

interface CreateRoleModalProps {
  open: boolean;
  permissions: Permission[];
  onSubmit: (data: CreateRoleFormData) => void;
  onClose: () => void;
  isSubmitting?: boolean;
}

export const CreateRoleModal: React.FC<CreateRoleModalProps> = ({
  open,
  permissions,
  onSubmit,
  onClose,
  isSubmitting = false,
}) => {
  const { control, handleSubmit, reset, watch } = useForm<CreateRoleFormData>({
    resolver: zodResolver(createRoleSchema),
    mode: "onBlur",
    defaultValues: {
      name: "",
      display_name: "",
      description: "",
      permission_ids: [],
    },
  });

  const permissionIds = watch("permission_ids");

  const handleClose = () => {
    reset();
    onClose();
  };

  const handleFormSubmit = (data: CreateRoleFormData) => {
    onSubmit(data);
    reset();
  };

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          border: "1px solid",
          borderColor: "divider",
        },
      }}
    >
      <DialogTitle>Create New Role</DialogTitle>
      <DialogContent>
        <Box
          component="form"
          onSubmit={handleSubmit(handleFormSubmit)}
          id="create-role-form"
          sx={{ display: "flex", flexDirection: "column", gap: 3, pt: 2 }}
        >
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: "repeat(2, 1fr)",
              gap: 2,
            }}
          >
            <FormTextField
              name="name"
              control={control}
              label="Role Name"
              placeholder="incident_responder"
              helperText="Lowercase with underscores only"
              required
              fullWidth
            />
            <FormTextField
              name="display_name"
              control={control}
              label="Display Name"
              placeholder="Incident Responder"
              required
              fullWidth
            />
          </Box>
          <FormTextField
            name="description"
            control={control}
            label="Description"
            placeholder="Can respond to incidents and manage alerts..."
            multiline
            rows={2}
            required
            fullWidth
          />
          <Controller
            name="permission_ids"
            control={control}
            render={({ field }) => (
              <PermissionsSelector
                permissions={permissions}
                selectedPermissionIds={field.value}
                onTogglePermission={(permissionId) => {
                  const newValue = field.value.includes(permissionId)
                    ? field.value.filter((id) => id !== permissionId)
                    : [...field.value, permissionId];
                  field.onChange(newValue);
                }}
              />
            )}
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
          form="create-role-form"
          variant="contained"
          color="error"
          disabled={permissionIds.length === 0 || isSubmitting}
        >
          {isSubmitting ? "Creating..." : "Create Role"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
