# Local Setup

How this machine is configured to run Keycloak Monitoring Tool against the
existing `keycloak-deployment` stack (RHBK 26.4 on host `:8080`).

## Prerequisites

- Docker + Docker Compose
- The `keycloak-deployment` stack running on the host (`keycloak-deployment-keycloak-1` published on `:8080`, admin/admin, prefix `/auth`)

## What's pre-configured

- `**.env**` holds the dashboard admin bootstrap credentials.
- `**config.yaml**` has one tenant `local-kc` pointing at the host's Keycloak via `host.docker.internal:8080/auth`.
- `**deployments/local/docker-compose.yml**` has two local deviations from the committed defaults:
  - `kmt-postgres` publishes `5433:5432` (avoids clash with `keycloak-deployment-postgres-1` on `:5432`).
  - The bundled `keycloak-1`, `keycloak-2`, and their DBs are under the `bundled-kc` Compose profile, so they are skipped by default.

## Run

```bash
make docker-build      # first time only, or after Go/React changes
make docker-up
make docker-build-web && make docker-down && make docker-up # to build and restart
```

  


## Verify

```bash
docker ps --filter name=kmt
# expect: kmt-postgres (healthy), kmt-server, kmt-web

curl -s -o /dev/null -w "%{http_code}\n" http://localhost:7888/health   # 200
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:7880/          # 200

docker logs kmt-server | grep -E "Keycloak client initialized|Health check completed" | head -3
```

You should see realms `master`, `AdaptiveAuth`, `SSOIdP`, `ExtensionTest` being polled and events being saved.

## Access


| Component | URL                                                      | Notes                                                                                                            |
| --------- | -------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Dashboard | [http://localhost:7880](http://localhost:7880)           | Login `admin` / value of `MONITORING_AUTH_SIMPLE_DEFAULT_PASS` in `.env`. Password change forced on first login. |
| API       | [http://localhost:7888](http://localhost:7888)           | REST + Swagger.                                                                                                  |
| Postgres  | localhost:5433                                           | user `monitoring`, db `monitoring`, password `monitoring_password`.                                              |
| Keycloak  | [http://localhost:8080/auth](http://localhost:8080/auth) | admin/admin, from `keycloak-deployment` (not managed here).                                                      |


## Common commands

```bash
make docker-logs                              # tail all services
docker logs -f kmt-server        # tail just the API server
make docker-down                              # stop the platform
make docker-rebuild                           # build + restart after code changes
make docker-clean                             # stop AND wipe the postgres volume (resets all data)
```

To opt into the bundled Keycloaks for a clean self-contained test (will conflict with the existing stack on `:8080`/`:5432`, so stop that first):

```bash
docker compose -f deployments/local/docker-compose.yml --profile bundled-kc up -d
```

## Reset everything

```bash
make docker-clean     # drops the platform's postgres data
docker compose -f deployments/local/docker-compose.yml down -v
```

Then `make docker-up` to start fresh. The first login will again prompt for an admin password change.

## Notes

- The platform polls Keycloak as `admin`/`admin` (set in `config.yaml` under `keycloak.tenants.local-kc`). This generates LOGIN events on the `master` realm every polling interval. To avoid that noise, create a dedicated service user in Keycloak with `view-realm` / `view-users` / `view-events` and switch the tenant credentials.
- `keycloak.global.connection.allow_private_ranges` must stay `true` for `host.docker.internal` to be accepted by the SSRF guard.
- `.env` and `config.yaml` are gitignored, so local changes won't be committed.

