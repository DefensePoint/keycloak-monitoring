import { useUserRoles, useUserPermissions, useIsAdmin } from "@/shared/hooks";
import {
  Box,
  Typography,
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Skeleton,
} from "@mui/material";
import { ExpandMore } from "@mui/icons-material";

/**
 * Component that displays the current user's roles and permissions
 */
export function UserRolesDisplay() {
  const { roles, isLoading: rolesLoading } = useUserRoles();
  const { permissions, isLoading: permsLoading } = useUserPermissions();
  const { isAdmin } = useIsAdmin();

  if (rolesLoading || permsLoading) {
    return (
      <Box>
        <Skeleton variant="text" width="75%" height={24} />
        <Skeleton variant="text" width="50%" height={24} />
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {/* Roles Section */}
      <Box>
        <Typography
          variant="subtitle2"
          color="text.secondary"
          gutterBottom
          sx={{ mb: 1.5 }}
        >
          Your Roles
        </Typography>
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
          {roles.length === 0 ? (
            <Typography variant="body2" color="text.secondary">
              No roles assigned
            </Typography>
          ) : (
            roles.map((userRole) => (
              <Box key={userRole.id} sx={{ display: "flex", gap: 0.5 }}>
                <Chip
                  label={
                    <Box
                      sx={{ display: "flex", alignItems: "center", gap: 0.5 }}
                    >
                      <span>
                        {userRole.role?.display_name || userRole.role?.name}
                      </span>
                      {userRole.tenant_id && (
                        <Typography
                          component="span"
                          variant="caption"
                          sx={{ opacity: 0.8 }}
                        >
                          ({userRole.tenant_id})
                        </Typography>
                      )}
                    </Box>
                  }
                  color="info"
                  size="small"
                />
                {isAdmin && (
                  <Chip
                    label="ADMIN"
                    color="secondary"
                    size="small"
                    sx={{
                      bgcolor: "secondary.dark",
                      fontWeight: 600,
                    }}
                  />
                )}
              </Box>
            ))
          )}
        </Box>
      </Box>

      {/* Permissions Section - Collapsible */}
      <Accordion defaultExpanded={false}>
        <AccordionSummary expandIcon={<ExpandMore />}>
          <Typography variant="subtitle2" color="text.secondary">
            Your Permissions ({permissions.length})
          </Typography>
        </AccordionSummary>
        <AccordionDetails>
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "repeat(2, 1fr)",
                md: "repeat(3, 1fr)",
              },
              gap: 1,
            }}
          >
            {permissions.length === 0 ? (
              <Box sx={{ gridColumn: "1 / -1" }}>
                <Typography variant="body2" color="text.secondary">
                  No permissions assigned
                </Typography>
              </Box>
            ) : (
              permissions.map((permission) => (
                <Box key={permission}>
                  <Chip
                    label={permission}
                    size="small"
                    variant="outlined"
                    sx={{
                      fontFamily: "monospace",
                      fontSize: "0.75rem",
                    }}
                  />
                </Box>
              ))
            )}
          </Box>
        </AccordionDetails>
      </Accordion>
    </Box>
  );
}

/**
 * Compact badge showing user's primary role
 */
export function UserRoleBadge() {
  const { roles } = useUserRoles();
  const { isAdmin } = useIsAdmin();

  if (roles.length === 0) {
    return null;
  }

  // Show admin badge if user is admin
  if (isAdmin) {
    return (
      <Chip
        label="Admin"
        color="secondary"
        size="small"
        sx={{
          bgcolor: "secondary.dark",
          fontWeight: 600,
          fontSize: "0.75rem",
        }}
      />
    );
  }

  // Show primary role (first role)
  const primaryRole = roles[0];
  return (
    <Chip
      label={primaryRole.role?.display_name || primaryRole.role?.name}
      color="info"
      size="small"
      sx={{
        fontSize: "0.75rem",
      }}
    />
  );
}
