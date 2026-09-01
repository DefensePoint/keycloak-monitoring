import { z } from "zod";
import {
  urlField,
  optionalString,
  tenantIdField,
} from "@/shared/lib/zodSchemas";

// AMFA settings. The base URL is required only when the integration is on, so a
// tenant can be saved with AMFA off and no endpoint.
const amfaFields = {
  amfa_enabled: z.boolean(),
  amfa_api_base_url: optionalString,
  amfa_events_lookback_days: z.coerce.number().int().min(0).max(90).optional(),
  amfa_api_timeout_seconds: z.coerce.number().int().min(0).max(300).optional(),
};

const requireBaseUrlWhenEnabled = <
  T extends { amfa_enabled: boolean; amfa_api_base_url?: string },
>(
  data: T,
  ctx: z.RefinementCtx,
) => {
  if (!data.amfa_enabled) return;
  const url = data.amfa_api_base_url?.trim() ?? "";
  if (!url) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ["amfa_api_base_url"],
      message: "AMFA endpoint is required when AMFA is enabled",
    });
    return;
  }
  if (!/^https?:\/\//.test(url)) {
    ctx.addIssue({
      code: z.ZodIssueCode.custom,
      path: ["amfa_api_base_url"],
      message: "Must start with http:// or https://",
    });
  }
};

export const createTenantSchema = z
  .object({
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
    ...amfaFields,
  })
  .superRefine(requireBaseUrlWhenEnabled);

export const updateTenantSchema = z
  .object({
    name: z.string().min(1, "Name is required"),
    description: optionalString,
    server_url: urlField,
    admin_realm: z.string().min(1, "Admin realm is required"),
    client_id: optionalString,
    client_secret: optionalString,
    enabled: z.boolean(),
    is_default: z.boolean(),
    default_realm: optionalString,
    ...amfaFields,
  })
  .superRefine(requireBaseUrlWhenEnabled);

export type CreateTenantFormData = z.infer<typeof createTenantSchema>;
export type UpdateTenantFormData = z.infer<typeof updateTenantSchema>;
