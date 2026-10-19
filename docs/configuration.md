# Configuration Guide

Complete reference for configuring the Keycloak Monitoring Tool.

## Overview

The Keycloak Monitoring Tool uses a YAML configuration file (`config.yaml`) as the primary configuration source. Settings with a built-in default or a key present in `config.yaml` can be overridden using the `MONITORING_` prefix.

### Configuration Priority

Configuration values are loaded in the following priority order (highest to lowest):

1. **Environment Variables** (e.g., `MONITORING_DATABASE_HOST`)
2. **Configuration File** (`config.yaml`)
3. **Default Values** (hardcoded in the application)

### Creating Your Configuration

```bash
# Copy the example configuration
cp config.yaml.example config.yaml

# Edit with your settings
vim config.yaml

# Or use environment variables for sensitive data
export MONITORING_DATABASE_PASSWORD=your_secure_password
# Tenant client secrets: reference a variable from config.yaml, e.g.
#   client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"
export PROD_KEYCLOAK_CLIENT_SECRET=your_keycloak_client_secret
```

## Configuration File

The configuration file is a YAML file typically named `config.yaml` and located in the project root directory.

### Specifying a Custom Configuration File

```bash
./bin/server -config /path/to/custom-config.yaml
```

Without `-config`, the server, web and MCP binaries read `config.yaml` from the
working directory. When that file is missing, they load only the built-in
defaults and `MONITORING_*` variables.

## Environment Variables

For eligible configuration keys, construct the environment variable using this convention:

1. Prefix with `MONITORING_`
2. Replace `.` (dots) with `_` (underscores)
3. Convert to UPPERCASE

An override only applies to a key that has a built-in default or is present in `config.yaml`. For settings of a single tenant under `keycloak.tenants`, use a `${NAME}` placeholder in the file instead; see "Environment Variables for Multi-Tenant Setup" under [Keycloak Monitoring](#keycloak-monitoring).

### Examples

| Config Key | Environment Variable |
|------------|---------------------|
| `database.host` | `MONITORING_DATABASE_HOST` |
| `keycloak.global.polling.metrics_interval` | `MONITORING_KEYCLOAK_GLOBAL_POLLING_METRICS_INTERVAL` |
| `http.server.port` | `MONITORING_HTTP_SERVER_PORT` |
| `auth.simple.default_pass` | `MONITORING_AUTH_SIMPLE_DEFAULT_PASS` |
| `auth.session.secret` | `MONITORING_AUTH_SESSION_SECRET` |
| `security.encryption_key` | `MONITORING_SECURITY_ENCRYPTION_KEY` |
| `security.run_secret_migration` | `MONITORING_SECURITY_RUN_SECRET_MIGRATION` |

### Environment Variable Example

```bash
export MONITORING_DATABASE_HOST=postgres.example.com
export MONITORING_DATABASE_PORT=5432
export MONITORING_DATABASE_PASSWORD=secure_password
export MONITORING_AUTH_SESSION_SECRET=$(openssl rand -base64 32)

./bin/server
```

### Deployment Variable Requirements

Use this reference for both local and hosted deployments. An environment variable
only overrides a key with a built-in default or a key present in `config.yaml`.
Keep a key without a built-in default present, even if empty, when supplying it
through `MONITORING_*`. Examples are `auth.simple.default_pass`,
`auth.session.secret`, and the `auth.oauth2` keys `provider_url`, `client_id`,
`client_secret` and `redirect_url`. A `${NAME}` placeholder instead uses that
exact environment variable name; it must appear in the configuration value to
have any effect.

| Variable | When it is needed |
| --- | --- |
| `MONITORING_DATABASE_PASSWORD` | Supply the database credential when password authentication is used; it overrides `database.password`. |
| `PROD_KEYCLOAK_CLIENT_SECRET` | Example placeholder name for a monitored tenant's client secret: `client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"`. It has no effect without that placeholder. Use a separate variable for each tenant. |
| `MONITORING_AUTH_SIMPLE_DEFAULT_USER`, `_EMAIL`, `_PASS` | Seed the initial administrator when `auth.simple.enabled` is true and the database has no users. `_USER` and `_EMAIL` fall back to `admin` and `admin@example.com`; `_PASS` has no default. Startup fails if a resolved value is empty or the password breaks the rules below. A populated database starts without the seed. With simple authentication enabled, every startup, including one on a populated database, also gives the account whose email matches `_EMAIL` the global administrator role if it lacks it. Because `_EMAIL` falls back to `admin@example.com`, this runs even when the variable is not set, and a revoked administrator role on that account returns at the next restart. If no account has that email, startup logs a warning and continues. |
| `MONITORING_AUTH_SESSION_SECRET` | Required for login: with an empty secret, every login returns HTTP 500 and the server logs `securecookie: hash key is not set`. Use a stable random value of at least 32 characters, and keep `auth.session.secret` present for the override. |
| `MONITORING_AUTH_OAUTH2_CLIENT_SECRET` | Required when `auth.oauth2.enabled` is true. An empty `client_secret`, `provider_url`, `client_id` or `redirect_url` disables OAuth2 login with a startup warning. Keep `auth.oauth2.client_secret` present for the override. |
| `MONITORING_SECURITY_ENCRYPTION_KEY` | Recommended for production, optional for a new plaintext deployment. Without it, the server warns that tenant secrets are stored as plaintext. If set, it must be the base64 encoding of 32 bytes; an invalid key prevents startup. Tenants with encrypted secrets cannot authenticate until their original key is restored. See [Secrets Encryption](#secrets-encryption). |
| `MONITORING_SECURITY_RUN_SECRET_MIGRATION` | Optional, deliberate migration of existing plaintext tenant secrets; only takes effect with the encryption key. |

The initial administrator password must contain at least 12 characters, an
uppercase letter, a lowercase letter, a number, and a special character. The
validator rejects any case-insensitive substring matching `password`, `admin`,
`welcome`, `letmein`, `qwerty`, `123456`, `secret`, `changeme`, or `default`.
Generate a unique password rather than reusing a documentation example. The
initial password is not rotated automatically; change it after first login.

## Configuration Sections

### HTTP Server

Configuration for the main API server.

```yaml
http:
  server:
    # Enable/disable the HTTP server
    # Default: true
    enabled: true

    # Host to bind to
    # - "0.0.0.0": Listen on all interfaces (Docker/production)
    # - "127.0.0.1" or "localhost": Local only (development)
    # Default: "0.0.0.0"
    host: "0.0.0.0"

    # Port to listen on
    # Default: 7888
    port: 7888
```

or

```bash
export MONITORING_HTTP_SERVER_ENABLED=true
export MONITORING_HTTP_SERVER_HOST=0.0.0.0
export MONITORING_HTTP_SERVER_PORT=7888
```

#### Cross-origin requests

By default only KMT's own web UI can use the API with the session cookie:

- No CORS headers are sent, so a page on another origin can't read API responses.
- A POST, PUT or DELETE that a browser marks as coming from another origin (`Sec-Fetch-Site`, or an `Origin` that doesn't match the request's host) is refused with 403 and logged as `Cross-origin request refused`. This holds whatever `auth.session.same_site` is set to.
- Requests without `Origin` or `Sec-Fetch-Site`, such as API-token clients, curl and the MCP server, are not affected.

The web server forwards the browser's `Host` to the API server, so its proxy needs no extra setting. A reverse proxy in front of the web server must pass the original `Host` through, port included: with nginx that is `proxy_set_header Host $http_host;`, not `$host`, which drops the port. Over plain HTTP on a non-standard port, browsers send no `Sec-Fetch-Site`, and a `Host` without the port makes the UI's own writes fail with 403.

To let a trusted browser app on another origin call the API, list its origin:

```yaml
http:
  cors:
    # scheme://host[:port], http or https. Lowercased, and :80 or :443 dropped,
    # to match the Origin header browsers send. Wildcards, paths, user info and
    # non-ASCII hosts (use the xn-- form) are rejected at startup.
    # Default: []
    allowed_origins:
      - "https://dashboard.example.com"
```

or

```bash
# Separate several origins with spaces or commas
export MONITORING_HTTP_CORS_ALLOWED_ORIGINS="https://dashboard.example.com"
```

#### HTTPS and the session cookie

A deployment served over HTTPS needs nothing set: these defaults keep the session cookie and every later visit on HTTPS.

- `auth.session.secure` defaults to `true`, so the session cookie, and the short-lived single sign-on login cookie, are marked `Secure` and never sent over plain HTTP. A cookie set over HTTPS is `Secure` whatever the setting says: TLS to KMT, or `X-Forwarded-Proto: https` from the proxy that terminates TLS (Traefik sends it).
- `http.security_headers.hsts` defaults to `max-age=31536000` (one year). The API and the web server send it on every response, the application page included, so after one HTTPS visit the browser upgrades a typed or linked `http://` address before sending anything. Browsers ignore it on a plain HTTP response.

Recommended deployed values, which are the defaults: remove any `secure: false` or `hsts: ""` a deployment's `config.yaml` carries from an older example.

```yaml
auth:
  session:
    secure: true
http:
  security_headers:
    hsts: "max-age=31536000"
```

Local development over plain HTTP sets `auth.session.secure: false` (the local `.env.example` sets `MONITORING_AUTH_SESSION_SECURE=false`): browsers drop a `Secure` cookie on a plain HTTP response, so sign-in would not stick. HSTS needs no change there.

### Web Server

Configuration for the web frontend server.

```yaml
web:
  server:
    # Enable/disable the web server
    # Default: true
    enabled: true

    # Host to bind to
    # Default: "0.0.0.0"
    host: "0.0.0.0"

    # Port to listen on
    # Default: 3000
    port: 3000

    # Directory containing static files (built React app)
    # Default: "/app/web/dist" (Docker) or "./web/dist" (local)
    static_dir: "/app/web/dist"

    # API backend URL for proxying
    # The web server proxies /api and /auth requests to this URL
    # Docker: "http://api-server:7888"
    # Local: "http://localhost:7888"
    # Default: "http://localhost:7888"
    api_host_url: "http://api-server:7888"
```

or

```bash
export MONITORING_WEB_SERVER_ENABLED=true
export MONITORING_WEB_SERVER_HOST=0.0.0.0
export MONITORING_WEB_SERVER_PORT=3000
export MONITORING_WEB_SERVER_STATIC_DIR=/app/web/dist
export MONITORING_WEB_SERVER_API_HOST_URL=http://api-server:7888
```

### Keycloak Monitoring

The current multi-tenant structure allows you to monitor multiple Keycloak instances from a single platform. Configuration is organized with **global defaults** that apply to all tenants, and **per-tenant overrides** for specific instances.

**Structure:**

```yaml
keycloak:
  # Global settings inherited by all tenants
  global:
    # Default realms, polling intervals, event settings, etc.

  # Individual Keycloak instances to monitor
  tenants:
    tenant-id-1:
      # Connection details and tenant-specific overrides
    tenant-id-2:
      # Another instance
```

A tenant's `config:` block takes the same keys as `keycloak.global`. Settings it leaves out keep the global value, and a list in it replaces the global list. These apply per tenant: `default_realm`, `realms`, `polling`, `events`, `connection` and `infinispan_port`. These are only read from `keycloak.global`: `config_checker`, `event_checker`, `amfa_checker`, `amfa_mirror`, `version_checker`, `infinispan_enabled` and `connection.allow_private_ranges`. Setting one of them for a tenant logs a warning at startup and has no effect. A value of the wrong type, such as `events_interval: soon`, stops startup; an unknown key is logged as a warning when the tenant's monitor starts.

**Complete Multi-Tenant Example:**

```yaml
keycloak:
  # ============================================================================
  # GLOBAL DEFAULTS
  # These settings are inherited by all tenants unless overridden
  # ============================================================================
  global:
    # Default realms to monitor (can be overridden per tenant)
    # Empty array = monitor all realms
    realms: []

    # Polling intervals for all tenants
    polling:
      # How often to collect metrics (users, clients, sessions)
      metrics_interval: 5m

      # How often to fetch authentication events
      events_interval: 30s

      # How often to check server health
      health_interval: 1m

      # How often to refresh realm information
      realm_info_interval: 15m

    # Event collection settings
    events:
      # Event types to collect (empty = all events)
      # Common types: LOGIN, LOGOUT, REGISTER, LOGIN_ERROR, etc.
      # Not case-sensitive. Names Keycloak does not list are skipped with a
      # warning; if none is known, nothing is collected and an error is logged.
      types: []

      # Maximum events to fetch per poll (prevents database overload)
      max_events_per_poll: 1000

      # Each poll reads only the events after the last one it saw. After a
      # start, new events are fetched at once while the day so far, from the
      # start of the day this long ago, is backfilled a few pages per poll.
      # Failed writes are retried until they are older than that.
      lookback_duration: 1m

      # Keycloak's transaction-default-timeout (5m by default): how long after
      # its time a successful event may become visible. Every 5 minutes one
      # poll reads back this far, plus the time since the last such sweep and
      # a minute's margin. Raise it with Keycloak's; tenants can override it.
      late_commit_window: 5m

    # Connection settings for Keycloak API
    connection:
      # HTTP timeout for API requests
      timeout: 30s

      # Skip TLS certificate verification (development only)
      skip_tls_verify: false

      # Retry settings for failed requests
      max_retries: 3
      retry_backoff: 5s

    # ========================================================================
    # CONFIGURATION CHECKER
    # Automated security configuration scanning
    # ========================================================================
    config_checker:
      # Master switch, off by default: nothing below runs until it is on.
      # When on, every tenant enabled at startup gets the checks enabled below;
      # a tenant added later is checked after a restart. Per-tenant
      # config_checker overrides are not applied yet. The realm and client
      # security checks are on by default and did not run in KMT 1.0.x, so
      # expect a first wave of alerts for existing misconfigurations.
      enabled: false

      # How often to scan configurations for security issues.
      # An alert follows its problem. It resolves on its own, with a
      # resolution notification, once two runs in a row have read its realm
      # without finding the problem (about two intervals). A resolved alert
      # whose problem is found again, because the fix did not take or it came
      # back, reopens as a new occurrence and notifies again. Acknowledged
      # alerts keep their status while the problem lasts; ignored alerts keep
      # theirs for good. A run that cannot read a realm, or an identity
      # provider's metadata, leaves that realm's or provider's alerts as they
      # are, and so does switching a check off below: it stops looking, it
      # does not fix anything. The alerts of a deleted or disabled realm
      # resolve the same way, two runs after it is gone. A realm the
      # monitoring client can no longer see (its roles there were removed)
      # keeps its alerts, and the server logs it once per tenant: "Monitoring
      # client cannot view this realm's settings", then "can view this realm's
      # settings again" once a check reads it.
      poll_interval: 5m

      checks:
        # ====================================================================
        # IDENTITY PROVIDER METADATA CHECKS
        # ====================================================================
        identity_provider_metadata:
          enabled: true

          # Only check SAML providers (skip OIDC, social, etc.)
          check_saml_only: true

          # Certificate expiration warning threshold (days)
          # A warning alert is raised when a certificate expires within this
          # window. Once it has expired, a separate critical alert is raised
          # and notified, and the warning is resolved without a notification.
          certificate_expiration_warning_days: 30

          # HTTP timeout for fetching SAML metadata XML
          certificate_check_timeout: 30s

          # Exclude specific realms from IDP checks
          excluded_realms: []
          # Example:
          # excluded_realms:
          #   - "test-realm"

          # Exclude specific providers by alias
          excluded_providers: []
          # Example:
          # excluded_providers:
          #   - "legacy-saml-idp"

        # ====================================================================
        # REALM SECURITY CHECKS
        # ====================================================================
        realm_security:
          enabled: true

          # Check if SSL is required for all connections
          check_ssl_required: true

          # Check if brute force detection is enabled
          check_brute_force: true

          # Check password policy strength
          check_password_policy: true

          # Check if email verification is required
          check_email_verification: true

          # Check if admin events are logged
          check_admin_events: true

          # Check if user events are logged
          check_user_events: true

          # Check if duplicate emails are prevented
          check_duplicate_emails: true

          # Password policy requirements
          min_password_length: 12  # Minimum password length (default: 8)
          require_password_complexity: true  # Require uppercase, lowercase, digits, special chars

          # Token and session lifetime limits
          max_access_token_lifespan_minutes: 15  # Maximum access token lifetime (default: 15)
          max_sso_session_idle_minutes: 30       # Maximum SSO idle time (default: 30)

        # ====================================================================
        # CLIENT SECURITY CHECKS
        # ====================================================================
        client_security:
          enabled: true

          # Check for insecure redirect URIs (wildcards, localhost in prod)
          check_redirect_uris: true

          # Check for misconfigured public clients
          check_public_clients: true

          # Check for deprecated direct access grants flow
          check_direct_access_grants: true

          # Check for missing client secrets on confidential clients
          check_client_secret: true

          # Check for deprecated implicit flow
          check_implicit_flow: true

          # Check standard flow configuration
          check_standard_flow: true

          # Production environment settings
          production_environment: true  # Enable production-specific checks
          allow_localhost_redirects: false  # Allow localhost in redirect URIs (dev only)

    # ========================================================================
    # VERSION CHECKER (optional)
    # Check for newer Keycloak versions
    # ========================================================================
    version_checker:
      # Cache duration for version information
      cache_ttl: 24h

      # HTTP timeout for GitHub API requests
      http_timeout: 10s

      # GitHub API configuration (for checking Keycloak releases)
      github_api_base_url: "https://api.github.com"
      keycloak_owner: "keycloak"
      keycloak_repo: "keycloak"


  # ============================================================================
  # TENANTS
  # Individual Keycloak instances to monitor
  # ============================================================================
  tenants:
    # Tenant ID: used in URLs and database (must be unique and URL-safe)
    production-keycloak:
      # Display name shown in UI
      name: "Production Keycloak"

      # Optional description
      description: "Main production identity provider"

      # Keycloak server URL (auto-detects /auth prefix for legacy versions)
      server_url: "https://keycloak.example.com"

      # Authentication: client_credentials grant via a dedicated confidential
      # client. client_id + client_secret are
      # required.
      admin_realm: "master"
      client_id: "monitoring-service"
      # ${NAME} is replaced with the NAME environment variable at startup;
      # an unset one is kept as literal text and logged as a warning.
      # Write $${NAME} for a literal ${NAME}.
      client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"

      # Enable/disable monitoring for this tenant
      enabled: true

      # Set as default tenant (automatically selected on app load)
      is_default: false

      # Optional: Tags for organization and filtering
      tags:
        - "production"
        - "critical"
        - "eu-west-1"

      # Optional: Owner/team identifier
      owner: "platform-team"

      # Optional: Override global settings for this tenant
      config:
        # Override specific realms to monitor
        realms:
          - "master"
          - "production-realm"

        # Realm the Dashboard and Events pages open on (empty = All Realms).
        # See "Initial Realm Selection" below.
        default_realm: "production-realm"

        # Override polling intervals for this tenant.
        # For high-volume and/or federated (external user-store) tenants,
        # prefer a SLOWER events interval (1m) to reduce load and log
        # noise on the Keycloak side. See "Recommended polling for large /
        # federated tenants" below.
        polling:
          events_interval: 1m

        # Override connection settings
        connection:
          timeout: 60s  # Longer timeout for this instance

    staging-keycloak:
      name: "Staging Environment"
      description: "Pre-production testing environment"
      server_url: "https://staging-keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${STAGING_KEYCLOAK_CLIENT_SECRET}"
      enabled: true
      tags:
        - "staging"
        - "non-production"
      owner: "dev-team"

      # Watch only the staging realm and fewer event types
      config:
        realms:
          - "staging"
        events:
          types: ["LOGIN_ERROR", "CLIENT_LOGIN_ERROR"]

    dev-keycloak:
      name: "Development Keycloak"
      server_url: "http://localhost:8080"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "dev-client-secret"
      enabled: true
      tags:
        - "development"
        - "local"
      owner: "dev-team"

      # Relaxed settings for development
      config:
        connection:
          skip_tls_verify: true  # OK for local development
        polling:
          metrics_interval: 10m  # Less frequent polling
```

**Environment Variables for Multi-Tenant Setup:**

Give each tenant's secret a `${NAME}` placeholder in `config.yaml` and set the variable in the environment:

```yaml
keycloak:
  tenants:
    production-keycloak:
      client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"
    staging-keycloak:
      client_secret: "${STAGING_KEYCLOAK_CLIENT_SECRET}"
```

Do not use `MONITORING_KEYCLOAK_TENANTS_<ID>_*` overrides for tenant settings. The variable name has to carry the tenant ID exactly, hyphens included (`MONITORING_KEYCLOAK_TENANTS_PRODUCTION-KEYCLOAK_CLIENT_SECRET`), which a shell cannot export, and it only applies when the key is already in `config.yaml`. Written with underscores instead, it is silently ignored and the file's value is used. Global settings have no such problem:

```bash
# Tenant-specific credentials, referenced from config.yaml as above
export PROD_KEYCLOAK_CLIENT_SECRET=secure_prod_client_secret
export STAGING_KEYCLOAK_CLIENT_SECRET=secure_staging_client_secret

# Override global polling for all tenants
export MONITORING_KEYCLOAK_GLOBAL_POLLING_METRICS_INTERVAL=10m
export MONITORING_KEYCLOAK_GLOBAL_POLLING_EVENTS_INTERVAL=1m

# Override configuration checker settings
export MONITORING_KEYCLOAK_GLOBAL_CONFIG_CHECKER_POLL_INTERVAL=10m
export MONITORING_KEYCLOAK_GLOBAL_CONFIG_CHECKER_CHECKS_REALM_SECURITY_MIN_PASSWORD_LENGTH=16
```

Change a specific tenant's settings, such as `enabled` or its polling intervals, in its block in `config.yaml`, using a `${NAME}` placeholder where the value should come from the environment.

**Adding Tenants via UI:**

You can also add and manage tenants through the web interface:

1. Navigate to **Tenants** page in the dashboard
2. Click **Add Tenant** button
3. Fill in the connection details:
   - Tenant ID (unique identifier)
   - Display name
   - Server URL
   - Admin credentials
4. Click **Create**
5. The system validates the connection and starts monitoring

**Managing Default Tenant via UI:**

- Only one tenant can be marked as default at a time
- The default tenant is automatically selected when users first load the application
- Default tenants are displayed with a "DEFAULT" badge in both the tenant list and selector
- To change the default tenant:
  1. Go to **Tenants** page
  2. Click **Edit** on the tenant you want to make default
  3. Check "Set as default tenant"
  4. Click **Save Changes**
- Note: If another tenant is already set as default, you'll receive an error. Uncheck the default option on the current default tenant first.

Tenants created via the UI are stored in the database and override any tenants defined in `config.yaml` with the same ID.

#### Initial Realm Selection

Dashboard and Events open on the tenant's `config.default_realm` when the
browser tab has no saved choice for that tenant. Picking a realm, including
All Realms, saves the choice for that tenant in the tab's `sessionStorage`,
and a saved choice wins over the default. A reload in the same tab keeps the
choice. A new tab opened from a bookmark or a typed URL starts with no saved
choice, and signing out clears the tab's saved choices. A saved or default
realm that is not in the realm list the caller can see falls back to All
Realms. Both pages show All Realms until the realm list loads. A realm page
always opens the realm named in its URL.

The UI uses the tenant default only; `keycloak.global.default_realm` is not a
fallback. Set `config.default_realm` in the tenant's block in `config.yaml`, as
in the `production-keycloak` example above. For a tenant created in the UI, set
its Default Realm on the **Tenants** page. This selection changes the initial
view, not the caller's permissions or the set of realms the server monitors.

#### Notification Configuration

Configure where security alerts are sent. Each notification channel has a minimum severity threshold to prevent alert fatigue.

**Severity Levels:** `info` < `warning` < `error` < `critical`

```yaml
notifications:
  # Base URL for Keycloak Monitoring Tool web UI
  # Used in notifications to provide clickable links to alert details
  # Example: "https://monitoring.example.com"
  # Leave empty to disable links in notifications
  base_url: "https://monitoring.example.com"

  # How often to retry a failed send of a tenant down/degraded, event storage
  # or Keycloak event alert, while the alert stays active and unacknowledged.
  # Only channels whose send failed are retried. Default: 5m
  retry_interval: 5m

  # ============================================================================
  # GITLAB ISSUE INTEGRATION
  # Automatically create GitLab issues for security alerts
  # ============================================================================
  gitlab:
    enabled: true

    # GitLab instance URL
    url: "https://gitlab.com"

    # Personal Access Token or Project Access Token
    # Requires: api scope
    # Production: Use environment variable MONITORING_NOTIFICATIONS_GITLAB_TOKEN
    token: "${GITLAB_TOKEN}"

    # Project ID or path (e.g., "12345" or "group/project")
    project_id: "security-team/keycloak-alerts"

    # Optional: Milestone to assign issues to
    # Can be milestone ID (integer) or title (string)
    milestone: "Security Sprint Q1"

    # Optional: Labels to add to created issues
    labels:
      - "security"
      - "keycloak"
      - "automated"
      - "compliance"

    # Minimum severity level to create GitLab issues
    # Options: info, warning, error, critical
    # Recommended: critical (only create issues for urgent problems)
    min_severity: "critical"

  # ============================================================================
  # SLACK INTEGRATION
  # Send real-time alerts to Slack channels
  # ============================================================================
  slack:
    enabled: true

    # Slack Incoming Webhook URL
    # Create at: https://api.slack.com/messaging/webhooks
    # Production: Use environment variable MONITORING_NOTIFICATIONS_SLACK_WEBHOOK_URL
    webhook_url: "${SLACK_WEBHOOK_URL}"

    # Optional: Override default channel from webhook
    channel: "#security-alerts"

    # Optional: Custom bot username
    username: "Keycloak Monitoring Tool"

    # Optional: Bot icon emoji
    icon_emoji: ":shield:"

    # Minimum severity level to send Slack messages
    # Options: info, warning, error, critical
    # Recommended: warning (get notified about most issues)
    min_severity: "warning"

  # ============================================================================
  # EMAIL INTEGRATION
  # Send email notifications for security alerts
  # ============================================================================
  email:
    enabled: true

    # SMTP server configuration
    smtp_host: "smtp.gmail.com"
    smtp_port: 587

    # SMTP authentication
    # Gmail users: Use App Password, not regular password
    # Production: Use environment variable MONITORING_NOTIFICATIONS_EMAIL_PASSWORD
    username: "alerts@example.com"
    password: "${SMTP_PASSWORD}"

    # Sender email address
    from: "Keycloak Monitoring Tool <alerts@example.com>"

    # Recipient email addresses
    to:
      - "security-team@example.com"
      - "devops@example.com"
      - "compliance@example.com"

    # Use TLS encryption
    use_tls: true

    # Skip TLS certificate verification (development only)
    skip_verify: false

    # Optional: Custom email subject template
    # Default: "[Keycloak Monitoring Tool] {severity} Alert: {check_name}"
    subject: "[Keycloak Security] {severity}: {check_name}"

    # Minimum severity level to send emails
    # Options: info, warning, error, critical
    # Recommended: error (reduce inbox noise)
    min_severity: "error"
```

**Environment Variables for Notifications:**

```bash
# Base URL for clickable links in notifications
export MONITORING_NOTIFICATIONS_BASE_URL=https://monitoring.example.com

# GitLab
export MONITORING_NOTIFICATIONS_GITLAB_ENABLED=true
export MONITORING_NOTIFICATIONS_GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
export MONITORING_NOTIFICATIONS_GITLAB_PROJECT_ID=12345
export MONITORING_NOTIFICATIONS_GITLAB_MIN_SEVERITY=critical

# Slack
export MONITORING_NOTIFICATIONS_SLACK_ENABLED=true
export MONITORING_NOTIFICATIONS_SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
export MONITORING_NOTIFICATIONS_SLACK_CHANNEL="#security"
export MONITORING_NOTIFICATIONS_SLACK_MIN_SEVERITY=warning

# Email
export MONITORING_NOTIFICATIONS_EMAIL_ENABLED=true
export MONITORING_NOTIFICATIONS_EMAIL_SMTP_HOST=smtp.gmail.com
export MONITORING_NOTIFICATIONS_EMAIL_SMTP_PORT=587
export MONITORING_NOTIFICATIONS_EMAIL_USERNAME=alerts@example.com
export MONITORING_NOTIFICATIONS_EMAIL_PASSWORD=your_app_password
export MONITORING_NOTIFICATIONS_EMAIL_FROM="Keycloak Monitoring Tool <alerts@example.com>"
export MONITORING_NOTIFICATIONS_EMAIL_MIN_SEVERITY=error
```

**Recommended Severity Configuration:**

```yaml
# Example: Tiered notification strategy
notifications:
  slack:
    min_severity: "warning"  # Get most alerts in Slack for visibility

  email:
    min_severity: "error"    # Only important issues via email

  gitlab:
    min_severity: "critical" # Track only urgent issues requiring action
```

**How Alerts Are Distributed:**

| Alert Severity | Slack | Email | GitLab Issue |
|---------------|-------|-------|--------------|
| **Info**      | No  | No  | No  |
| **Warning**   | Yes | No  | No  |
| **Error**     | Yes | Yes | No  |
| **Critical**  | Yes | Yes | Yes |

This configuration ensures:

- **Info** alerts are logged but not sent anywhere (low noise)
- **Warning** alerts go to Slack for team awareness
- **Error** alerts go to both Slack and Email for attention
- **Critical** alerts trigger all channels including GitLab issue tracking

### Alert Silences

Alert silences allow you to suppress expected or non-actionable alerts. All events automatically become alerts based on their severity, and silences help filter out noise.

**Important**: Alert silences are distinct from notification thresholds. Silences prevent alerts from being created at all, while notification thresholds control which alerts trigger notifications.

**Use Cases**:

- Silence test/development environment alerts
- Temporary silences during planned maintenance
- Suppress known issues during migration periods
- Filter low-severity alerts from non-production tenants

```yaml
alert_silences:
  # Example: Permanent silence for test environment
  - id: "silence-test-realm"
    name: "Test Realm Alerts"
    description: "Silence all alerts from test realm - used for development only"
    enabled: true
    matchers:
      realm_name: "test-realm"        # Matches realm name exactly
    # duration: ""                    # Empty = permanent silence
    created_by: "system"
    comment: "Test realm generates expected failures during QA testing"

  # Example: Temporary silence for maintenance window
  - id: "silence-client-maintenance"
    name: "Client Credential Rotation Maintenance"
    description: "Silence client auth errors during planned credential rotation"
    enabled: false                    # Enable this when maintenance starts
    matchers:
      alert_type: "CLIENT_LOGIN_ERROR"
      client_id: "production-app"    # Specific client undergoing maintenance
    duration: "2h"                    # Silence for 2 hours
    # starts_at: "2025-11-01T02:00:00Z"  # Optional: schedule for specific time
    created_by: "system"
    comment: "Rotating client secrets - expect temporary auth failures"

  # Example: Silence low-severity development alerts
  - id: "silence-dev-info-alerts"
    name: "Development Environment Info Alerts"
    description: "Silence info-level alerts from development realm"
    enabled: true
    matchers:
      realm_name: "development"
      severity: "info"                # Only silence info level
    created_by: "system"
    comment: "Development generates many info alerts during testing"

  # Example: Wildcard pattern matching
  - id: "silence-test-clients"
    name: "Test Client Alerts"
    description: "Silence alerts from all test clients"
    enabled: true
    matchers:
      client_id: "test-*"            # Matches test-app, test-service, etc.
      severity: "*"                  # All severity levels
    created_by: "system"
    comment: "Test clients are temporary and generate noise"

  # Example: Scheduled maintenance window
  - id: "silence-weekend-maintenance"
    name: "Weekend Database Maintenance"
    description: "Silence all alerts during weekend maintenance window"
    enabled: true
    matchers:
      severity: "*"                  # Silence all severities
      realm_name: "production"       # Only production realm
    starts_at: "2025-11-15T00:00:00Z"  # Start time (RFC3339)
    ends_at: "2025-11-15T04:00:00Z"    # End time (RFC3339)
    created_by: "ops-team"
    comment: "Database migration - expect service interruptions"

  # Example: Tenant-specific silence
  - id: "silence-tenant-migration"
    name: "Tenant Migration"
    description: "Silence alerts during tenant migration to new infrastructure"
    enabled: true
    tenant_id: "staging-keycloak"    # Apply only to this tenant
    matchers:
      alert_type: "*"                # All alert types
    duration: "4h"
    created_by: "migration-team"
    comment: "Migrating tenant data - expect temporary issues"

  # Example: Specific error type silence
  - id: "silence-known-idp-issue"
    name: "Known IDP Configuration Issue"
    description: "Silence IDP errors for provider undergoing reconfiguration"
    enabled: true
    matchers:
      alert_type: "IDENTITY_PROVIDER_*"  # Matches all IDP error types
      realm_name: "production"
    duration: "24h"
    created_by: "security-team"
    comment: "JIRA-1234: Reconfiguring SAML provider certificates"
```

**Silence Configuration Options**:

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique identifier for the silence |
| `name` | Yes | Display name for the silence |
| `description` | Yes | Detailed description of why this silence exists |
| `enabled` | Yes | Enable/disable the silence |
| `matchers` | Yes | Key-value pairs to match alerts (supports wildcards with `*`) |
| `tenant_id` | No | Apply silence only to specific tenant |
| `duration` | No | How long silence lasts (e.g., "2h", "30m", "7d"). Empty = permanent |
| `starts_at` | No | When silence starts (RFC3339). If not set, starts immediately |
| `ends_at` | No | When silence ends (RFC3339). Alternative to `duration` |
| `created_by` | Yes | Who created this silence (for audit trail) |
| `comment` | Yes | Reason for creating this silence |

**Matcher Fields**:

Common matcher fields include:

- `realm_name` - Keycloak realm name
- `client_id` - OAuth2/OIDC client ID
- `alert_type` - Type of alert (e.g., "CLIENT_LOGIN_ERROR", "IDENTITY_PROVIDER_*")
- `severity` - Alert severity ("info", "warning", "error", "critical", "*")
- `username` - Username associated with the alert
- `ip_address` - IP address associated with the alert

**Wildcard Support**:

Use `*` for wildcard matching:

- `"test-*"` matches "test-app", "test-service", "test-anything"
- `"*"` matches all values
- `"IDENTITY_PROVIDER_*"` matches all identity provider alerts

**Silence Metrics**:

Silence metrics are tracked per rule including:

- Total silenced alerts
- Breakdown by tenant, type, and rule
- View metrics via API or UI dashboards

**Environment Variables**:

```bash
# Alert silences cannot be configured via environment variables
# They must be defined in config.yaml
```

### Database

PostgreSQL database configuration.

```yaml
database:
  # PostgreSQL hostname or IP
  # Docker: Use service name from docker-compose.yml
  # Local: "localhost" or "127.0.0.1"
  # Default: "localhost"
  host: "kmt-postgres"

  # PostgreSQL port
  # Default: 5432
  port: 5432

  # Database name
  # Default: "monitoring"
  database: "monitoring"

  # Database username
  # Default: "monitoring"
  user: "monitoring"

  # Database password
  # Required: there is no built-in password, and the server refuses to start
  # with an empty one or an unexpanded ${NAME} placeholder. Set it here or
  # through MONITORING_DATABASE_PASSWORD. mcp.database.password works the
  # same way. The development password the local stacks use only logs a
  # warning.
  password: ""

  # SSL mode for connections
  # Options: disable, require, verify-ca, verify-full
  #   - disable: No SSL (development only)
  #   - require: SSL without verification
  #   - verify-ca: Verify server certificate
  #   - verify-full: Full verification
  # Default when omitted: "require"
  # Set "disable" only for a database on a private network with TLS off, as
  # the Compose stacks do for their own Postgres.
  ssl_mode: "require"

  # Maximum connections in pool
  # Default: 25
  max_conns: 25

  # Minimum idle connections in pool
  # Default: 5
  min_conns: 5

  # How long to wait for a new database connection to open, and for the
  # startup check. A database that accepts the connection but never answers
  # fails startup after this long instead of hanging. It is not a query
  # timeout, and pooled connections are kept regardless of it (each is
  # replaced after 30 minutes). mcp.database.timeout works the same way,
  # and defaults to 10s when omitted.
  # Valid units: ns, us/µs, ms, s, m, h
  # Default: 30s
  timeout: 30s
```

or

```bash
export MONITORING_DATABASE_HOST=postgres.example.com
export MONITORING_DATABASE_PORT=5432
export MONITORING_DATABASE_DATABASE=monitoring
export MONITORING_DATABASE_USER=monitoring
export MONITORING_DATABASE_PASSWORD=secure_password
export MONITORING_DATABASE_SSL_MODE=require
export MONITORING_DATABASE_MAX_CONNS=50
export MONITORING_DATABASE_MIN_CONNS=10
export MONITORING_DATABASE_TIMEOUT=30s
```

### Authentication

Configuration for user authentication.

```yaml
auth:
  # Simple username/password authentication
  simple:
    # Enable/disable simple auth
    # Default: true
    enabled: true

    # Initial admin credentials (created only when no users exist)
    # SECURITY: MUST be set via environment variables in production
    # NEVER commit actual passwords to version control
    # Use: MONITORING_AUTH_SIMPLE_DEFAULT_USER, DEFAULT_PASS, DEFAULT_EMAIL
    # Password rules: see Deployment Variable Requirements above
    default_user: ""
    default_pass: ""
    default_email: ""

  # OAuth2/OIDC authentication (Keycloak SSO)
  oauth2:
    # Enable/disable OAuth2 auth
    # Default: false
    enabled: false

    # OIDC provider URL (Keycloak realm)
    # Example: "https://keycloak.example.com/realms/myrealm"
    # Must point to your Keycloak realm
    provider_url: "https://keycloak.example.com/auth/realms/master"

    # OAuth2 client ID (registered in Keycloak)
    # Required if oauth2.enabled is true
    client_id: "monitoring-dashboard"

    # OAuth2 client secret (from Keycloak)
    # Set it through MONITORING_AUTH_OAUTH2_CLIENT_SECRET rather than here.
    # Required if oauth2.enabled is true
    client_secret: ""

    # OAuth2 redirect URL (callback URL)
    # Must match configuration in Keycloak
    # Format: http(s)://your-domain/auth/callback
    redirect_url: "http://localhost:7880/auth/callback"

    # OAuth2 scopes to request
    # Default: ["openid", "profile", "email"]
    scopes:
      - "openid"
      - "profile"
      - "email"

    # Skip issuer verification (development only)
    # Default: false
    skip_issuer_check: false

    # Skip token expiry check (development only)
    # Default: false
    skip_expiry_check: false

  # Session configuration
  session:
    # Session secret key for cookie encryption
    # MUST be at least 32 characters
    # Generate with: openssl rand -base64 32
    # Production: Use environment variable MONITORING_AUTH_SESSION_SECRET
    # Required: Yes. The server refuses to start while it is empty or one of
    # the example values this project has published.
    secret: ""

    # Session cookie name
    # Default: "monitoring_session"
    name: "monitoring_session"

    # Session maximum age
    # How long sessions remain valid
    # Valid units: ns, us/µs, ms, s, m, h
    # Default: 24h
    max_age: 24h

    # Mark the session cookie Secure (HTTPS only). Over HTTPS it is Secure
    # whatever this says; false only for local development over plain HTTP.
    # See "HTTPS and the session cookie".
    # Default: true
    secure: true

    # SameSite cookie attribute (CSRF protection)
    # Options: lax, strict, none
    #   - lax: Recommended for most apps
    #   - strict: Stricter CSRF protection
    #   - none: Allows cross-site (requires secure: true)
    # Default: "lax"
    same_site: "lax"

    # OAuth2 state parameter max age
    # How long state is valid during login flow
    # Valid units: ns, us/µs, ms, s, m, h
    # Default: 10m
    state_max_age: 10m
```

or, with the keys that have no built-in default kept present in `config.yaml`
(see [Deployment Variable Requirements](#deployment-variable-requirements)):

```bash
# Simple Auth
export MONITORING_AUTH_SIMPLE_ENABLED=true
export MONITORING_AUTH_SIMPLE_DEFAULT_USER=admin
export MONITORING_AUTH_SIMPLE_DEFAULT_PASS='Init!2026-Qx7nK2'
export MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL=admin@example.com

# OAuth2
export MONITORING_AUTH_OAUTH2_ENABLED=false
export MONITORING_AUTH_OAUTH2_PROVIDER_URL=https://keycloak.example.com/realms/myrealm
export MONITORING_AUTH_OAUTH2_CLIENT_ID=monitoring-dashboard
export MONITORING_AUTH_OAUTH2_CLIENT_SECRET=your_secret
export MONITORING_AUTH_OAUTH2_REDIRECT_URL=https://monitoring.example.com/auth/callback

# Session
export MONITORING_AUTH_SESSION_SECRET=$(openssl rand -base64 32)
export MONITORING_AUTH_SESSION_NAME=monitoring_session
export MONITORING_AUTH_SESSION_MAX_AGE=24h
export MONITORING_AUTH_SESSION_SECURE=true
export MONITORING_AUTH_SESSION_SAME_SITE=lax
```

### Secrets Encryption

Configuration for encrypting sensitive values stored in the database, currently the `client_secret` of each monitored Keycloak tenant.

```yaml
security:
  # Base64-encoded 32-byte AES-256 key used to encrypt tenant client_secret
  # values at rest. Generate with: openssl rand -base64 32
  #
  # SECURITY: set via environment variable, never commit a real key here.
  # Use: MONITORING_SECURITY_ENCRYPTION_KEY
  #
  # Leave unset (default) to keep client_secret stored as plaintext, the
  # existing/default behavior. Once set:
  #   - Every newly created or updated tenant has its client_secret
  #     encrypted automatically before being written to the database.
  #   - Tenants that already exist in the database at the time the key is
  #     set are NOT retroactively encrypted by simply setting the key — they
  #     keep working as plaintext until they're next updated (via the UI,
  #     API, or config.yaml sync) or until a separate migration is run.
  #   - If the key is set but invalid (wrong length, not valid base64), the
  #     application fails to start rather than silently falling back to
  #     plaintext.
  encryption_key: ""

  # One-time migration: encrypts every tenant's client_secret that isn't
  # already encrypted. Only takes effect when encryption_key is also set.
  # Deliberately NOT automatic on every boot — set both this and
  # encryption_key, restart once, check the logs for the result
  # ("Finished encrypting tenant client_secret values" with counts), then
  # unset run_secret_migration again. Leaving it on has no further effect on
  # subsequent restarts (already-encrypted rows are skipped), but it's meant
  # to be a deliberate one-shot step, not a standing setting.
  run_secret_migration: false
```

or

```bash
export MONITORING_SECURITY_ENCRYPTION_KEY=$(openssl rand -base64 32)
export MONITORING_SECURITY_RUN_SECRET_MIGRATION=true
```

### Logging

Application logging configuration.

```yaml
logging:
  # Log level
  # Options: error, warn, info, debug
  #   - error: Only errors
  #   - warn: Warnings and errors
  #   - info: Info, warnings, errors (recommended)
  #   - debug: Verbose logging (development/troubleshooting)
  # Default: "info"
  level: "info"

  # Log format
  # Options: json, console
  #   - json: Structured JSON (production, log aggregation)
  #   - console: Human-readable (development)
  # Default: "json"
  format: "json"

  # Log output configuration (logs are written to stdout)
  output:
    # Human-readable console lines instead of JSON lines
    # Production: false (compact logs)
    # Development: true (readable logs)
    # Default: false
    pretty: false
```

or

```bash
export MONITORING_LOGGING_LEVEL=info
export MONITORING_LOGGING_FORMAT=json
export MONITORING_LOGGING_OUTPUT_PRETTY=false
```

### MCP Server

Configuration for the read-only Model Context Protocol (MCP) server, which serves monitoring data to MCP clients. It runs as its own binary (`cmd/mcp-server`); see `cmd/mcp-server/README.md` for the tool reference and client setup.

```yaml
mcp:
  # Enable/disable the MCP server
  # The mcp-server binary exits immediately when disabled
  # Default: false
  enabled: false

  # Port for the MCP endpoint (/mcp), /health, /ready and /metrics
  # Default: 7889
  port: 7889

  # Page size applied when a tool call does not request one
  # Default: 50
  default_page_size: 50

  # Hard cap on any page size a tool call does request
  # These two replace the older max_page_size (the default) and
  # absolute_max_page_size (the cap); a config still carrying
  # absolute_max_page_size fails startup, since max_page_size now caps what it
  # used to default
  # Default: 500
  max_page_size: 500

  # Keyed-hash message authentication code (HMAC) key signing the pagination
  # cursors handed to clients. Required, minimum 32 bytes.
  # Generate with: openssl rand -base64 32
  # The server refuses to start on an empty or shorter key rather than fall
  # back to a per-process one, whose cursors stop verifying after a restart
  # Default: "" (which fails startup)
  cursor_hmac_key: ""

  # Exact Origin header values accepted on MCP requests
  # An empty list rejects every request carrying an Origin header, which is
  # the DNS-rebinding defense; MCP clients are not browsers and send none
  # Default: [] (reject all origins)
  origin_allowlist: []

  # Prometheus metrics on the MCP port
  metrics:
    # Expose /metrics
    # Default: false
    enabled: false

    # Bearer token required on /metrics. /metrics is served outside the MCP
    # authentication middleware, so this is required whenever enabled is
    # true; the server refuses to start on an enabled endpoint with no token
    # Default: ""
    auth_token: ""

  # Token-bucket rate limits; a rate of 0 or less disables that limiter
  rate:
    # Tool calls per user per minute, applied after authentication
    # One budget shared by all of a user's tokens
    # Default: 120
    per_user_per_minute: 120

    # Requests per minute per remote address, applied before authentication
    # Default: 300
    per_ip_per_minute: 300

    # Instantaneous allowance of both buckets
    # Default: 30
    burst: 30

  # Maximum serialized size of a tool result in bytes; a larger result is
  # replaced with an error telling the caller to retry with a smaller limit
  # 0 or less falls back to the default; the cap cannot be disabled
  # Default: 1048576 (1 MiB)
  max_response_bytes: 1048576

  # Required: the read-only role the MCP server runs under. Same keys as the
  # top-level database block. There is no fallback to that block, which names
  # the read-write application role, so the MCP server refuses to start when
  # this block is missing.
  #
  # These keys are deliberately not registered as defaults, so an environment
  # variable is only picked up for a key that is present here: keep
  # password: "" in the file and set MONITORING_MCP_DATABASE_PASSWORD.
  database:
    host: "kmt-postgres"
    port: 5432
    database: "monitoring"
    user: "monitoring_readonly"
    password: ""
    ssl_mode: "require"
    max_conns: 10
    min_conns: 2
    timeout: 30s
```

or

```bash
export MONITORING_MCP_ENABLED=true
export MONITORING_MCP_PORT=7889
export MONITORING_MCP_DEFAULT_PAGE_SIZE=50
export MONITORING_MCP_MAX_PAGE_SIZE=500
export MONITORING_MCP_CURSOR_HMAC_KEY=$(openssl rand -base64 32)
export MONITORING_MCP_METRICS_ENABLED=false
export MONITORING_MCP_METRICS_AUTH_TOKEN=metrics_scrape_token
export MONITORING_MCP_RATE_PER_USER_PER_MINUTE=120
export MONITORING_MCP_RATE_PER_IP_PER_MINUTE=300
export MONITORING_MCP_RATE_BURST=30
export MONITORING_MCP_MAX_RESPONSE_BYTES=1048576
```

## Configuration Examples

### Development Configuration

```yaml
http:
  server:
    host: "127.0.0.1"
    port: 7888

web:
  server:
    host: "127.0.0.1"
    port: 3000
    api_host_url: "http://localhost:7888"

database:
  host: "localhost"
  port: 5432
  database: "monitoring"
  user: "monitoring"
  password: "monitoring_password"
  ssl_mode: "disable"

keycloak:
  global:
    polling:
      events_interval: 10s  # Faster polling for development
      metrics_interval: 5m
    config_checker:
      poll_interval: 15m  # Less frequent checks in dev
      checks:
        client_security:
          allow_localhost_redirects: true  # Allow localhost in dev
          production_environment: false

  tenants:
    local-keycloak:
      name: "Local Development Keycloak"
      server_url: "http://localhost:8080"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "dev-client-secret"
      enabled: true
      tags: ["development", "local"]

auth:
  simple:
    enabled: true
    default_user: "admin"
    default_pass: ""  # Set by MONITORING_AUTH_SIMPLE_DEFAULT_PASS
  oauth2:
    enabled: false
  session:
    secret: ""  # Set by MONITORING_AUTH_SESSION_SECRET
    secure: false

logging:
  level: "debug"
  format: "console"
  output:
    pretty: true

notifications:
  gitlab:
    enabled: false
  slack:
    enabled: false
  email:
    enabled: false
```

### Production Configuration

```yaml
http:
  server:
    host: "0.0.0.0"
    port: 7888

web:
  server:
    host: "0.0.0.0"
    port: 7880
    api_host_url: "http://api-server:7888"

database:
  host: "postgres.example.com"
  port: 5432
  database: "monitoring"
  user: "monitoring"
  # Use environment variable: MONITORING_DATABASE_PASSWORD
  ssl_mode: "require"
  max_conns: 50
  min_conns: 10

keycloak:
  global:
    polling:
      metrics_interval: 5m
      events_interval: 30s
      health_interval: 1m
      realm_info_interval: 15m

    connection:
      skip_tls_verify: false
      timeout: 30s
      max_retries: 3

    config_checker:
      poll_interval: 5m
      checks:
        identity_provider_metadata:
          enabled: true
          certificate_expiration_warning_days: 30
        realm_security:
          enabled: true
          min_password_length: 12
          check_ssl_required: true
          check_brute_force: true
        client_security:
          enabled: true
          production_environment: true
          allow_localhost_redirects: false

  tenants:
    prod-keycloak:
      name: "Production Keycloak"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"
      enabled: true
      tags: ["production", "critical"]
      owner: "platform-team"
      config:
        realms:
          - "master"
          - "production"

    staging-keycloak:
      name: "Staging Keycloak"
      server_url: "https://staging.keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${STAGING_KEYCLOAK_CLIENT_SECRET}"
      enabled: true
      tags: ["staging", "non-production"]

auth:
  simple:
    enabled: false  # Use OAuth2 in production
  oauth2:
    enabled: true
    provider_url: "https://keycloak.example.com/realms/production"
    client_id: "monitoring-dashboard"
    # Set by MONITORING_AUTH_OAUTH2_CLIENT_SECRET; the key must stay present
    # (empty is fine) or the override is ignored.
    client_secret: ""
    redirect_url: "https://monitoring.example.com/auth/callback"
    scopes:
      - "openid"
      - "profile"
      - "email"
  session:
    # Set by MONITORING_AUTH_SESSION_SECRET; the key must stay present
    # (empty is fine) or the override is ignored.
    secret: ""
    max_age: 24h
    secure: true
    same_site: "lax"

logging:
  level: "info"
  format: "json"
  output:
    pretty: false

notifications:
  gitlab:
    enabled: true
    url: "https://gitlab.com"
    # Use environment variable: MONITORING_NOTIFICATIONS_GITLAB_TOKEN
    project_id: "security-team/keycloak-alerts"
    labels: ["security", "keycloak", "automated"]
    min_severity: "critical"

  slack:
    enabled: true
    # Use environment variable: MONITORING_NOTIFICATIONS_SLACK_WEBHOOK_URL
    channel: "#security-alerts"
    username: "Keycloak Monitoring Tool"
    min_severity: "warning"

  email:
    enabled: true
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    # Use environment variables:
    # MONITORING_NOTIFICATIONS_EMAIL_USERNAME
    # MONITORING_NOTIFICATIONS_EMAIL_PASSWORD
    from: "Keycloak Monitoring Tool <alerts@example.com>"
    to:
      - "security-team@example.com"
    use_tls: true
    min_severity: "error"
```

### Docker Compose Configuration

```yaml
# Optimized for Docker Compose deployment
http:
  server:
    host: "0.0.0.0"
    port: 7888

web:
  server:
    host: "0.0.0.0"
    port: 7880
    static_dir: "/app/web/dist"
    api_host_url: "http://api-server:7888"

database:
  host: "kmt-postgres"  # Docker service name
  port: 5432
  database: "monitoring"
  user: "monitoring"
  password: ""                       # from MONITORING_DATABASE_PASSWORD
  ssl_mode: "disable"                # the Compose Postgres has no TLS

keycloak:
  global:
    polling:
      metrics_interval: 5m
      events_interval: 30s
    config_checker:
      poll_interval: 5m
      checks:
        identity_provider_metadata:
          enabled: true
        realm_security:
          enabled: true
        client_security:
          enabled: true

  tenants:
    my-keycloak:
      name: "My Keycloak Instance"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"  # Use env var
      enabled: true
      tags: ["production"]

auth:
  simple:
    enabled: true
    default_user: "${MONITORING_AUTH_SIMPLE_DEFAULT_USER}"  # Use env var
    default_pass: "${MONITORING_AUTH_SIMPLE_DEFAULT_PASS}"  # REQUIRED: Strong password
    default_email: "${MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL}"  # Use env var
  session:
    secret: "${MONITORING_AUTH_SESSION_SECRET}"  # REQUIRED: openssl rand -base64 32

logging:
  level: "info"
  format: "json"

notifications:
  gitlab:
    enabled: false
  slack:
    enabled: false
  email:
    enabled: false
```

## Best Practices

### Security

1. **Never commit secrets to version control**

   ```bash
   # Add config.yaml to .gitignore
   echo "config.yaml" >> .gitignore
   ```

2. **Use environment variables for sensitive data in production**

   Keep `auth.session.secret` present in `config.yaml`, even if empty, for its
   override to apply ([Deployment Variable Requirements](#deployment-variable-requirements)).

   ```bash
   export MONITORING_DATABASE_PASSWORD=$(cat /run/secrets/db_password)
   export MONITORING_AUTH_SESSION_SECRET=$(openssl rand -base64 32)
   # Tenant secrets, referenced from config.yaml as client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"
   export PROD_KEYCLOAK_CLIENT_SECRET=$(cat /run/secrets/keycloak_client_secret)
   ```

3. **Generate strong session secrets**

   ```bash
   openssl rand -base64 32
   ```

4. **Enable SSL/TLS in production**

   ```yaml
   database:
     ssl_mode: "require"
   keycloak:
     connection:
       skip_tls_verify: false
   auth:
     session:
       secure: true  # the default
   http:
     security_headers:
       hsts: "max-age=31536000"  # the default
   ```

### Performance

1. **Adjust connection pool sizes based on load**

   ```yaml
   database:
     max_conns: 100  # For high-traffic environments
     min_conns: 20
   ```

2. **Tune polling intervals**

   ```yaml
   keycloak:
     polling:
       metrics_interval: 10m  # Less frequent for large deployments
       events_interval: 1m    # Balance between real-time and load
   ```

3. **Limit event collection**

   ```yaml
   keycloak:
     events:
       max_events_per_poll: 500  # Prevent overwhelming database
       types:  # Only collect specific events
         - "LOGIN"
         - "LOGIN_ERROR"
   ```

### Monitoring

1. **Use structured logging in production**

   ```yaml
   logging:
     level: "info"
     format: "json"  # For log aggregation tools
   ```

2. **Enable debug logging for troubleshooting**

   ```yaml
   logging:
     level: "debug"
     format: "console"
     output:
       pretty: true
   ```

### High Availability

1. **Use connection pooling**

   ```yaml
   database:
     max_conns: 50
     min_conns: 10
   ```

2. **Enable health checks**

   ```yaml
   keycloak:
     polling:
       health_interval: 1m
   ```

3. **Configure retry logic**

   ```yaml
   keycloak:
     connection:
       max_retries: 5
       retry_backoff: 10s
   ```
