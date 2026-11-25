import json
import os
import sys
import time
from http.client import HTTPException
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


# The demo realm every bundled Keycloak gets: users, a client for password
# logins and event logging. Mounted next to this script in the setup container.
DEMO_REALM_FILE = os.environ.get(
    "DEMO_REALM_FILE", os.path.join(os.path.dirname(os.path.abspath(__file__)), "demo-realm.json")
)
# A user the demo realm does not have, for a failed login.
UNKNOWN_DEMO_USER = "demo-nobody"


def load_demo_realm():
    with open(DEMO_REALM_FILE, encoding="utf-8") as file:
        return json.load(file)


class SetupError(Exception):
    def __init__(self, message, status=None):
        super().__init__(message)
        self.status = status


def error_message(exc):
    try:
        body = json.loads(exc.read(4096) or b"null")
    except (OSError, HTTPException, ValueError):
        return ""
    if not isinstance(body, dict):
        return ""
    fields = [str(body[key]) for key in ("error", "error_description", "errorMessage") if body.get(key)]
    return " (" + ": ".join(fields) + ")" if fields else ""


class Keycloak:
    def __init__(self, url):
        self.url = url.rstrip("/")
        self.token = None
        self.step = None

    def request(self, method, path, data=None, form=False, auth=True):
        self.step = method + " " + path
        headers = {}
        if self.token and auth:
            headers["Authorization"] = "Bearer " + self.token
        body = None
        if data is not None:
            body = urlencode(data).encode() if form else json.dumps(data).encode()
            headers["Content-Type"] = (
                "application/x-www-form-urlencoded" if form else "application/json"
            )
        request = Request(self.url + path, data=body, headers=headers, method=method)
        try:
            with urlopen(request, timeout=10) as response:
                content = response.read()
        except HTTPError as exc:
            with exc:
                raise SetupError(f"HTTP {exc.code} {exc.reason}{error_message(exc)}", exc.code) from None
        except (OSError, HTTPException) as exc:
            reason = exc.reason if isinstance(exc, URLError) else exc
            raise SetupError(str(reason) or type(reason).__name__) from None
        return json.loads(content) if content else None

    def login(self, username, password, attempts=60):
        last_error = None
        for attempt in range(attempts):
            try:
                result = self.request("POST", "/realms/master/protocol/openid-connect/token", {
                    "grant_type": "password", "client_id": "admin-cli",
                    "username": username, "password": password,
                }, form=True)
                self.token = result["access_token"]
                return
            except SetupError as exc:
                if exc.status is not None and exc.status < 500:
                    raise
                last_error = exc
            if attempt + 1 < attempts:
                time.sleep(2)
        raise SetupError(f"did not become ready after {attempts} attempts, last error: {last_error}")

    def provision(self, secret, amfa_audience=False):
        realm_path = "/admin/realms/master"
        client_query = "/clients?" + urlencode({"clientId": "monitoring-service"})
        clients = self.request("GET", realm_path + client_query)
        settings = {
            "clientId": "monitoring-service", "enabled": True,
            "protocol": "openid-connect", "publicClient": False,
            "serviceAccountsEnabled": True, "standardFlowEnabled": False,
            "directAccessGrantsEnabled": False, "secret": secret,
            "attributes": {"access.token.lifespan": "180"},
        }
        if not clients:
            self.request("POST", realm_path + "/clients", settings)
            clients = self.request("GET", realm_path + client_query)
        if len(clients) != 1:
            raise SetupError(f"expected one monitoring-service client, found {len(clients)}")
        client_id = clients[0]["id"]
        self.request("PUT", realm_path + "/clients/" + client_id, settings)
        if amfa_audience:
            self.add_amfa_audience(realm_path + "/clients/" + client_id)
        # Builds with the kc-admin-api client scope (RHBK 26.6.9 and later) reject
        # Admin API tokens without it, and only admin-cli gets it by default.
        scopes = self.request("GET", realm_path + "/client-scopes")
        for scope in scopes:
            if scope["name"] == "kc-admin-api":
                self.request("PUT", realm_path + "/clients/" + client_id + "/default-client-scopes/" + scope["id"])
        account = self.request("GET", realm_path + "/clients/" + client_id + "/service-account-user")
        role = self.request("GET", realm_path + "/roles/admin")
        self.request("POST", realm_path + "/users/" + account["id"] + "/role-mappings/realm", [role])
        self.import_demo_realm()
        for realm in ("master", "demo"):
            self.request("PUT", "/admin/realms/" + realm + "/events/config", {
                "eventsEnabled": True, "eventsExpiration": 86400,
                "adminEventsEnabled": True, "adminEventsDetailsEnabled": False,
                "enabledEventTypes": ["CLIENT_LOGIN", "CLIENT_LOGIN_ERROR", "LOGIN", "LOGIN_ERROR", "LOGOUT"],
            })
        self.verify(secret)
        return self.generate_activity()

    def add_amfa_audience(self, client_path):
        """Put the AMFA engine in the client's token audience.

        The engine accepts only tokens for its audience, and KMT reads AMFA over
        its monitoring API with this client's service-account token.
        """
        path = client_path + "/protocol-mappers/models"
        if any(m.get("name") == "amfa-audience" for m in self.request("GET", path)):
            return
        self.request("POST", path, {
            "name": "amfa-audience", "protocol": "openid-connect",
            "protocolMapper": "oidc-audience-mapper",
            "config": {"included.custom.audience": "amfa", "access.token.claim": "true",
                       "id.token.claim": "false"},
        })

    def import_demo_realm(self):
        demo = load_demo_realm()
        realms = self.request("GET", "/admin/realms")
        if not any(realm["realm"] == demo["realm"] for realm in realms):
            self.request("POST", "/admin/realms", demo)
            return
        # A realm kept from an earlier demo, possibly the empty one older setups
        # created: add what it lacks and leave what it has.
        self.request("POST", "/admin/realms/" + demo["realm"] + "/partialImport", {
            "ifResourceExists": "SKIP", "users": demo["users"], "clients": demo["clients"],
        })

    def generate_activity(self):
        """Log the demo users in, plus two failed logins, so the dashboard has events."""
        demo = load_demo_realm()
        path = "/realms/" + demo["realm"] + "/protocol/openid-connect/token"

        def password_login(username, password):
            self.request("POST", path, {
                "grant_type": "password", "client_id": "demo-app",
                "username": username, "password": password,
            }, form=True, auth=False)

        def refused(exc):
            # Keycloak 26.7 answers a refused login with 400 invalid_grant, older builds 401.
            return exc.status in (400, 401) and "invalid_grant" in str(exc)

        users = demo["users"]
        logins = 0
        for user in users:
            try:
                password_login(user["username"], user["credentials"][0]["value"])
                logins += 1
            except SetupError as exc:
                if not refused(exc):
                    raise
                # Changed in Keycloak since the import, which a demo may well do.
                print(f"WARNING: demo user {user['username']} could not log in with its"
                      " password from demo-realm.json; skipped", flush=True)
        failed = [(users[0]["username"], "not-the-password"), (UNKNOWN_DEMO_USER, "not-the-password")]
        for username, password in failed:
            try:
                password_login(username, password)
            except SetupError as exc:
                if not refused(exc):
                    raise
            else:
                raise SetupError(f"a wrong password logged {username} in")
        return logins, len(failed)

    def verify(self, secret):
        result = self.request("POST", "/realms/master/protocol/openid-connect/token", {
            "grant_type": "client_credentials", "client_id": "monitoring-service", "client_secret": secret,
        }, form=True)
        self.token = result["access_token"]
        self.request("GET", "/admin/serverinfo")
        for realm in self.request("GET", "/admin/realms"):
            path = "/admin/realms/" + realm["realm"]
            for suffix in ("", "/users/count", "/clients", "/events"):
                self.request("GET", path + suffix)


def main():
    # The third Keycloak is the AMFA tenant, there only with make docker-demo-up AMFA=1.
    numbers = (1, 2, 3) if os.environ.get("BUNDLED_AMFA") == "true" else (1, 2)
    required = ["BUNDLED_KC_ADMIN_PASSWORD"] + [f"BUNDLED_KC{number}_CLIENT_SECRET" for number in numbers]
    missing = [key for key in required if not os.environ.get(key)]
    if missing:
        raise SetupError("set " + ", ".join(missing) + " in .env")
    username = os.environ.get("BUNDLED_KC_ADMIN_USER", "admin")
    for number in numbers:
        client = Keycloak(os.environ.get(f"BUNDLED_KC{number}_URL", f"http://keycloak-{number}:8080"))
        try:
            client.login(username, os.environ["BUNDLED_KC_ADMIN_PASSWORD"])
            logins, failed = client.provision(os.environ[f"BUNDLED_KC{number}_CLIENT_SECRET"],
                                              amfa_audience=number == 3)
        except (SetupError, LookupError, TypeError, ValueError) as exc:
            detail = exc if isinstance(exc, SetupError) else f"{type(exc).__name__}: {exc}"
            raise SetupError(f"Keycloak {number} at {client.url}, {client.step}: {detail}") from None
        print(f"Keycloak {number}: monitoring client and realm read access verified", flush=True)
        print(f"Keycloak {number}: demo realm ready, {logins} demo logins and {failed} failed logins recorded", flush=True)


if __name__ == "__main__":
    try:
        main()
    except SetupError as exc:
        sys.exit(f"Keycloak setup failed: {exc}")
