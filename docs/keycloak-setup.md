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

- **Scoped read roles (least privilege).** Instead of `admin`, assign four read
  roles for each monitored realm: `view-realm`, `view-users`, `view-clients` and
  `view-events`. Assign them on the service account from that realm's
  `<realm>-realm` client in the admin realm (`master-realm` for `master`, needed
  only if `master` is monitored). `view-users` already includes `query-users`
  and `query-groups`, and `view-clients` includes `query-clients`, so the query
  roles need no separate grant. Smaller blast radius, but each new realm must be
  granted these roles (manually or via automation) or it goes unmonitored. See
  [what each scoped role powers](#what-each-scoped-role-powers) and
  [when role changes take effect](#when-role-changes-take-effect).

##### What each scoped role powers

Tested on Keycloak 26.0.8 in auto-discovery mode (`realms: []`), removing one
`demo` role at a time from a client that kept the other three and all four on
`master`. The `view-realm` case without another readable realm was tested with
a client granted on `demo` only. The `view-realm` row and the loss of every
role in one realm were retested on Keycloak 26.7.5. Rows marked *from the
code* were not run against Keycloak.

| Role | What it powers | When it is missing | Loud or silent |
|---|---|---|---|
| `view-realm` | Realm list and realm details, session counts, AMFA detection | Keycloak lists the realm by name only, with no ID and none of its settings, and answers the same when asked for the realm. The platform keeps the realm's last stored row as it was and does not retire it, so the realm stays in the realm list and on the dashboard with its last known settings. The Realms page marks it "Can't read" with the time it was last read, its detail page says so in a banner, and the dashboard no longer counts it as healthy. Metrics and events are still collected, but session counts are stored as 0. Granting `view-realm` again updates the row on the next realm list refresh. With an explicit `realms` list the realm is handled the same way (*from the code*). AMFA detection reports the realm as not using AMFA (*from the code*). | The warning `Monitoring client cannot view this realm's settings; keeping its stored realm information as it was`, once per realm, with the tenant ID and realm, and the info line `Monitoring client can view this realm's settings again` once the role is back. `Failed to get sessions count` on every metrics poll. Health stays UP |
| `view-users` | User counts, the Users panel on the realm page, user details, usernames on AMFA events (*from the code*) | `Failed to get users count` is logged on every metrics poll and no metrics are stored for the realm, so its client and session counts stop updating too, and the dashboard keeps showing the last stored values. The users API returns 500, but the page shows no error: the Users panel shows "No users found" next to the last stored user total, and the user details page shows "User not found or unable to load user details". With only `query-users`, user counts are stored as 0, the Users panel shows "No users found" with a total of 0, and nothing is logged by the polls. The user details page shows the same message. | Loud in the logs only; the page shows no error. Silent with only `query-users` |
| `view-clients` | Client count and the Clients panel on the realm page | Keycloak returns an empty client list instead of an error, because `view-users` already allows listing clients. The platform stores 0 clients and the Clients panel shows "No clients found". `query-clients` alone behaves the same. | Silent |
| `view-events` | Events, event statistics, login and failed-login counts, event alerts (*from the code*) | Every events poll logs `Failed to fetch events from Keycloak API` for the realm and stores no new events. | Loud |
| Any one of the four, in any realm (*from the code*; tested with all four and with none) | Health checks, and the Keycloak version up to Keycloak 26.6 | With no roles at all, health is DOWN with a 403 from `/admin/serverinfo` and a critical `Tenant '<name>' is DOWN` alert is raised. The realm list, metrics and events polls log 403 errors, and no realm is retired. **Test connection** still passes, because it only checks that a token is issued. | Loud |
| The master **`admin`** role, from Keycloak 26.7 | The Keycloak version | Keycloak 26.7 leaves the version out of `/admin/serverinfo` for any client without the master `admin` role; none of the four roles, nor any other read-only master role, brings it back (tested on 26.7.5; 26.6.3 still shows it). The Health page and the dashboard show no version, and the check for a newer Keycloak release is skipped. Events are not affected: when the version is not shown, the platform tells from Keycloak's reply to its first events request whether event times can be read to the millisecond (the log line `Keycloak event time precision` with `from`). | Silent |

A client that loses all four roles in a realm, but keeps roles in another,
no longer sees that realm in Keycloak's realm list, yet Keycloak still answers
200 for it. The platform handles it like a realm without `view-realm`: its row
is kept, the same warning is logged once, and its users count and events polls
log 403 errors. Only a realm Keycloak answers 404 for, one deleted there, is
retired. To stop monitoring a realm, leave it out of an explicit `realms` list
rather than removing the client's roles there.

`master` behaves the same way. Without the `master-realm` roles Keycloak leaves
it out of the realm list. A client that never had them never stores `master`,
so it is simply not monitored. A client that loses them keeps `master` listed
with its last stored row, as above. Health stays UP as long as another realm is
granted. If `master` was the only granted realm, the client has no roles left,
so it behaves as in the last row of the table.
`manage-*` roles are not needed: the four roles above were enough for every
feature.

##### When role changes take effect

Keycloak checks the roles carried in the access token, so a grant or revoke
reaches the platform with its next token. In testing, a token issued before a
revoke kept its access to the revoked endpoints, and a realm list refresh made
after a grant but still on the old token did not show the granted realm. The
platform keeps a token until 30 seconds before it expires, so a change can take
up to the token lifespan minus 30 seconds to apply: about 2.5 minutes with the
3-minute lifespan from Step 5. The same delay applies to the `admin` role: a
realm created after the current token was issued stays invisible to the
platform until the next token (*from the code*).

After that, the platform refreshes the realm list every 15 minutes
(`realm_info_interval`), metrics every 5 minutes (`metrics_interval`) and events
every minute (`events_interval`), as set in `config.yaml.example`. Without
`events_interval` in the config, events are polled every 30 seconds. A newly
created or newly granted realm therefore appears within
(token lifespan - 30 s) + `realm_info_interval`, about 17.5 minutes with these
intervals and the Step 5 lifespan, and its first metrics follow within one more
`metrics_interval`.

- **Revoking a realm's roles (auto-discovery):** Keycloak stops listing the
  realm, so the platform treats it as deleted and retires it (log line
  `Retired realms deleted in Keycloak`). Granting the roles again brings it back
  on the next realm list refresh. If it was the only granted realm, the client
  has no roles left: nothing is retired and health goes DOWN, as in the last row
  of the table.
- **Revoking with an explicit `realms` list:** nothing is retired; the metrics
  and events polls log a 403 for that realm on every cycle (*from the code*).
- **Restoring `view-events`:** a failed events poll does not move the realm's
  position, so the next one that succeeds continues from the last event it
  saw and the events in between are backfilled. After a restart the day is
  backfilled from the start of the day `lookback_duration` ago.

Measured with short test intervals (`realm_info_interval` 30 s,
`metrics_interval` 15 s, `events_interval` 10 s):

- A new realm with no roles granted did not appear for the scoped client
  (60-second token) in more than 4 minutes, and nothing was logged about it. A
  client with the `admin` role (3-minute token) listed it 41 seconds after it
  was created.
- After the four roles were granted, the scoped client listed the realm 32
  seconds later and had its metrics within 52 seconds. Its events from earlier
  the same day were collected on the first events poll.

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

    # After a start, the day so far is backfilled from the start of the day
    # this long ago, beside new events; polls read only events not yet seen
    lookback_duration: 1m

    # Keycloak's transaction-default-timeout (5m by default); raise it with
    # Keycloak's so events committed late are still collected
    late_commit_window: 5m

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

**Require PKCE (S256)**. The admin console moved this setting between Keycloak
releases; use whichever of the two your console shows. Both store the client
attribute `pkce.code.challenge.method` = `S256`:

- Newer consoles (checked on 26.6.3): **Settings** tab → **Capability config** →
  **Require PKCE** On, then **PKCE Method** S256.
- Older consoles (checked on 26.0.8): **Advanced** tab → **Advanced settings** →
  **Proof Key for Code Exchange Code Challenge Method** S256.

KMT 2.0.0 and later send a PKCE challenge (S256) and a nonce with every SSO
login, and redeem the code with the matching verifier, so a stolen or injected
authorization code is useless. Requiring S256 on the client makes Keycloak refuse
any login without it. Set it only once KMT 2.0.0 or later runs in that
environment: KMT 1.0.x sends no PKCE, and its SSO login fails against a
client that requires it.

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
4. For each address in `admin_users`, turn on **Email verified** (or let the
   user complete Keycloak's email verification). The platform grants the admin
   role only to a verified address, since anyone who can register or edit their
   own profile could otherwise enter an address from the list.

Now users can log in to the monitoring dashboard using their Keycloak credentials.

### Monitoring SSO Availability

The `pmp_sso_provider_up` gauge reports whether SSO login can work:

| Value | Meaning |
|---|---|
| `1` | The platform reached the identity provider and loaded its OIDC discovery document. |
| `0` | SSO is enabled but every SSO login fails. Either the identity provider has not answered discovery yet, and the platform keeps retrying, or `provider_url`, `client_id`, `client_secret` or `redirect_url` is missing or invalid, and logins answer "OAuth2 service not configured" until the configuration is fixed and the server restarted. |
| absent | SSO is disabled (`auth.oauth2.enabled: false`). |

The gauge covers the startup case, not every SSO failure. It stays `1` once the platform has connected, even if Keycloak goes down afterwards or the client secret is rotated in Keycloak; logins then fail while the gauge still reads 1.

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
# Then check each Step 4 role against a monitored realm (replace <realm>).
# Request a new token after any role change: a token keeps the roles it was
# issued with.
#   curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer <token>" \
#     "https://keycloak.example.com/admin/realms/<realm>/users/count"
# Expect HTTP 200 from each of these paths:
#   /admin/realms/<realm>/client-session-stats   (view-realm)
#   /admin/realms/<realm>/users/count            (view-users)
#   /admin/realms/<realm>/events?max=1           (view-events)
# /admin/realms/<realm>/clients?max=1 (view-clients) returns 200 even without
# the role, but with an empty list ([]), so check that the list is not empty.
# users/count also returns 200 with only query-users, but as 0, so in a realm
# that has users, check that the count is not 0.
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
   - The roles from [Step 4](#step-4-assign-service-account-roles) are
     assigned. A missing role does not stop the token; it shows up as 403
     errors or silent zeros, listed in
     [what each scoped role powers](#what-each-scoped-role-powers)

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

5. Check that the service account has `view-events` for the realm (or the
   `admin` role). Without it, every events poll logs
   `Failed to fetch events from Keycloak API` for that realm. See
   [what each scoped role powers](#what-each-scoped-role-powers).

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
# Don't store the client secret in config files: reference a variable
# from the tenant block instead, as client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"
export PROD_KEYCLOAK_CLIENT_SECRET=$(cat /run/secrets/keycloak_secret)
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

The monitor only reads, so the least-privilege grant for a service account is
four roles per monitored realm, from that realm's `<realm>-realm` client in the
admin realm:

- `view-realm` - Realm list and details, session counts, AMFA detection
- `view-users` - User counts, users and user details
- `view-clients` - Client counts and clients
- `view-events` - Events and login statistics

Not needed: `query-users`, `query-groups` and `query-clients`, which are already
part of `view-users` and `view-clients` (granted instead of them, they give zero
counts and empty lists), `query-realms`, and the `manage-*` roles. Tested on Keycloak 26.0.8;
[Step 4](#step-4-assign-service-account-roles) lists what breaks when each role
is missing.

> **Deployment note:** the current deployment grants the service account the
> master `admin` role instead, as a deliberate operability choice — `admin`
> auto-extends to newly created realms, avoiding per-realm role maintenance.
> The trade-off is full read/write super-admin (larger leak blast radius). The
> scoped read roles above are the least-privilege alternative; see
> [Step 4](#step-4-assign-service-account-roles).

### 6. Rotate Credentials Regularly

1. Generate a new client secret in Keycloak.
2. Set the variable that the tenant's `client_secret: "${NAME}"` placeholder references to the new secret, wherever the service reads its environment: `.env` for the local Docker stack, `kmt.env` next to `docker-compose.yml` on a Compose server.
3. Recreate the containers so they read it. A plain restart keeps the old environment:

```bash
docker compose -f deployments/local/docker-compose.yml up -d --force-recreate
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
