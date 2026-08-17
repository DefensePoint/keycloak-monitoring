import React from "react";
import { Link } from "react-router-dom";
import { Box, Typography, Button, Chip, alpha, useTheme } from "@mui/material";
import { Logout } from "@mui/icons-material";
import { TimeSelector } from "@/shared/components/TimeSelector";
import { RealmSelector } from "./RealmSelector";
import type { RealmPageHeaderProps } from "../types";

export const RealmPageHeader: React.FC<RealmPageHeaderProps> = ({
  realmName,
  tenantId,
  selectedRealm,
  allRealms,
  defaultRealm,
  userName,
  userEmail,
  onRealmChange,
  onTimeRangeChange,
  onLogout,
}) => {
  const theme = useTheme();

  return (
    <Box
      sx={{
        bgcolor: alpha(theme.palette.background.paper, 0.5),
        borderBottom: "1px solid",
        borderColor: "divider",
      }}
    >
      <Box
        sx={{
          maxWidth: 1280,
          mx: "auto",
          px: 4,
          py: 2,
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Link to={`/${tenantId}`} style={{ textDecoration: "none" }}>
            <Box
              component="img"
              src="/defensepoint-logo.svg"
              alt="DefensePoint"
              sx={{ width: 32, height: 32 }}
            />
          </Link>
          <Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Link
                to={`/${tenantId}`}
                style={{
                  textDecoration: "none",
                  color: theme.palette.text.primary,
                  fontSize: "0.875rem",
                }}
              >
                KMT
              </Link>
              <Typography color="text.secondary">/</Typography>
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 700,
                  letterSpacing: "-0.02em",
                }}
              >
                {realmName}
              </Typography>
              {realmName === defaultRealm && (
                <Chip
                  label="Default"
                  size="small"
                  sx={{
                    bgcolor: alpha(theme.palette.error.main, 0.2),
                    color: theme.palette.error.main,
                    border: "1px solid",
                    borderColor: alpha(theme.palette.error.main, 0.3),
                    fontWeight: 600,
                    textTransform: "uppercase",
                    fontSize: "0.65rem",
                    height: 20,
                  }}
                />
              )}
            </Box>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Keycloak Realm
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <RealmSelector
            selectedRealm={selectedRealm}
            onRealmChange={onRealmChange}
            realms={allRealms}
            defaultRealm={defaultRealm}
          />
          <TimeSelector onTimeRangeChange={onTimeRangeChange} />
          {userName && (
            <>
              <Box sx={{ textAlign: "right" }}>
                <Typography variant="body2" fontWeight={600}>
                  {userName}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {userEmail}
                </Typography>
              </Box>
              <Button
                variant="outlined"
                onClick={onLogout}
                startIcon={<Logout />}
                sx={{
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  fontWeight: 600,
                }}
              >
                Logout
              </Button>
            </>
          )}
        </Box>
      </Box>
    </Box>
  );
};
