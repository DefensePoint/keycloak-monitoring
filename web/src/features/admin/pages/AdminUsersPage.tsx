import { useState, useMemo } from "react";
import { Box, Alert, AlertTitle, IconButton } from "@mui/material";
import { Close } from "@mui/icons-material";
import { useAuth, useTenant, useToast } from "@/shared/context";
import { ConfirmDialog, LoadingSkeleton } from "@/shared/components";
import { getErrorMessage } from "@/shared/utils";
import {
  AdminUsersPageHeader,
  UsersTable,
  CreateUserModal,
  EditUserModal,
  AssignRoleModal,
} from "../components";
import {
  useUsers,
  useRoles,
  useCreateUser,
  useUpdateUser,
  useDeleteUser,
  useUsersRoles,
  useAssignRole,
  useRevokeRole,
} from "../hooks";
import type { UserWithRoles, CreateUserRequest } from "../types";
import type { UpdateUserFormData } from "@/shared/lib/zodFormSchemas";

export function AdminUsersPage() {
  const { user: currentUser } = useAuth();
  const { tenants } = useTenant();
  const { showToast } = useToast();
  const [selectedUser, setSelectedUser] = useState<UserWithRoles | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [showEditModal, setShowEditModal] = useState(false);
  const [showRoleModal, setShowRoleModal] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [userToDelete, setUserToDelete] = useState<number | null>(null);
  const [revokeDialogOpen, setRevokeDialogOpen] = useState(false);
  const [roleToRevoke, setRoleToRevoke] = useState<{
    userId: number;
    roleId: number;
    tenantId?: string | null;
  } | null>(null);

  const [selectedRoleId, setSelectedRoleId] = useState<number | null>(null);
  const [selectedTenantId, setSelectedTenantId] = useState<string | null>(null);

  // React Query hooks
  const { data: usersData = [], isLoading: isLoadingUsers } = useUsers();
  const { data: roles = [] } = useRoles();
  const { mutate: createUser, isPending: isCreating } = useCreateUser();
  const { mutate: updateUser, isPending: isUpdating } = useUpdateUser();
  const { mutate: deleteUser, isPending: isDeleting } = useDeleteUser();
  const { mutate: assignRole, isPending: isAssigning } = useAssignRole();
  const { mutate: revokeRole, isPending: isRevoking } = useRevokeRole();

  // Load role assignments for each user
  const roleQueries = useUsersRoles(usersData);

  // Combine users with their role assignments
  const users: UserWithRoles[] = useMemo(() => {
    return usersData.map((user, index) => ({
      ...user,
      roleAssignments: roleQueries[index]?.data ?? [],
    }));
  }, [usersData, roleQueries]);

  const isLoading = isLoadingUsers || roleQueries.some((q) => q.isLoading);

  const handleCreateUser = (data: CreateUserRequest) => {
    createUser(data, {
      onSuccess: () => {
        setShowCreateModal(false);
        showToast({ message: "User created successfully", type: "success" });
      },
      onError: (err: unknown) => {
        showToast({
          message: getErrorMessage(err, "Failed to create user"),
          type: "error",
        });
        setShowCreateModal(false);
      },
    });
  };

  const handleDeleteClick = (userId: number) => {
    // Prevent self-deletion
    if (userId === currentUser?.id) {
      showToast({
        message: "You cannot deactivate your own account",
        type: "error",
      });
      return;
    }
    setUserToDelete(userId);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (!userToDelete) return;

    deleteUser(userToDelete, {
      onSuccess: () => {
        setDeleteDialogOpen(false);
        setUserToDelete(null);
        showToast({ message: "User deactivated", type: "success" });
      },
      onError: (err: unknown) => {
        showToast({
          message: getErrorMessage(err, "Failed to deactivate user"),
          type: "error",
        });
        setDeleteDialogOpen(false);
      },
    });
  };

  const handleDeleteCancel = () => {
    setDeleteDialogOpen(false);
    setUserToDelete(null);
  };

  const handleOpenEditModal = (user: UserWithRoles) => {
    setSelectedUser(user);
    setShowEditModal(true);
  };

  const handleEditUser = (data: UpdateUserFormData) => {
    if (!selectedUser) return;

    const updates: {
      email?: string;
      name?: string;
      is_active?: boolean;
      is_blocked?: boolean;
      username?: string;
      password?: string;
    } = {
      email: data.email,
      name: data.name,
      is_active: data.is_active,
      is_blocked: data.is_blocked,
    };

    // Add username if it changed
    if (data.username && data.username !== selectedUser.preferred_username) {
      updates.username = data.username;
    }

    // Add password if it was provided
    if (data.password && data.password.length > 0) {
      updates.password = data.password;
    }

    updateUser(
      { userId: selectedUser.id, updates },
      {
        onSuccess: () => {
          setShowEditModal(false);
          setSelectedUser(null);
          showToast({ message: "User updated successfully", type: "success" });
        },
        onError: (err: unknown) => {
          showToast({
            message: getErrorMessage(err, "Failed to update user"),
            type: "error",
          });
          setShowEditModal(false);
          setSelectedUser(null);
        },
      },
    );
  };

  const handleAssignRole = (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedUser || !selectedRoleId || !currentUser?.email) {
      return;
    }

    assignRole(
      {
        user_id: selectedUser.id,
        role_id: selectedRoleId,
        tenant_id: selectedTenantId || undefined,
      },
      {
        onSuccess: () => {
          setShowRoleModal(false);
          setSelectedRoleId(null);
          setSelectedTenantId(null);
          setSelectedUser(null);
          showToast({ message: "Role assigned successfully", type: "success" });
        },
        onError: (err: unknown) => {
          showToast({
            message: getErrorMessage(err, "Failed to assign role"),
            type: "error",
          });
          setShowRoleModal(false);
          setSelectedRoleId(null);
          setSelectedTenantId(null);
          setSelectedUser(null);
        },
      },
    );
  };

  const handleRevokeRoleClick = (
    userId: number,
    roleId: number,
    tenantId?: string | null,
  ) => {
    setRoleToRevoke({ userId, roleId, tenantId });
    setRevokeDialogOpen(true);
  };

  const handleRevokeRoleConfirm = () => {
    if (!roleToRevoke) return;

    revokeRole(
      {
        userId: roleToRevoke.userId,
        roleId: roleToRevoke.roleId,
        tenantId: roleToRevoke.tenantId || undefined,
      },
      {
        onSuccess: () => {
          setRevokeDialogOpen(false);
          setRoleToRevoke(null);
          showToast({ message: "Role revoked successfully", type: "success" });
        },
        onError: (err: unknown) => {
          showToast({
            message: getErrorMessage(err, "Failed to revoke role"),
            type: "error",
          });
          setRevokeDialogOpen(false);
        },
      },
    );
  };

  const handleRevokeRoleCancel = () => {
    setRevokeDialogOpen(false);
    setRoleToRevoke(null);
  };

  if (isLoading) {
    return <LoadingSkeleton variant="table" rows={10} />;
  }

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <AdminUsersPageHeader
        usersCount={users.length}
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

      <UsersTable
        users={users}
        roles={roles}
        tenants={tenants}
        onEdit={handleOpenEditModal}
        onDelete={handleDeleteClick}
        onManageRoles={(user) => {
          setSelectedUser(user);
          setShowRoleModal(true);
        }}
        onRevokeRole={handleRevokeRoleClick}
      />

      <CreateUserModal
        open={showCreateModal}
        onSubmit={handleCreateUser}
        onClose={() => setShowCreateModal(false)}
        isSubmitting={isCreating}
      />

      <EditUserModal
        open={showEditModal}
        user={selectedUser}
        onSubmit={handleEditUser}
        onClose={() => {
          setShowEditModal(false);
          setSelectedUser(null);
        }}
        isSubmitting={isUpdating}
      />

      <AssignRoleModal
        open={showRoleModal}
        user={selectedUser}
        roles={roles}
        tenants={tenants}
        selectedRoleId={selectedRoleId}
        selectedTenantId={selectedTenantId}
        onRoleChange={setSelectedRoleId}
        onTenantChange={setSelectedTenantId}
        onSubmit={handleAssignRole}
        onClose={() => {
          setShowRoleModal(false);
          setSelectedRoleId(null);
          setSelectedTenantId(null);
          setSelectedUser(null);
        }}
        isSubmitting={isAssigning}
      />

      <ConfirmDialog
        open={deleteDialogOpen}
        title="Deactivate user"
        message="They will be signed out of the platform and cannot sign in again until they are reactivated. Their account, roles and history are kept. Any session they already have stays valid until it expires, for up to 24 hours."
        onConfirm={handleDeleteConfirm}
        onCancel={handleDeleteCancel}
        isLoading={isDeleting}
      />

      <ConfirmDialog
        open={revokeDialogOpen}
        title="Revoke Role"
        message="Are you sure you want to revoke this role?"
        onConfirm={handleRevokeRoleConfirm}
        onCancel={handleRevokeRoleCancel}
        isLoading={isRevoking}
      />
    </Box>
  );
}
