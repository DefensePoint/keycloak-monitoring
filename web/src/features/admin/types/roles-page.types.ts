import type { Role, Permission, CreateRoleRequest } from "./rbac.types";

export interface AdminRolesPageHeaderProps {
  rolesCount: number;
  onCreateClick: () => void;
}

export interface RoleCardProps {
  role: Role;
  onView: (role: Role) => void;
  onDelete: (roleId: number, isSystem: boolean) => void;
}

export interface RolesGridProps {
  roles: Role[];
  onView: (role: Role) => void;
  onDelete: (roleId: number, isSystem: boolean) => void;
}

export interface PermissionsSelectorProps {
  permissions: Permission[];
  selectedPermissionIds: number[];
  onTogglePermission: (permissionId: number) => void;
}

export interface CreateRoleModalProps {
  open: boolean;
  permissions: Permission[];
  formData: CreateRoleRequest;
  onFormChange: (data: CreateRoleRequest) => void;
  onSubmit: (e: React.FormEvent) => void;
  onClose: () => void;
}

export interface ViewRoleModalProps {
  open: boolean;
  role: Role | null;
  onClose: () => void;
}
