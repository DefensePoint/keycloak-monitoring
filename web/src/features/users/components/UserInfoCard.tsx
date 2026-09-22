import React from "react";
import { Box, Typography, Chip, Divider, alpha, useTheme } from "@mui/material";
import {
  SectionCard,
  FieldDisplay,
  LoadingSkeleton,
} from "@/shared/components";
import type { UserInfoCardProps, KeycloakGroup, KeycloakRole } from "../types";

export const UserInfoCard: React.FC<UserInfoCardProps> = ({
  userDetails,
  userGroups,
  roleMappings,
  loading,
}) => {
  const theme = useTheme();

  if (loading) {
    return (
      <SectionCard title="User Information">
        <LoadingSkeleton variant="text" lines={6} />
      </SectionCard>
    );
  }

  if (!userDetails) {
    return (
      <SectionCard title="User Information">
        <Box sx={{ textAlign: "center", py: 4 }}>
          <Typography color="text.secondary">
            User not found or unable to load user details
          </Typography>
        </Box>
      </SectionCard>
    );
  }

  const fullName =
    userDetails.firstName || userDetails.lastName
      ? `${userDetails.firstName || ""} ${userDetails.lastName || ""}`.trim()
      : "N/A";

  return (
    <SectionCard title="User Information">
      {/* Basic Info */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            md: "repeat(2, 1fr)",
            lg: "repeat(3, 1fr)",
          },
          gap: 3,
          mb: 3,
        }}
      >
        <FieldDisplay label="Username" value={userDetails.username || "N/A"} />
        <FieldDisplay label="Email" value={userDetails.email || "N/A"} />
        <FieldDisplay label="Full Name" value={fullName} />

        <Box>
          <Typography
            variant="caption"
            sx={{
              color: "text.secondary",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              display: "block",
              mb: 1,
            }}
          >
            Status
          </Typography>
          <Chip
            label={userDetails.enabled ? "Enabled" : "Disabled"}
            sx={{
              bgcolor: userDetails.enabled
                ? alpha(theme.palette.success.main, 0.2)
                : alpha(theme.palette.error.main, 0.2),
              color: userDetails.enabled
                ? theme.palette.success.main
                : theme.palette.error.main,
              fontSize: "0.75rem",
            }}
          />
        </Box>

        <Box>
          <Typography
            variant="caption"
            sx={{
              color: "text.secondary",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              display: "block",
              mb: 1,
            }}
          >
            Email Verified
          </Typography>
          <Chip
            label={userDetails.emailVerified ? "Verified" : "Not Verified"}
            sx={{
              bgcolor: userDetails.emailVerified
                ? alpha(theme.palette.success.main, 0.2)
                : alpha(theme.palette.warning.main, 0.2),
              color: userDetails.emailVerified
                ? theme.palette.success.main
                : theme.palette.warning.main,
              fontSize: "0.75rem",
            }}
          />
        </Box>

        <FieldDisplay
          label="Created"
          value={
            userDetails.createdTimestamp
              ? new Date(userDetails.createdTimestamp).toLocaleString()
              : "N/A"
          }
        />

        <Box sx={{ gridColumn: { md: "span 2", lg: "span 3" } }}>
          <Typography
            variant="caption"
            sx={{
              color: "text.secondary",
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              display: "block",
              mb: 1,
            }}
          >
            User ID
          </Typography>
          <Typography
            variant="body2"
            sx={{
              color: "text.secondary",
              fontFamily: "monospace",
              fontSize: "0.75rem",
              wordBreak: "break-all",
            }}
          >
            {userDetails.id}
          </Typography>
        </Box>
      </Box>

      {/* Groups Section */}
      <Divider sx={{ my: 3 }} />
      <Box sx={{ mb: 3 }}>
        <Typography
          variant="h6"
          sx={{
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            mb: 2,
          }}
        >
          Groups
        </Typography>
        {userGroups.length > 0 ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
            {userGroups.map((group: KeycloakGroup) => (
              <Chip
                key={group.id}
                label={group.name}
                sx={{
                  bgcolor: "background.default",
                  border: "1px solid",
                  borderColor: "divider",
                }}
              />
            ))}
          </Box>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No groups assigned
          </Typography>
        )}
      </Box>

      {/* Roles Section */}
      <Divider sx={{ my: 3 }} />
      <Box>
        <Typography
          variant="h6"
          sx={{
            fontWeight: 600,
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            mb: 2,
          }}
        >
          Roles
        </Typography>

        {/* Realm Roles */}
        {roleMappings?.realmMappings &&
          roleMappings.realmMappings.length > 0 && (
            <Box sx={{ mb: 2 }}>
              <Typography
                variant="subtitle2"
                sx={{
                  color: "text.secondary",
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 1,
                  fontSize: "0.75rem",
                }}
              >
                Realm Roles
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                {roleMappings.realmMappings.map((role: KeycloakRole) => (
                  <Chip
                    key={role.id}
                    label={role.name}
                    sx={{
                      bgcolor: alpha(theme.palette.info.main, 0.2),
                      border: "1px solid",
                      borderColor: alpha(theme.palette.info.main, 0.3),
                      color: theme.palette.info.main,
                    }}
                  />
                ))}
              </Box>
            </Box>
          )}

        {/* Client Roles */}
        {roleMappings?.clientMappings &&
          Object.keys(roleMappings.clientMappings).length > 0 && (
            <Box>
              <Typography
                variant="subtitle2"
                sx={{
                  color: "text.secondary",
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  mb: 1,
                  fontSize: "0.75rem",
                }}
              >
                Client Roles
              </Typography>
              {Object.entries(roleMappings.clientMappings).map(
                ([clientName, clientRoles]) => (
                  <Box key={clientName} sx={{ mb: 2 }}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ mb: 0.5, display: "block" }}
                    >
                      {clientRoles.client || clientName}
                    </Typography>
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                      {clientRoles.mappings.map((role: KeycloakRole) => (
                        <Chip
                          key={role.id}
                          label={role.name}
                          sx={{
                            bgcolor: alpha(theme.palette.secondary.main, 0.2),
                            border: "1px solid",
                            borderColor: alpha(
                              theme.palette.secondary.main,
                              0.3,
                            ),
                            color: theme.palette.secondary.main,
                          }}
                        />
                      ))}
                    </Box>
                  </Box>
                ),
              )}
            </Box>
          )}

        {(!roleMappings?.realmMappings ||
          roleMappings.realmMappings.length === 0) &&
          (!roleMappings?.clientMappings ||
            Object.keys(roleMappings.clientMappings).length === 0) && (
            <Typography variant="body2" color="text.secondary">
              No roles assigned
            </Typography>
          )}
      </Box>
    </SectionCard>
  );
};
