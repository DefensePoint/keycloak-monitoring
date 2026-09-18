import React from "react";
import { Box, TextField, Button } from "@mui/material";
import { FilterAltOff, Refresh } from "@mui/icons-material";
import { FilterCard } from "@/shared/components";

interface OperatorDateRangeFilterProps {
  startDate: string;
  endDate: string;
  onStartDateChange: (date: string) => void;
  onEndDateChange: (date: string) => void;
  onRefresh: () => void;
}

export const OperatorDateRangeFilter: React.FC<
  OperatorDateRangeFilterProps
> = ({ startDate, endDate, onStartDateChange, onEndDateChange, onRefresh }) => {
  const handleReset = () => {
    const endDate = new Date();
    const startDate = new Date();
    startDate.setDate(startDate.getDate() - 30);
    onStartDateChange(startDate.toISOString().split("T")[0]);
    onEndDateChange(endDate.toISOString().split("T")[0]);
  };

  return (
    <FilterCard sx={{ mb: 3 }}>
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            sm: "1fr 1fr auto auto",
          },
          gap: 2,
          alignItems: "center",
        }}
      >
        <TextField
          type="date"
          label="Start Date"
          value={startDate}
          onChange={(e) => onStartDateChange(e.target.value)}
          InputLabelProps={{
            shrink: true,
            sx: { textTransform: "uppercase", fontSize: "0.75rem" },
          }}
          size="small"
          fullWidth
        />
        <TextField
          type="date"
          label="End Date"
          value={endDate}
          onChange={(e) => onEndDateChange(e.target.value)}
          InputLabelProps={{
            shrink: true,
            sx: { textTransform: "uppercase", fontSize: "0.75rem" },
          }}
          size="small"
          fullWidth
        />
        <Button
          variant="outlined"
          size="small"
          startIcon={<FilterAltOff />}
          onClick={handleReset}
        >
          Reset
        </Button>
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
};
