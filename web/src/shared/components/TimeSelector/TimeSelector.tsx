import { useState, useEffect, useRef, memo } from "react";
import {
  Button,
  Popover,
  Tabs,
  Tab,
  Box,
  TextField,
  List,
  ListItemButton,
} from "@mui/material";
import { AccessTime, ExpandMore } from "@mui/icons-material";
import { useTimeRangeStorage } from "@/shared/hooks";

export interface TimeRange {
  start: Date;
  end: Date;
  label: string;
}

interface TimeSelectorProps {
  readonly onTimeRangeChange: (start: Date | null, end: Date | null) => void;
  readonly className?: string;
  readonly initialStart?: Date;
  readonly initialEnd?: Date;
}

const QUICK_RANGES = [
  { label: "Last 5 minutes", value: "5m", minutes: 5 },
  { label: "Last 15 minutes", value: "15m", minutes: 15 },
  { label: "Last 30 minutes", value: "30m", minutes: 30 },
  { label: "Last 1 hour", value: "1h", minutes: 60 },
  { label: "Last 3 hours", value: "3h", minutes: 180 },
  { label: "Last 6 hours", value: "6h", minutes: 360 },
  { label: "Last 12 hours", value: "12h", minutes: 720 },
  { label: "Last 24 hours", value: "24h", minutes: 1440 },
  { label: "Last 7 days", value: "7d", minutes: 10080 },
  { label: "Last 30 days", value: "30d", minutes: 43200 },
  { label: "All time", value: "all", minutes: 0 },
];

export const TimeSelector = memo(function TimeSelector({
  onTimeRangeChange,
  className = "",
  initialStart,
  initialEnd,
}: TimeSelectorProps) {
  const { stored, saveTimeRange } = useTimeRangeStorage();
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const [activeTab, setActiveTab] = useState<number>(0);
  const [selectedRange, setSelectedRange] = useState<string>("7d");
  const [displayLabel, setDisplayLabel] = useState<string>("Last 7 days");
  const [tempStart, setTempStart] = useState<string>("");
  const [tempEnd, setTempEnd] = useState<string>("");
  const buttonRef = useRef<HTMLButtonElement>(null);
  const onTimeRangeChangeRef = useRef(onTimeRangeChange);
  const initializedRef = useRef(false);

  const isOpen = Boolean(anchorEl);

  // Keep callback ref updated
  useEffect(() => {
    onTimeRangeChangeRef.current = onTimeRangeChange;
  }, [onTimeRangeChange]);

  // Helper to get stored label
  const getStoredLabel = (storedValue: string): string => {
    if (storedValue === "absolute") return "Custom range";
    const range = QUICK_RANGES.find((r) => r.value === storedValue);
    return range?.label || "Last 7 days";
  };

  // Initialize: URL params > sessionStorage > default (7 days)
  useEffect(() => {
    if (initializedRef.current) return;
    initializedRef.current = true;

    // Priority 1: URL params
    if (
      initialStart &&
      initialEnd &&
      Number.isFinite(initialStart.getTime()) &&
      Number.isFinite(initialEnd.getTime())
    ) {
      setDisplayLabel(
        `${formatDateTime(initialStart)} to ${formatDateTime(initialEnd)}`,
      );
      setSelectedRange("absolute");
      setTempStart(formatDateTimeInput(initialStart));
      setTempEnd(formatDateTimeInput(initialEnd));
      onTimeRangeChangeRef.current(initialStart, initialEnd);
      return;
    }

    // Priority 2: sessionStorage (via hook)
    if (stored) {
      setSelectedRange(stored.value);
      setDisplayLabel(getStoredLabel(stored.value));

      if (stored.value === "absolute" && stored.start && stored.end) {
        const start = new Date(stored.start);
        const end = new Date(stored.end);
        setTempStart(formatDateTimeInput(start));
        setTempEnd(formatDateTimeInput(end));
        setDisplayLabel(`${formatDateTime(start)} to ${formatDateTime(end)}`);
        onTimeRangeChangeRef.current(start, end);
      } else if (stored.value === "all") {
        onTimeRangeChangeRef.current(null, null);
      } else {
        const range = QUICK_RANGES.find((r) => r.value === stored.value);
        if (range) {
          const end = new Date();
          const start = new Date(end.getTime() - range.minutes * 60 * 1000);
          onTimeRangeChangeRef.current(start, end);
        }
      }
      return;
    }

    // Priority 3: Default to 7 days
    const end = new Date();
    const start = new Date(end.getTime() - 7 * 24 * 60 * 60 * 1000);
    onTimeRangeChangeRef.current(start, end);
  }, [initialStart, initialEnd, stored]);

  const handleQuickRangeSelect = (range: (typeof QUICK_RANGES)[0]) => {
    setSelectedRange(range.value);
    setDisplayLabel(range.label);
    saveTimeRange(range.value);

    if (range.value === "all") {
      // "All time" - no time filter
      onTimeRangeChange(null, null);
    } else {
      const end = new Date();
      const start = new Date(end.getTime() - range.minutes * 60 * 1000);
      onTimeRangeChange(start, end);
    }
    setAnchorEl(null);
  };

  const handleAbsoluteApply = () => {
    if (tempStart && tempEnd) {
      const start = new Date(tempStart);
      const end = new Date(tempEnd);
      if (Number.isFinite(start.getTime()) && Number.isFinite(end.getTime())) {
        setDisplayLabel(`${formatDateTime(start)} to ${formatDateTime(end)}`);
        setSelectedRange("absolute");
        saveTimeRange("absolute", start.toISOString(), end.toISOString());
        onTimeRangeChange(start, end);
        setAnchorEl(null);
      }
    }
  };

  const formatDateTime = (date: Date): string => {
    return date.toLocaleString("en-US", {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const formatDateTimeInput = (date: Date): string => {
    const year = date.getFullYear();
    const month = String(date.getMonth() + 1).padStart(2, "0");
    const day = String(date.getDate()).padStart(2, "0");
    const hours = String(date.getHours()).padStart(2, "0");
    const minutes = String(date.getMinutes()).padStart(2, "0");
    return `${year}-${month}-${day}T${hours}:${minutes}`;
  };

  const handleAbsoluteTabOpen = () => {
    // Set default values when opening absolute tab
    if (!tempEnd) {
      const now = new Date();
      setTempEnd(formatDateTimeInput(now));
    }
    if (!tempStart) {
      const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000);
      setTempStart(formatDateTimeInput(oneHourAgo));
    }
  };

  return (
    <Box className={className}>
      <Button
        ref={buttonRef}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        variant="outlined"
        startIcon={<AccessTime />}
        endIcon={
          <ExpandMore
            sx={{
              transform: isOpen ? "rotate(180deg)" : "rotate(0deg)",
              transition: "transform 0.2s",
            }}
          />
        }
        sx={{
          textTransform: "uppercase",
          fontWeight: 500,
        }}
      >
        {displayLabel}
      </Button>

      <Popover
        open={isOpen}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
        slotProps={{
          paper: {
            sx: {
              width: 384,
              maxWidth: "calc(100vw - 2rem)",
              mt: 1,
            },
          },
        }}
      >
        <Tabs
          value={activeTab}
          onChange={(_, newValue) => {
            setActiveTab(newValue);
            if (newValue === 1) {
              handleAbsoluteTabOpen();
            }
          }}
          variant="fullWidth"
          sx={{ borderBottom: 1, borderColor: "divider" }}
        >
          <Tab label="Relative time ranges" sx={{ textTransform: "none" }} />
          <Tab label="Absolute time range" sx={{ textTransform: "none" }} />
        </Tabs>

        <Box sx={{ p: 2 }}>
          {activeTab === 0 ? (
            <List sx={{ maxHeight: 320, overflow: "auto", p: 0 }}>
              {QUICK_RANGES.map((range) => (
                <ListItemButton
                  key={range.value}
                  selected={selectedRange === range.value}
                  onClick={() => handleQuickRangeSelect(range)}
                >
                  {range.label}
                </ListItemButton>
              ))}
            </List>
          ) : (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <TextField
                label="From"
                type="datetime-local"
                value={tempStart}
                onChange={(e) => setTempStart(e.target.value)}
                fullWidth
                InputLabelProps={{ shrink: true }}
              />
              <TextField
                label="To"
                type="datetime-local"
                value={tempEnd}
                onChange={(e) => setTempEnd(e.target.value)}
                fullWidth
                InputLabelProps={{ shrink: true }}
              />
              <Box sx={{ display: "flex", gap: 1, pt: 1 }}>
                <Button
                  onClick={handleAbsoluteApply}
                  disabled={!tempStart || !tempEnd}
                  variant="contained"
                  fullWidth
                >
                  Apply time range
                </Button>
                <Button
                  onClick={() => setAnchorEl(null)}
                  variant="outlined"
                  sx={{ minWidth: 100 }}
                >
                  Cancel
                </Button>
              </Box>
            </Box>
          )}
        </Box>
      </Popover>
    </Box>
  );
});
