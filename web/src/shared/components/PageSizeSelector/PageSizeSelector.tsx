import { useState, useRef, memo } from "react";
import { Button, Menu, MenuItem, Box, Typography } from "@mui/material";
import { ExpandMore, ViewList } from "@mui/icons-material";

interface PageSizeSelectorProps {
  readonly value: number;
  readonly onChange: (value: number) => void;
  readonly options?: number[];
  readonly label?: string;
  readonly className?: string;
}

export const PageSizeSelector = memo(function PageSizeSelector({
  value,
  onChange,
  options = [25, 50, 100, 200],
  label = "items",
  className = "",
}: PageSizeSelectorProps) {
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const isOpen = Boolean(anchorEl);

  return (
    <Box className={className}>
      <Button
        ref={buttonRef}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        variant="outlined"
        startIcon={<ViewList />}
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
        {value} {label}
      </Button>

      <Menu
        anchorEl={anchorEl}
        open={isOpen}
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
              minWidth: 140,
              mt: 1,
            },
          },
        }}
      >
        {options.map((option) => (
          <MenuItem
            key={option}
            selected={value === option}
            onClick={() => {
              onChange(option);
              setAnchorEl(null);
            }}
          >
            <Typography variant="body2">
              {option} {label}
            </Typography>
          </MenuItem>
        ))}
      </Menu>
    </Box>
  );
});
