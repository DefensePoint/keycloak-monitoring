# Quick Start Guide

This guide will help you get Keycloak Monitoring Tool up and running in under 10 minutes.

## What is Keycloak Monitoring Tool?

Keycloak Monitoring Tool is a comprehensive monitoring and security compliance solution for Keycloak identity and access management systems. It provides:

- **Multi-Tenant Monitoring**: Monitor multiple Keycloak instances from a single dashboard
- **Real-Time Event Tracking**: Track logins, logouts, registrations, and security events
- **Security Configuration Checks**: Automated scanning for misconfigurations and security issues
- **Alert Management**: Centralized alert system with severity-based filtering
- **Notification Integrations**: Send alerts to Slack, Email, and create GitLab issues
- **Dashboard Analytics**: Interactive charts and statistics for monitoring trends

## Prerequisites

### Required

- **Docker** and **Docker Compose** (recommended) OR
- **Go 1.23+** and **Node.js 18+** for local development
- A **Keycloak instance** to monitor (version 17+ or legacy)

### Optional

- **GitLab** account (for issue tracking integration)
- **Slack** workspace (for real-time notifications)
- **SMTP** server (for email alerts)

## Quick Start with Docker (Recommended)

This is the fastest way to get started. The entire stack (API server, web frontend, and PostgreSQL) runs in Docker containers.

### Step 1: Clone the Repository

```bash
git clone https://github.com/DefensePoint/keycloak-monitoring.git
cd keycloak-monitoring
```

### Step 2: Create Configuration

```bash
# Copy the example configuration
cp config.yaml.example config.yaml
```

### Step 3: Configure Your Keycloak Instance

Edit `config.yaml` and add your Keycloak connection details:

```yaml
keycloak:
  tenants:
    # Add your Keycloak instance
    my-keycloak:
      enabled: true
      name: "My Keycloak"
      description: "Production Keycloak instance"

      # Your Keycloak server URL
      server_url: "https://keycloak.example.com"

      # Authentication: client_credentials grant via a dedicated confidential
      # client (see docs/keycloak-setup.md to create it).
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "your-client-secret"

      # Optional: categorize with tags
      tags:
        - "production"
      owner: "platform-team"
```

**Security Note**: For production, use environment variables for sensitive data:

```bash
export MONITORING_KEYCLOAK_TENANTS_MY_KEYCLOAK_CLIENT_SECRET=your-client-secret
```

### Step 4: Configure Initial Admin Credentials

**REQUIRED**: Set the initial admin credentials via environment variables:

```bash
export MONITORING_AUTH_SIMPLE_DEFAULT_USER="admin"
export MONITORING_AUTH_SIMPLE_DEFAULT_PASS="YourS3cur3P@ssw0rd!"  # Use a strong password
export MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL="admin@example.com"
```

**Password Requirements:**

- Minimum 12 characters
- At least one uppercase letter, lowercase letter, number, and special character
- Cannot be a common weak password (admin123, password123, etc.)

### Step 5: Build and Start Services

```bash
# Build Docker images
make docker-build

# Start all services (API, Web, PostgreSQL)
make docker-up

# Check if services are running
docker-compose -f deployments/local/docker-compose.yml ps
```

### Step 6: Access the Dashboard

Open your browser to **<http://localhost:7880>**

**Login with your configured credentials:**

- Username: The value you set in `MONITORING_AUTH_SIMPLE_DEFAULT_USER`
- Password: The value you set in `MONITORING_AUTH_SIMPLE_DEFAULT_PASS`

**IMPORTANT**: Change your password after first login. The initial password is supplied via environment variable and remains valid until changed.

### Step 7: Verify Monitoring

1. Navigate to the **Dashboard** page
2. Select your Keycloak tenant from the dropdown
3. You should see:
   - User and client statistics
   - Recent authentication events
   - System health status

If you see data, congratulations! Your Keycloak instance is being monitored.

## Local Development Setup

For development or if you prefer running without Docker:

### Step 1: Start PostgreSQL

```bash
docker run -d --name postgres -p 5432:5432 \
  -e POSTGRES_DB=monitoring \
  -e POSTGRES_USER=monitoring \
  -e POSTGRES_PASSWORD=monitoring_password \
  timescale/timescaledb:latest-pg17
```

### Step 2: Configure Application

```bash
cp config.yaml.example config.yaml
# Edit config.yaml with your Keycloak details
```

### Step 3: Run Backend API Server

```bash
# Install Go dependencies
go mod download

# Run the server
make run
# Or: go run ./cmd/server
```

The API server will start on **<http://localhost:7888>**

### Step 4: Run Frontend (New Terminal)

```bash
cd web

# Install dependencies (first time only)
npm install

# Start development server
npm run dev
```

The web UI will be available at **<http://localhost:7880>**

## First Steps

### Understanding the Dashboard

After logging in, you'll see the main dashboard with:

1. **Tenant Selector** (top right): Switch between monitored Keycloak instances
2. **Realm Selector**: Filter data by specific realms
3. **Statistics Cards**: Overview of users, clients, sessions, and events
4. **Charts**: Visual representation of authentication patterns
5. **Recent Events**: Real-time stream of authentication events

### Exploring the Navigation

- **Dashboard**: Overview and real-time statistics
- **Events**: Detailed event logs with filtering
- **Users**: Browse users across all realms
- **Clients**: View and inspect OAuth2/OIDC clients
- **Alerts**: Security configuration alerts (see below)
- **Tenants**: Manage monitored Keycloak instances

## Adding Your First Keycloak Instance

You can add Keycloak instances via configuration file or the UI.

### Via UI (Recommended for Testing)

1. Go to **Tenants** page
2. Click **Add Tenant**
3. Fill in the form:
   - **Tenant ID**: Unique identifier (e.g., `prod-keycloak`)
   - **Name**: Display name (e.g., `Production Keycloak`)
   - **Server URL**: Your Keycloak URL
   - **Admin Credentials**: Username and password
4. Click **Create**
5. The system will validate the connection and start monitoring

### Via Configuration File (Recommended for Production)

Edit `config.yaml`:

```yaml
keycloak:
  tenants:
    prod-keycloak:
      enabled: true
      name: "Production Keycloak"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${KEYCLOAK_CLIENT_SECRET}"  # Use env var
      tags: ["production", "critical"]
      owner: "platform-team"

    staging-keycloak:
      enabled: true
      name: "Staging Environment"
      server_url: "https://staging-keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${STAGING_KEYCLOAK_CLIENT_SECRET}"
      tags: ["staging", "non-production"]
      owner: "dev-team"
```

Restart the application to load the new configuration.

## Configuring Alerts

Keycloak Monitoring Tool automatically scans your Keycloak configuration for security issues and misconfigurations.

### What Gets Checked

The platform performs automated checks for:

#### Realm Security

- SSL/TLS requirements
- Brute force protection
- Password policy strength
- Email verification requirements
- Admin and user event logging
- Duplicate email prevention

#### Client Security

- Insecure redirect URIs (wildcards, localhost in production)
- Public clients with service accounts
- Deprecated OAuth2 flows (implicit flow, direct access grants)
- Missing client secrets for confidential clients
- HTTP (non-HTTPS) redirect URIs

#### Identity Provider Security

- Missing SAML metadata descriptor URLs
- Expired or expiring certificates
- Certificate expiration warnings (30 days before expiry)

### Viewing Alerts

1. Navigate to **Alerts** page
2. Filter by:
   - **Status**: Active, Acknowledged, Resolved, Ignored
   - **Severity**: Critical, Error, Warning, Info
   - **Realm**: Specific Keycloak realm
   - **Resource Type**: realm, client, identity_provider

### Taking Action on Alerts

For each alert, you can:

- **View Details**: See full description and remediation steps
- **Acknowledge**: Mark as "being worked on"
- **Ignore**: Suppress future alerts for this specific issue
- **Resolve**: Mark as fixed (will be verified on next check)

### Customizing Configuration Checks

Edit `config.yaml` to customize which checks run:

```yaml
keycloak:
  global:
    config_checker:
      poll_interval: 5m  # How often to run checks

      checks:
        # Identity Provider checks
        identity_provider_metadata:
          enabled: true
          certificate_expiration_warning_days: 30

        # Realm security checks
        realm_security:
          enabled: true
          check_ssl_required: true
          check_brute_force: true
          check_password_policy: true
          min_password_length: 12  # Minimum acceptable password length

        # Client security checks
        client_security:
          enabled: true
          check_redirect_uris: true
          check_public_clients: true
          allow_localhost_redirects: false  # Strict for production
```

## Setting Up Notifications

Get notified about critical security issues via Slack, Email, or GitLab.

### Slack Integration

1. Create a Slack incoming webhook:
   - Go to <https://api.slack.com/messaging/webhooks>
   - Create a new webhook for your workspace
   - Copy the webhook URL

2. Configure in `config.yaml`:

    ```yaml
    notifications:
      slack:
        enabled: true
        webhook_url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
        channel: "#security-alerts"  # Optional: override webhook channel
        username: "Keycloak Monitoring Tool"
        icon_emoji: ":shield:"

        # Only send warnings and above to Slack
        min_severity: "warning"
    ```

3. Restart the application

You'll now receive Slack messages for new alerts!

### Email Integration

Configure SMTP settings for email notifications:

```yaml
notifications:
  email:
    enabled: true
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    username: "alerts@example.com"
    password: "${SMTP_PASSWORD}"  # Use environment variable
    from: "alerts@example.com"
    to:
      - "security-team@example.com"
      - "devops@example.com"
    use_tls: true

    # Only send error and critical via email
    min_severity: "error"
```

**Gmail Users**: Use an [App Password](https://support.google.com/accounts/answer/185833) instead of your regular password.

### GitLab Issue Integration

Automatically create GitLab issues for critical alerts:

```yaml
notifications:
  gitlab:
    enabled: true
    url: "https://gitlab.com"  # Or your GitLab instance
    token: "${GITLAB_TOKEN}"   # Personal or project access token
    project_id: "12345"        # Or "group/project"
    milestone: "Security Sprint"  # Optional
    labels:
      - "security"
      - "keycloak"
      - "automated"

    # Only create GitLab issues for critical alerts
    min_severity: "critical"
```

### Understanding Severity Levels

Configure different notification thresholds for each channel:

- **`info`**: Lowest severity - informational alerts
- **`warning`**: Medium severity - potential issues
- **`error`**: High severity - confirmed security issues
- **`critical`**: Highest severity - urgent security risks

**Recommended Configuration:**

- **Slack**: `warning` (get notified about most issues)
- **Email**: `error` (avoid notification fatigue)
- **GitLab**: `critical` (track only urgent issues)

Example alert distribution:

- **Info** alert: Not sent anywhere
- **Warning** alert: Sent to Slack only
- **Error** alert: Sent to Slack + Email
- **Critical** alert: Sent to Slack + Email + GitLab issue

## Next Steps

### Explore Advanced Features

- **[Multi-Tenancy](configuration.md#multi-tenancy)**: Monitor multiple Keycloak environments
- **[Custom Dashboards](architecture.md)**: Understanding the data model

### Secure Your Installation

1. **Change default credentials**:

   ```yaml
   auth:
     simple:
       default_user: "admin"
       default_pass: "YOUR_SECURE_PASSWORD"
   ```

2. **Use environment variables** for sensitive data:

   ```bash
   export MONITORING_AUTH_SIMPLE_DEFAULT_PASS=secure_password
   export MONITORING_KEYCLOAK_TENANTS_PROD_CLIENT_SECRET=keycloak_client_secret
   ```

3. **Enable HTTPS** in production (see [deployment guide](deployment.md))

4. **Set up OAuth2 authentication** with Keycloak SSO:

   ```yaml
   auth:
     simple:
       enabled: false
     oauth2:
       enabled: true
       provider_url: "https://keycloak.example.com/realms/master"
       client_id: "monitoring-dashboard"
       client_secret: "${OAUTH_CLIENT_SECRET}"
   ```

### Optimize Performance

- Adjust polling intervals based on your needs:

  ```yaml
  keycloak:
    global:
      polling:
        metrics_interval: 5m      # User/client counts
        events_interval: 30s      # Authentication events
        health_interval: 1m       # Server health
        realm_info_interval: 15m  # Realm configuration
  ```

- Configure event filtering to reduce data volume:

  ```yaml
  keycloak:
    global:
      events:
        types:
          - "LOGIN"
          - "LOGIN_ERROR"
          - "LOGOUT"
          - "REGISTER"
        max_events_per_poll: 1000
  ```

### Get Help

- **Documentation**: See [docs/](../) directory
- **Troubleshooting**: Check [troubleshooting.md](troubleshooting.md)
- **Issues**: Report bugs or request features on GitLab

## Common Issues

### "Connection refused" when accessing dashboard

- Verify services are running: `docker-compose ps`
- Check logs: `docker-compose logs -f`
- Ensure ports 7880 (web) and 7888 (api) are not in use

### "Failed to connect to Keycloak"

- Verify Keycloak URL is correct
- Check admin credentials are valid
- Ensure network connectivity between Keycloak Monitoring Tool and Keycloak
- For Keycloak < 17, include `/auth` in server_url

### No events showing up

- Check polling intervals in configuration
- Verify Keycloak event logging is enabled in your realms
- Check API server logs for errors: `docker-compose logs api-server`

### Alerts not appearing

- Verify configuration checker is enabled:

  ```yaml
  keycloak:
    global:
      config_checker:
        checks:
          realm_security:
            enabled: true
  ```

- Check that realms are being monitored
- Look for configuration checker logs in API server

Success!

You now have a fully functional Keycloak monitoring platform! 🎉

The system is:

- Monitoring your Keycloak instance(s)
- Collecting authentication events
- Scanning for security misconfigurations
- Sending alerts to your configured channels

For more advanced configuration and deployment options, check out the complete [documentation](../README.md#documentation).
