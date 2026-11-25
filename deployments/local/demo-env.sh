#!/bin/sh
# Prepares .env for the bundled demo, so make docker-demo-up works from a clean
# checkout:
#
#   sh deployments/local/demo-env.sh <.env> <.env.example> <compose project>
#
# Creates .env from the example when it is missing, then fills every demo value
# that is empty or absent with a random one. A value that is already set is
# never changed, so the passwords stored in retained demo volumes keep
# matching. Generated values use only letters, digits, - and !, which Compose
# and the shell pass through unchanged.
set -eu

env_file=$1
example=$2
project=$3

hex() {
	od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
}

# The server's password rules: at least 12 characters with upper and lower
# case, a digit and a special character, and no common weak password such as
# 123456 inside it. Hex is the only random part that could contain one.
dashboard_password() {
	while :; do
		value="Kmt-$(hex 12)!9"
		case $value in
		*123456*) ;;
		*) echo "$value"; return ;;
		esac
	done
}

if [ ! -f "$env_file" ]; then
	cp "$example" "$env_file"
	chmod 600 "$env_file"
	echo "Created $env_file from $example."
fi

generated=""

fill() {
	key=$1
	value=$2
	if grep -q "^$key=" "$env_file"; then
		# Empty, or empty quotes, which Compose also reads as empty.
		grep -Eq "^$key=(''|\"\")?\$" "$env_file" || return 0
		tmp="$env_file.tmp.$$"
		awk -v key="$key" -v value="$value" '
			$0 == key "=" || $0 == key "='"''"'" || $0 == key "=\"\"" { print key "=" value; next }
			{ print }' "$env_file" > "$tmp"
		cat "$tmp" > "$env_file"
		rm -f "$tmp"
	else
		# An .env created before the demo existed has no line for it.
		[ -z "$(tail -c 1 "$env_file")" ] || echo >> "$env_file"
		echo "$key=$value" >> "$env_file"
	fi
	generated="$generated $key"
}

fill MONITORING_AUTH_SIMPLE_DEFAULT_PASS "$(dashboard_password)"
case $generated in
*MONITORING_AUTH_SIMPLE_DEFAULT_PASS*) password_generated=yes ;;
*) password_generated=no ;;
esac
fill BUNDLED_DB_PASSWORD "$(hex 24)"
fill BUNDLED_KC_ADMIN_PASSWORD "$(hex 24)"
fill BUNDLED_KC1_CLIENT_SECRET "$(hex 24)"
fill BUNDLED_KC2_CLIENT_SECRET "$(hex 24)"
fill BUNDLED_SESSION_SECRET "$(hex 24)"
# Used only by the AMFA tenant (make docker-demo-up AMFA=1).
fill BUNDLED_KC3_CLIENT_SECRET "$(hex 24)"
fill BUNDLED_AMFA_DB_PASSWORD "$(hex 24)"

[ -n "$generated" ] || exit 0

echo "Generated in $env_file:$generated"
if [ "$password_generated" = yes ]; then
	user=$(sed -n 's/^MONITORING_AUTH_SIMPLE_DEFAULT_USER=//p' "$env_file")
	pass=$(sed -n 's/^MONITORING_AUTH_SIMPLE_DEFAULT_PASS=//p' "$env_file")
	echo "Dashboard login: ${user:-operator} / $pass (also in $env_file)"
fi

# The database and Keycloak admin passwords are set when their volumes are
# first created. New ones do not match volumes kept from an earlier demo. The
# Keycloak admin password is in every Keycloak database, the AMFA tenant's too;
# the AMFA database password only in the AMFA database.
volumes() {
	command -v docker >/dev/null 2>&1 && docker volume ls -q 2>/dev/null
}
stale=no
case $generated in
*BUNDLED_DB_PASSWORD* | *BUNDLED_KC_ADMIN_PASSWORD*)
	volumes | grep -q "^${project}_" && stale=yes
	;;
esac
case $generated in
*BUNDLED_AMFA_DB_PASSWORD*)
	volumes | grep -q "^${project}_kmt-amfa-postgres-data\$" && stale=yes
	;;
esac
if [ "$stale" = yes ]; then
	echo "WARNING: demo volumes from an earlier run exist (project $project), and their"
	echo "passwords do not match the new values. Remove them with: make docker-demo-clean"
fi
