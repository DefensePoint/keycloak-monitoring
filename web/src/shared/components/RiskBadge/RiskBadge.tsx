import { Chip } from "@mui/material";
import type { RiskLevel } from "@/shared/types";

interface Props {
  risk: RiskLevel | null | undefined;
}

const colorMap: Record<RiskLevel, "success" | "info" | "warning" | "error"> = {
  1: "success",
  2: "info",
  3: "warning",
  4: "error",
};

/**
 * Numeric risk-level chip (1 green .. 4 red). Renders a dash when the row has
 * no risk level (e.g. non-AMFA events in the Events list).
 */
export function RiskBadge({ risk }: Props) {
  if (risk == null) {
    return <span>—</span>;
  }
  return <Chip label={String(risk)} color={colorMap[risk]} size="small" />;
}
