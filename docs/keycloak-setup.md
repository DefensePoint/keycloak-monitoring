# Keycloak Setup Guide

This guide explains how to configure Keycloak for monitoring with the Keycloak Monitoring Tool.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Quick Setup](#quick-setup)
- [Detailed Setup](#detailed-setup)
  - [Method 1: Admin CLI / password grant — REMOVED](#method-1-admin-cli--password-grant--removed)
  - [Method: Dedicated Client (required)](#method-dedicated-client-required)
- [Enabling Event Logging](#enabling-event-logging)
- [OAuth2 Integration](#oauth2-integration)
- [Testing the Connection](#testing-the-connection)
- [Troubleshooting](#troubleshooting)
- [Security Best Practices](#security-best-practices)

## Overview

The Keycloak Monitoring Tool monitors Keycloak by:

1. Connecting to the Keycloak Admin API
2. Collecting metrics (users, clients, sessions)
3. Fetching events (logins, logouts, errors)
4. Storing data in PostgreSQL/TimescaleDB

The platform supports both Keycloak legacy versions (< 17 with `/auth` prefix) and modern versions (17+).

## Prerequisites

- Keycloak instance (version 11+ recommended)
- Admin credentials for Keycloak
- Network connectivity from the monitoring platform to Keycloak
- Keycloak event logging enabled (for event collection)

## Quick Setup

The platform authenticates to the Keycloak Admin API using the OAuth2
**client_credentials** grant with a dedicated confidential client. The older
admin username/password (password grant) method is **no longer supported** — see
[Detailed Setup](#detailed-setup) for why and how to create the client.

> **⚠️ Migration (breaking change).** Switching from the password grant to
> `client_credentials` is a breaking change. Any tenant without a confidential
> `client_id` + `client_secret` will fail validation at client creation and stop
> being monitored. Before deploying, create the dedicated client and reconfigure
> **every** tenant. Sequence: **create the client → set config → deploy.**

### 1. Configure in config.yaml

```yaml
keycloak:
  server_url: "https://your-keycloak.example.com"
  admin_realm: "master"
  client_id: "monitoring-service"      # dedicated confidential client (create it first)
  client_secret: "your-client-secret"  # from the client's Credentials tab
  realms:
    - "master"
    - "your-realm"
```

### 2. Enable Event Logging in Keycloak

1. Log in to Keycloak Admin Console
2. Select your realm
3. Go to **Events** → **Config**
4. Enable:
   - **Save Events**: ON
   - **Save Admin Events**: ON (optional)
5. Set **Expiration**: 30 days (or your preference)
6. Click **Save**

### 3. Start the Monitoring Platform

```bash
make run

# Or with Docker:
make docker-up
```

The platform will automatically:

- Detect Keycloak version (with or without /auth)
- Authenticate using admin credentials
- Start monitoring configured realms

## Detailed Setup

### Method 1: Admin CLI / password grant — REMOVED

> **No longer supported.** The platform previously authenticated with the
> `admin-cli` client using a username/password (password) grant. That path has
> been removed: it logged the monitoring service in as a *human user* on every
> token refresh (creating `LOGIN` events ~2×/min and coupling monitoring to a
> live admin user), and `admin-cli` is a public client that cannot perform the
> `client_credentials` grant. Use the dedicated client below instead.

### Method: Dedicated Client (required)

The platform authenticates as a dedicated **confidential** client via the
`client_credentials` grant. Its service account — not a human user — carries the
permissions, and auth events are `CLIENT_LOGIN` rather than `LOGIN`.

#### Step 1: Create a Client in Keycloak

1. Log in to Keycloak Admin Console
2. Select the **master** realm (or your admin realm)
3. Go to **Clients** → **Create**
4. Configure the client:

   ```bash
   Client ID: monitoring-service
   Client Protocol: openid-connect
   ```

5. Click **Save**

#### Step 2: Configure Client Settings

**Settings tab**:

```bash
Client authentication: ON        # confidential
Standard Flow Enabled: OFF
Direct Access Grants Enabled: OFF # password grant is not used
Service Accounts Enabled: ON      # enables the client_credentials grant
```

**Save** the configuration.

#### Step 3: Get Client Secret

1. Go to the **Credentials** tab
2. Copy the **Secret** value
3. Save it securely (you'll need it for configuration)

#### Step 4: Assign Service Account Roles

The service account has no permissions by default — without roles the token is
issued but every Admin API call returns 403. The client is created in the
**admin realm (`master`)** and one token is used across all monitored realms, so
it needs cross-realm read access.

**Two options:**

- **`admin` realm role (current deployment choice).** On the **Service Account
  Roles** tab assign the master **`admin`** realm role. It is a composite that
  **auto-extends to newly created realms**, so no per-realm maintenance is
  needed as realms are added. Trade-off: it is full read **and write**
  super-admin across all realms — broader than the monitor's read-only needs,
  and a larger blast radius if the secret leaks. This is a deliberate
  operability-over-least-privilege decision.

- **Scoped read roles (least privilege).** Instead of `admin`, assign — for each
  monitored realm, from that realm's `<realm>-realm` client — the read roles:
  `view-realm`, `view-users`, `view-clients`, `view-events`, `query-users`,
  `query-clients`. Smaller blast radius, but each new realm must be granted
  these roles (manually or via automation) or it goes unmonitored.

#### Step 5: Set the Access Token Lifespan

On the client's **Advanced** tab, set **Access Token Lifespan** to **3 minutes**
(override of the realm default).

The platform caches each token and refreshes it 30 seconds before expiry, so the
token lifespan directly controls auth frequency. The default ~60s tokens cause a
`CLIENT_LOGIN` roughly every ~30s (~120/hour); a 3-minute lifespan cuts that to
roughly one every ~2.5 min (~24/hour) — far less event noise and far fewer
token-endpoint requests, with no loss of monitoring fidelity. Keep the lifespan
well above the 30s refresh buffer — never set it at or below ~30s, or the client
would re-authenticate on every request.

#### Step 6: Configure Monitoring Platform

```yaml
keycloak:
  server_url: "https://keycloak.example.com"
  admin_realm: "master"
  client_id: "monitoring-service"
  client_secret: "your-client-secret-from-step-3"
  realms:
    - "master"
    - "your-realm"
```

> Use an environment variable / secret manager for `client_secret` in real
> deployments rather than committing it to the config file.

## Enabling Event Logging

Event logging must be enabled in Keycloak for the platform to collect authentication events.

### Enable for a Specific Realm

1. **Log in to Keycloak Admin Console**

2. **Select your realm** (e.g., "master" or "myrealm")

3. **Go to Events → Config**

4. **Configure User Events**:

   ```bash
   Save Events: ON
   Expiration: 30 days  (adjust as needed)

   Event Listeners:
     - jboss-logging
     - email (optional)
   ```

5. **Select Event Types** (or leave all):
   - LOGIN
   - LOGIN_ERROR
   - LOGOUT
   - REGISTER
   - REGISTER_ERROR
   - CODE_TO_TOKEN
   - REFRESH_TOKEN
   - CLIENT_LOGIN
   - And more...

6. **Configure Admin Events** (optional):

   ```bash
   Save Admin Events: ON
   Include Representation: ON (optional, includes full data)
   ```

7. **Click Save**

### Verify Event Logging

1. Go to **Events** → **Login Events**
2. Perform a test login
3. Verify the event appears in the list

### Event Configuration in Monitoring Platform

```yaml
keycloak:
  events:
    # Specific event types to collect (empty = all)
    types:
      - "LOGIN"
      - "LOGIN_ERROR"
      - "LOGOUT"
      - "REGISTER"
      - "CODE_TO_TOKEN"

    # Maximum events per poll (prevents overload)
    max_events_per_poll: 1000

    # How far back to look for events
    lookback_duration: 1m

  polling:
    # How often to fetch events
    events_interval: 30s
```

---

## OAuth2 Integration

You can also use Keycloak as an SSO provider for the monitoring platform itself.

### Step 1: Create OAuth2 Client in Keycloak

1. **Select your realm** (e.g., "production")
2. **Go to Clients → Create**
3. **Configure client**:

   ```bash
   Client ID: monitoring-dashboard
   Client Protocol: openid-connect
   Root URL: https://monitoring.example.com
   ```

### Step 2: Configure Client Settings

**Settings tab**:

```bash
Access Type: confidential
Standard Flow Enabled: ON
Direct Access Grants Enabled: OFF
Valid Redirect URIs: https://monitoring.example.com/auth/callback
Web Origins: https://monitoring.example.com
```

**Save** the configuration.

### Step 3: Get Client Secret

1. Go to **Credentials** tab
2. Copy the **Secret**

### Step 4: Configure Monitoring Platform

```yaml
auth:
  simple:
    enabled: false  # Disable simple auth

  oauth2:
    enabled: true
    provider_url: "https://keycloak.example.com/realms/production"
    client_id: "monitoring-dashboard"
    client_secret: "your-oauth2-client-secret"
    redirect_url: "https://monitoring.example.com/auth/callback"
    scopes:
      - "openid"
      - "profile"
      - "email"
```

### Step 5: Create Users

1. In Keycloak, go to **Users** → **Add user**
2. Create users who should access the monitoring dashboard
3. Set passwords and enable accounts

Now users can log in to the monitoring dashboard using their Keycloak credentials.

## Testing the Connection

### Manual Test with curl

Test authentication to Keycloak Admin API:

```bash
# Get access token (client_credentials grant)
curl -X POST "https://keycloak.example.com/realms/master/protocol/openid-connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=monitoring-service" \
  -d "client_secret=your-client-secret"

# You should receive a JSON response with an access_token.
# Then confirm the service account can reach the Admin API (expect HTTP 200):
#   curl -H "Authorization: Bearer <token>" \
#     "https://keycloak.example.com/admin/realms/master/users/count"
```

### Test with the Monitoring Platform

```bash
# Start the server with debug logging
export MONITORING_LOGGING_LEVEL=debug
make run

# Watch the logs for Keycloak connection messages
# Should see:
# - "Keycloak client initialized successfully"
# - "Auto-detected Keycloak version ..."
# - "Successfully authenticated with Keycloak"
```

### Check the Dashboard

1. Access the web UI: `http://localhost:7880`
2. Log in
3. Navigate to the Keycloak page
4. Verify data is appearing:
   - Realm statistics
   - User counts
   - Client counts
   - Events list

## Troubleshooting

### Connection Refused

**Symptom**: `connection refused` error in logs

**Solutions**:

1. Verify Keycloak is running and accessible
2. Check firewall rules
3. Verify the URL is correct
4. Test with curl:

   ```bash
   curl https://keycloak.example.com
   ```

### Authentication Failed

**Symptom**: `authentication failed` error

**Solutions**:

1. Verify admin username and password
2. Check if the user account is enabled
3. For service accounts, verify:
   - Client secret is correct
   - Service account is enabled
   - Correct roles are assigned

### TLS Certificate Errors

**Symptom**: `x509: certificate signed by unknown authority`

**Solutions**:

1. Add CA certificate to system trust store
2. For development only:

   ```yaml
   keycloak:
     connection:
       skip_tls_verify: true  # NOT for production
   ```

### No Events Appearing

**Symptom**: Events endpoint returns empty array

**Solutions**:

1. Verify event logging is enabled in Keycloak
2. Check realm configuration: Events → Config → Save Events
3. Perform a test login to generate events
4. Check event types filter in config:

   ```yaml
   keycloak:
     events:
       types: []  # Empty = collect all
   ```

### Version Detection Failed

**Symptom**: `failed to authenticate with Keycloak (tried with and without /auth)`

**Solutions**:

1. Manually specify the correct URL:
   - Keycloak < 17: `https://keycloak.example.com/auth`
   - Keycloak 17+: `https://keycloak.example.com`
2. Check network connectivity
3. Verify credentials

### Rate Limiting

**Symptom**: HTTP 429 errors

**Solutions**:

1. Reduce polling frequency:

   ```yaml
   keycloak:
     polling:
       metrics_interval: 10m
       events_interval: 2m
   ```

2. Increase rate limits in Keycloak
3. Use multiple monitoring instances with load distribution

## Security Best Practices

### 1. Use Dedicated Service Accounts

Don't use the main admin account for monitoring.

```yaml
# Good: Dedicated client with limited permissions
keycloak:
  client_id: "monitoring-service"
  client_secret: "service-account-secret"

# Bad: using the built-in admin account — no client_credentials support
# (admin_username / admin_password are no longer accepted)
```

### 2. Use Environment Variables for Secrets

```bash
# Don't store the client secret in config files
export MONITORING_KEYCLOAK_CLIENT_SECRET=$(cat /run/secrets/keycloak_secret)
```

Separately from how the secret reaches the app, `client_secret` can also be encrypted at rest in the database — see [Secrets Encryption](configuration.md#secrets-encryption) in the configuration guide.

### 3. Enable TLS Verification

```yaml
keycloak:
  connection:
    skip_tls_verify: false  # Always verify certificates
```

### 4. Limit Monitored Realms

Only monitor realms you need:

```yaml
keycloak:
  realms:
    - "production"  # Don't include test or development realms
```

### 5. Use Minimal Permissions

The monitor only reads, so the least-privilege grant for a service account is:

- `view-realm` - View realm information
- `view-users` - View user data
- `view-clients` - View clients
- `view-events` - View events

Avoid (not needed for monitoring): `manage-*` roles.

> **Deployment note:** the current deployment grants the service account the
> master `admin` role instead, as a deliberate operability choice — `admin`
> auto-extends to newly created realms, avoiding per-realm role maintenance.
> The trade-off is full read/write super-admin (larger leak blast radius). The
> scoped read roles above are the least-privilege alternative; see
> [Step 4](#step-4-assign-service-account-roles).

### 6. Rotate Credentials Regularly

```bash
# Generate new client secret in Keycloak
# Update configuration
export MONITORING_KEYCLOAK_CLIENT_SECRET=new-secret

# Restart service
make docker-restart
```

### 7. Monitor Access Logs

Regularly check Keycloak logs for:

- Unusual API access patterns
- Failed authentication attempts
- Unexpected data access

### 8. Use Network Segmentation

- Place monitoring platform in a secure network
- Restrict access to Keycloak Admin API
- Use VPN or private networks when possible

## Multi-Realm Monitoring

To monitor multiple realms:

```yaml
keycloak:
  realms:
    - "master"
    - "production"
    - "staging"
    - "development"
```

Or monitor all realms (leave empty):

```yaml
keycloak:
  realms: []  # Will auto-discover and monitor all realms
```

## Advanced Configuration

### Custom Event Types

```yaml
keycloak:
  events:
    types:
      - "LOGIN"
      - "LOGIN_ERROR"
      - "LOGOUT"
      - "CODE_TO_TOKEN"
      - "REFRESH_TOKEN"
      - "REGISTER"
      - "UPDATE_PASSWORD"
      - "UPDATE_PROFILE"
```

### Polling Optimization

```yaml
keycloak:
  polling:
    # Event collection. For high-volume tenants
    # use a slower interval — see
    # configuration.md, "Recommended polling for large / federated tenants".
    events_interval: 1m

    # Less frequent metrics for resource efficiency
    metrics_interval: 10m

    # Regular health checks
    health_interval: 2m

    # Infrequent realm info updates
    realm_info_interval: 30m
```

### Connection Tuning

```yaml
keycloak:
  connection:
    # Timeout for slow networks
    timeout: 60s

    # More aggressive retries
    max_retries: 5
    retry_backoff: 10s
```

 d
