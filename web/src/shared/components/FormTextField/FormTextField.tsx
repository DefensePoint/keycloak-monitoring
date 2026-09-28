import { Controller, FieldValues } from "react-hook-form";
import { TextField } from "@mui/material";
import type { FormTextFieldProps } from "@/shared/types";

/**
 * FormTextField component integrated with React Hook Form
 *
 * A wrapper around MUI TextField that connects to React Hook Form via Controller.
 * Automatically handles validation errors and displays them below the field.
 *
 * @example
 * ```tsx
 * const { control } = useForm<FormData>({
 *   resolver: zodResolver(schema)
 * });
 *
 * <FormTextField
 *   name="email"
 *   control={control}
 *   label="Email"
 *   type="email"
 *   required
 * />
 * ```
 */
export function FormTextField<TFieldValues extends FieldValues>({
  name,
  control,
  ...textFieldProps
}: FormTextFieldProps<TFieldValues>) {
  return (
    <Controller
      name={name}
      control={control}
      render={({ field, fieldState: { error } }) => (
        <TextField
          {...field}
          {...textFieldProps}
          error={!!error}
          helperText={error?.message || textFieldProps.helperText}
          // Ensure value is never undefined to avoid uncontrolled -> controlled warning
          value={field.value ?? ""}
        />
      )}
    />
  );
}
