import { useState, useRef } from "react";
import { useTenant } from "@/shared/context";
import { Link } from "react-router-dom";
import {
  Box,
  Button,
  Typography,
  Menu,
  MenuItem,
  Chip,
  Divider,
  CircularProgress,
} from "@mui/material";
import { ExpandMore, Circle } from "@mui/icons-material";

export function TenantSelector() {
  const { tenants, selectedTenant, selectTenant, isLoading } = useTenant();
  const [anchorEl, setAnchorEl] = useState<HTMLButtonElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const isOpen = Boolean(anchorEl);

  const getHealthStatusColor = (status: string): string => {
    switch (status.toLowerCase()) {
      case "healthy":
        return "success.main";
      case "unhealthy":
        return "error.main";
      case "unknown":
      default:
        return "text.secondary";
    }
  };

  if (isLoading) {
    return (
      <Box
        sx={{ px: 2, py: 1.5, display: "flex", alignItems: "center", gap: 1 }}
      >
        <CircularProgress size={16} />
        <Typography variant="body2" color="text.secondary">
          Loading tenants...
        </Typography>
      </Box>
    );
  }

  if (tenants.length === 0) {
    return (
      <Box sx={{ px: 2, py: 1.5 }}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          No tenants configured
        </Typography>
        <Link to="/tenants" style={{ textDecoration: "none" }}>
          <Typography
            variant="body2"
            color="primary"
            sx={{
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              "&:hover": { opacity: 0.8 },
            }}
          >
            Add Tenant
          </Typography>
        </Link>
      </Box>
    );
  }

  return (
    <Box sx={{ px: 2, py: 1.5 }}>
      <Typography
        variant="overline"
        color="text.secondary"
        sx={{ display: "block", mb: 1, fontSize: "0.75rem" }}
      >
        Current Tenant
      </Typography>
      <Button
        ref={buttonRef}
        onClick={(e) => setAnchorEl(e.currentTarget)}
        variant="outlined"
        fullWidth
        endIcon={
          <ExpandMore
            sx={{
              transform: isOpen ? "rotate(180deg)" : "rotate(0deg)",
              transition: "transform 0.2s",
            }}
          />
        }
        sx={{
          justifyContent: "space-between",
          textTransform: "none",
          py: 1,
        }}
      >
        <Box
          sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}
        >
          <Circle
            sx={{
              fontSize: 8,
              color: getHealthStatusColor(
                selectedTenant?.health_status || "unknown",
              ),
            }}
          />
          <Typography
            variant="body2"
            sx={{
              fontWeight: 500,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {selectedTenant?.name || "Select Tenant"}
          </Typography>
        </Box>
      </Button>

      <Menu
        anchorEl={anchorEl}
        open={isOpen}
        onClose={() => setAnchorEl(null)}
        slotProps={{
          paper: {
            sx: {
              width: anchorEl?.offsetWidth || 250,
              maxHeight: 288,
            },
          },
        }}
      >
        {tenants.map((tenant) => (
          <MenuItem
            key={tenant.tenant_id}
            onClick={() => {
              selectTenant(tenant);
              setAnchorEl(null);
            }}
            disabled={!tenant.enabled}
            selected={selectedTenant?.tenant_id === tenant.tenant_id}
            sx={{
              display: "flex",
              gap: 1.5,
              py: 1.5,
              borderLeft:
                selectedTenant?.tenant_id === tenant.tenant_id
                  ? "2px solid"
                  : "2px solid transparent",
              borderColor: "primary.main",
            }}
          >
            <Circle
              sx={{
                fontSize: 8,
                color: getHealthStatusColor(tenant.health_status),
                flexShrink: 0,
              }}
            />
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 500,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {tenant.name}
                </Typography>
                {tenant.is_default && (
                  <Chip
                    label="Default"
                    size="small"
                    color="primary"
                    sx={{
                      height: 20,
                      fontSize: "0.625rem",
                      fontWeight: 600,
                      opacity: 0.8,
                    }}
                  />
                )}
              </Box>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap",
                  display: "block",
                }}
              >
                {tenant.server_url}
              </Typography>
            </Box>
            {!tenant.enabled && (
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ textTransform: "uppercase", flexShrink: 0 }}
              >
                Disabled
              </Typography>
            )}
          </MenuItem>
        ))}
        <Divider />
        <MenuItem
          onClick={() => setAnchorEl(null)}
          component={Link}
          to="/tenants"
          sx={{
            color: "primary.main",
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
          }}
        >
          Manage Tenants
        </MenuItem>
      </Menu>
    </Box>
  );
}
