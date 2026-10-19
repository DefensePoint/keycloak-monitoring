# Local setup

## Self-contained demo

Use Docker Desktop (or Docker Engine) and Docker Compose 2.24.4 or newer. The
bundled demo starts two Keycloaks, configures their monitoring clients, and
connects both tenants without manual Keycloak administration.

From a clean checkout:

```bash
cp .env.example .env
# Edit .env: set MONITORING_AUTH_SIMPLE_DEFAULT_PASS and all five BUNDLED_* secrets.
# Generate independent bundled secrets with: openssl rand -hex 24
make docker-demo-up
```

The dashboard password must satisfy the requirements in `.env.example`. The
example dashboard user is `operator`. Values containing `$` must be single
quoted in `.env`. Keep this file private and retain it between restarts.

No root `config.yaml` is needed for the demo. It mounts
`deployments/local/config.bundled.yaml`, whose secrets come from `.env`, and uses
a separate Compose project, `kmt-demo`, with its own containers and volumes.
The ordinary `make docker-up` stack keeps using your existing `config.yaml`.

| Component | Default address |
| --- | --- |
| Dashboard | http://localhost:17880 |
| API and Swagger | http://localhost:17888 |
| Keycloak 1 | http://localhost:18080 |
| Keycloak 2 | http://localhost:18081 |
| PostgreSQL | localhost:15433 |

Log in to the dashboard with `MONITORING_AUTH_SIMPLE_DEFAULT_USER` and `_PASS`
from `.env`. Open Tenants and confirm `Bundled Keycloak 1` and `Bundled Keycloak 2`
are healthy. Select each tenant and check its `master` and `demo` realms. Event
logging is enabled, so monitoring client authentications also produce events.
The Keycloak consoles use `admin` and `BUNDLED_KC_ADMIN_PASSWORD` by default.

The setup container waits for Keycloak authentication to become available,
creates or updates each confidential `monitoring-service` client, assigns its
service account the master `admin` role, creates a `demo` realm, and verifies
realm/user/client/event reads and server information access. The broad `admin`
role is the existing deployment choice described in [Keycloak setup](keycloak-setup.md);
this disposable local demo is not a production deployment template.

Setup can run again against retained data without duplicating clients or realms.
It repairs client settings and reconciles changed client secrets. The Keycloak
bootstrap admin and PostgreSQL passwords are initial credentials: editing them
in `.env` does not rotate the passwords already stored in retained databases.
Keep them unchanged, change them explicitly in the services, or reset the demo.

```bash
make docker-demo-logs
make docker-demo-restart   # retains data; reruns setup before the API starts
make docker-demo-down      # retains data
make docker-demo-up        # reapplies setup and starts monitoring
make docker-demo-clean     # deletes only the demo volumes
make docker-demo-up        # clean initialization using the retained .env
```

The demo defaults avoid ports 8080/8081 and the ordinary platform's ports. Set
`BUNDLED_KC1_PORT`, `BUNDLED_KC2_PORT`, `BUNDLED_API_PORT`, `BUNDLED_WEB_PORT`,
or `BUNDLED_DB_PORT` in `.env` if those demo ports are occupied.
All demo ports bind only to localhost. For a second demo project use
`make docker-demo-up DOCKER_DEMO_PROJECT=kmt-demo-other` with different ports;
use that same project value for its down, restart and clean commands.

For direct Compose use, always include both files and the profile:

```bash
docker compose --env-file .env -p kmt-demo \
  -f deployments/local/docker-compose.yml \
  -f deployments/local/docker-compose.bundled.yml --profile bundled-kc up -d --build
```

The `bundled-kc` profile by itself starts bare Keycloaks. The override supplies
the working demo configuration, setup service, modern bootstrap variables, and
separate ports. If setup fails, `make docker-demo-up` and `make docker-demo-restart`
print its log, which names the Keycloak, the request and the error; the API will
not start until setup succeeds.

`python3 -m unittest discover -s deployments/local/tests` runs
`setup-keycloak.py` against a fake Keycloak. These tests do not start the demo;
only `make docker-demo-up` shows that it starts.

## Monitor an existing Keycloak

```bash
cp .env.example .env
cp config.yaml.example config.yaml
# Set the dashboard password in .env and your tenant/client configuration in config.yaml.
make docker-build
make docker-up
```

Create a confidential monitoring client first, following [Keycloak setup](keycloak-setup.md).
For a Keycloak running on this host, use `host.docker.internal` in the tenant URL.
`keycloak.global.connection.allow_private_ranges` is on by default; keep it on for
this local connection. The normal stack uses dashboard port 7880, API port 7888,
and PostgreSQL port 5433.

### Verify

```bash
docker ps --filter name=kmt
# expect: kmt-postgres (healthy), kmt-server, kmt-web

curl -s -o /dev/null -w "%{http_code}\n" http://localhost:7888/health   # 200
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:7880/          # 200

docker logs kmt-server | grep -E "Keycloak client initialized|Health check completed" | head -3
```

### Reset

```bash
make docker-clean   # stops the stack and deletes its database
make docker-up      # starts fresh; the dashboard admin is re-created from .env
```

Use `make docker-down` to stop the stack and keep its data. These commands manage
the ordinary stack; the demo targets manage the demo project separately.
