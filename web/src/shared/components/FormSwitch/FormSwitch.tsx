import { Controller, FieldValues } from "react-hook-form";
import { Switch, FormControlLabel, FormHelperText } from "@mui/material";
import type { FormSwitchProps } from "@/shared/types";

/**
 * FormSwitch component integrated with React Hook Form
 *
 * A wrapper around MUI Switch with FormControlLabel that connects to React Hook Form via Controller.
 * Automatically handles validation errors and displays them below the field.
 *
 * @example
 * ```tsx
 * const { control } = useForm<FormData>({
 *   resolver: zodResolver(schema)
 * });
 *
 * <FormSwitch
 *   name="is_default"
 *   control={control}
 *   label="Set as default tenant"
 * />
 * ```
 */
export function FormSwitch<TFieldValues extends FieldValues>({
  name,
  control,
  label,
  ...switchProps
}: FormSwitchProps<TFieldValues>) {
  return (
    <Controller
      name={name}
      control={control}
      render={({ field, fieldState: { error } }) => (
        <div>
          <FormControlLabel
            control={
              <Switch {...field} {...switchProps} checked={!!field.value} />
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
