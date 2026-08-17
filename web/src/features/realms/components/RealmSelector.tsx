import { useState, useRef } from "react";
import { Button, Menu, MenuItem, Chip, Box, Typography } from "@mui/material";
import { ExpandMore, Inventory } from "@mui/icons-material";

interface RealmSelectorProps {
  readonly selectedRealm: string;
  readonly onRealmChange: (realm: string) => void;
  readonly realms: Array<{ realm_name: string }>;
  readonly className?: string;
  readonly defaultRealm?: string;
}

export function RealmSelector({
  selectedRealm,
  onRealmChange,
  realms,
  className = "",
  defaultRealm,
}: RealmSelectorProps) {
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const isOpen = Boolean(anchorEl);

  return (
    <Box className={className}>
      <Button
        ref={buttonRef}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        variant="outlined"
        startIcon={<Inventory />}
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
        {selectedRealm === "all" ? "All Realms" : selectedRealm}
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
              width: 256,
              maxHeight: 320,
              mt: 1,
            },
          },
        }}
      >
        <MenuItem
          selected={selectedRealm === "all"}
          onClick={() => {
            onRealmChange("all");
            setAnchorEl(null);
          }}
        >
          All Realms
        </MenuItem>
        {realms.map((realm) => (
          <MenuItem
            key={realm.realm_name}
            selected={selectedRealm === realm.realm_name}
            onClick={() => {
              onRealmChange(realm.realm_name);
              setAnchorEl(null);
            }}
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
            }}
          >
            <Typography variant="body2">{realm.realm_name}</Typography>
            {defaultRealm === realm.realm_name && (
              <Chip
                label="Default"
                size="small"
                color="primary"
                sx={{
                  height: 20,
                  fontWeight: 600,
                  opacity: 0.8,
                }}
              />
            )}
          </MenuItem>
        ))}
      </Menu>
    </Box>
  );
}
