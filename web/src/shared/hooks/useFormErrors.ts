import { useEffect } from "react";
import { UseFormSetError, FieldValues, Path } from "react-hook-form";

/**
 * API error response structure
 */
export interface ApiFormError {
  field?: string;
  message: string;
}

/**
 * Options for useFormErrors hook
 */
export interface UseFormErrorsOptions<TFieldValues extends FieldValues> {
  /**
   * React Hook Form setError function
   */
  setError: UseFormSetError<TFieldValues>;

  /**
   * Error from API/mutation (can be Error, string, or structured error)
   */
  error: unknown;

  /**
   * Optional custom error mapper to transform API errors
   */
  errorMapper?: (error: unknown) => ApiFormError[] | null;
}

/**
 * Default error mapper that handles common API error formats
 */
function defaultErrorMapper(error: unknown): ApiFormError[] | null {
  // Handle Error instances
  if (error instanceof Error) {
    return [{ message: error.message }];
  }

  // Handle string errors
  if (typeof error === "string") {
    return [{ message: error }];
  }

  // Handle structured API errors with validation fields
  if (
    error &&
    typeof error === "object" &&
    "response" in error &&
    error.response &&
    typeof error.response === "object" &&
    "data" in error.response
  ) {
    const data = (error.response as { data: unknown }).data;

    // Handle array of field errors: [{ field: 'email', message: 'Invalid' }]
    if (Array.isArray(data)) {
      return data;
    }

    // Handle object with errors field: { errors: [...] }
    if (
      data &&
      typeof data === "object" &&
      "errors" in data &&
      Array.isArray((data as { errors: unknown }).errors)
    ) {
      return (data as { errors: ApiFormError[] }).errors;
    }

    // Handle object with message field: { message: 'Error' }
    if (data && typeof data === "object" && "message" in data) {
      return [{ message: String((data as { message: unknown }).message) }];
    }
  }

  return null;
}

/**
 * Custom hook for handling API/server errors and mapping them to form fields
 *
 * Automatically sets field-specific errors or root form errors based on API response.
 * Useful for displaying server-side validation errors in forms.
 *
 * @example
 * ```tsx
 * const loginSchema = z.object({
 *   username: z.string(),
 *   password: z.string()
 * });
 *
 * function LoginForm() {
 *   const { control, setError, handleSubmit } = useAppForm({
 *     schema: loginSchema
 *   });
 *
 *   const { mutate, error } = useMutation(loginUser);
 *
 *   // Automatically map API errors to form fields
 *   useFormErrors({ setError, error });
 *
 *   const onSubmit = (data) => mutate(data);
 *
 *   return <form onSubmit={handleSubmit(onSubmit)}>...</form>;
 * }
 * ```
 */
export function useFormErrors<TFieldValues extends FieldValues>({
  setError,
  error,
  errorMapper = defaultErrorMapper,
}: UseFormErrorsOptions<TFieldValues>) {
  useEffect(() => {
    if (!error) return;

    const mappedErrors = errorMapper(error);
    if (!mappedErrors) return;

    mappedErrors.forEach((err) => {
      if (err.field) {
        // Set field-specific error
        setError(err.field as Path<TFieldValues>, {
          type: "server",
          message: err.message,
        });
      } else {
        // Set root error if no field is specified
        setError("root" as Path<TFieldValues>, {
          type: "server",
          message: err.message,
        });
      }
    });
  }, [error, setError, errorMapper]);
}
