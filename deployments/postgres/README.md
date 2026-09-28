# Postgres provisioning for the MCP server

Two scripts, both run manually with psql, once per environment; neither is part
of any migration or pipeline.

- `mcp-readonly-role.sql` creates the least-privilege `monitoring_readonly`
  role the MCP server (`cmd/mcp-server`) connects with.
- `events-keyset-index.sql` builds the keyset-pagination index `list_events`
  needs, with `CONCURRENTLY` so an environment that already holds events does
  not block writes for the length of the build.

## Prerequisites

- `security.encryption_key` should be configured before granting SELECT on
  `keycloak_tenants`: without it, `client_secret` is stored in plaintext and
  is readable by the role.
- The `mcp:` block must be present in the deployment's `config.yaml`. The
  shipped default (`config.yaml.example`) keeps the MCP server disabled.

## Ordering

1. On an environment whose `events` table already holds rows, build the keyset
   index before deploying the release that introduces it. `AutoMigrate` would
   otherwise build the same index at startup while holding an ACCESS EXCLUSIVE
   lock on `events`, blocking every write until it finished; pre-creating it
   under the name `AutoMigrate` looks for turns that step into a no-op:

   ```bash
   psql -U "${POSTGRES_USER}" -d monitoring -f events-keyset-index.sql
   ```

   A database that has never been migrated has no `events` table for the
   script to index, so on a new environment this step moves after step 2,
   where `AutoMigrate` has already created the table together with the index
   and the script is left with only its verification query.

2. Deploy and start `api-server` (`cmd/server`). It runs the schema migrations
   and owns every table. The MCP server never migrates, so the tables the role
   script grants on must exist before it runs.

3. Run the role script against the environment's `monitoring` database,
   connecting as the compose superuser (`${POSTGRES_USER}` from the
   deployment's environment):

   ```bash
   psql -U "${POSTGRES_USER}" -d monitoring \
     -v mcp_password='<strong-password>' \
     -f mcp-readonly-role.sql
   ```

   The script is not idempotent: `CREATE ROLE` fails if `monitoring_readonly`
   already exists. To extend an existing role with a table added later, run
   only the new `GRANT`.

4. Point `mcp.database` in `config.yaml` at the role, supplying the password
   through `MONITORING_MCP_DATABASE_PASSWORD` while keeping an empty
   `password: ""` key in the file, and set `mcp.enabled: true`.

5. Start `mcp-server`. It refuses to start until `mcp.database` names a role,
   and fails to connect until that role exists, so the read-only split cannot
   be lost by omission.

## Limitations

The `mcp-server` container mounts the full `config.yaml`, which includes the
read-write database credentials, so the read-only split holds at the Postgres
role level but not at the filesystem level inside the container. A stripped
MCP-only config file is a possible follow-up.
