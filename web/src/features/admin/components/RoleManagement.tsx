import { useState } from "react";
import type { Role, RoleAssignment } from "../types";
import {
  LoadingSkeleton,
  AlertBanner,
  EmptyState,
  PermissionGate,
} from "@/shared/components";
import { PERMISSIONS } from "@/shared/constants";
import {
  Box,
  Typography,
  Button,
  Card,
  CardContent,
  Chip,
  List,
  ListItemButton,
  Select,
  MenuItem,
  TextField,
  FormControl,
  InputLabel,
} from "@mui/material";
import { Lock } from "@mui/icons-material";
import {
  useRoles,
  usePermissions,
  useUserRoles,
  useAssignRole,
  useRevokeRole,
} from "../hooks";

/**
 * Component for managing roles and permissions (Admin only)
 */
export function RoleManagement() {
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);

  const {
    data: roles = [],
    isLoading: isLoadingRoles,
    error: rolesError,
    refetch: refetchRoles,
  } = useRoles();

  const {
    data: permissions = [],
    isLoading: isLoadingPermissions,
    error: permissionsError,
    refetch: refetchPermissions,
  } = usePermissions();

  const isLoading = isLoadingRoles || isLoadingPermissions;
  const error = rolesError || permissionsError;

  const handleRefresh = () => {
    refetchRoles();
    refetchPermissions();
  };

  if (isLoading) {
    return <LoadingSkeleton variant="list" items={5} />;
  }

  if (error) {
    return (
      <AlertBanner
        severity="error"
        message={
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <span>
              {error instanceof Error ? error.message : "An error occurred"}
            </span>
            <Button color="inherit" size="small" onClick={handleRefresh}>
              Retry
            </Button>
          </Box>
        }
      />
    );
  }

  return (
    <PermissionGate requireAdmin fallback={<AccessDenied />}>
      <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          <Typography variant="h4">Role Management</Typography>
          <Button onClick={handleRefresh} variant="contained">
            Refresh
          </Button>
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", lg: "1fr 1fr" },
            gap: 3,
          }}
        >
          {/* Roles List */}
          <Card>
            <CardContent>
              <Typography variant="h6" gutterBottom>
                Roles ({roles.length})
              </Typography>
              <List sx={{ p: 0 }}>
                {roles.map((role) => (
                  <ListItemButton
                    key={role.id}
                    onClick={() => setSelectedRole(role)}
                    selected={selectedRole?.id === role.id}
                    sx={{ borderRadius: 1, mb: 1 }}
                  >
                    <Box sx={{ flex: 1 }}>
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                        }}
                      >
                        <Box>
                          <Typography variant="body1">
                            {role.display_name}
                          </Typography>
                          <Typography variant="body2" color="text.secondary">
                            {role.name}
                          </Typography>
                        </Box>
                        {role.is_system && (
                          <Chip
                            label="System"
                            size="small"
                            variant="outlined"
                          />
                        )}
                      </Box>
                      {role.description && (
                        <Typography
                          variant="body2"
                          color="text.secondary"
                          sx={{ mt: 0.5 }}
                        >
                          {role.description}
                        </Typography>
                      )}
                    </Box>
                  </ListItemButton>
                ))}
              </List>
            </CardContent>
          </Card>

          {/* Role Details */}
          <Card>
            <CardContent sx={{ minHeight: 400 }}>
              {selectedRole ? (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <Box>
                    <Typography variant="h6">
                      {selectedRole.display_name}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                      {selectedRole.name}
                    </Typography>
                    {selectedRole.description && (
                      <Typography
                        variant="body2"
                        color="text.secondary"
                        sx={{ mt: 1 }}
                      >
                        {selectedRole.description}
                      </Typography>
                    )}
                  </Box>

                  <Box>
                    <Typography
                      variant="subtitle2"
                      color="text.secondary"
                      gutterBottom
                    >
                      Permissions ({selectedRole.permissions?.length || 0})
                    </Typography>
                    <Box sx={{ maxHeight: 384, overflow: "auto" }}>
                      {selectedRole.permissions &&
                      selectedRole.permissions.length > 0 ? (
                        <List sx={{ p: 0 }}>
                          {selectedRole.permissions.map((perm) => (
                            <Box
                              key={perm.id}
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "space-between",
                                px: 2,
                                py: 1,
                                bgcolor: "background.default",
                                borderRadius: 1,
                                mb: 0.5,
                              }}
                            >
                              <Box>
                                <Typography variant="body2">
                                  {perm.name}
                                </Typography>
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                >
                                  {perm.display_name}
                                </Typography>
                              </Box>
                              <Chip
                                label={`${perm.resource}:${perm.action}`}
                                size="small"
                                color="info"
                              />
                            </Box>
                          ))}
                        </List>
                      ) : (
                        <EmptyState
                          message="No permissions assigned"
                          minHeight={100}
                        />
                      )}
                    </Box>
                  </Box>
                </Box>
              ) : (
                <EmptyState
                  message="Select a role to view details"
                  minHeight="100%"
                  icon={null}
                />
              )}
            </CardContent>
          </Card>
        </Box>

        {/* All Permissions */}
        <Card>
          <CardContent>
            <Typography variant="h6" gutterBottom>
              All Permissions ({permissions.length})
            </Typography>
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: {
                  xs: "1fr",
                  md: "repeat(2, 1fr)",
                  lg: "repeat(3, 1fr)",
                },
                gap: 2,
              }}
            >
              {permissions.map((perm) => (
                <Card variant="outlined" key={perm.id}>
                  <CardContent>
                    <Typography variant="body2">{perm.name}</Typography>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ display: "block", mt: 0.5 }}
                    >
                      {perm.display_name}
                    </Typography>
                    <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
                      <Chip label={perm.resource} size="small" color="info" />
                      <Chip label={perm.action} size="small" color="success" />
                    </Box>
                  </CardContent>
                </Card>
              ))}
            </Box>
          </CardContent>
        </Card>
      </Box>
    </PermissionGate>
  );
}

function AccessDenied() {
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        height: 256,
      }}
    >
      <Box sx={{ textAlign: "center" }}>
        <Lock sx={{ fontSize: 48, color: "text.secondary", mb: 2 }} />
        <Typography variant="subtitle1" gutterBottom>
          Access Denied
        </Typography>
        <Typography variant="body2" color="text.secondary">
          You need administrator privileges to access role management.
        </Typography>
      </Box>
    </Box>
  );
}

/**
 * Component for assigning roles to users
 */
export function UserRoleAssignment({ userId }: { userId: number }) {
  const [selectedRoleId, setSelectedRoleId] = useState<number | null>(null);
  const [tenantId, setTenantId] = useState<string>("");

  const { data: userRoles = [], isLoading: isLoadingUserRoles } =
    useUserRoles(userId);
  const { data: availableRoles = [], isLoading: isLoadingRoles } = useRoles();

  const { mutate: assignRole, isPending: isAssigning } = useAssignRole();
  const { mutate: revokeRole } = useRevokeRole();

  const isLoading = isLoadingUserRoles || isLoadingRoles;

  const handleAssignRole = () => {
    if (!selectedRoleId) return;

    assignRole(
      {
        user_id: userId,
        role_id: selectedRoleId,
        tenant_id: tenantId || undefined,
      },
      {
        onSuccess: () => {
          setSelectedRoleId(null);
          setTenantId("");
        },
      },
    );
  };

  const handleRemoveRole = (roleId: number, roleTenantId: string | null) => {
    revokeRole({
      userId,
      roleId,
      tenantId: roleTenantId || undefined,
    });
  };

  if (isLoading) {
    return <LoadingSkeleton variant="list" items={3} sx={{ minHeight: 128 }} />;
  }

  return (
    <PermissionGate permission={PERMISSIONS.ROLES.ASSIGN}>
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        <Typography variant="h6">Assigned Roles</Typography>

        {/* Current Roles */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
          {userRoles.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No roles assigned
            </Typography>
          ) : (
            userRoles.map((userRole: RoleAssignment) => (
              <Card key={userRole.id} variant="outlined">
                <CardContent
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    py: 1.5,
                    "&:last-child": { pb: 1.5 },
                  }}
                >
                  <Box>
                    <Typography variant="body2">
                      {userRole.role?.display_name}
                    </Typography>
                    {userRole.tenant_id && (
                      <Typography variant="caption" color="text.secondary">
                        Tenant: {userRole.tenant_id}
                      </Typography>
                    )}
                  </Box>
                  <Button
                    onClick={() =>
                      handleRemoveRole(
                        userRole.role_id,
                        userRole.tenant_id || null,
                      )
                    }
                    color="error"
                    size="small"
                  >
                    Remove
                  </Button>
                </CardContent>
              </Card>
            ))
          )}
        </Box>

        {/* Assign New Role */}
        <Box sx={{ borderTop: 1, borderColor: "divider", pt: 2 }}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Assign New Role
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <FormControl fullWidth size="small">
              <InputLabel>Select a role</InputLabel>
              <Select
                value={selectedRoleId || ""}
                onChange={(e) => setSelectedRoleId(Number(e.target.value))}
                label="Select a role"
              >
                <MenuItem value="">Select a role...</MenuItem>
                {availableRoles.map((role) => (
                  <MenuItem key={role.id} value={role.id}>
                    {role.display_name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <TextField
              placeholder="Tenant ID (optional)"
              value={tenantId}
              onChange={(e) => setTenantId(e.target.value)}
              size="small"
              sx={{ width: 192 }}
            />
            <Button
              onClick={handleAssignRole}
              disabled={!selectedRoleId || isAssigning}
              variant="contained"
            >
              Assign
            </Button>
          </Box>
        </Box>
      </Box>
    </PermissionGate>
  );
}
