import React from "react";
import {
  Box,
  TextField,
  MenuItem,
  Button,
  FormControl,
  InputLabel,
  Select,
} from "@mui/material";
import { TimeSelector } from "@/shared/components/TimeSelector";
import type { UserEventsFiltersProps } from "../types";

export const UserEventsFilters: React.FC<UserEventsFiltersProps> = ({
  searchTerm,
  severityFilter,
  sourceFilter,
  limit,
  onSearchChange,
  onSeverityChange,
  onSourceChange,
  onLimitChange,
  onReset,
  onTimeRangeChange,
}) => {
  return (
    <Box sx={{ mb: 3 }}>
      {/* Time Selector and Limit */}
      <Box
        sx={{
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          justifyContent: { sm: "space-between" },
          alignItems: { sm: "center" },
          gap: 2,
          mb: 3,
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "flex-start", sm: "center" },
            gap: 2,
          }}
        >
          <TimeSelector onTimeRangeChange={onTimeRangeChange} />
          <FormControl size="small" sx={{ minWidth: 150 }}>
            <InputLabel id="limit-select-label">Show</InputLabel>
            <Select
              labelId="limit-select-label"
              id="limit-select"
              value={limit}
              label="Show"
              onChange={(e) => onLimitChange(Number(e.target.value))}
            >
              <MenuItem value={25}>25 events</MenuItem>
              <MenuItem value={50}>50 events</MenuItem>
              <MenuItem value={100}>100 events</MenuItem>
              <MenuItem value={200}>200 events</MenuItem>
            </Select>
          </FormControl>
        </Box>
      </Box>

      {/* Filters */}
      <Box
        sx={{
          bgcolor: "background.paper",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
          p: 2,
        }}
      >
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "1fr",
              sm: "repeat(2, 1fr)",
              lg: "repeat(4, 1fr)",
            },
            gap: 2,
          }}
        >
          <TextField
            size="small"
            label="Search"
            placeholder="Type, description..."
            value={searchTerm}
            onChange={(e) => onSearchChange(e.target.value)}
          />

          <FormControl size="small">
            <InputLabel id="severity-filter-label">Severity</InputLabel>
            <Select
              labelId="severity-filter-label"
              id="severity-filter"
              value={severityFilter}
              label="Severity"
              onChange={(e) => onSeverityChange(e.target.value)}
            >
              <MenuItem value="">All Severities</MenuItem>
              <MenuItem value="info">Info</MenuItem>
              <MenuItem value="warning">Warning</MenuItem>
              <MenuItem value="error">Error</MenuItem>
            </Select>
          </FormControl>

          <TextField
            size="small"
            label="Source"
            placeholder="Filter by source..."
            value={sourceFilter}
            onChange={(e) => onSourceChange(e.target.value)}
          />

          <Box sx={{ display: "flex", alignItems: "flex-end" }}>
            <Button
              fullWidth
              variant="outlined"
              onClick={onReset}
              sx={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                fontWeight: 600,
              }}
            >
              Reset Filters
            </Button>
          </Box>
        </Box>
      </Box>
    </Box>
  );
};
