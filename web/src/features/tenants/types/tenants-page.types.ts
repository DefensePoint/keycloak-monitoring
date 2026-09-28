import type {
  Tenant,
  TenantCreate,
  TenantUpdate,
  RealmListItem,
} from "@/shared/types";

// Component Props
export interface TenantsPageHeaderProps {
  onAddClick: () => void;
  tenantsCount: number;
}

export interface TenantFormProps {
  open: boolean;
  isCreating: boolean;
  editingTenant: Tenant | null;
  realms: RealmListItem[];
  error: string | null;
  isSubmitting: boolean;
  onSubmit: (data: TenantCreate | TenantUpdate) => void;
  onCancel: () => void;
}

export interface TenantCardProps {
  tenant: Tenant;
  onEdit: (tenant: Tenant) => void;
  onDelete: (tenantId: string) => void;
  onToggleEnabled: (tenant: Tenant) => void;
}

export interface TenantsListProps {
  tenants: Tenant[];
  onEdit: (tenant: Tenant) => void;
  onDelete: (tenantId: string) => void;
  onToggleEnabled: (tenant: Tenant) => void;
  onCreateClick: () => void;
}
