# Postgres provisioning for the MCP server

Scripts run manually with psql, once per environment; none is part of any
migration or pipeline.

- `mcp-readonly-role.sql` creates the least-privilege `monitoring_readonly`
  role the MCP server (`cmd/mcp-server`) connects with.
- `events-keyset-index.sql` builds the keyset-pagination index `list_events`
  needs, with `CONCURRENTLY` so an environment that already holds events does
  not block writes for the length of the build.
- `alerts-list-index.sql` builds `idx_alerts_tenant_first_detected_id` on
  `configuration_alerts` for listings ordered by `first_detected DESC, id DESC`,
  with `CONCURRENTLY` for the same reason. Listing stays `LIMIT`/`OFFSET`.
- `alerts-status-index.sql` builds `idx_alerts_tenant_status_first_detected_id`
  on `(tenant_id, status, first_detected DESC, id DESC)` for the Alerts page,
  which defaults to `status=active`, with `CONCURRENTLY` for the same reason.

## Running the scripts

The commands below run `psql` where it can reach the database and
`${POSTGRES_USER}` is set. A Compose server (`deployments/server`) has
neither on the host: Postgres is not published and the variable lives only
in the container. There, run each script through the `postgres` container
from the deploy directory, with the single quotes so the container expands
the variable:

```bash
docker compose exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d monitoring' \
  < /path/to/deployments/postgres/alerts-list-index.sql
```

For the role script, pass the password into the container as well:

```bash
docker compose exec -T -e MCP_PASSWORD='<strong-password>' postgres \
  sh -c 'psql -U "$POSTGRES_USER" -d monitoring -v mcp_password="$MCP_PASSWORD"' \
  < /path/to/deployments/postgres/mcp-readonly-role.sql
```

## Prerequisites

- `security.encryption_key` should be configured before granting SELECT on
  `keycloak_tenants`: without it, `client_secret` is stored in plaintext and
  is readable by the role.
- The `mcp:` block must be present in the deployment's `config.yaml`. The
  shipped default (`config.yaml.example`) keeps the MCP server disabled.

## Ordering

1. On an environment whose `events` table already holds rows, build the keyset
   index before deploying the release that introduces it. `AutoMigrate` would
   otherwise build the same index at startup while holding a SHARE lock on
   `events`, which blocks writes and still allows reads. Pre-creating it
   under the name `AutoMigrate` looks for turns that step into a no-op:

   ```bash
   psql -U "${POSTGRES_USER}" -d monitoring -f events-keyset-index.sql
   ```

   A database that has never been migrated has no `events` table for the
   script to index, so on a new environment this step moves after step 2,
   where `AutoMigrate` has already created the table together with the index
   and the script is left with only its verification query.

   The same applies to `configuration_alerts`. Run `alerts-list-index.sql`
   before the release that adds `idx_alerts_tenant_first_detected_id`, and
   `alerts-status-index.sql` before the release that adds
   `idx_alerts_tenant_status_first_detected_id`, when that table already holds
   rows. A new database gets both indexes from `AutoMigrate` in step 2.

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
