-- Read-only Postgres role for the MCP server (cmd/mcp-server).
--
-- Run manually with psql, once per environment, after cmd/server has migrated
-- the schema (cmd/server owns migrations; the MCP server never migrates):
--
--   psql -U "${POSTGRES_USER}" -d monitoring \
--     -v mcp_password='<strong-password>' \
--     -f mcp-readonly-role.sql
--
-- Then point the mcp.database block of the deployed config.yaml at this role.
--
-- Grants are table-level, not column-level. The MCP server reads rows through
-- GORM, which selects every model column, so a column-level grant withholding
-- keycloak_tenants.client_secret or users.password_hash would make Postgres
-- reject those queries outright. Both columns therefore stay readable by this
-- role: client_secret is protected by application-level encryption only when
-- security.encryption_key is configured (plaintext in the database otherwise),
-- password_hash is a bcrypt hash, and the MCP server never returns either
-- field to clients.
--
-- The data tables below cover every shipped tool; the remaining tables are the
-- authentication and RBAC tables the server needs to resolve a token to a user
-- and that user to an effective scope.
--
-- No ALTER DEFAULT PRIVILEGES on purpose: a table added by a future migration
-- must not become readable implicitly, since it may hold data the MCP server
-- has no business seeing. When the MCP server needs a table beyond this set,
-- add it to the GRANT list below and run just that GRANT per environment (the
-- script is not idempotent; CREATE ROLE fails on an existing role).

\set ON_ERROR_STOP on

BEGIN;

CREATE ROLE monitoring_readonly WITH LOGIN PASSWORD :'mcp_password';

GRANT CONNECT ON DATABASE monitoring TO monitoring_readonly;
GRANT USAGE ON SCHEMA public TO monitoring_readonly;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

GRANT SELECT ON
    events,
    keycloak_realms,
    keycloak_tenants,
    configuration_alerts,
    keycloak_health,
    keycloak_events,
    keycloak_metrics,
    users,
    api_tokens,
    roles,
    permissions,
    role_permissions,
    user_roles,
    tenant_policies
TO monitoring_readonly;

-- Deliberately not granted: sessions, notification_logs, operator_actions,
-- alert_rules, pending_tenant_purges, amfa_mirror_watermarks.

COMMIT;
