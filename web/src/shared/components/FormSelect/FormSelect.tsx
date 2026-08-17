import { Controller, FieldValues } from "react-hook-form";
import { FormControl, FormHelperText, InputLabel, Select } from "@mui/material";
import type { FormSelectProps } from "@/shared/types";

/**
 * FormSelect component integrated with React Hook Form
 *
 * A wrapper around MUI Select that connects to React Hook Form via Controller.
 * Automatically handles validation errors and displays them below the field.
 *
 * @example
 * ```tsx
 * const { control } = useForm<FormData>({
 *   resolver: zodResolver(schema)
 * });
 *
 * <FormSelect
 *   name="realm"
 *   control={control}
 *   label="Realm"
 *   required
 * >
 *   <MenuItem value="">None</MenuItem>
 *   <MenuItem value="master">Master</MenuItem>
 * </FormSelect>
 * ```
 */
export function FormSelect<TFieldValues extends FieldValues>({
  name,
  control,
  label,
  children,
  ...selectProps
}: FormSelectProps<TFieldValues>) {
  return (
    <Controller
      name={name}
      control={control}
      render={({ field, fieldState: { error } }) => (
        <FormControl fullWidth error={!!error}>
          {label && <InputLabel>{label}</InputLabel>}
          <Select
            {...field}
            {...selectProps}
            label={label}
            // Ensure value is never undefined to avoid uncontrolled -> controlled warning
            value={field.value ?? ""}
          >
            {children}
          </Select>
          {error && <FormHelperText>{error.message}</FormHelperText>}
        </FormControl>
      )}
    />
  );
}
