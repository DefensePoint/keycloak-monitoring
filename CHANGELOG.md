# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.2] - 2026-10-20

### Fixed

- The AMFA geolocation map on the events page shows again; every production build showed "AMFA Geo Map unavailable" with "Minified React error #130"

## [2.0.1] - 2026-10-20

Everyone signs in once more after upgrading: sessions from 2.0.0 carry no account ID and are refused.

### Fixed

- Deactivating, blocking or reactivating a user, or changing their password, ends their existing web sessions at once, on every server instance; before, a session stayed valid until it expired
- The session of an inactive or blocked account, or of a username no account holds any more, is refused
- Renaming a simple-auth user no longer fails with "Failed to check username availability"
- A session from before a rename can no longer act as a new account that took the old username

## [2.0.0] - 2026-10-19

### Changed

- Builds with Go 1.27.2 and Node.js 26; base images are pinned by digest and run on Debian 13
- The server refuses to start with an empty, unexpanded or published session secret, and `config.yaml` has no built-in database password
- `${NAME}` placeholders in `config.yaml` are expanded from the environment
- Logout is a POST request, and the session cookie is never sent over plain HTTP
- The event alert checker and the configuration checks are off until enabled in `config.yaml`
- The server Compose template publishes the API and web on loopback only, and so does the local stack for the MCP server

### Added

- PKCE and a nonce on the SSO login, bound to the browser; see the PKCE note in `docs/keycloak-setup.md` before requiring it in Keycloak
- A bundled two-Keycloak demo: `make docker-demo-up`
- Events API filters by type, severity, minimum risk and user, and a lookup of one event by id
- Reading AMFA over its read-only monitoring API instead of its database
- Deactivating and reactivating custom roles from the Roles page
- An MCP limit that stops an SSO user's tokens after a period without SSO sign-in
- Alerts when a tenant's monitor cannot start or its polled events cannot be stored
- Indexes and pre-creation scripts for the alert listings (`deployments/postgres`)
- Upgrade path for databases from early single-tenant releases

### Fixed

- Admin is granted by email only when the identity provider verified the address
- A duplicate verified email can no longer take over an SSO account
- Cross-origin credentialed and state-changing requests are refused
- Cloud metadata endpoints are refused outside link-local, and the SAML metadata fetch is guarded and size-capped
- Deleted and inactive roles no longer grant access, and role permissions are saved on create
- Event polling on Keycloak before 26.2 reads every event of a burst
- Events are polled from where the last poll stopped, and events without a Keycloak id get a stable id
- Configuration, health and threshold alerts resolve when fixed and reopen when they recur
- Alert silences apply to the event, AMFA and configuration checkers
- Notifications are sent once per occurrence, with timeouts and retries, and GitLab issues close only when their last alert resolves
- A re-saved Keycloak event keeps the risk level merged from AMFA
- Client secrets are no longer returned from the clients route
- Many smaller fixes to tenants, RBAC, reports and the MCP server

## [1.0.1] - 2026-09-22

### Added

- Multi-tenant Keycloak monitoring from a single platform
- Real-time event tracking for logins, logouts, registrations, and security events
- Automated security configuration scanning for realms, clients, and identity providers
- Alert management with severity-based filtering and lifecycle tracking
- Notification integrations for Slack, email, and GitLab
- Dashboard analytics with interactive charts
- Role-based access control (RBAC) with Admin, Operator, and Viewer roles
- Dual authentication support (simple and OAuth2/OIDC)
- TimescaleDB integration for time-series event storage
- RESTful API with Swagger documentation
- MCP server for exposing monitoring data to MCP clients
- Docker Compose deployment with multi-stage builds
- SECURITY.md, CODE_OF_CONDUCT.md, and CONTRIBUTING.md

[2.0.2]: https://github.com/DefensePoint/keycloak-monitoring/releases/tag/v2.0.2
[2.0.1]: https://github.com/DefensePoint/keycloak-monitoring/releases/tag/v2.0.1
[2.0.0]: https://github.com/DefensePoint/keycloak-monitoring/releases/tag/v2.0.0
[1.0.1]: https://github.com/DefensePoint/keycloak-monitoring/releases/tag/v1.0.1
