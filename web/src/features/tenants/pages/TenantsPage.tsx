import { useState } from "react";
import { Box, Alert } from "@mui/material";
import { useTenant, useToast } from "@/shared/context";
import { useKeycloakDashboard } from "@/shared/hooks";
import { ConfirmDialog } from "@/shared/components";
import { TenantsPageHeader, TenantsList, TenantForm } from "../components";
import { useCreateTenant, useUpdateTenant, useDeleteTenant } from "../hooks";
import type { Tenant, TenantCreate, TenantUpdate } from "@/shared/types";

export function TenantsPage() {
  const { tenants, refreshTenants } = useTenant();
  const { showToast } = useToast();
  const [isCreating, setIsCreating] = useState(false);
  const [editingTenant, setEditingTenant] = useState<Tenant | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [tenantToDelete, setTenantToDelete] = useState<string | null>(null);

  // React Query hooks
  const { data: keycloakDashboard } = useKeycloakDashboard({
    tenantId: editingTenant?.tenant_id,
  });
  const { mutate: createTenant, isPending: isCreatingTenant } =
    useCreateTenant();
  const { mutate: updateTenant, isPending: isUpdatingTenant } =
    useUpdateTenant();
  const { mutate: deleteTenant } = useDeleteTenant();

  const isSubmitting = isCreatingTenant || isUpdatingTenant;

  const realms =
    keycloakDashboard && "realms" in keycloakDashboard
      ? keycloakDashboard.realms
      : [];

  const handleCreate = () => {
    setIsCreating(true);
    setEditingTenant(null);
    setError(null);
  };

  const handleEdit = (tenant: Tenant) => {
    setEditingTenant(tenant);
    setIsCreating(false);
    setError(null);
  };

  const handleCancel = () => {
    setIsCreating(false);
    setEditingTenant(null);
    setError(null);
  };

  const handleSubmit = (data: TenantCreate | TenantUpdate) => {
    setError(null);

    const onSuccess = async () => {
      await refreshTenants();
      handleCancel();
      showToast({
        message: isCreating
          ? "Tenant created successfully"
          : "Tenant updated successfully",
        type: "success",
      });
    };

    const onError = (err: unknown) => {
      const error = err as {
        response?: {
          status?: number;
          data?: { existing_default?: string; existing_name?: string };
        };
      };
      if (
        error?.response?.status === 409 &&
        error?.response?.data?.existing_default
      ) {
        const existingName =
          error.response.data.existing_name ||
          error.response.data.existing_default;
        const errorMessage = `Cannot set as default: "${existingName}" is already the default tenant. Please uncheck the default option on that tenant first.`;
        setError(errorMessage);
        showToast({ message: errorMessage, type: "error" });
      } else {
        const errorMessage =
          err instanceof Error ? err.message : "Operation failed";
        setError(errorMessage);
        showToast({ message: errorMessage, type: "error" });
      }
    };

    if (isCreating) {
      createTenant(data as TenantCreate, { onSuccess, onError });
    } else if (editingTenant) {
      updateTenant(
        { tenantId: editingTenant.tenant_id, data: data as TenantUpdate },
        { onSuccess, onError },
      );
    }
  };

  const handleDeleteClick = (tenantId: string) => {
    setTenantToDelete(tenantId);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (!tenantToDelete) return;

    deleteTenant(tenantToDelete, {
      onSuccess: async () => {
        await refreshTenants();
        setDeleteDialogOpen(false);
        setTenantToDelete(null);
        showToast({ message: "Tenant deleted successfully", type: "success" });
      },
      onError: (err) => {
        const errorMessage =
          err instanceof Error ? err.message : "Failed to delete tenant";
        setError(errorMessage);
        showToast({ message: errorMessage, type: "error" });
        setDeleteDialogOpen(false);
      },
    });
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setTenantToDelete(null);
  };

  const handleToggleEnabled = (tenant: Tenant) => {
    updateTenant(
      {
        tenantId: tenant.tenant_id,
        data: { enabled: !tenant.enabled },
      },
      {
        onSuccess: () => {
          refreshTenants();
          showToast({
            message: `Tenant ${!tenant.enabled ? "enabled" : "disabled"} successfully`,
            type: "success",
          });
        },
        onError: (err) => {
          const errorMessage =
            err instanceof Error ? err.message : "Failed to update tenant";
          setError(errorMessage);
          showToast({ message: errorMessage, type: "error" });
        },
      },
    );
  };

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <TenantsPageHeader
        onAddClick={handleCreate}
        tenantsCount={tenants.length}
      />

      {error && !isCreating && !editingTenant && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error}
        </Alert>
      )}

      <TenantsList
        tenants={tenants}
        onEdit={handleEdit}
        onDelete={handleDeleteClick}
        onToggleEnabled={handleToggleEnabled}
        onCreateClick={handleCreate}
      />

      <TenantForm
        open={isCreating || !!editingTenant}
        isCreating={isCreating}
        editingTenant={editingTenant}
        realms={realms}
        error={error}
        isSubmitting={isSubmitting}
        onSubmit={handleSubmit}
        onCancel={handleCancel}
      />

      <ConfirmDialog
        open={deleteDialogOpen}
        title="Delete Tenant"
        message="Are you sure you want to delete this tenant? This action cannot be undone."
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
      />
    </Box>
  );
}
