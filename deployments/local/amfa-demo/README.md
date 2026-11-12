# AMFA demo data

Fills the AMFA parts of KMT's Events page (the KPI row, the geolocation map and
the AMFA rows of the table) with plausible login data from a local AMFA engine
stack, so the feature can be demoed or eyeballed without driving hundreds of
real logins through Keycloak.
The defaults fit the [open-source AMFA engine](https://github.com/DefensePoint/keycloak-adaptive-mfa-engine)'s
Compose stack (`config/keycloak`), with its `test-amfa` realm.

```bash
export CURL_CA_BUNDLE="$(mkcert -CAROOT)/rootCA.pem"   # its Keycloak uses an mkcert certificate
RO_PASSWORD='<a password for the kmt_ro role>' ./seed.sh   # create demo users, seed ~730 events
./teardown.sh                                              # remove all of it again
```

## What it creates

| Where | What |
|---|---|
| Keycloak (`test-amfa`) | 15 `demo-*` users |
| AMFA Postgres | `kmt_ro` role, ~730 events over 30 days |

The dataset spans 14 cities with real coordinates, weighted so the map shows
genuine hotspots. Its IP addresses come only from the ranges reserved for
documentation (RFC 5737), so no real host appears as a login source. Risk
levels run 1 to 4 (roughly 55/25/15/5), about 12% of logins are VPN-flagged,
and two anomalies are planted deliberately:

- **Impossible travel**: `demo-alice`, Berlin then Singapore twelve minutes later
- **Deny burst**: `demo-mallory`, six risk-4 rejections from an Icelandic VPN exit,
  a country that appears nowhere else in the data

Roughly one ordinary login in seven carries a blank `city_name` on purpose. Real
AMFA emits the key but leaves it empty wherever GeoIP resolves a country and no
city, so a city on every row would flatter production. KMT reads it through
`NULLIF(...,'')`, which turns blank into absent, hiding the City field and
falling the map caption back to the country alone. Both planted anomalies always
keep their city. The summary's `with_city` count is how many rows take the
labelled path.

Re-running `seed.sh` replaces the data rather than doubling it. The generator is
seeded with a fixed value, so repeat runs produce identical output.

## Showing it in KMT

KMT shows AMFA events for the realms of a tenant it monitors, so one tenant must
monitor the AMFA stack's Keycloak and carry an `amfa` block reading its
database. `seed.sh` ends by printing that tenant block, filled in for the
database it seeded, and the steps to apply it. Two things to know first:

**KMT builds its AMFA connections once, at startup.** A tenant whose AMFA
database was unreachable at start stays unregistered until the server restarts.
It fails quietly: the page reports "not configured" rather than an error. Check
with:

```bash
docker logs kmt-server 2>&1 | grep "AMFA module ready"
# enabled_tenant_count:0 means look at the warning logged just above it
```

**Both stacks publish Postgres on host port 5433**, so KMT's local stack and
the engine stack cannot run side by side as shipped. Neither the seeder nor a
KMT joined to the engine's network uses the engine's published port, so move it
with an override next to the engine's compose file, which that repository's
`.gitignore` already leaves out. On a running stack, apply it with
`docker compose down` and `docker compose up -d` there, which keeps its data:
recreating only Postgres leaves Keycloak and the engine holding connections to
the old container, and their next requests fail.

```yaml
# keycloak-adaptive-mfa-engine/config/keycloak/docker-compose.override.yml
services:
  postgres:
    ports: !override
      - "5434:5432"
```

**The `amfa` block must be in `config.yaml`**, not only on a tenant created in
the UI: KMT builds the AMFA connections from `keycloak.tenants` in the
configuration.

**The table needs the AMFA mirror.** On the Events page, pick the tenant and the
`test-amfa` realm. The KPI row and the map read the AMFA database directly, so
they show the data as soon as the tenant is registered. The table lists AMFA
logins only once KMT has copied them into its own events table, which
`keycloak.global.amfa_mirror` does. It is off by default, and its first run per
tenant copies only the last `backfill_days` (1 by default), so set both before
the first start that reads this data:

```yaml
keycloak:
  global:
    amfa_mirror:
      enabled: true
      backfill_days: 30
```

**`teardown.sh` does not reach KMT's database.** The mirror never deletes, so
copies it made stay in KMT's events table. Their ids keep the seed's marker, so
this removes exactly them, with the local stack's default database user:

```bash
docker exec -i kmt-postgres psql -U monitoring -d monitoring \
  -c "DELETE FROM events WHERE event_id LIKE 'amfa-de300002-de30-4de3-8de3-%'"
```

## Why `auth_context` stays empty

The seed writes `auth_process.auth_context_json` and leaves the `auth_context`
table alone, which mirrors production: the engine never writes that table, its
`AuthContextService` only computes a hash. Every KMT query therefore COALESCEs
from the table through to the JSON snapshot. Seeding `auth_context` would
exercise a path that never runs for real and would mask regressions in the
fallback that does.

## Files

| File | Purpose |
|---|---|
| `seed.sh` | Driver. Creates users, then runs 01 and 02. |
| `teardown.sh` | Runs 03, then deletes the demo users. `--keep-users` to keep them. |
| `01-create-ro-role.sql` | Creates `kmt_ro` with SELECT-only grants. |
| `02-seed-demo-data.sql` | Generates the dataset. Needs `:demo_users` from `seed.sh`. |
| `03-purge-demo-data.sql` | Deletes only the demo rows. Safe against real history. |

Every seeded row carries a marker in its primary key (`de300001-de30-4de3-8de3-`
and similar), which is what lets the purge be surgical. Real engine rows use
`uuid_generate_v4()` and will not collide.

## Overrides

Both scripts read the same environment variables:

```
AMFA_CONTAINER     amfa-postgres-1
AMFA_DB            adaptive_mfa
AMFA_SUPERUSER     keycloak
KC_URL             https://keycloak:8443
KC_ADMIN           admin
KC_ADMIN_PASSWORD  admin
KC_CLIENT_ID       (empty; set it and KC_CLIENT_SECRET to use a service account instead)
KC_REALM           test-amfa
KMT_CONTAINER      kmt-server
RO_PASSWORD        (seed.sh only, required)
```

The realm matters: it is written into every row as
`auth_context_json.realm_id`, and KMT filters on it exactly.
