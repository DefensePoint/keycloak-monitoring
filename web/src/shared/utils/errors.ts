interface ApiErrorResponse {
  response?: {
    data?: {
      error?: string;
      message?: string;
    };
  };
}

/**
 * Patterns that indicate sensitive information in error messages
 */
const SENSITIVE_PATTERNS = [
  /stack\s*trace/i,
  /at\s+\w+\s+\(/i, // Stack trace line pattern
  /\/home\//i,
  /\/var\//i,
  /\/usr\//i,
  /C:\\|D:\\/i, // Windows paths
  /password/i,
  /secret/i,
  /token/i,
  /SQL|SELECT|INSERT|UPDATE|DELETE|FROM|WHERE/i, // SQL keywords
  /ECONNREFUSED|ETIMEDOUT|ENOTFOUND/i, // Network errors
  /errno|syscall/i, // System errors
];

/**
 * Checks if an error message contains sensitive information
 */
function containsSensitiveInfo(message: string): boolean {
  return SENSITIVE_PATTERNS.some((pattern) => pattern.test(message));
}

/**
 * Extracts error message from API error responses
 * Sanitizes messages to prevent leaking sensitive system information
 * @param err - The error object from API call
 * @param defaultMessage - Fallback message if no error message found
 * @returns The error message string (sanitized)
 */
export function getErrorMessage(err: unknown, defaultMessage: string): string {
  if (err && typeof err === "object") {
    const apiError = err as ApiErrorResponse;
    const errorMsg =
      apiError.response?.data?.error || apiError.response?.data?.message;

    // Only return API error messages if they don't contain sensitive info
    if (errorMsg && !containsSensitiveInfo(errorMsg)) {
      return errorMsg;
    }

    // For Error objects, only use message if it's safe
    if (err instanceof Error && !containsSensitiveInfo(err.message)) {
      return err.message;
    }
  }
  return defaultMessage;
}
