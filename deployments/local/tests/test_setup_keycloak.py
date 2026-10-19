import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch
from urllib.parse import parse_qs, urlparse

SCRIPT = Path(__file__).resolve().parents[1] / "setup-keycloak.py"
SPEC = importlib.util.spec_from_file_location("setup_keycloak", SCRIPT)
setup = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(setup)

BUILTIN_CLIENTS = [
    {"id": client_id + "-uuid", "clientId": client_id}
    for client_id in ("account", "account-console", "admin-cli", "broker", "security-admin-console")
]


class FakeKeycloak:
    def __init__(self):
        self.client = None
        self.realms = {"master": {"realm": "master"}}
        self.events = {}
        self.admin_role = False
        self.requests = []
        self.token_failures = []
        self.denied = None
        state = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                self.handle_api()

            do_POST = do_GET
            do_PUT = do_GET

            def handle_api(self):
                url = urlparse(self.path)
                path = url.path
                raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
                form = self.headers.get("Content-Type") == "application/x-www-form-urlencoded"
                data = {k: v[0] for k, v in parse_qs(raw.decode()).items()} if form else json.loads(raw or "null")
                state.requests.append((self.command, path, data))
                status, response = 200, {}
                if path.endswith("/token"):
                    if state.token_failures:
                        status = state.token_failures.pop(0)
                        if status == "reset":
                            self.close_connection = True
                            return
                    elif data.get("grant_type") == "password":
                        if data.get("password") == "admin-pass":
                            response = {"access_token": "admin-token"}
                        else:
                            status = 401
                            response = {"error": "invalid_grant", "error_description": "Invalid user credentials"}
                    else:
                        valid = state.client and data.get("client_secret") == state.client["secret"]
                        status = 200 if valid else 401
                        response = {"access_token": "service-token"}
                elif self.headers.get("Authorization") == "Bearer service-token":
                    if not state.admin_role or path == state.denied:
                        status = 403
                    elif path == "/admin/realms":
                        response = list(state.realms.values())
                elif self.headers.get("Authorization") != "Bearer admin-token":
                    status = 401
                elif path == "/admin/realms/master/clients" and self.command == "GET":
                    wanted = parse_qs(url.query).get("clientId")
                    clients = BUILTIN_CLIENTS + ([state.client] if state.client else [])
                    response = [c for c in clients if not wanted or c["clientId"] == wanted[0]]
                elif path == "/admin/realms/master/clients" and self.command == "POST":
                    if state.client:
                        status = 409
                    else:
                        state.client = dict(data, id="client-uuid")
                        status = 201
                elif path == "/admin/realms/master/clients/client-uuid" and self.command == "PUT":
                    state.client.update(data)
                    status = 204
                elif path.endswith("/service-account-user"):
                    response = {"id": "account-uuid"}
                elif path == "/admin/realms/master/roles/admin":
                    response = {"id": "role-uuid", "name": "admin", "composite": True}
                elif path.endswith("/role-mappings/realm"):
                    state.admin_role = any(role["name"] == "admin" for role in data)
                    status = 204
                elif path == "/admin/realms" and self.command == "GET":
                    response = list(state.realms.values())
                elif path == "/admin/realms" and self.command == "POST":
                    if data["realm"] in state.realms:
                        status = 409
                    else:
                        state.realms[data["realm"]] = data
                        status = 201
                elif path.endswith("/events/config"):
                    state.events[path.split("/")[3]] = data
                    status = 204
                else:
                    status = 404
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                if status != 204:
                    self.wfile.write(json.dumps(response).encode())

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.url = f"http://127.0.0.1:{self.server.server_port}"

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class ProvisioningTests(unittest.TestCase):
    def setUp(self):
        self.fake = FakeKeycloak()
        self.addCleanup(self.fake.close)
        self.client = setup.Keycloak(self.fake.url)

    def provision(self, secret="first-$secret"):
        self.client.login("admin", "admin-pass", attempts=1)
        self.client.provision(secret)

    def test_create_and_repeat_preserve_identity_and_repair_settings(self):
        self.provision()
        self.fake.client.update(publicClient=True, serviceAccountsEnabled=False)
        self.fake.admin_role = False
        self.provision("rotated-$secret")
        self.assertEqual(self.fake.client["id"], "client-uuid")
        self.assertEqual(self.fake.client["secret"], "rotated-$secret")
        self.assertFalse(self.fake.client["publicClient"])
        self.assertFalse(self.fake.client["directAccessGrantsEnabled"])
        self.assertFalse(self.fake.client["standardFlowEnabled"])
        self.assertTrue(self.fake.client["serviceAccountsEnabled"])
        self.assertEqual(self.fake.client["attributes"]["access.token.lifespan"], "180")
        self.assertTrue(self.fake.admin_role)
        self.assertEqual(set(self.fake.realms), {"master", "demo"})
        creates = [r for r in self.fake.requests if r[:2] == ("POST", "/admin/realms/master/clients")]
        self.assertEqual(len(creates), 1)
        self.assertTrue(all(v["eventsEnabled"] for v in self.fake.events.values()))

    def test_invalid_admin_credentials_fail_without_retry_or_mutation(self):
        with patch.object(setup.time, "sleep") as sleep:
            with self.assertRaises(setup.SetupError) as error:
                self.client.login("admin", "bad", attempts=3)
        self.assertEqual(error.exception.status, 401)
        sleep.assert_not_called()
        self.assertIsNone(self.fake.client)

    def test_not_ready_retries_then_succeeds(self):
        self.fake.token_failures = [503, 500]
        with patch.object(setup.time, "sleep") as sleep:
            self.client.login("admin", "admin-pass", attempts=3)
        self.assertEqual(sleep.call_count, 2)
        self.assertEqual(self.client.token, "admin-token")

    def test_connection_reset_during_readiness_is_retried(self):
        self.fake.token_failures = ["reset", "reset"]
        with patch.object(setup.time, "sleep") as sleep:
            self.client.login("admin", "admin-pass", attempts=3)
        self.assertEqual(sleep.call_count, 2)
        self.assertEqual(self.client.token, "admin-token")

    def test_not_ready_exhausts_timeout_and_reports_last_error(self):
        self.fake.token_failures = ["reset", 503]
        with patch.object(setup.time, "sleep"):
            with self.assertRaisesRegex(setup.SetupError, "after 2 attempts, last error: HTTP 503"):
                self.client.login("admin", "admin-pass", attempts=2)

    def test_verify_checks_permissions_beyond_token(self):
        self.provision()
        self.fake.denied = "/admin/realms/demo/events"
        with self.assertRaises(setup.SetupError) as error:
            self.client.verify("first-$secret")
        self.assertEqual(error.exception.status, 403)
        self.assertIn(("GET", "/admin/serverinfo", None), self.fake.requests)

    def test_verify_rejects_wrong_secret(self):
        self.provision()
        with self.assertRaises(setup.SetupError) as error:
            self.client.verify("wrong")
        self.assertEqual(error.exception.status, 401)

    def test_main_requires_all_secrets_before_network(self):
        with patch.dict(os.environ, {"BUNDLED_KC1_CLIENT_SECRET": "one"}, clear=True):
            with patch.object(setup, "Keycloak") as client:
                with self.assertRaisesRegex(
                    setup.SetupError, "set BUNDLED_KC_ADMIN_PASSWORD, BUNDLED_KC2_CLIENT_SECRET in .env"
                ):
                    setup.main()
        client.assert_not_called()

    def test_failure_names_server_and_step(self):
        other = FakeKeycloak()
        self.addCleanup(other.close)
        self.fake.denied = "/admin/realms/demo/events"
        env = {"BUNDLED_KC_ADMIN_PASSWORD": "admin-pass", "BUNDLED_KC1_CLIENT_SECRET": "one-$secret",
               "BUNDLED_KC2_CLIENT_SECRET": "two-$secret", "BUNDLED_KC1_URL": other.url,
               "BUNDLED_KC2_URL": self.fake.url}
        with patch.dict(os.environ, env, clear=True), contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaises(setup.SetupError) as error:
                setup.main()
        self.assertEqual(
            str(error.exception),
            f"Keycloak 2 at {self.fake.url}, GET /admin/realms/demo/events: HTTP 403 Forbidden",
        )

    def test_script_failure_reports_server_step_and_message(self):
        env = {"BUNDLED_KC_ADMIN_PASSWORD": "wrong-$password", "BUNDLED_KC1_CLIENT_SECRET": "one-$secret",
               "BUNDLED_KC2_CLIENT_SECRET": "two-$secret", "BUNDLED_KC1_URL": self.fake.url}
        result = subprocess.run([sys.executable, str(SCRIPT)], env=env, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, "")
        self.assertEqual(
            result.stderr.strip(),
            f"Keycloak setup failed: Keycloak 1 at {self.fake.url}, "
            "POST /realms/master/protocol/openid-connect/token: "
            "HTTP 401 Unauthorized (invalid_grant: Invalid user credentials)",
        )
        self.assertIsNone(self.fake.client)

    def test_main_provisions_two_servers_with_distinct_secrets_without_logging_them(self):
        other = FakeKeycloak()
        self.addCleanup(other.close)
        env = {"BUNDLED_KC_ADMIN_PASSWORD": "admin-pass", "BUNDLED_KC1_CLIENT_SECRET": "one-$secret",
               "BUNDLED_KC2_CLIENT_SECRET": "two-$secret", "BUNDLED_KC1_URL": self.fake.url,
               "BUNDLED_KC2_URL": other.url}
        output = io.StringIO()
        with patch.dict(os.environ, env, clear=True), contextlib.redirect_stdout(output):
            setup.main()
        self.assertEqual(self.fake.client["secret"], "one-$secret")
        self.assertEqual(other.client["secret"], "two-$secret")
        self.assertNotIn("secret", output.getvalue())
        self.assertIn("Keycloak 2", output.getvalue())


if __name__ == "__main__":
    unittest.main()
