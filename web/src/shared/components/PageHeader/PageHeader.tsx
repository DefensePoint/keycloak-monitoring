import { ReactNode, useMemo } from "react";
import { Box, Typography, Breadcrumbs, Link as MuiLink } from "@mui/material";
import { Link, useLocation, useParams } from "react-router-dom";
import { NavigateNext as NavigateNextIcon } from "@mui/icons-material";
import { useTenant } from "@/shared/context";

export interface PageHeaderProps {
  /**
   * Page title
   */
  title: string;
  /**
   * Optional subtitle/description
   */
  subtitle?: string;
  /**
   * Optional actions/buttons to display on the right
   */
  children?: ReactNode;
  /**
   * Whether to show the breadcrumb. Default: true
   */
  showBreadcrumb?: boolean;
}

interface BreadcrumbItem {
  label: string;
  path?: string;
}

/**
 * PageHeader - Standardized header component for all pages
 *
 * Provides consistent styling with:
 * - Responsive flexbox layout
 * - Automatic breadcrumb based on current route
 * - Optional actions slot for buttons
 *
 * @example
 * ```tsx
 * <PageHeader
 *   title="Configuration Alerts"
 *   subtitle="Monitor and manage Keycloak configuration issues"
 * >
 *   <Button>Add Alert</Button>
 * </PageHeader>
 * ```
 */
export function PageHeader({
  title,
  subtitle,
  children,
  showBreadcrumb = true,
}: PageHeaderProps) {
  const location = useLocation();
  const params = useParams<{
    tenantId?: string;
    realmName?: string;
    userId?: string;
    alertId?: string;
  }>();
  const { selectedTenant } = useTenant();

  const breadcrumbs = useMemo<BreadcrumbItem[]>(() => {
    const items: BreadcrumbItem[] = [];
    const pathParts = location.pathname.split("/").filter(Boolean);

    // Handle different route patterns
    if (pathParts[0] === "tenants") {
      items.push({ label: "Admin", path: "/tenants" });
      items.push({ label: "Tenants" });
      return items;
    }

    if (pathParts[0] === "admin") {
      items.push({ label: "Admin", path: "/tenants" });
      if (pathParts[1] === "users") {
        items.push({ label: "Users" });
      } else if (pathParts[1] === "roles") {
        items.push({ label: "Roles" });
      }
      return items;
    }

    // Tenant-scoped routes
    const tenantId = params.tenantId || pathParts[0];
    if (tenantId && selectedTenant) {
      // Add tenant as first item
      items.push({
        label: selectedTenant.name,
        path: `/${tenantId}`,
      });

      const section = pathParts[1];

      switch (section) {
        case "realms":
          items.push({ label: "Realms" });
          break;

        case "realm":
          items.push({
            label: "Realms",
            path: `/${tenantId}/realms`,
          });
          if (params.realmName) {
            if (pathParts[4] === "user" && params.userId) {
              // User detail page
              items.push({
                label: params.realmName,
                path: `/${tenantId}/realm/${params.realmName}`,
              });
              items.push({ label: "User Details" });
            } else {
              // Realm detail page
              items.push({ label: params.realmName });
            }
          }
          break;

        case "events":
          items.push({ label: "Events" });
          break;

        case "alerts":
          if (params.alertId) {
            items.push({
              label: "Alerts",
              path: `/${tenantId}/alerts`,
            });
            items.push({ label: "Alert Details" });
          } else {
            items.push({ label: "Alerts" });
          }
          break;

        case "metrics":
          items.push({ label: "Metrics" });
          break;

        case "health":
          items.push({ label: "Health" });
          break;

        case "settings":
          items.push({ label: "Settings" });
          break;

        default:
          // Dashboard - no additional items
          break;
      }
    }

    return items;
  }, [location.pathname, params, selectedTenant]);

  // Don't show breadcrumb if disabled or empty
  // Show breadcrumb with 1+ items (e.g., Tenants page has only 1 item but should show)
  const shouldShowBreadcrumb = showBreadcrumb && breadcrumbs.length >= 1;

  return (
    <Box sx={{ mb: 3 }}>
      {/* Breadcrumb */}
      {shouldShowBreadcrumb && (
        <Breadcrumbs
          separator={<NavigateNextIcon fontSize="small" />}
          sx={{ mb: 2 }}
        >
          {breadcrumbs.map((item, index) => {
            const isLast = index === breadcrumbs.length - 1;

            if (isLast || !item.path) {
              return (
                <Typography
                  key={index}
                  variant="body2"
                  color={isLast ? "text.primary" : "text.secondary"}
                  sx={{ fontWeight: isLast ? 500 : 400 }}
                >
                  {item.label}
                </Typography>
              );
            }

            return (
              <MuiLink
                key={index}
                component={Link}
                to={item.path}
                sx={{
                  color: "text.secondary",
                  textDecoration: "none",
                  "&:hover": {
                    color: "primary.main",
                    textDecoration: "underline",
                  },
                }}
              >
                <Typography variant="body2">{item.label}</Typography>
              </MuiLink>
            );
          })}
        </Breadcrumbs>
      )}

      {/* Header */}
      <Box
        sx={{
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          justifyContent: "space-between",
          alignItems: { xs: "flex-start", sm: "center" },
          gap: 2,
        }}
      >
        <Box>
          <Typography variant="h3">{title}</Typography>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              {subtitle}
            </Typography>
          )}
        </Box>
        {children && (
          <Box
            sx={{
              display: "flex",
              flexDirection: { xs: "column", sm: "row" },
              alignItems: { xs: "flex-start", sm: "center" },
              gap: 1.5,
            }}
          >
            {children}
          </Box>
        )}
      </Box>
    </Box>
  );
}
