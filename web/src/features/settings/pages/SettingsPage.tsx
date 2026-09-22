import { Box, Card, CardContent, Typography, Chip } from "@mui/material";
import { useAuth, useTenant } from "@/shared/context";
import { useVersion } from "@/shared/hooks";
import { PageHeader } from "@/shared/components";

export function SettingsPage() {
  const { user } = useAuth();
  const { selectedTenant } = useTenant();

  const { data: versionInfo } = useVersion(selectedTenant?.tenant_id);

  return (
    <Box sx={{ p: { xs: 2, sm: 4 } }}>
      <PageHeader
        title="Settings"
        subtitle="Configure application preferences and user settings"
      />

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: {
            xs: "1fr",
            md: "repeat(2, 1fr)",
            lg: "repeat(3, 1fr)",
          },
          gap: 3,
        }}
      >
        {/* User Profile */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 3,
              }}
            >
              User Profile
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              {user?.name && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                      mb: 0.5,
                    }}
                  >
                    Full Name
                  </Typography>
                  <Typography variant="body2">{user.name}</Typography>
                </Box>
              )}
              <Box>
                <Typography
                  variant="caption"
                  sx={{
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    color: "text.secondary",
                    display: "block",
                    mb: 0.5,
                  }}
                >
                  Email
                </Typography>
                <Typography variant="body2">{user?.email}</Typography>
              </Box>
              {user?.preferred_username && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                      mb: 0.5,
                    }}
                  >
                    Username
                  </Typography>
                  <Typography variant="body2">
                    {user.preferred_username}
                  </Typography>
                </Box>
              )}
            </Box>
          </CardContent>
        </Card>

        {/* Display Settings */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 3,
              }}
            >
              Display Settings
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Theme
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Current: Dark (Default)
                  </Typography>
                </Box>
                <Chip
                  label="Coming Soon"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Timezone
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Current: {Intl.DateTimeFormat().resolvedOptions().timeZone}
                  </Typography>
                </Box>
                <Chip
                  label="Auto-detected"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
            </Box>
          </CardContent>
        </Card>

        {/* Notification Settings */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 3,
              }}
            >
              Notifications
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Email Notifications
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Receive alerts via email
                  </Typography>
                </Box>
                <Chip
                  label="Coming Soon"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Slack Integration
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Send critical alerts to Slack
                  </Typography>
                </Box>
                <Chip
                  label="Coming Soon"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
            </Box>
          </CardContent>
        </Card>

        {/* Security Settings */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 3,
              }}
            >
              Security
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Session Timeout
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Auto-logout after inactivity
                  </Typography>
                </Box>
                <Chip
                  label="24 hours"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Two-Factor Authentication
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Add an extra layer of security
                  </Typography>
                </Box>
                <Chip
                  label="Coming Soon"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
            </Box>
          </CardContent>
        </Card>

        {/* Monitoring Configuration */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 3,
              }}
            >
              Monitoring Configuration
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Event Collection Interval
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    How often to poll for new events
                  </Typography>
                </Box>
                <Chip
                  label="30 seconds"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Metrics Refresh Interval
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Dashboard data refresh rate
                  </Typography>
                </Box>
                <Chip
                  label="10 seconds"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                }}
              >
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
                    Event Retention
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    How long to keep event data
                  </Typography>
                </Box>
                <Chip
                  label="Coming Soon"
                  size="small"
                  sx={{
                    bgcolor: "background.default",
                    color: "text.secondary",
                    fontSize: "0.75rem",
                  }}
                />
              </Box>
            </Box>
          </CardContent>
        </Card>

        {/* About */}
        <Card>
          <CardContent sx={{ p: 3 }}>
            <Typography
              variant="h6"
              sx={{
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.02em",
                mb: 2,
              }}
            >
              About
            </Typography>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Box>
                <Typography
                  variant="caption"
                  sx={{
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    color: "text.secondary",
                    display: "block",
                  }}
                >
                  Application
                </Typography>
                <Typography variant="body2">
                  Keycloak Monitoring Tool
                </Typography>
              </Box>
              <Box>
                <Typography
                  variant="caption"
                  sx={{
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                    color: "text.secondary",
                    display: "block",
                  }}
                >
                  Version
                </Typography>
                <Typography variant="body2">
                  {versionInfo?.version || "0.1.0"}
                </Typography>
              </Box>
              {versionInfo?.git_commit && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                    }}
                  >
                    Git Commit
                  </Typography>
                  <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                    {versionInfo.git_commit}
                  </Typography>
                </Box>
              )}
              {versionInfo?.build_date && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                    }}
                  >
                    Build Date
                  </Typography>
                  <Typography variant="body2">
                    {new Date(versionInfo.build_date).toLocaleString()}
                  </Typography>
                </Box>
              )}
              {versionInfo?.go_version && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                    }}
                  >
                    Go Version
                  </Typography>
                  <Typography variant="body2">
                    {versionInfo.go_version}
                  </Typography>
                </Box>
              )}
              {versionInfo?.platform && (
                <Box>
                  <Typography
                    variant="caption"
                    sx={{
                      textTransform: "uppercase",
                      letterSpacing: "0.05em",
                      color: "text.secondary",
                      display: "block",
                    }}
                  >
                    Platform
                  </Typography>
                  <Typography variant="body2">
                    {versionInfo.platform}
                  </Typography>
                </Box>
              )}
            </Box>
          </CardContent>
        </Card>
      </Box>
    </Box>
  );
}
