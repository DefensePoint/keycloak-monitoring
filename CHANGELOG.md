# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- The AMFA geolocation map and event details place a failed login that AMFA stored without a risk decision, such as a wrong password or an unknown user, by the location AMFA recorded for its address. This needs an AMFA engine that stores coordinates for every login (engine migration `5ae8978564f9`); with an older engine these failures stay off the map, and KMT works as before.
- A configuration check warns before the client secret of an identity provider expires. Keycloak does not know the date, so it is listed per provider under `keycloak.global.config_checker.checks.identity_provider_secret_expiry.secrets`, as the provider shows it. A warning comes 30 days before, an error 7 days before, which email also gets with the default thresholds, and a critical alert from that day on. A listed provider or realm Keycloak does not have raises a warning, and an entry naming no enabled tenant is logged.

### Fixed

- AMFA events show the IP address, country and VPN flag of a failed login that AMFA stored without a risk decision, such as a wrong password or an unknown user, and a failed login from a VPN address counts as a flagged IP. In API mode this needs an AMFA engine that includes the same fix. These failures have no coordinates, so they are still not on the geolocation map.
- A tenant created or edited in the interface gets AMFA alerts and mirrored AMFA events when its AMFA toggle is on, and loses them when AMFA is turned off or the tenant is disabled or deleted, without a restart. Before, the AMFA checker and mirror only ran for `config.yaml` tenants. A save that leaves the tenant's AMFA connection as it was keeps them running. When AMFA is turned off, or the tenant is disabled or deleted, the tenant's open AMFA alerts are resolved with a notice saying AMFA is no longer checked for it, and startup does the same for every tenant without a checker. If the tenants cannot be loaded at startup for AMFA, startup now fails instead of logging a warning and running without AMFA alerts and mirroring.
- The configuration and event checks start and stop with tenant changes: a tenant created or enabled in the interface is checked without a restart, and one disabled or deleted stops being checked at once. Disabling a tenant also resolves its open configuration and event alerts, with a resolution notice saying it is no longer monitored; nothing would ever have resolved them. A tenant already disabled when the server starts, such as one disabled before the upgrade, gets the same at startup. Deleting a tenant removes its alerts, as before.
- Upgrade note: with `event_checker.enabled: true`, the event checks now cover every enabled tenant, including tenants created in the interface, which before had them only if they were in `config.yaml`. Those tenants can raise event alerts from the first run after the upgrade. There is no per-tenant switch: to keep one tenant quiet, add an alert silence with its `tenant_id`, narrowed with `alert_type` (such as `LOGIN_ERROR`) if needed.
- The configuration checks read every identity provider of a realm; Keycloak lists only the first 100 unless asked for more, so SAML certificate and metadata checks skipped the rest
- The event type filter on the events page suggests every event type in the selected time range and realm. Before, it suggested only the types on the current page, and once a type was chosen, only that type. `GET /api/tenants/{tenantID}/stats` now also returns `by_type`, the event count per type.

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
