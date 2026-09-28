import {
  Box,
  Card,
  CardContent,
  Button,
  Typography,
  Alert,
  Divider,
} from "@mui/material";
import { VpnKey as KeyIcon } from "@mui/icons-material";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useAuth } from "@/shared/context";
import { useLoginSimple } from "../hooks";
import { FormTextField } from "@/shared/components";
import { loginSchema, type LoginFormData } from "../schemas";

export function LoginPage() {
  const { authConfig, loginOAuth2 } = useAuth();
  const { mutate: login, isPending: isLoading, error } = useLoginSimple();

  const { control, handleSubmit } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
    mode: "onBlur",
    defaultValues: {
      username: "",
      password: "",
    },
  });

  const handleSimpleLogin = (data: LoginFormData) => {
    login(data);
  };

  const simpleEnabled = authConfig?.simple_enabled ?? false;
  const oauth2Enabled = authConfig?.oauth2_enabled ?? false;

  return (
    <Box
      sx={{
        minHeight: "100vh",
        bgcolor: "background.default",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        p: 2,
      }}
    >
      <Card
        sx={{
          maxWidth: 480,
          width: "100%",
          bgcolor: "background.paper",
          border: 1,
          borderColor: "divider",
        }}
      >
        <CardContent sx={{ p: 4 }}>
          <Box sx={{ textAlign: "center", mb: 4 }}>
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                gap: 1.5,
                mb: 1,
              }}
            >
              <Box
                component="img"
                src="/defensepoint-logo.svg"
                alt="DefensePoint"
                sx={{ width: 48, height: 48 }}
              />
              <Typography
                variant="h3"
                sx={{
                  color: "text.primary",
                }}
              >
                Keycloak
              </Typography>
            </Box>
            <Typography variant="body2" sx={{ color: "text.secondary" }}>
              MONITORING TOOL
            </Typography>
          </Box>

          {error && (
            <Alert severity="error" sx={{ mb: 3 }}>
              {error instanceof Error ? error.message : "Login failed"}
            </Alert>
          )}

          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {simpleEnabled && (
              <Box component="form" onSubmit={handleSubmit(handleSimpleLogin)}>
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <FormTextField
                    name="username"
                    control={control}
                    label="Username"
                    type="text"
                    placeholder="Enter your username"
                    required
                    disabled={isLoading}
                    fullWidth
                  />
                  <FormTextField
                    name="password"
                    control={control}
                    label="Password"
                    type="password"
                    placeholder="Enter your password"
                    required
                    disabled={isLoading}
                    fullWidth
                  />
                  <Button
                    type="submit"
                    variant="contained"
                    color="primary"
                    disabled={isLoading}
                    fullWidth
                  >
                    {isLoading ? "Signing in..." : "Sign in"}
                  </Button>
                </Box>
              </Box>
            )}

            {/* Divider if both methods are enabled */}
            {simpleEnabled && oauth2Enabled && (
              <Divider sx={{ my: 1 }}>
                <Typography
                  variant="body2"
                  sx={{ color: "text.secondary", px: 1 }}
                >
                  OR
                </Typography>
              </Divider>
            )}

            {/* OAuth2 SSO */}
            {oauth2Enabled && (
              <Box>
                <Button
                  onClick={loginOAuth2}
                  variant="outlined"
                  fullWidth
                  startIcon={<KeyIcon />}
                >
                  Sign in with SSO
                </Button>
                <Typography
                  variant="body2"
                  sx={{
                    color: "text.secondary",
                    textAlign: "center",
                    mt: 1,
                  }}
                >
                  You will be redirected to your organization's login page
                </Typography>
              </Box>
            )}
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
}
