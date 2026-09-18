import React from "react";
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  TextField,
  Typography,
  Alert,
  Chip,
  Switch,
  FormControlLabel,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Tabs,
  Tab,
  LinearProgress,
  IconButton,
  Tooltip,
  Divider,
  Stack,
} from "@mui/material";
import {
  Add as AddIcon,
  Delete as DeleteIcon,
  Edit as EditIcon,
  Warning as WarningIcon,
  CheckCircle as CheckCircleIcon,
  Error as ErrorIcon,
  Info as InfoIcon,
} from "@mui/icons-material";
import { defensePointColors } from ".";

export function ThemeTestPage() {
  const [tabValue, setTabValue] = React.useState(0);
  const [switchChecked, setSwitchChecked] = React.useState(true);

  return (
    <Box className="p-6 space-y-8">
      {/* Header */}
      <Box>
        <Typography variant="h4" gutterBottom>
          DefensePoint Theme Test
        </Typography>
        <Typography variant="body1" color="text.secondary">
          Demonstration page for Material UI components with the DefensePoint
          theme.
        </Typography>
      </Box>

      <Divider />

      {/* Color Palette */}
      <Card>
        <CardHeader title="Color Palette" />
        <CardContent>
          <Typography variant="subtitle2" gutterBottom>
            Primary (Brand Red)
          </Typography>
          <Stack direction="row" spacing={1} className="mb-4">
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.red.light }}
            >
              Light
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.red.primary }}
            >
              Primary
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.red.dark }}
            >
              Dark
            </Box>
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            Backgrounds
          </Typography>
          <Stack direction="row" spacing={1} className="mb-4">
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs border border-defense-border-primary"
              sx={{ bgcolor: defensePointColors.background.primary }}
            >
              Primary
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs border border-defense-border-primary"
              sx={{ bgcolor: defensePointColors.background.secondary }}
            >
              Secondary
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs border border-defense-border-primary"
              sx={{ bgcolor: defensePointColors.background.elevated }}
            >
              Elevated
            </Box>
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            Status Colors
          </Typography>
          <Stack direction="row" spacing={1}>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.accent.success }}
            >
              Success
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.accent.warning }}
            >
              Warning
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.accent.error }}
            >
              Error
            </Box>
            <Box
              className="w-20 h-20 rounded flex items-center justify-center text-xs"
              sx={{ bgcolor: defensePointColors.accent.info }}
            >
              Info
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* Typography */}
      <Card>
        <CardHeader title="Typography" />
        <CardContent className="space-y-2">
          <Typography variant="h1">H1 - DefensePoint</Typography>
          <Typography variant="h2">H2 - Security Platform</Typography>
          <Typography variant="h3">H3 - Monitoring Dashboard</Typography>
          <Typography variant="h4">H4 - Real-time Alerts</Typography>
          <Typography variant="h5">H5 - User Management</Typography>
          <Typography variant="h6">H6 - Settings</Typography>
          <Divider className="my-4" />
          <Typography variant="subtitle1">
            Subtitle 1 - Important information
          </Typography>
          <Typography variant="subtitle2">
            SUBTITLE 2 - SECTION HEADER
          </Typography>
          <Typography variant="body1">
            Body 1 - This is the default body text style used for paragraphs and
            general content.
          </Typography>
          <Typography variant="body2">
            Body 2 - Smaller body text for secondary content and descriptions.
          </Typography>
          <Typography variant="caption" display="block">
            Caption - Small text for labels and metadata
          </Typography>
          <Typography variant="overline" display="block">
            Overline - Category label
          </Typography>
        </CardContent>
      </Card>

      {/* Buttons */}
      <Card>
        <CardHeader title="Buttons" />
        <CardContent>
          <Typography variant="subtitle2" gutterBottom>
            Contained
          </Typography>
          <Stack direction="row" spacing={2} className="mb-4">
            <Button variant="contained" color="primary">
              Primary
            </Button>
            <Button variant="contained" color="secondary">
              Secondary
            </Button>
            <Button variant="contained" color="success">
              Success
            </Button>
            <Button variant="contained" color="error">
              Error
            </Button>
            <Button variant="contained" disabled>
              Disabled
            </Button>
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            Outlined
          </Typography>
          <Stack direction="row" spacing={2} className="mb-4">
            <Button variant="outlined" color="primary">
              Primary
            </Button>
            <Button variant="outlined" color="inherit">
              Inherit
            </Button>
            <Button variant="outlined" color="success">
              Success
            </Button>
            <Button variant="outlined" color="error">
              Error
            </Button>
            <Button variant="outlined" disabled>
              Disabled
            </Button>
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            Text
          </Typography>
          <Stack direction="row" spacing={2} className="mb-4">
            <Button variant="text" color="primary">
              Primary
            </Button>
            <Button variant="text" color="inherit">
              Inherit
            </Button>
            <Button variant="text" color="success">
              Success
            </Button>
            <Button variant="text" color="error">
              Error
            </Button>
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            With Icons
          </Typography>
          <Stack direction="row" spacing={2}>
            <Button variant="contained" startIcon={<AddIcon />}>
              Add
            </Button>
            <Button variant="outlined" startIcon={<EditIcon />}>
              Edit
            </Button>
            <Button
              variant="contained"
              color="error"
              startIcon={<DeleteIcon />}
            >
              Delete
            </Button>
          </Stack>
        </CardContent>
      </Card>

      {/* Form Controls */}
      <Card>
        <CardHeader title="Form Fields" />
        <CardContent>
          <Stack spacing={3}>
            <Stack direction="row" spacing={2}>
              <TextField label="Default field" placeholder="Type here..." />
              <TextField
                label="With value"
                defaultValue="admin@example.com"
              />
              <TextField
                label="Disabled"
                disabled
                defaultValue="Not editable"
              />
            </Stack>

            <Stack direction="row" spacing={2}>
              <TextField
                label="With error"
                error
                helperText="This field is required"
              />
              <TextField
                label="Multiline"
                multiline
                rows={3}
                placeholder="Type a description..."
              />
              <TextField
                label="Password"
                type="password"
                defaultValue="password123"
              />
            </Stack>

            <Stack direction="row" spacing={2} alignItems="center">
              <FormControlLabel
                control={
                  <Switch
                    checked={switchChecked}
                    onChange={(e) => setSwitchChecked(e.target.checked)}
                  />
                }
                label="Active switch"
              />
              <FormControlLabel
                control={<Switch disabled />}
                label="Disabled switch"
              />
            </Stack>
          </Stack>
        </CardContent>
      </Card>

      {/* Alerts */}
      <Card>
        <CardHeader title="Alerts" />
        <CardContent>
          <Stack spacing={2}>
            <Alert severity="error" icon={<ErrorIcon />}>
              Critical error: Keycloak authentication failed.
            </Alert>
            <Alert severity="warning" icon={<WarningIcon />}>
              Warning: SSL certificate expires in 7 days.
            </Alert>
            <Alert severity="success" icon={<CheckCircleIcon />}>
              Success: Configuration saved successfully.
            </Alert>
            <Alert severity="info" icon={<InfoIcon />}>
              Info: New version available for update.
            </Alert>
          </Stack>
        </CardContent>
      </Card>

      {/* Chips */}
      <Card>
        <CardHeader title="Chips / Tags" />
        <CardContent>
          <Typography variant="subtitle2" gutterBottom>
            Status
          </Typography>
          <Stack direction="row" spacing={1} className="mb-4">
            <Chip label="Online" color="success" size="small" />
            <Chip label="Offline" color="error" size="small" />
            <Chip label="Pending" color="warning" size="small" />
            <Chip label="Info" color="info" size="small" />
            <Chip label="Default" size="small" />
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            Severity
          </Typography>
          <Stack direction="row" spacing={1} className="mb-4">
            <Chip
              label="CRITICAL"
              size="small"
              sx={{
                bgcolor: defensePointColors.severity.critical,
                color: "white",
              }}
            />
            <Chip
              label="HIGH"
              size="small"
              sx={{ bgcolor: defensePointColors.severity.high, color: "white" }}
            />
            <Chip
              label="MEDIUM"
              size="small"
              sx={{
                bgcolor: defensePointColors.severity.medium,
                color: "white",
              }}
            />
            <Chip
              label="LOW"
              size="small"
              sx={{ bgcolor: defensePointColors.severity.low, color: "white" }}
            />
            <Chip
              label="INFO"
              size="small"
              sx={{ bgcolor: defensePointColors.severity.info, color: "white" }}
            />
          </Stack>

          <Typography variant="subtitle2" gutterBottom>
            With Actions
          </Typography>
          <Stack direction="row" spacing={1}>
            <Chip label="Deletable" onDelete={() => {}} color="primary" />
            <Chip
              label="Clickable"
              onClick={() => {}}
              color="primary"
              variant="outlined"
            />
            <Chip
              label="With icon"
              icon={<CheckCircleIcon />}
              color="success"
              variant="outlined"
            />
          </Stack>
        </CardContent>
      </Card>

      {/* Tabs */}
      <Card>
        <CardHeader title="Tabs" />
        <CardContent>
          <Tabs value={tabValue} onChange={(_, v) => setTabValue(v)}>
            <Tab label="Overview" />
            <Tab label="Events" />
            <Tab label="Alerts" />
            <Tab label="Settings" />
          </Tabs>
          <Box className="p-4 border border-defense-border-primary rounded-b">
            {tabValue === 0 && <Typography>Overview tab content</Typography>}
            {tabValue === 1 && <Typography>Events tab content</Typography>}
            {tabValue === 2 && <Typography>Alerts tab content</Typography>}
            {tabValue === 3 && <Typography>Settings tab content</Typography>}
          </Box>
        </CardContent>
      </Card>

      {/* Table */}
      <Card>
        <CardHeader title="Table" />
        <CardContent>
          <TableContainer component={Paper}>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>User</TableCell>
                  <TableCell>Email</TableCell>
                  <TableCell>Role</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {[
                  {
                    user: "admin",
                    email: "admin@example.com",
                    role: "Admin",
                    status: "Active",
                  },
                  {
                    user: "john.doe",
                    email: "john@example.com",
                    role: "Operator",
                    status: "Active",
                  },
                  {
                    user: "jane.smith",
                    email: "jane@example.com",
                    role: "Viewer",
                    status: "Inactive",
                  },
                ].map((row) => (
                  <TableRow key={row.user}>
                    <TableCell>{row.user}</TableCell>
                    <TableCell>{row.email}</TableCell>
                    <TableCell>
                      <Chip label={row.role} size="small" variant="outlined" />
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={row.status}
                        size="small"
                        color={row.status === "Active" ? "success" : "default"}
                      />
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title="Edit">
                        <IconButton size="small">
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete">
                        <IconButton size="small" color="error">
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>

      {/* Progress */}
      <Card>
        <CardHeader title="Progress" />
        <CardContent>
          <Stack spacing={3}>
            <Box>
              <Typography variant="body2" className="mb-1">
                CPU Usage: 45%
              </Typography>
              <LinearProgress variant="determinate" value={45} />
            </Box>
            <Box>
              <Typography variant="body2" className="mb-1">
                Memory Usage: 78%
              </Typography>
              <LinearProgress
                variant="determinate"
                value={78}
                color="warning"
              />
            </Box>
            <Box>
              <Typography variant="body2" className="mb-1">
                Disk Usage: 92%
              </Typography>
              <LinearProgress variant="determinate" value={92} color="error" />
            </Box>
            <Box>
              <Typography variant="body2" className="mb-1">
                Loading...
              </Typography>
              <LinearProgress />
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* Mixed Tailwind + MUI */}
      <Card>
        <CardHeader title="Tailwind + MUI (Coexistence)" />
        <CardContent>
          <Typography variant="body2" color="text.secondary" className="mb-4">
            Demonstration of how to use Tailwind classes alongside MUI
            components.
          </Typography>

          <div className="grid grid-cols-3 gap-4">
            <div className="bg-defense-bg-secondary p-4 rounded border border-defense-border-primary">
              <Typography variant="subtitle2" gutterBottom>
                Tailwind Card
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Container using Tailwind classes
              </Typography>
              <Button variant="contained" size="small" className="mt-3">
                Action
              </Button>
            </div>

            <Card className="shadow-glow-red">
              <CardContent>
                <Typography variant="subtitle2" gutterBottom>
                  MUI Card + Shadow
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  MUI Card with Tailwind shadow
                </Typography>
              </CardContent>
            </Card>

            <div className="flex flex-col gap-2 p-4 bg-defense-bg-elevated rounded">
              <Button variant="contained" fullWidth>
                Button 1
              </Button>
              <Button variant="outlined" fullWidth>
                Button 2
              </Button>
              <Button variant="text" fullWidth>
                Button 3
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </Box>
  );
}
