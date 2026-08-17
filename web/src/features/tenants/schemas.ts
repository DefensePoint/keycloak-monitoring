import { z } from "zod";
import {
  urlField,
  optionalString,
  tenantIdField,
} from "@/shared/lib/zodSchemas";

export const createTenantSchema = z.object({
  tenant_id: tenantIdField,
  name: z.string().min(1, "Name is required"),
  description: optionalString,
  server_url: urlField,
  admin_realm: z.string().min(1, "Admin realm is required"),
  client_id: z.string().min(1, "Client ID is required"),
  client_secret: z.string().min(1, "Client secret is required"),
  enabled: z.boolean(),
  is_default: z.boolean(),
  default_realm: optionalString,
});

export const updateTenantSchema = z.object({
  name: z.string().min(1, "Name is required"),
  description: optionalString,
  server_url: urlField,
  admin_realm: z.string().min(1, "Admin realm is required"),
  client_id: optionalString,
  client_secret: optionalString,
  enabled: z.boolean(),
  is_default: z.boolean(),
  default_realm: optionalString,
});

export type CreateTenantFormData = z.infer<typeof createTenantSchema>;
export type UpdateTenantFormData = z.infer<typeof updateTenantSchema>;
