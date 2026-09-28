import { useState } from "react";
import { Box, Alert, AlertTitle, IconButton } from "@mui/material";
import { Close } from "@mui/icons-material";
import { useToast } from "@/shared/context";
import { ConfirmDialog, LoadingSkeleton } from "@/shared/components";
import { getErrorMessage } from "@/shared/utils";
import {
  AdminRolesPageHeader,
  RolesGrid,
  CreateRoleModal,
  ViewRoleModal,
} from "../components";
import {
  useRoles,
  useRole,
  usePermissions,
  useCreateRole,
  useDeleteRole,
} from "../hooks";
import type { Role, CreateRoleRequest } from "../types";

export function AdminRolesPage() {
  const { showToast } = useToast();
  const [selectedRoleId, setSelectedRoleId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showViewModal, setShowViewModal] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [roleToDelete, setRoleToDelete] = useState<{
    id: number;
    isSystem: boolean;
  } | null>(null);

  // React Query hooks
  const { data: roles = [], isLoading } = useRoles();
  const { data: permissions = [] } = usePermissions();
  const { data: selectedRole } = useRole(selectedRoleId ?? undefined);
  const { mutate: createRole, isPending: isCreating } = useCreateRole();
  const { mutate: deleteRole, isPending: isDeleting } = useDeleteRole();

  const handleCreateRole = (data: CreateRoleRequest) => {
    createRole(data, {
      onSuccess: () => {
        setShowCreateModal(false);
        showToast({ message: "Role created successfully", type: "success" });
      },
      onError: (err: unknown) => {
        showToast({
          message: getErrorMessage(err, "Failed to create role"),
          type: "error",
        });
        setShowCreateModal(false);
      },
    });
  };

  const handleDeleteClick = (roleId: number, isSystem: boolean) => {
    if (isSystem) {
      showToast({
        message: "Cannot delete system roles (admin, operator, viewer)",
        type: "warning",
      });
      return;
    }
    setRoleToDelete({ id: roleId, isSystem });
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (!roleToDelete) return;

    deleteRole(roleToDelete.id, {
      onSuccess: () => {
        setDeleteDialogOpen(false);
        setRoleToDelete(null);
        showToast({ message: "Role deleted successfully", type: "success" });
      },
      onError: (err: unknown) => {
        showToast({
          message: getErrorMessage(err, "Failed to delete role"),
          type: "error",
        });
        setDeleteDialogOpen(false);
      },
    });
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setRoleToDelete(null);
  };

  const handleViewRole = (role: Role) => {
    setSelectedRoleId(role.id);
    setShowViewModal(true);
  };

  if (isLoading) {
    return <LoadingSkeleton variant="card" count={6} />;
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <AdminRolesPageHeader
        rolesCount={roles.length}
        onCreateClick={() => setShowCreateModal(true)}
      />

      {error && (
        <Alert
          severity="error"
          sx={{ mb: 3 }}
          action={
            <IconButton
              aria-label="close"
              size="small"
              onClick={() => setError(null)}
              sx={{ color: "error.main" }}
            >
              <Close fontSize="small" />
            </IconButton>
          }
        >
          <AlertTitle>Error</AlertTitle>
          {error}
        </Alert>
      )}

      <RolesGrid
        roles={roles}
        onView={handleViewRole}
        onDelete={handleDeleteClick}
      />

      <CreateRoleModal
        open={showCreateModal}
        permissions={permissions}
        onSubmit={handleCreateRole}
        onClose={() => setShowCreateModal(false)}
        isSubmitting={isCreating}
      />

      <ViewRoleModal
        open={showViewModal}
        role={selectedRole ?? null}
        onClose={() => {
          setShowViewModal(false);
          setSelectedRoleId(null);
        }}
      />

      <ConfirmDialog
        open={deleteDialogOpen}
        title="Delete Role"
        message="Are you sure you want to delete this role?"
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
        isLoading={isDeleting}
      />
    </Box>
  );
}
