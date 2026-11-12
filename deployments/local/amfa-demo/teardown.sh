#!/usr/bin/env bash
#
# Reverses seed.sh: removes the seeded AMFA rows and deletes the demo-* users
# from Keycloak. Leaves the kmt_ro role in place, since KMT needs it to read
# any AMFA data, demo or real.
#
#   ./teardown.sh              remove demo data and demo users
#   ./teardown.sh --keep-users remove demo data only
#
# Reads the same environment variables as seed.sh, with the same defaults.

set -euo pipefail

AMFA_CONTAINER="${AMFA_CONTAINER:-amfa-postgres-1}"
AMFA_DB="${AMFA_DB:-adaptive_mfa}"
AMFA_SUPERUSER="${AMFA_SUPERUSER:-keycloak}"

KC_URL="${KC_URL:-https://keycloak:8443}"
KC_ADMIN="${KC_ADMIN:-admin}"
KC_ADMIN_PASSWORD="${KC_ADMIN_PASSWORD:-admin}"
KC_CLIENT_ID="${KC_CLIENT_ID:-}"
KC_CLIENT_SECRET="${KC_CLIENT_SECRET:-}"
KC_REALM="${KC_REALM:-test-amfa}"

KMT_CONTAINER="${KMT_CONTAINER:-kmt-server}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

KEEP_USERS=0
[ "${1:-}" = "--keep-users" ] && KEEP_USERS=1

USERNAMES=(
    demo-alice demo-bob    demo-carol  demo-dave   demo-erin
    demo-frank demo-grace  demo-mallory demo-niaj  demo-olivia
    demo-peggy demo-rupert demo-sybil  demo-trent  demo-victor
)

die() { echo "error: $*" >&2; exit 1; }

echo "==> Removing seeded AMFA rows"
docker exec -i "$AMFA_CONTAINER" \
    psql -v ON_ERROR_STOP=1 -U "$AMFA_SUPERUSER" -d "$AMFA_DB" \
    < "${SCRIPT_DIR}/03-purge-demo-data.sql"

if [ "$KEEP_USERS" -eq 1 ]; then
    echo "==> Keeping demo users (--keep-users)"
    exit 0
fi

echo "==> Deleting demo users from realm '${KC_REALM}'"
echo "    Only users with no remaining auth_event rows are removed. A demo-*"
echo "    username can be shared with data this script did not seed, and"
echo "    deleting the user would leave those rows with a dangling user_id"
echo "    that cannot be repaired: Keycloak assigns its own UUID on create."

if [ -n "$KC_CLIENT_ID" ]; then
    TOKEN=$(curl -sS -m 10 \
        -d "grant_type=client_credentials" \
        -d "client_id=${KC_CLIENT_ID}" -d "client_secret=${KC_CLIENT_SECRET}" \
        "${KC_URL}/realms/master/protocol/openid-connect/token" \
        | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
else
    TOKEN=$(curl -sS -m 10 \
        -d "client_id=admin-cli" -d "username=${KC_ADMIN}" \
        -d "password=${KC_ADMIN_PASSWORD}" -d "grant_type=password" \
        "${KC_URL}/realms/master/protocol/openid-connect/token" \
        | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
fi
[ -n "$TOKEN" ] || die "could not get a Keycloak admin token from ${KC_URL} (an HTTPS Keycloak may need CURL_CA_BUNDLE)"

deleted=0
kept=0
for username in "${USERNAMES[@]}"; do
    uid=$(curl -sS -m 10 -H "Authorization: Bearer ${TOKEN}" \
        "${KC_URL}/admin/realms/${KC_REALM}/users?username=${username}&exact=true" \
        | sed -n 's/^\[{"id":"\([^"]*\)".*/\1/p')
    [ -n "$uid" ] || continue

    # The purge above removed every row this tool seeded. Anything still
    # referencing this user therefore belongs to someone else, and deleting
    # the user would orphan it permanently.
    refs=$(docker exec "$AMFA_CONTAINER" \
        psql -tAq -U "$AMFA_SUPERUSER" -d "$AMFA_DB" \
        -c "SELECT count(*) FROM auth_event WHERE user_id = '${uid}'::uuid;" 2>/dev/null || echo 0)
    if [ "${refs:-0}" -gt 0 ]; then
        echo "    keeping ${username}: ${refs} auth_event row(s) still reference it"
        kept=$((kept + 1))
        continue
    fi

    curl -sS -m 10 -o /dev/null -X DELETE \
        -H "Authorization: Bearer ${TOKEN}" \
        "${KC_URL}/admin/realms/${KC_REALM}/users/${uid}"
    deleted=$((deleted + 1))
done

echo "    ${deleted} deleted, ${kept} kept"
echo "==> Done. Restart ${KMT_CONTAINER} to clear KMT's cache of AMFA usernames."
