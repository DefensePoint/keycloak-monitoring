import { z } from "zod";
import {
  usernameField,
  emailField,
  strongPasswordField,
  optionalPasswordField,
  requiredString,
  roleNameField,
  nonEmptyArray,
} from "./zodSchemas";

export const createUserSchema = z.object({
  username: usernameField,
  email: emailField,
  name: requiredString,
  password: strongPasswordField,
});

export const updateUserSchema = z.object({
  username: usernameField,
  email: emailField,
  name: requiredString,
  password: optionalPasswordField,
  is_active: z.boolean(),
  is_blocked: z.boolean(),
});

export type CreateUserFormData = z.infer<typeof createUserSchema>;
export type UpdateUserFormData = z.infer<typeof updateUserSchema>;

export const createRoleSchema = z.object({
  name: roleNameField,
  display_name: requiredString,
  description: requiredString,
  permission_ids: nonEmptyArray(z.number()),
});

export const updateRoleSchema = z.object({
  display_name: requiredString,
  description: requiredString,
  permission_ids: z.array(z.number()),
});

export type CreateRoleFormData = z.infer<typeof createRoleSchema>;
export type UpdateRoleFormData = z.infer<typeof updateRoleSchema>;
