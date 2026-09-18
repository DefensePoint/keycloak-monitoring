import type {
  TextFieldProps,
  CheckboxProps,
  SelectProps,
  SwitchProps,
} from "@mui/material";
import type { Control, FieldPath, FieldValues } from "react-hook-form";

/**
 * Base props for all form components integrated with React Hook Form
 */
export interface BaseFormFieldProps<TFieldValues extends FieldValues> {
  /**
   * Name of the field in the form (must match a key in the form schema)
   */
  name: FieldPath<TFieldValues>;

  /**
   * React Hook Form control instance
   */
  control: Control<TFieldValues>;
}

/**
 * Props for FormTextField component
 */
export interface FormTextFieldProps<TFieldValues extends FieldValues>
  extends Omit<TextFieldProps, "name">, BaseFormFieldProps<TFieldValues> {}

/**
 * Props for FormSelect component
 */
export interface FormSelectProps<TFieldValues extends FieldValues>
  extends Omit<SelectProps, "name">, BaseFormFieldProps<TFieldValues> {}

/**
 * Props for FormCheckbox component
 */
export interface FormCheckboxProps<TFieldValues extends FieldValues>
  extends Omit<CheckboxProps, "name">, BaseFormFieldProps<TFieldValues> {
  /**
   * Label for the checkbox
   */
  label?: string;
}

/**
 * Props for FormSwitch component
 */
export interface FormSwitchProps<TFieldValues extends FieldValues>
  extends Omit<SwitchProps, "name">, BaseFormFieldProps<TFieldValues> {
  /**
   * Label for the switch
   */
  label?: string;
}
