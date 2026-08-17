import { z } from "zod";

/**
 * Common reusable Zod schemas for form validation
 *
 * These schemas provide consistent validation across the application.
 * Compose them together to build complex form schemas.
 */

// ============================================================================
// Basic Fields
// ============================================================================

/**
 * Required string field (non-empty)
 */
export const requiredString = z.string().min(1, "This field is required");

/**
 * Optional string field (can be empty or undefined)
 */
export const optionalString = z.string().optional();

/**
 * Required email field with validation
 */
export const emailField = z
  .string()
  .min(1, "Email is required")
  .email("Invalid email address");

/**
 * Optional email field with validation
 */
export const optionalEmailField = z
  .string()
  .email("Invalid email address")
  .optional()
  .or(z.literal(""));

/**
 * URL field with validation
 */
export const urlField = z
  .string()
  .min(1, "URL is required")
  .url("Invalid URL format");

/**
 * Optional URL field with validation
 */
export const optionalUrlField = z
  .string()
  .url("Invalid URL format")
  .optional()
  .or(z.literal(""));

// ============================================================================
// Password Fields
// ============================================================================

/**
 * Strong password validation
 * - Minimum 12 characters
 * - At least one uppercase letter
 * - At least one lowercase letter
 * - At least one number
 * - At least one special character
 */
export const strongPasswordField = z
  .string()
  .min(12, "Password must be at least 12 characters")
  .regex(/[A-Z]/, "Password must contain at least one uppercase letter")
  .regex(/[a-z]/, "Password must contain at least one lowercase letter")
  .regex(/[0-9]/, "Password must contain at least one number")
  .regex(
    /[^A-Za-z0-9]/,
    "Password must contain at least one special character",
  );

/**
 * Basic password validation (minimum 8 characters)
 */
export const passwordField = z
  .string()
  .min(8, "Password must be at least 8 characters");

/**
 * Optional password field (used for edit forms where password change is optional)
 */
export const optionalPasswordField = z.string().optional().or(z.literal(""));

/**
 * Optional strong password field
 * If provided, must meet strong password requirements.
 * Empty string or undefined is allowed.
 */
export const optionalStrongPasswordField = z
  .string()
  .optional()
  .refine(
    (val) => {
      // Empty or undefined is valid
      if (!val || val.length === 0) return true;
      // If provided, must pass strong password validation
      return strongPasswordField.safeParse(val).success;
    },
    {
      message:
        "Password must be at least 12 characters with uppercase, lowercase, number, and special character",
    },
  );

// ============================================================================
// Numeric Fields
// ============================================================================

/**
 * Positive integer field
 */
export const positiveIntField = z.number().int().positive("Must be positive");

/**
 * Non-negative integer field (includes 0)
 */
export const nonNegativeIntField = z
  .number()
  .int()
  .nonnegative("Cannot be negative");

/**
 * Port number field (1-65535)
 */
export const portField = z
  .number()
  .int()
  .min(1, "Port must be at least 1")
  .max(65535, "Port must be at most 65535");

// ============================================================================
// Boolean Fields
// ============================================================================

/**
 * Required boolean field
 */
export const booleanField = z.boolean();

/**
 * Checkbox that must be checked (useful for terms acceptance)
 */
export const requiredCheckbox = z
  .boolean()
  .refine((val) => val === true, "You must accept to continue");

// ============================================================================
// Date/Time Fields
// ============================================================================

/**
 * Date field (accepts Date object)
 */
export const dateField = z.date();

/**
 * ISO date string field
 */
export const isoDateField = z.string().datetime();

/**
 * Optional ISO date string field
 */
export const optionalIsoDateField = z.string().datetime().optional();

// ============================================================================
// ID Fields
// ============================================================================

/**
 * UUID field validation
 */
export const uuidField = z.string().uuid("Invalid UUID format");

/**
 * Slug field (lowercase, hyphens, underscores only)
 */
export const slugField = z
  .string()
  .min(1, "Slug is required")
  .regex(
    /^[a-z0-9_-]+$/,
    "Slug must contain only lowercase letters, numbers, hyphens, and underscores",
  );

/**
 * Username field (alphanumeric, underscores, hyphens, 3-30 chars)
 */
export const usernameField = z
  .string()
  .min(3, "Username must be at least 3 characters")
  .max(30, "Username must be at most 30 characters")
  .regex(
    /^[a-zA-Z0-9_-]+$/,
    "Username can only contain letters, numbers, underscores, and hyphens",
  );

// ============================================================================
// Array Fields
// ============================================================================

/**
 * Non-empty array field
 */
export const nonEmptyArray = <T extends z.ZodTypeAny>(schema: T) =>
  z.array(schema).min(1, "At least one item is required");

/**
 * Array with specific length
 */
export const arrayWithLength = <T extends z.ZodTypeAny>(
  schema: T,
  length: number,
) => z.array(schema).length(length, `Must have exactly ${length} items`);

// ============================================================================
// Keycloak/Auth Specific
// ============================================================================

/**
 * Keycloak realm name field
 */
export const realmNameField = z
  .string()
  .min(1, "Realm name is required")
  .regex(
    /^[a-zA-Z0-9_-]+$/,
    "Realm name can only contain letters, numbers, underscores, and hyphens",
  );

/**
 * Tenant ID field
 */
export const tenantIdField = z
  .string()
  .min(1, "Tenant ID is required")
  .regex(
    /^[a-z0-9_-]+$/,
    "Tenant ID must contain only lowercase letters, numbers, hyphens, and underscores",
  );

/**
 * Role name field (lowercase with underscores)
 */
export const roleNameField = z
  .string()
  .min(1, "Role name is required")
  .regex(
    /^[a-z_]+$/,
    "Role name must contain only lowercase letters and underscores",
  );
