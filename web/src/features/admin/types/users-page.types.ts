import type { Role, UserWithRoles, CreateUserRequest } from "./rbac.types";

export interface AdminUsersPageHeaderProps {
  usersCount: number;
  onCreateClick: () => void;
}

export interface UsersTableProps {
  users: UserWithRoles[];
  roles: Role[];
  tenants: { tenant_id: string; name: string }[];
  onEdit: (user: UserWithRoles) => void;
  onDelete: (userId: number) => void;
  onManageRoles: (user: UserWithRoles) => void;
  onRevokeRole: (
    userId: number,
    roleId: number,
    tenantId?: string | null,
  ) => void;
}

export interface CreateUserModalProps {
  open: boolean;
  formData: CreateUserRequest;
  onFormChange: (data: CreateUserRequest) => void;
  onSubmit: (e: React.FormEvent) => void;
  onClose: () => void;
}

export interface AssignRoleModalProps {
  open: boolean;
  user: UserWithRoles | null;
  roles: Role[];
  tenants: { tenant_id: string; name: string }[];
  selectedRoleId: number | null;
  selectedTenantId: string | null;
  onRoleChange: (roleId: number) => void;
  onTenantChange: (tenantId: string | null) => void;
  onSubmit: (e: React.FormEvent) => void;
  onClose: () => void;
  isSubmitting?: boolean;
}
