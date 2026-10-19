# Redeployment Checklist — New Keycloak Monitoring Tool Version

**Breaking change (Authentication switch)**:  
The monitor now authenticates to the Keycloak Admin API with the OAuth2  
`client_credentials` grant (a dedicated confidential client), instead of a user  
admin user/password.  A tenant without a confidential `client_id` +  
`client_secret` fails validation on startup and stops being monitored.  
The Keycloak client must exist **before** the new version is deployed.

---

## A. Keycloak side

In the **master realm**:

1. **Create a confidential client** `monitoring-service`:
  - Client authentication: **ON**
  - Service accounts roles: **ON**
  - Standard flow: **OFF**, Direct access grants: **OFF**
2. **Grant roles to its service account**:

- assign the master `admin` realm
role.

1. **Advanced tab of the client → Access Token Lifespan = 3 minutes to avoid login bursts.**
2. **Credentials tab → copy the client secret**
3. **Ensure event logging is ON** in each monitored realm (Realm settings →
  Events → *Save events* ON), required for event collection.
4. **If the dashboard uses SSO**: the SSO client (e.g. `monitoring-dashboard`,
  see [OAuth2 Integration](keycloak-setup.md#oauth2-integration)) gets **PKCE
  required** only *after* the deploy, in step B.4.5 below. Leave it unchanged
  for now.

## B. Cloud / deployment team

1. **Store the client secret** in the secret manager; expose it as an env var,
  e.g. `MONITORING_KEYCLOAK_TENANTS_<TENANT_ID>_CLIENT_SECRET`
2. **Update  tenant's config:**
  - `client_id: monitoring-service`
  - `client_secret:` ← from the env var / secret manager
3. Set `keycloak.global.polling.events_interval: 1m` in `config.yaml`
4. **Deploy sequencing (must be in this order):**
  1. Keycloak client + roles created
  2. Secret + config in place
  3. Deploy the new version
  4. If the dashboard uses SSO and the new version is KMT 2.0.0 or later: require
     PKCE with method **S256** on the SSO client (newer consoles: Settings →
     Capability config → Require PKCE; older ones: Advanced → Advanced
     settings → Proof Key for Code Exchange Code Challenge Method; see
     [OAuth2 Integration](keycloak-setup.md#oauth2-integration)). Not before
     step 3: KMT 1.0.x sends no PKCE, so its SSO login fails once
     Keycloak requires it.

---

## C. Verify after deploy

Each tenant runs four polls: health, realm info, events, and metrics (users
and sessions). Each poll runs once as soon as the server starts, then on its
own interval, so on a redeploy every value in the table shows within a minute.
What takes longer is a change made in Keycloak after the start: it shows only
at the next poll of its kind, so new user and session counts can take up to
5 min, a deleted realm up to 15 min, and a new realm about 17.5 min (see
below). The times below are the values in `config.yaml.example`, which polls
events every minute (`events_interval`). Without `events_interval` in the
config, events are polled every 30 seconds. The server logs the interval in
use at startup, in the `interval` field of the `Starting health polling
worker`, `Starting realm info polling worker`, `Starting events polling worker`
and `Starting metrics polling worker` lines, written as `1m0s`, `15m0s`, `1m0s`
and `5m0s` for those values. An events line showing `30s` means the config
sets no `events_interval`. These lines carry no tenant name, so there is one of
each per tenant. If you set an events interval in B.3 and no `Starting events
polling worker` line shows it, the setting did not apply.

| Value | First appears | Then refreshed | Where to see it |
| --- | --- | --- | --- |
| Tenant health | within 1 min | every 1 min | Tenant Management page |
| Realm list | within 1 min | every 15 min | Realm selector |
| Events and login counts | within 1 min | every 1 min (30 s without `events_interval`) | Events page; Dashboard **Total Events** and **Failed Logins** |
| Users and sessions | within 1 min on a redeploy; up to 5 min for a newly added tenant | every 5 min | Dashboard **Total Users** and active sessions |
| Event-check alerts, if enabled | first check at start | every 5 min | Alerts page |

- After the deploy, new events are fetched at once, and the day so far is
  backfilled from the start of the day `lookback_duration` ago, 20 pages of
  `max_events_per_poll` per poll, so earlier events also load. With
  `lookback_duration: 1m` from `config.yaml.example` that is since the start
  of today, which is less than an hour during the first hour of a day.
  Without `lookback_duration` in the config it is at least the last hour.
  Later polls read only the events after the last one seen.
- A realm created in Keycloak after the deploy is listed within
  (token lifespan - 30 s) + 15 min, about 17.5 min with the 3-minute lifespan
  from section A, and its events follow within one more events interval. Its
  users and sessions show within one more 5 min metrics poll, about 22.5 min.
  A realm deleted in Keycloak drops off within 15 min.
- Event-check alerts run only when `keycloak.global.event_checker.enabled` is
  true (it is off by default), and only for tenants defined in the config
  file. An alert about newly collected events can take one more cycle.
- These times apply to tenants on the global settings. If
  `keycloak.global.realms` lists realms, only those are polled, and a realm
  created later never appears. A tenant given its own `configuration` through
  the tenant API does not use the global settings and runs on built-in values
  instead (for example, its realm list refreshes every 10 min).

1. **Tenant health.** The server starts, and within a minute every enabled
   tenant's card on the Tenant Management page shows **HEALTHY** and no
   **ERROR** chip. Hovering over an **ERROR** chip shows the tenant's last
   error. An error left from before the deploy clears at the first healthy
   check. The page refreshes its tenant list only once a minute, so reload it
   before reading. The card's **Last Check** time moves forward on each reload,
   within a minute. The server log has no `failed to create Keycloak client`
   lines and no 403 responses from Keycloak.
2. **Monitor authentication.** In the master realm's saved events, the
   monitor's client (`monitoring-service`) logs in with `CLIENT_LOGIN`, not
   `LOGIN`. Check in the Keycloak admin console (master realm, Events, User
   events) or on the Events page with realm `master`: event type
   `CLIENT_LOGIN` lists rows whose **Client** is `monitoring-service`, and
   event type `LOGIN` lists none from that client. `LOGIN` rows from other
   clients, such as `admin-cli`, are other sign-ins. Both need Save events ON
   in master. If it is off, record the check as not done and why.
   - For a realm backed by a federated user-storage provider (for example
     LDAP): the Keycloak server log no longer shows a storage-provider WARN for
     each polled event. For a realm without one, record that this does not
     apply.
3. **Data per realm.** With each monitored realm selected, the Dashboard shows
   **Total Users**, active sessions and **Total Events** within the times in
   the table above. Zero can be right: a realm with no users shows 0 users and
   0 sessions. Its card on the Realms page shows **Users** and **Sessions**
   once a metrics poll has stored them; a card without them has no metrics
   yet. Events need Save events ON in that realm (the section A step "Ensure
   event logging is ON"). The Dashboard and Events pages keep the window from
   the moment the range was picked: the Dashboard's **Updated** time keeps
   moving, but the window does not. To see newer events, pick the range again.
   Reloading the Events page keeps the old window.

4. **Time range.** Use one tenant with **All Realms** selected.
   1. Precondition: the last 7 days hold at least one `LOGIN` or
      `LOGIN_ERROR` event older than 5 minutes (Events page, **Event type**
      filter). If not, make one failed login on a monitored realm and wait 6
      minutes. Without such an event the two windows cannot differ, so record
      the check as inconclusive, not passed.
   2. Dashboard: pick **Last 7 days** and note **Total Events**, **Failed
      Logins** with its successful count, **Total Users** and active sessions.
      Pick **Last 5 minutes** and note them again. Total Events drops, and so
      does at least one of the two login counts. Each **Recent Activity** entry
      shows a time within the last 5 minutes, or the card shows "No recent
      events". Total Users and active sessions do not follow the window: they
      are current counts and change only when a metrics poll lands in between.
   3. Events page: pick **Last 7 days** explicitly, because the page starts
      from the range last picked on any page. Note the "N total events" figure
      under the table, then pick **Last 5 minutes**. The total drops, and the
      `start` and `end` values in the address bar change.
   4. Do not compare against **All time**. It shows Total Events for all
      stored events but login counts for the last 24 hours only.
   5. To see newer events, pick the range again. Reloading the Events page
      keeps the old window.

5. **SSO sign-in.** If the dashboard uses SSO, **Sign in with SSO** still works
   after PKCE is required on the client (step B.4.4).

6. **Admin review, once, when upgrading from KMT 1.0.x.** Those versions
   granted the admin role to any SSO user whose email matched
   `auth.oauth2.admin_users`, even when the identity provider had not verified
   the address, so anyone who could register or edit their own email could
   take an address from the list. Newer versions refuse that, but an admin
   role granted before the upgrade stays. On the platform's database, list the
   admins granted this way whose address is still unverified:

   ```sql
   SELECT u.id, u.email, u.email_verified, ur.assigned_at
   FROM users u
   JOIN user_roles ur ON ur.user_id = u.id
   JOIN roles r ON r.id = ur.role_id
   WHERE r.name = 'admin' AND ur.assigned_by = 'system'
     AND u.auth_method = 'oauth' AND NOT u.email_verified;
   ```

   Review each row by hand before acting: a real admin may simply have an
   unverified address. For an account that should not be admin, remove the
   role on the **Users** page; for a real admin, mark the address verified in
   the identity provider.
