import { z } from "zod";

/**
 * Custom Zod validators for common validation patterns
 *
 * Use these to create custom validation logic beyond basic type checking.
 */

/**
 * Validates that a password confirmation matches the password
 *
 * @example
 * ```tsx
 * const schema = z.object({
 *   password: z.string().min(8),
 *   confirmPassword: z.string()
 * }).refine(passwordsMatch, {
 *   message: "Passwords don't match",
 *   path: ["confirmPassword"]
 * });
 * ```
 */
export const passwordsMatch = (data: {
  password: string;
  confirmPassword: string;
}) => data.password === data.confirmPassword;

/**
 * Validates that a value is one of the allowed values
 *
 * @example
 * ```tsx
 * const severityField = z.string().refine(
 *   isOneOf(['critical', 'high', 'medium', 'low']),
 *   { message: 'Invalid severity level' }
 * );
 * ```
 */
export const isOneOf =
  <T>(allowedValues: readonly T[]) =>
  (value: T) =>
    allowedValues.includes(value);

/**
 * Validates that a string doesn't contain whitespace
 */
export const noWhitespace = (value: string) => !/\s/.test(value);

/**
 * Validates that a string is a valid Keycloak server URL
 * Must be HTTPS in production
 */
export const isValidKeycloakUrl = (url: string) => {
  try {
    const parsed = new URL(url);
    // Allow HTTP only for localhost/127.0.0.1 (dev environments)
    if (parsed.protocol === "http:") {
      return (
        parsed.hostname === "localhost" ||
        parsed.hostname === "127.0.0.1" ||
        parsed.hostname.endsWith(".local")
      );
    }
    return parsed.protocol === "https:";
  } catch {
    return false;
  }
};

/**
 * Validates that at least one checkbox in a group is selected
 *
 * @example
 * ```tsx
 * const schema = z.object({
 *   permissions: z.array(z.number())
 * }).refine(
 *   (data) => atLeastOneSelected(data.permissions),
 *   { message: 'Select at least one permission', path: ['permissions'] }
 * );
 * ```
 */
export const atLeastOneSelected = <T>(arr: T[]) => arr.length > 0;

/**
 * Validates that a date is in the future
 */
export const isFutureDate = (date: Date) => date > new Date();

/**
 * Validates that a date is in the past
 */
export const isPastDate = (date: Date) => date < new Date();

/**
 * Validates that a date range is valid (start before end)
 */
export const isValidDateRange = (data: { start: Date; end: Date }) =>
  data.start < data.end;

/**
 * Validates that a file size is within limit (in bytes)
 *
 * @example
 * ```tsx
 * const MAX_FILE_SIZE = 5 * 1024 * 1024; // 5MB
 * const fileField = z.instanceof(File).refine(
 *   (file) => isFileSizeValid(file, MAX_FILE_SIZE),
 *   { message: 'File must be less than 5MB' }
 * );
 * ```
 */
export const isFileSizeValid = (file: File, maxSize: number) =>
  file.size <= maxSize;

/**
 * Validates that a file type is allowed
 *
 * @example
 * ```tsx
 * const imageField = z.instanceof(File).refine(
 *   (file) => isFileTypeValid(file, ['image/png', 'image/jpeg']),
 *   { message: 'Only PNG and JPEG images are allowed' }
 * );
 * ```
 */
export const isFileTypeValid = (file: File, allowedTypes: string[]) =>
  allowedTypes.includes(file.type);

/**
 * Validates IPv4 address format
 */
export const isIPv4 = (value: string) => {
  const ipv4Regex =
    /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
  return ipv4Regex.test(value);
};

/**
 * Validates that a string is alphanumeric
 */
export const isAlphanumeric = (value: string) => /^[a-zA-Z0-9]+$/.test(value);

/**
 * Validates that a string contains only lowercase letters and underscores
 * (common for database/API identifiers)
 */
export const isSnakeCase = (value: string) => /^[a-z_]+$/.test(value);

/**
 * Validates that a string is in kebab-case format
 */
export const isKebabCase = (value: string) =>
  /^[a-z0-9]+(-[a-z0-9]+)*$/.test(value);

/**
 * Creates a min-max length validator with custom messages
 */
export const stringLength = (min: number, max: number) =>
  z
    .string()
    .min(min, `Must be at least ${min} characters`)
    .max(max, `Must be at most ${max} characters`);

/**
 * Creates a numeric range validator
 */
export const numberRange = (min: number, max: number) =>
  z
    .number()
    .min(min, `Must be at least ${min}`)
    .max(max, `Must be at most ${max}`);
