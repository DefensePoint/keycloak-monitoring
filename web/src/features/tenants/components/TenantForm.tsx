import React, { useEffect } from "react";
import {
  Box,
  Typography,
  Button,
  MenuItem,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  IconButton,
} from "@mui/material";
import { Close as CloseIcon } from "@mui/icons-material";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FormTextField, FormCheckbox, FormSelect } from "@/shared/components";
import {
  createTenantSchema,
  updateTenantSchema,
  type CreateTenantFormData,
  type UpdateTenantFormData,
} from "../schemas";
import type { Tenant, RealmListItem } from "@/shared/types";

interface TenantFormProps {
  open: boolean;
  isCreating: boolean;
  editingTenant: Tenant | null;
  realms: RealmListItem[];
  error: string | null;
  isSubmitting: boolean;
  onSubmit: (data: CreateTenantFormData | UpdateTenantFormData) => void;
  onCancel: () => void;
}

export const TenantForm: React.FC<TenantFormProps> = ({
  open,
  isCreating,
  editingTenant,
  realms,
  error,
  isSubmitting,
  onSubmit,
  onCancel,
}) => {
  const schema = isCreating ? createTenantSchema : updateTenantSchema;

  const { control, handleSubmit, reset } = useForm<
    CreateTenantFormData | UpdateTenantFormData
  >({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: isCreating
      ? {
          tenant_id: "",
          name: "",
          description: "",
          server_url: "",
          admin_realm: "master",
          client_id: "",
          client_secret: "",
          enabled: true,
          is_default: false,
          default_realm: "",
        }
      : {
          name: editingTenant?.name || "",
          description: editingTenant?.description || "",
          server_url: editingTenant?.server_url || "",
          admin_realm: editingTenant?.admin_realm || "master",
          client_id: editingTenant?.client_id || "",
          client_secret: "",
          enabled: editingTenant?.enabled ?? true,
          is_default: editingTenant?.is_default ?? false,
          default_realm: editingTenant?.default_realm || "",
        },
  });

  useEffect(() => {
    if (!isCreating && editingTenant) {
      reset({
        name: editingTenant.name,
        description: editingTenant.description || "",
        server_url: editingTenant.server_url,
        admin_realm: editingTenant.admin_realm,
        client_id: editingTenant.client_id || "",
        client_secret: "",
        enabled: editingTenant.enabled,
        is_default: editingTenant.is_default,
        default_realm: editingTenant.default_realm || "",
      });
    }
  }, [editingTenant, isCreating, reset]);

  const handleFormSubmit = (
    data: CreateTenantFormData | UpdateTenantFormData,
  ) => {
    // On edit, the secret field is write-only and starts blank (the stored
    // secret is never shown). An empty value means "untouched", not "clear
    // the secret" — drop it so the update leaves the stored secret as-is.
    if (!isCreating && !data.client_secret) {
      // JSON.stringify omits undefined values, so the field is left out of the
      // request body entirely and the server keeps the stored secret.
      onSubmit({ ...data, client_secret: undefined });
      return;
    }
    onSubmit(data);
  };

  return (
    <Dialog open={open} onClose={onCancel} maxWidth="sm" fullWidth>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          borderBottom: 1,
          borderColor: "divider",
        }}
      >
        <Typography variant="h6" sx={{ fontWeight: 600 }}>
          {isCreating ? "Create New Tenant" : `Edit Tenant`}
        </Typography>
        <IconButton onClick={onCancel} size="small" disabled={isSubmitting}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <form onSubmit={handleSubmit(handleFormSubmit)}>
        <DialogContent
          sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: 3 }}
        >
          {error && (
            <Alert severity="error" sx={{ mb: 1 }}>
              {error}
            </Alert>
          )}

          {isCreating && (
            <FormTextField
              name="tenant_id"
              control={control}
              label="Tenant ID"
              placeholder="e.g., prod-keycloak"
              helperText="Unique identifier for this tenant"
              required
              fullWidth
            />
          )}

          <FormTextField
            name="name"
            control={control}
            label="Name"
            placeholder="e.g., Production Keycloak"
            required
            fullWidth
          />

          <FormTextField
            name="description"
            control={control}
            label="Description"
            placeholder="Optional description"
            fullWidth
          />

          <FormTextField
            name="server_url"
            control={control}
            label="Server URL"
            type="url"
            placeholder="https://keycloak.example.com"
            required
            fullWidth
          />

          <FormTextField
            name="admin_realm"
            control={control}
            label="Admin Realm"
            placeholder="master"
            required
            fullWidth
          />

          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: "repeat(2, 1fr)",
              gap: 2,
            }}
          >
            <FormTextField
              name="client_id"
              control={control}
              label="Client ID"
              placeholder="monitoring-service"
              required={isCreating}
              fullWidth
            />

            <FormTextField
              name="client_secret"
              control={control}
              label={`Client Secret${isCreating ? " *" : ""}`}
              type="password"
              placeholder={
                isCreating ? "Client secret" : "Leave blank to keep unchanged"
              }
              required={isCreating}
              fullWidth
            />
          </Box>

          <FormCheckbox
            name="enabled"
            control={control}
            label="Enable monitoring for this tenant"
          />

          <FormCheckbox
            name="is_default"
            control={control}
            label="Set as default tenant"
          />

          {!isCreating && realms.length > 0 && (
            <FormSelect
              name="default_realm"
              control={control}
              label="Default Realm"
              fullWidth
            >
              <MenuItem value="">None (Show all realms)</MenuItem>
              {realms.map((realm) => (
                <MenuItem key={realm.realm_name} value={realm.realm_name}>
                  {realm.realm_name}
                </MenuItem>
              ))}
            </FormSelect>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, pb: 3, gap: 1 }}>
          <Button variant="outlined" onClick={onCancel} disabled={isSubmitting}>
            Cancel
          </Button>
          <Button type="submit" variant="contained" disabled={isSubmitting}>
            {isSubmitting
              ? "Saving..."
              : isCreating
                ? "Create Tenant"
                : "Save Changes"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
