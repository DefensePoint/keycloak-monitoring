#!/usr/bin/env bash
#
# Populates KMT's AMFA Events page (table, KPIs, geolocation map) with demo
# data from a local AMFA engine stack.
#
# It does four things, in order:
#   1. creates demo-* users in the target Keycloak realm (or reuses them)
#   2. creates the kmt_ro role KMT connects as
#   3. seeds ~730 login events across 30 days into the AMFA database
#   4. prints the config.yaml block KMT needs, and how to restart it
#
# The users exist so KMT's Keycloak enrichment can resolve each event's
# user_id into a username. Seeding synthetic UUIDs instead leaves the events
# table showing raw UUIDs, because enrichment 404s and falls back to the id.
#
# Everything is idempotent: re-running replaces the demo data rather than
# doubling it, and reuses any demo-* user that already exists.
#
# Undo with teardown.sh.

set -euo pipefail

# Defaults fit the open-source AMFA engine's Compose stack
# (keycloak-adaptive-mfa-engine, config/keycloak).
AMFA_CONTAINER="${AMFA_CONTAINER:-amfa-postgres-1}"
AMFA_DB="${AMFA_DB:-adaptive_mfa}"
AMFA_SUPERUSER="${AMFA_SUPERUSER:-keycloak}"

# Password given to the kmt_ro role. It must match the tenant's
# amfa.database.password in config.yaml, so the same value sets the role and
# appears in the instructions printed at the end. There is no default: a
# password every checkout shares is not one.
RO_PASSWORD="${RO_PASSWORD:-}"

# The engine stack serves Keycloak over HTTPS with an mkcert certificate.
# curl does not always read the store `mkcert -install` writes to, so pass it
# mkcert's CA:  export CURL_CA_BUNDLE="$(mkcert -CAROOT)/rootCA.pem"
KC_URL="${KC_URL:-https://keycloak:8443}"
KC_REALM="${KC_REALM:-test-amfa}"

# Credentials for creating the demo users. Either a password-grant admin
# (KC_ADMIN/KC_ADMIN_PASSWORD) or a service account holding cross-realm admin
# (KC_CLIENT_ID/KC_CLIENT_SECRET), for a Keycloak whose master admin account
# is disabled.
KC_ADMIN="${KC_ADMIN:-admin}"
KC_ADMIN_PASSWORD="${KC_ADMIN_PASSWORD:-admin}"
KC_CLIENT_ID="${KC_CLIENT_ID:-}"
KC_CLIENT_SECRET="${KC_CLIENT_SECRET:-}"

# Extra JSON spliced into each user-creation payload, for realms whose user
# profile declares required attributes. A realm that does not declare one
# REJECTS the unknown attribute, so this stays empty by default.
#   KC_EXTRA_USER_JSON='"attributes":{"department":["demo"]},'
KC_EXTRA_USER_JSON="${KC_EXTRA_USER_JSON:-}"

KMT_CONTAINER="${KMT_CONTAINER:-kmt-server}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

USERNAMES=(
    demo-alice demo-bob    demo-carol  demo-dave   demo-erin
    demo-frank demo-grace  demo-mallory demo-niaj  demo-olivia
    demo-peggy demo-rupert demo-sybil  demo-trent  demo-victor
)

die() { echo "error: $*" >&2; exit 1; }

# Master-realm admin tokens expire in about 60 seconds, so this is called
# again before each phase rather than once at the top.
kc_token() {
    local token
    if [ -n "$KC_CLIENT_ID" ]; then
        token=$(curl -sS -m 10 \
            -d "grant_type=client_credentials" \
            -d "client_id=${KC_CLIENT_ID}" \
            -d "client_secret=${KC_CLIENT_SECRET}" \
            "${KC_URL}/realms/master/protocol/openid-connect/token" \
            | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
        [ -n "$token" ] || die "could not get a token for client ${KC_CLIENT_ID} at ${KC_URL}"
    else
        token=$(curl -sS -m 10 \
            -d "client_id=admin-cli" \
            -d "username=${KC_ADMIN}" \
            -d "password=${KC_ADMIN_PASSWORD}" \
            -d "grant_type=password" \
            "${KC_URL}/realms/master/protocol/openid-connect/token" \
            | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
        [ -n "$token" ] || die "could not get a Keycloak admin token from ${KC_URL} (is the admin account enabled? try KC_CLIENT_ID/KC_CLIENT_SECRET)"
    fi
    printf '%s' "$token"
}

# ---------------------------------------------------------------------------
echo "==> Checking prerequisites"

[ -n "$RO_PASSWORD" ] || die "set RO_PASSWORD to the password the kmt_ro role should get, e.g. RO_PASSWORD=\$(openssl rand -hex 16)"

docker inspect -f '{{.State.Running}}' "$AMFA_CONTAINER" 2>/dev/null | grep -q true \
    || die "AMFA Postgres container '${AMFA_CONTAINER}' is not running"

curl -sS -m 10 -o /dev/null "${KC_URL}/realms/master/.well-known/openid-configuration" \
    || die "Keycloak is not reachable at ${KC_URL} (an HTTPS Keycloak may need CURL_CA_BUNDLE)"

echo "    AMFA Postgres: ${AMFA_CONTAINER} (${AMFA_DB})"
echo "    Keycloak:      ${KC_URL} realm '${KC_REALM}'"

# ---------------------------------------------------------------------------
echo "==> Ensuring ${#USERNAMES[@]} demo users exist in realm '${KC_REALM}'"

TOKEN=$(kc_token)
declare -a USER_IDS=()
created=0
reused=0

for username in "${USERNAMES[@]}"; do
    # Keycloak 26 ignores a client-supplied "id" on create, so the real id has
    # to be read back rather than chosen up front.
    uid=$(curl -sS -m 10 -H "Authorization: Bearer ${TOKEN}" \
        "${KC_URL}/admin/realms/${KC_REALM}/users?username=${username}&exact=true" \
        | sed -n 's/^\[{"id":"\([^"]*\)".*/\1/p')

    if [ -z "$uid" ]; then
        given="${username#demo-}"
        curl -sS -m 10 -o /dev/null -X POST \
            -H "Authorization: Bearer ${TOKEN}" \
            -H "Content-Type: application/json" \
            -d "{\"username\":\"${username}\",\"enabled\":true,
                 \"email\":\"${username}@example.com\",\"emailVerified\":true,
                 ${KC_EXTRA_USER_JSON}
                 \"firstName\":\"${given}\",\"lastName\":\"Demo\"}" \
            "${KC_URL}/admin/realms/${KC_REALM}/users"

        uid=$(curl -sS -m 10 -H "Authorization: Bearer ${TOKEN}" \
            "${KC_URL}/admin/realms/${KC_REALM}/users?username=${username}&exact=true" \
            | sed -n 's/^\[{"id":"\([^"]*\)".*/\1/p')
        [ -n "$uid" ] || die "created ${username} but could not read its id back"
        created=$((created + 1))
    else
        reused=$((reused + 1))
    fi

    USER_IDS+=("$uid")
done

echo "    ${created} created, ${reused} reused"

# Build the VALUES tuple list 02-seed-demo-data.sql expects.
DEMO_USERS=""
for i in "${!USERNAMES[@]}"; do
    [ -n "$DEMO_USERS" ] && DEMO_USERS+=","
    DEMO_USERS+="(${i},'${USER_IDS[$i]}'::uuid,'${USERNAMES[$i]}')"
done

# ---------------------------------------------------------------------------
echo "==> Creating the kmt_ro role"

docker exec -i "$AMFA_CONTAINER" \
    psql -v ON_ERROR_STOP=1 -q -U "$AMFA_SUPERUSER" -d "$AMFA_DB" \
    -v ro_password="$RO_PASSWORD" \
    -v db="$AMFA_DB" \
    < "${SCRIPT_DIR}/01-create-ro-role.sql"

# ---------------------------------------------------------------------------
echo "==> Seeding AMFA demo data"

docker exec -i "$AMFA_CONTAINER" \
    psql -v ON_ERROR_STOP=1 -U "$AMFA_SUPERUSER" -d "$AMFA_DB" \
    -v demo_users="$DEMO_USERS" \
    -v realm="$KC_REALM" \
    < "${SCRIPT_DIR}/02-seed-demo-data.sql"

# ---------------------------------------------------------------------------
# The Alembic head this database is actually on. Printing a hardcoded value
# instead sends you to config.yaml with the wrong expected_schema_version,
# which KMT then reports as a schema-check warning. Absent on AMFA builds
# that create their schema without Alembic, which KMT tolerates.
SCHEMA_VERSION=$(docker exec -i "$AMFA_CONTAINER" \
    psql -tAq -U "$AMFA_SUPERUSER" -d "$AMFA_DB" \
    -c "SELECT version_num FROM alembic_version" 2>/dev/null \
    | tr -d '\r' | head -1) || SCHEMA_VERSION=""
: "${SCHEMA_VERSION:=<no alembic_version table; omit this key>}"

PG_PORT=$(docker port "$AMFA_CONTAINER" 5432 2>/dev/null | head -1 | sed 's/.*://')
: "${PG_PORT:=<the host port ${AMFA_CONTAINER} publishes 5432 on>}"
AMFA_NETWORK=$(docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$AMFA_CONTAINER" 2>/dev/null | awk '{print $1}')
: "${AMFA_NETWORK:=<the network ${AMFA_CONTAINER} is on>}"

cat <<EOF

==> Done seeding.

KMT shows AMFA events for the realms of a tenant it monitors, so one of its
tenants must monitor the Keycloak that holds realm '${KC_REALM}', and that
tenant needs an amfa block. KMT builds its AMFA connections once, at startup.

The simplest way for the local stack's ${KMT_CONTAINER} to reach both is to join
the AMFA stack's network (step 2 below). Add to config.yaml, under
keycloak.tenants:

    amfa-demo:
      enabled: true
      # The AMFA stack's Keycloak over plain HTTP inside its network. It needs
      # a monitoring client, as docs/keycloak-setup.md describes.
      server_url: "http://keycloak:8080"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "\${AMFA_DEMO_CLIENT_SECRET}"
      amfa:
        enabled: true
        events_lookback_days: 30
        expected_schema_version: "${SCHEMA_VERSION}"
        database:
          # Without the network connect above, use host.docker.internal and
          # port ${PG_PORT} instead (localhost if KMT runs as a native process).
          host: "postgres"
          port: 5432
          database: "${AMFA_DB}"
          user: "kmt_ro"
          password: "\${AMFA_DEMO_DB_PASSWORD}"
          ssl_mode: "disable"

and put the secrets in the root .env, which the containers read:

    AMFA_DEMO_CLIENT_SECRET='<the monitoring client's secret>'
    AMFA_DEMO_DB_PASSWORD='${RO_PASSWORD}'

Then, from the repository root, in this order:

    make docker-up                                     # 1. recreates ${KMT_CONTAINER} with the new .env
    docker network connect ${AMFA_NETWORK} ${KMT_CONTAINER}   # 2. a recreate drops this, so join after it
    docker restart ${KMT_CONTAINER}                    # 3. start again with both reachable
    docker logs ${KMT_CONTAINER} 2>&1 | grep "AMFA module ready"

A healthy start reports enabled_tenant_count of 1 or more. If it reports 0,
the log line just above it says why. For the AMFA rows of the Events table as
well as its KPI row and map, turn on keycloak.global.amfa_mirror with
backfill_days: 30 first; README.md explains why.
EOF
