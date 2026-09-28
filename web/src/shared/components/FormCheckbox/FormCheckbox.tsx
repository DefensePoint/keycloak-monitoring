import { Controller, FieldValues } from "react-hook-form";
import { Checkbox, FormControlLabel, FormHelperText } from "@mui/material";
import type { FormCheckboxProps } from "@/shared/types";

/**
 * FormCheckbox component integrated with React Hook Form
 *
 * A wrapper around MUI Checkbox with FormControlLabel that connects to React Hook Form via Controller.
 * Automatically handles validation errors and displays them below the field.
 *
 * @example
 * ```tsx
 * const { control } = useForm<FormData>({
 *   resolver: zodResolver(schema)
 * });
 *
 * <FormCheckbox
 *   name="enabled"
 *   control={control}
 *   label="Enable monitoring for this tenant"
 * />
 * ```
 */
export function FormCheckbox<TFieldValues extends FieldValues>({
  name,
  control,
  label,
  ...checkboxProps
}: FormCheckboxProps<TFieldValues>) {
  return (
    <Controller
      name={name}
      control={control}
      render={({ field, fieldState: { error } }) => (
        <div>
          <FormControlLabel
            control={
              <Checkbox {...field} {...checkboxProps} checked={!!field.value} />
            }
            label={label || ""}
          />
          {error && (
            <FormHelperText error sx={{ mt: 0, ml: 0 }}>
              {error.message}
            </FormHelperText>
          )}
        </div>
      )}
    />
  );
}
