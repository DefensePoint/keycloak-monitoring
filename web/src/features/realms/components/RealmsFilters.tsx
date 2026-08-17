import React from "react";
import {
  Box,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Button,
} from "@mui/material";
import { FilterAltOff } from "@mui/icons-material";
import type { RealmsFiltersProps } from "../types";

export const RealmsFilters: React.FC<RealmsFiltersProps> = ({
  searchTerm,
  statusFilter,
  healthFilter,
  eventWarningFilter,
  onSearchChange,
  onStatusChange,
  onHealthChange,
  onEventWarningChange,
  onReset,
}) => {
  const hasActiveFilters =
    searchTerm || statusFilter || healthFilter || eventWarningFilter !== null;

  return (
    <Box
      sx={{
        bgcolor: "background.paper",
        border: "1px solid",
        borderColor: "divider",
        borderRadius: 1,
        p: 2,
        mb: 3,
      }}
    >
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          gap: 2,
          alignItems: "center",
        }}
      >
        <TextField
          size="small"
          label="Search"
          placeholder="Realm name..."
          value={searchTerm}
          onChange={(e) => onSearchChange(e.target.value)}
          sx={{ minWidth: 180 }}
        />

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Status</InputLabel>
          <Select
            value={statusFilter}
            label="Status"
            onChange={(e) => onStatusChange(e.target.value)}
          >
            <MenuItem value="">All</MenuItem>
            <MenuItem value="enabled">Enabled</MenuItem>
            <MenuItem value="disabled">Disabled</MenuItem>
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Health</InputLabel>
          <Select
            value={healthFilter}
            label="Health"
            onChange={(e) => onHealthChange(e.target.value)}
          >
            <MenuItem value="">All</MenuItem>
            <MenuItem value="healthy">Healthy</MenuItem>
            <MenuItem value="unhealthy">Unhealthy</MenuItem>
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel>Events</InputLabel>
          <Select
            value={
              eventWarningFilter === null ? "" : eventWarningFilter.toString()
            }
            label="Events"
            onChange={(e) =>
              onEventWarningChange(
                e.target.value === "" ? null : e.target.value === "true",
              )
            }
          >
            <MenuItem value="">All</MenuItem>
            <MenuItem value="false">Configured</MenuItem>
            <MenuItem value="true">Warning</MenuItem>
          </Select>
        </FormControl>

        <Box sx={{ flex: 1 }} />

        {hasActiveFilters && (
          <Button
            variant="contained"
            size="small"
            startIcon={<FilterAltOff />}
            onClick={onReset}
          >
            Reset
          </Button>
        )}
      </Box>
    </Box>
  );
};
