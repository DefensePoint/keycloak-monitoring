import {
  Autocomplete,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  TextField,
} from "@mui/material";
import type { EventFilters as EventFilterValues } from "@/shared/services";

const SEVERITIES = ["info", "warning", "error", "critical"] as const;
const RISK_LEVELS = [1, 2, 3, 4] as const;

export interface EventFiltersProps {
  value: EventFilterValues;
  onChange: (next: EventFilterValues) => void;
  // The risk filter only means something where AMFA scores events, so it is
  // shown only when the Risk column is.
  showMinRisk: boolean;
  // Suggestions for the type field. Any type can still be typed in.
  typeOptions: string[];
}

// EventFilters holds the server-side type, severity and minimum-risk filters
// for the events list. Each change reports the full filter set.
export function EventFilters({
  value,
  onChange,
  showMinRisk,
  typeOptions,
}: EventFiltersProps) {
  return (
    <>
      {/* Committed on Enter, pick or blur rather than per keystroke, so a
          half-typed type never fires a query. */}
      <Autocomplete
        freeSolo
        autoSelect
        size="small"
        sx={{ minWidth: 200 }}
        options={typeOptions}
        value={value.type ?? null}
        onChange={(_, next) => {
          const type = next?.trim() || undefined;
          if (type !== value.type) onChange({ ...value, type });
        }}
        renderInput={(params) => <TextField {...params} label="Event type" />}
      />
      {/* The selects show "All"/"Any" when empty, so their labels are
          always shrunk; otherwise the label sits on top of that text. */}
      <FormControl size="small" sx={{ minWidth: 140 }}>
        <InputLabel id="event-severity-filter-label" shrink>
          Severity
        </InputLabel>
        <Select
          labelId="event-severity-filter-label"
          label="Severity"
          value={value.severity ?? ""}
          displayEmpty
          notched
          onChange={(e) =>
            onChange({ ...value, severity: e.target.value || undefined })
          }
        >
          <MenuItem value="">All</MenuItem>
          {SEVERITIES.map((s) => (
            <MenuItem key={s} value={s}>
              {s.charAt(0).toUpperCase() + s.slice(1)}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      {showMinRisk && (
        <FormControl size="small" sx={{ minWidth: 120 }}>
          <InputLabel id="event-min-risk-filter-label" shrink>
            Min risk
          </InputLabel>
          <Select
            labelId="event-min-risk-filter-label"
            label="Min risk"
            value={value.minRisk === undefined ? "" : String(value.minRisk)}
            displayEmpty
            notched
            onChange={(e) =>
              onChange({
                ...value,
                minRisk:
                  e.target.value === "" ? undefined : Number(e.target.value),
              })
            }
          >
            <MenuItem value="">Any</MenuItem>
            {RISK_LEVELS.map((r) => (
              <MenuItem key={r} value={String(r)}>
                {r}+
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      )}
    </>
  );
}
