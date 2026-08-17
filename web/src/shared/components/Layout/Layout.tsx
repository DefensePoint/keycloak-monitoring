import { ReactNode } from "react";
import { Link, useLocation, useParams } from "react-router-dom";
import { useAuth, useTenant } from "@/shared/context";
import { useIsAdmin } from "@/shared/hooks";
import { TenantSelector } from "../TenantSelector";
import { TenantAccessGuard } from "../TenantAccessGuard";
import {
  Box,
  Drawer,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Typography,
  Button,
} from "@mui/material";
import {
  Home,
  Inventory,
  Assignment,
  Notifications,
  BarChart,
  Favorite,
  Storage,
  Settings,
  People,
  Shield,
} from "@mui/icons-material";

interface LayoutProps {
  children: ReactNode;
}

export function Layout({ children }: LayoutProps) {
  const { user, logout } = useAuth();
  const location = useLocation();
  const { tenantId } = useParams<{ tenantId: string }>();
  const { selectedTenant } = useTenant();
  const { isAdmin } = useIsAdmin();

  const navigation = [
    { name: "Dashboard", path: "/", icon: <Home /> },
    { name: "Realms", path: "/realms", icon: <Inventory /> },
    { name: "Events", path: "/events", icon: <Assignment /> },
    { name: "Alerts", path: "/alerts", icon: <Notifications /> },
    { name: "Metrics", path: "/metrics", icon: <BarChart /> },
    { name: "Health", path: "/health", icon: <Favorite /> },
    { name: "Settings", path: "/settings", icon: <Settings /> },
  ];

  // Build tenant-aware navigation paths
  const getNavPath = (basePath: string) => {
    // Use tenant ID from URL or selected tenant
    const currentTenantId = tenantId || selectedTenant?.tenant_id;
    if (!currentTenantId) {
      return basePath;
    }
    // For root path, just return tenant ID
    if (basePath === "/") {
      return `/${currentTenantId}`;
    }
    if (basePath === "/realms") {
      return `/${currentTenantId}/realms`;
    }
    // For other paths, prefix with tenant ID
    return `/${currentTenantId}${basePath}`;
  };

  const isActive = (path: string) => {
    const navPath = getNavPath(path);
    if (path === "/") {
      // Dashboard is active if we're at /:tenantId exactly
      return location.pathname === navPath;
    }
    if (path === "/realms") {
      const currentTenantId = tenantId || selectedTenant?.tenant_id;
      if (!currentTenantId) return false;
      return (
        location.pathname === `/${currentTenantId}/realms` ||
        location.pathname === `/${currentTenantId}/realm` ||
        location.pathname.startsWith(`/${currentTenantId}/realm/`)
      );
    }
    // For other paths, check if current path starts with the nav path
    return location.pathname.startsWith(navPath);
  };

  return (
    <Box sx={{ display: "flex", minHeight: "100vh" }}>
      {/* Sidebar */}
      <Drawer
        variant="permanent"
        sx={{
          width: 256,
          flexShrink: 0,
          "& .MuiDrawer-paper": {
            width: 256,
            boxSizing: "border-box",
            display: "flex",
            flexDirection: "column",
            height: "100vh",
          },
        }}
      >
        {/* Logo */}
        <Box sx={{ p: 3, borderBottom: 1, borderColor: "divider" }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1.5,
            }}
          >
            <Box
              component="img"
              src="/defensepoint-logo.svg"
              alt="DefensePoint"
              sx={{ width: 32, height: 32 }}
            />
            <Box>
              <Typography
                variant="h6"
                color="text.primary"
                sx={{ fontWeight: 700, letterSpacing: "-0.02em" }}
              >
                Keycloak
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Monitoring Tool
              </Typography>
            </Box>
          </Box>
        </Box>

        {/* Tenant Selector */}
        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <TenantSelector />
        </Box>

        {/* Navigation */}
        <Box
          sx={{ flex: 1, minHeight: 0, overflowY: "auto", overflowX: "hidden" }}
        >
          <List sx={{ py: 1, px: 2 }}>
            {navigation.map((item) => {
              const active = isActive(item.path);
              const navPath = getNavPath(item.path);

              return (
                <ListItemButton
                  key={item.path}
                  component={Link}
                  to={navPath}
                  selected={active}
                >
                  <ListItemIcon>{item.icon}</ListItemIcon>
                  <ListItemText
                    primary={item.name}
                    primaryTypographyProps={{ variant: "body2" }}
                  />
                </ListItemButton>
              );
            })}
          </List>

          {/* Admin Section - Only visible to admins */}
          {isAdmin && (
            <Box sx={{ mt: 2 }}>
              <Typography
                variant="overline"
                color="text.secondary"
                sx={{ px: 2, display: "block", mb: 0.5, fontWeight: 600 }}
              >
                Administration
              </Typography>
              <List sx={{ py: 0, px: 2 }}>
                <ListItemButton
                  component={Link}
                  to="/tenants"
                  selected={location.pathname === "/tenants"}
                >
                  <ListItemIcon>
                    <Storage />
                  </ListItemIcon>
                  <ListItemText
                    primary="Tenants"
                    primaryTypographyProps={{ variant: "body2" }}
                  />
                </ListItemButton>
                <ListItemButton
                  component={Link}
                  to="/admin/users"
                  selected={location.pathname === "/admin/users"}
                >
                  <ListItemIcon>
                    <People />
                  </ListItemIcon>
                  <ListItemText
                    primary="Users"
                    primaryTypographyProps={{ variant: "body2" }}
                  />
                </ListItemButton>
                <ListItemButton
                  component={Link}
                  to="/admin/roles"
                  selected={location.pathname === "/admin/roles"}
                >
                  <ListItemIcon>
                    <Shield />
                  </ListItemIcon>
                  <ListItemText
                    primary="Roles"
                    primaryTypographyProps={{ variant: "body2" }}
                  />
                </ListItemButton>
              </List>
            </Box>
          )}
        </Box>

        {/* User Info */}
        {user && (
          <Box sx={{ borderTop: 1, borderColor: "divider", p: 2 }}>
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" sx={{ fontWeight: 500 }} noWrap>
                {user.name || user.email}
              </Typography>
              <Typography variant="caption" color="text.secondary" noWrap>
                {user.email}
              </Typography>
            </Box>
            <Button onClick={logout} variant="outlined" fullWidth size="small">
              Logout
            </Button>
          </Box>
        )}
      </Drawer>

      {/* Main Content */}
      <Box component="main" sx={{ flex: 1, overflow: "auto" }}>
        <TenantAccessGuard>{children}</TenantAccessGuard>
      </Box>
    </Box>
  );
}
