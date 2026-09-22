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

## B. Cloud / deployment team

1. **Store the client secret** in the secret manager; expose it as an env var,
  e.g. `MONITORING_KEYCLOAK_TENANTS_<TENANT_ID>_CLIENT_SECRET`
2. **Update  tenant's config:**
  - `client_id: monitoring-service`
  - `client_secret:` ← from the env var / secret manager
3. Set `config.polling.events_interval=1m`
4. **Deploy sequencing (must be in this order):**
  1. Keycloak client + roles created
  2. Secret + config in place
  3. Deploy the new version

---

## C. Verify after deploy

- Server starts; all tenants report **healthy** (no client-creation/403 errors
in the logs).
- Keycloak auth events for the monitor are `**CLIENT_LOGIN`** (not `LOGIN`), and
the per-event `UNKNOWN`-style storage-provider WARNs are gone.
- Dashboard shows non-empty **users / sessions / events** for each realm  
(allow up to one polling interval for first values).

