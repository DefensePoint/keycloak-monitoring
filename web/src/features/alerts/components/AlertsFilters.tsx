import { memo } from "react";
import {
  Box,
  FormControl,
  Select,
  MenuItem,
  InputLabel,
  Button,
  Typography,
  Chip,
} from "@mui/material";
import { FilterAltOff, Refresh, Sync } from "@mui/icons-material";
import { FilterCard } from "@/shared/components";

type ChipColor = "error" | "warning" | "success" | "default";

const STATUS_OPTIONS: { value: string; label: string; color: ChipColor }[] = [
  { value: "active", label: "Active", color: "error" },
  { value: "acknowledged", label: "Acknowledged", color: "warning" },
  { value: "resolved", label: "Resolved", color: "success" },
  { value: "ignored", label: "Ignored", color: "default" },
];

interface AlertsFiltersProps {
  statusFilter: string;
  severityFilter: string;
  realmFilter: string;
  resourceFilter: string;
  uniqueRealms: string[];
  uniqueResourceTypes: string[];
  filteredCount: number;
  totalCount: number;
  onStatusChange: (value: string) => void;
  onSeverityChange: (value: string) => void;
  onRealmChange: (value: string) => void;
  onResourceChange: (value: string) => void;
  onClearFilters: () => void;
  onRefresh: () => void;
  showReloadAmfa?: boolean;
  onReloadAmfa?: () => void;
  reloadingAmfa?: boolean;
}

export const AlertsFilters = memo(function AlertsFilters({
  statusFilter,
  severityFilter,
  realmFilter,
  resourceFilter,
  uniqueRealms,
  uniqueResourceTypes,
  filteredCount,
  totalCount,
  onStatusChange,
  onSeverityChange,
  onRealmChange,
  onResourceChange,
  onClearFilters,
  onRefresh,
  showReloadAmfa = false,
  onReloadAmfa,
  reloadingAmfa = false,
}: AlertsFiltersProps) {
  const hasActiveFilters =
    severityFilter !== "all" ||
    realmFilter !== "all" ||
    resourceFilter !== "all";

  const selectedStatus = STATUS_OPTIONS.find((s) => s.value === statusFilter);

  return (
    <FilterCard
      sx={{ mb: 3 }}
      footer={
        <Typography variant="body2" color="text.secondary">
          Showing {filteredCount} of {totalCount} matching alerts
        </Typography>
      }
    >
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            sm: "repeat(2, 1fr)",
            lg: "1fr 1fr 1fr 1fr auto auto auto",
          },
          gap: 2,
          alignItems: "center",
        }}
      >
        <FormControl size="small" fullWidth>
          <InputLabel
            id="status-filter-label"
            sx={{ textTransform: "uppercase", fontSize: "0.75rem" }}
          >
            Status
          </InputLabel>
          <Select
            labelId="status-filter-label"
            id="status-filter"
            value={statusFilter}
            onChange={(e) => onStatusChange(e.target.value)}
            label="Status"
            renderValue={() => (
              <Chip
                label={selectedStatus?.label}
                color={selectedStatus?.color}
                size="small"
                sx={{ fontWeight: 600 }}
              />
            )}
          >
            {STATUS_OPTIONS.map((option) => (
              <MenuItem key={option.value} value={option.value}>
                <Chip
                  label={option.label}
                  color={option.color}
                  size="small"
                  sx={{ fontWeight: 600 }}
                />
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" fullWidth>
          <InputLabel
            id="severity-filter-label"
            sx={{ textTransform: "uppercase", fontSize: "0.75rem" }}
          >
            Severity
          </InputLabel>
          <Select
            labelId="severity-filter-label"
            id="severity-filter"
            value={severityFilter}
            onChange={(e) => onSeverityChange(e.target.value)}
            label="Severity"
          >
            <MenuItem value="all">All Severities</MenuItem>
            <MenuItem value="critical">Critical</MenuItem>
            <MenuItem value="error">Error</MenuItem>
            <MenuItem value="warning">Warning</MenuItem>
            <MenuItem value="info">Info</MenuItem>
          </Select>
        </FormControl>
        <FormControl size="small" fullWidth>
          <InputLabel
            id="realm-filter-label"
            sx={{ textTransform: "uppercase", fontSize: "0.75rem" }}
          >
            Realm
          </InputLabel>
          <Select
            labelId="realm-filter-label"
            id="realm-filter"
            value={realmFilter}
            onChange={(e) => onRealmChange(e.target.value)}
            label="Realm"
          >
            <MenuItem value="all">All Realms</MenuItem>
            {uniqueRealms.map((realm) => (
              <MenuItem key={realm} value={realm}>
                {realm}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small" fullWidth>
          <InputLabel
            id="resource-filter-label"
            sx={{ textTransform: "uppercase", fontSize: "0.75rem" }}
          >
            Resource Type
          </InputLabel>
          <Select
            labelId="resource-filter-label"
            id="resource-filter"
            value={resourceFilter}
            onChange={(e) => onResourceChange(e.target.value)}
            label="Resource Type"
          >
            <MenuItem value="all">All Resources</MenuItem>
            {uniqueResourceTypes.map((type) => (
              <MenuItem key={type} value={type}>
                {type}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <Button
          variant="outlined"
          size="small"
          startIcon={<FilterAltOff />}
          onClick={onClearFilters}
          disabled={!hasActiveFilters}
        >
          Reset
        </Button>
        {showReloadAmfa && (
          <Button
            variant="outlined"
            size="small"
            startIcon={<Sync />}
            onClick={onReloadAmfa}
            disabled={reloadingAmfa}
          >
            {reloadingAmfa ? "Reload AMFA alerts..." : "Reload AMFA alerts"}
          </Button>
        )}
        <Button
          variant="contained"
          size="small"
          startIcon={<Refresh />}
          onClick={onRefresh}
        >
          Refresh
        </Button>
      </Box>
    </FilterCard>
  );
});
