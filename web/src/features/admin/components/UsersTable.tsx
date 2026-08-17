import React, { useMemo, useCallback } from "react";
import {
  Box,
  Chip,
  IconButton,
  alpha,
  useTheme,
  Typography,
} from "@mui/material";
import {
  Edit,
  Delete,
  ManageAccounts,
  Verified,
  Bolt,
  Key,
  Close,
} from "@mui/icons-material";
import {
  MaterialReactTable,
  type MRT_ColumnDef,
  type MRT_Row,
} from "material-react-table";
import { PermissionGate } from "@/shared/components";
import { usePermission } from "@/shared/hooks";
import { MRT_OPTIONS_WITH_TOOLBAR, PERMISSIONS } from "@/shared/constants";
import type { UsersTableProps } from "../types";
import type { UserWithRoles, RoleAssignment } from "../types";

export const UsersTable: React.FC<UsersTableProps> = ({
  users,
  tenants,
  onEdit,
  onDelete,
  onManageRoles,
  onRevokeRole,
}) => {
  const theme = useTheme();
  const canAssignRoles = usePermission("roles:assign");

  const getRoleBadgeColor = useCallback(
    (roleName: string): { bgcolor: string; color: string } => {
      switch (roleName) {
        case "admin":
          return {
            bgcolor: alpha(theme.palette.error.main, 0.2),
            color: theme.palette.error.light,
          };
        case "operator":
          return {
            bgcolor: alpha(theme.palette.warning.main, 0.2),
            color: theme.palette.warning.light,
          };
        case "viewer":
          return {
            bgcolor: alpha(theme.palette.info.main, 0.2),
            color: theme.palette.info.light,
          };
        default:
          return {
            bgcolor: alpha(theme.palette.text.secondary, 0.2),
            color: theme.palette.text.secondary,
          };
      }
    },
    [theme],
  );

  const columns = useMemo<MRT_ColumnDef<UserWithRoles>[]>(
    () => [
      {
        accessorKey: "name",
        header: "User",
        size: 200,
        Cell: ({ row }: { row: MRT_Row<UserWithRoles> }) => (
          <Box>
            <Box
              sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}
            >
              <Typography variant="body2" fontWeight={600}>
                {row.original.name || row.original.preferred_username}
              </Typography>
              {row.original.auth_method === "oauth" ? (
                <Chip
                  icon={<Bolt sx={{ fontSize: 14 }} />}
                  label="OAuth"
                  size="small"
                  title="OAuth authenticated user - identity managed by OAuth provider"
                  sx={{
                    bgcolor: alpha(theme.palette.info.main, 0.2),
                    color: theme.palette.info.light,
                    "& .MuiChip-icon": { color: "inherit" },
                  }}
                />
              ) : (
                <Chip
                  icon={<Key sx={{ fontSize: 14 }} />}
                  label="Simple"
                  size="small"
                  title="Simple username/password authentication"
                  sx={{
                    bgcolor: alpha(theme.palette.text.secondary, 0.2),
                    color: theme.palette.text.secondary,
                    "& .MuiChip-icon": { color: "inherit" },
                  }}
                />
              )}
            </Box>
            <Typography variant="caption" color="text.secondary">
              @{row.original.preferred_username}
            </Typography>
          </Box>
        ),
      },
      {
        accessorKey: "email",
        header: "Email",
        size: 220,
        Cell: ({ row }: { row: MRT_Row<UserWithRoles> }) => (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="body2">{row.original.email}</Typography>
            {row.original.email_verified && (
              <Verified
                sx={{ fontSize: 16, color: theme.palette.success.main }}
              />
            )}
          </Box>
        ),
      },
      {
        accessorKey: "roleAssignments",
        header: "Roles",
        size: 300,
        Cell: ({ row }: { row: MRT_Row<UserWithRoles> }) => (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
            {row.original.roleAssignments &&
            row.original.roleAssignments.length > 0 ? (
              row.original.roleAssignments.map((assignment: RoleAssignment) => {
                const badgeColors = getRoleBadgeColor(
                  assignment.role?.name || "",
                );
                return (
                  <Chip
                    key={assignment.id}
                    label={
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 0.5,
                        }}
                      >
                        <span>
                          {assignment.role?.display_name ||
                            assignment.role?.name}
                        </span>
                        {assignment.tenant_id && (
                          <Typography
                            component="span"
                            sx={{ opacity: 0.7, fontSize: "0.65rem" }}
                          >
                            @{" "}
                            {tenants.find(
                              (t) => t.tenant_id === assignment.tenant_id,
                            )?.name || assignment.tenant_id}
                          </Typography>
                        )}
                      </Box>
                    }
                    size="small"
                    {...(canAssignRoles && {
                      onDelete: () =>
                        onRevokeRole(
                          row.original.id,
                          assignment.role_id,
                          assignment.tenant_id,
                        ),
                      deleteIcon: <Close sx={{ fontSize: 14 }} />,
                    })}
                    sx={{
                      bgcolor: badgeColors.bgcolor,
                      color: badgeColors.color,
                      "& .MuiChip-deleteIcon": {
                        color: "inherit",
                        "&:hover": {
                          color: theme.palette.error.light,
                        },
                      },
                    }}
                    title={
                      assignment.tenant_id
                        ? `Scoped to tenant: ${assignment.tenant_id}`
                        : "Global role (all tenants)"
                    }
                  />
                );
              })
            ) : (
              <Typography
                variant="caption"
                color="text.secondary"
                fontStyle="italic"
              >
                No roles assigned
              </Typography>
            )}
          </Box>
        ),
      },
      {
        accessorKey: "is_active",
        header: "Status",
        size: 140,
        Cell: ({ row }: { row: MRT_Row<UserWithRoles> }) => (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
            {row.original.is_active ? (
              <Chip
                label="Active"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.success.main, 0.2),
                  color: theme.palette.success.light,
                  width: "fit-content",
                }}
              />
            ) : (
              <Chip
                label="Inactive"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.text.secondary, 0.2),
                  color: theme.palette.text.secondary,
                  width: "fit-content",
                }}
              />
            )}
            {row.original.is_blocked && (
              <Chip
                label="Blocked"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.error.main, 0.2),
                  color: theme.palette.error.light,
                  width: "fit-content",
                }}
              />
            )}
            {row.original.must_change_password && (
              <Chip
                label="Must Change Password"
                size="small"
                sx={{
                  bgcolor: alpha(theme.palette.warning.main, 0.2),
                  color: theme.palette.warning.light,
                  width: "fit-content",
                }}
              />
            )}
          </Box>
        ),
      },
      {
        accessorKey: "last_accessed",
        header: "Last Accessed",
        size: 180,
        Cell: ({ row }: { row: MRT_Row<UserWithRoles> }) => (
          <Typography variant="body2" color="text.secondary">
            {row.original.last_accessed
              ? new Date(row.original.last_accessed).toLocaleString()
              : "Never"}
          </Typography>
        ),
      },
    ],
    [theme, tenants, onRevokeRole, getRoleBadgeColor, canAssignRoles],
  );

  return (
    <MaterialReactTable
      {...(MRT_OPTIONS_WITH_TOOLBAR as object)}
      columns={columns}
      data={users}
      enableRowActions
      positionActionsColumn="last"
      initialState={{
        density: "compact",
      }}
      renderRowActions={({ row }: { row: MRT_Row<UserWithRoles> }) => (
        <Box sx={{ display: "flex", gap: 0.5 }}>
          <PermissionGate permission={PERMISSIONS.ROLES.ASSIGN}>
            <IconButton
              size="small"
              onClick={() => onManageRoles(row.original)}
              sx={{ color: theme.palette.error.main }}
              title="Manage roles"
            >
              <ManageAccounts fontSize="small" />
            </IconButton>
          </PermissionGate>
          <PermissionGate permission={PERMISSIONS.PLATFORM_USERS.WRITE}>
            <IconButton
              size="small"
              onClick={() => onEdit(row.original)}
              sx={{ color: theme.palette.info.main }}
              title="Edit user"
            >
              <Edit fontSize="small" />
            </IconButton>
          </PermissionGate>
          <PermissionGate permission={PERMISSIONS.PLATFORM_USERS.DELETE}>
            <IconButton
              size="small"
              onClick={() => onDelete(row.original.id)}
              sx={{ color: theme.palette.error.light }}
              title="Delete user"
            >
              <Delete fontSize="small" />
            </IconButton>
          </PermissionGate>
        </Box>
      )}
      renderTopToolbarCustomActions={() => (
        <Box sx={{ px: 1, py: 0.5 }}>
          <Typography
            variant="h6"
            sx={{
              fontWeight: 700,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
            }}
          >
            Platform Users
          </Typography>
        </Box>
      )}
      enableColumnOrdering={false}
      enableGlobalFilter={false}
      enableFilters={false}
    />
  );
};
