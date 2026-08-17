import { Routes, Route } from "react-router-dom";
import { ProtectedRoute } from "./ProtectedRoute";
import { AdminRoute } from "./AdminRoute";
import { PermissionRoute } from "./PermissionRoute";
import { HomeRedirect } from "./HomeRedirect";
import { RouteErrorBoundary, NotFoundPage } from "@/shared/components";
import { PERMISSIONS } from "@/shared/constants";

// Feature pages
import { DashboardPage } from "@/features/dashboard";
import { RealmsPage, RealmPage } from "@/features/realms";
import { EventsPage } from "@/features/events";
import { UserDetailsPage } from "@/features/users";
import { HealthPage } from "@/features/health";
import { SettingsPage } from "@/features/settings";
import { AlertsPage, AlertDetailPage } from "@/features/alerts";
import { OperatorMetricsPage } from "@/features/metrics/pages";
import { TenantsPage } from "@/features/tenants";
import { AdminUsersPage, AdminRolesPage } from "@/features/admin";

// Development pages
import { ThemeTestPage } from "@/theme/ThemeTestPage";

export function AppRoutes() {
  return (
    <Routes>
      {/* All routes are wrapped in ProtectedRoute */}
      <Route
        path="/*"
        element={
          <ProtectedRoute>
            <Routes>
              {/* Tenant management - Admin only */}
              <Route
                path="/tenants"
                element={
                  <AdminRoute>
                    <RouteErrorBoundary routeId="Tenants">
                      <TenantsPage />
                    </RouteErrorBoundary>
                  </AdminRoute>
                }
              />

              {/* Admin routes - protected by AdminRoute */}
              <Route
                path="/admin/users"
                element={
                  <AdminRoute>
                    <RouteErrorBoundary routeId="AdminUsers">
                      <AdminUsersPage />
                    </RouteErrorBoundary>
                  </AdminRoute>
                }
              />
              <Route
                path="/admin/roles"
                element={
                  <AdminRoute>
                    <RouteErrorBoundary routeId="AdminRoles">
                      <AdminRolesPage />
                    </RouteErrorBoundary>
                  </AdminRoute>
                }
              />

              {/* Development/Test routes */}
              <Route
                path="/theme-test"
                element={
                  <RouteErrorBoundary routeId="ThemeTest">
                    <ThemeTestPage />
                  </RouteErrorBoundary>
                }
              />

              {/* Root redirect - go to first available tenant dashboard */}
              <Route path="/" element={<HomeRedirect />} />

              {/* Tenant-scoped routes with tenant ID in URL */}
              <Route
                path="/:tenantId"
                element={
                  <PermissionRoute permission={PERMISSIONS.KEYCLOAK.READ}>
                    <RouteErrorBoundary routeId="Dashboard">
                      <DashboardPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/realms"
                element={
                  <PermissionRoute permission={PERMISSIONS.REALMS.READ}>
                    <RouteErrorBoundary routeId="Realms">
                      <RealmsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/realm"
                element={
                  <PermissionRoute permission={PERMISSIONS.REALMS.READ}>
                    <RouteErrorBoundary routeId="Realms">
                      <RealmsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/realm/:realmName"
                element={
                  <PermissionRoute permission={PERMISSIONS.REALMS.READ}>
                    <RouteErrorBoundary routeId="RealmDetail">
                      <RealmPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/realm/:realmName/user/:userId"
                element={
                  <PermissionRoute permission={PERMISSIONS.USERS.READ}>
                    <RouteErrorBoundary routeId="UserDetails">
                      <UserDetailsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/events"
                element={
                  <PermissionRoute permission={PERMISSIONS.EVENTS.READ}>
                    <RouteErrorBoundary routeId="Events">
                      <EventsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/alerts"
                element={
                  <PermissionRoute permission={PERMISSIONS.ALERTS.READ}>
                    <RouteErrorBoundary routeId="Alerts">
                      <AlertsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/alerts/:alertId"
                element={
                  <PermissionRoute permission={PERMISSIONS.ALERTS.READ}>
                    <RouteErrorBoundary routeId="AlertDetail">
                      <AlertDetailPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/metrics"
                element={
                  <PermissionRoute permission={PERMISSIONS.METRICS.READ}>
                    <RouteErrorBoundary routeId="OperatorMetrics">
                      <OperatorMetricsPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/health"
                element={
                  <PermissionRoute permission={PERMISSIONS.KEYCLOAK.READ}>
                    <RouteErrorBoundary routeId="Health">
                      <HealthPage />
                    </RouteErrorBoundary>
                  </PermissionRoute>
                }
              />
              <Route
                path="/:tenantId/settings"
                element={
                  <RouteErrorBoundary routeId="Settings">
                    <SettingsPage />
                  </RouteErrorBoundary>
                }
              />

              {/* Catch-all 404 page for unknown routes */}
              <Route path="*" element={<NotFoundPage />} />
            </Routes>
          </ProtectedRoute>
        }
      />
    </Routes>
  );
}
