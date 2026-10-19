import json
import os
import sys
import time
from http.client import HTTPException
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode
from urllib.request import Request, urlopen


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

    def request(self, method, path, data=None, form=False):
        self.step = method + " " + path
        headers = {}
        if self.token:
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

    def provision(self, secret):
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
        account = self.request("GET", realm_path + "/clients/" + client_id + "/service-account-user")
        role = self.request("GET", realm_path + "/roles/admin")
        self.request("POST", realm_path + "/users/" + account["id"] + "/role-mappings/realm", [role])
        realms = self.request("GET", "/admin/realms")
        if not any(realm["realm"] == "demo" for realm in realms):
            self.request("POST", "/admin/realms", {"realm": "demo", "enabled": True})
        for realm in ("master", "demo"):
            self.request("PUT", "/admin/realms/" + realm + "/events/config", {
                "eventsEnabled": True, "eventsExpiration": 86400,
                "adminEventsEnabled": True, "adminEventsDetailsEnabled": False,
                "enabledEventTypes": ["CLIENT_LOGIN", "CLIENT_LOGIN_ERROR", "LOGIN", "LOGIN_ERROR", "LOGOUT"],
            })
        self.verify(secret)

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
    required = ["BUNDLED_KC_ADMIN_PASSWORD", "BUNDLED_KC1_CLIENT_SECRET", "BUNDLED_KC2_CLIENT_SECRET"]
    missing = [key for key in required if not os.environ.get(key)]
    if missing:
        raise SetupError("set " + ", ".join(missing) + " in .env")
    username = os.environ.get("BUNDLED_KC_ADMIN_USER", "admin")
    for number in (1, 2):
        client = Keycloak(os.environ.get(f"BUNDLED_KC{number}_URL", f"http://keycloak-{number}:8080"))
        try:
            client.login(username, os.environ["BUNDLED_KC_ADMIN_PASSWORD"])
            client.provision(os.environ[f"BUNDLED_KC{number}_CLIENT_SECRET"])
        except (SetupError, LookupError, TypeError, ValueError) as exc:
            detail = exc if isinstance(exc, SetupError) else f"{type(exc).__name__}: {exc}"
            raise SetupError(f"Keycloak {number} at {client.url}, {client.step}: {detail}") from None
        print(f"Keycloak {number}: monitoring client and realm read access verified", flush=True)


if __name__ == "__main__":
    try:
        main()
    except SetupError as exc:
        sys.exit(f"Keycloak setup failed: {exc}")
