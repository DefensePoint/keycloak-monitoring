# Security Scan Baseline

Baseline for the container and code scans introduced by DFPT-1080. Scans run
warn-only in CI (`allow_failure: true`, `exit_code: 0`). This document is the
snapshot of findings the reviewer signs off before the pipeline flips to
blocking.

**Escalation bar for the initial rollout.** Only Critical findings gate the
open-source release. High and Medium findings are informational, listed here
for visibility, tracked in follow-up work, not a blocker.

## Summary

| Scanner              | Critical | High | Medium | Info | Status                                   |
|----------------------|---------:|-----:|-------:|-----:|------------------------------------------|
| Trivy fs             |        2 |   20 |      0 |    0 | Fix action noted, bump deps in follow-up |
| Trivy image (server) |        3 |   39 |      0 |    0 | Same pgx + stdlib CVEs; fix via rebuild after bumps |
| Trivy image (web)    |        1 |   22 |      0 |    0 | Stdlib CVE dominates; rebuild after Go 1.25.13     |
| Semgrep (SAST)       |        0 |    0 |      0 |    0 | 110 rules ran, no ERROR findings         |
| Gitleaks             |        0 |    0 |      0 |    0 | Three matches, all allowlisted false positives |
| Hadolint             |        0 |    3 |      0 |    1 | Best-practice warnings, not blocking     |

## Local reproduction

```
make security-scan
```

Runs the same tools CI runs and writes JSON reports to `reports/security/`.
Per-tool sub-targets: `make sec-hadolint`, `make sec-trivy-fs`,
`make sec-gitleaks`, `make sec-semgrep`, `make sec-container`. Tool
prerequisites are checked by `make sec-preflight`.

## Trivy filesystem scan

`trivy fs --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed .`

Covers Go modules (`go.mod`) and npm lockfiles (`web/package-lock.json`).

### Critical

| Package                     | Installed | Fixed | CVE               | Fix action                          |
|-----------------------------|-----------|-------|-------------------|-------------------------------------|
| github.com/jackc/pgx/v5     | 5.7.6     | 5.9.0 | CVE-2026-33815    | Bump `pgx/v5` to 5.9.0 (follow-up). |
| github.com/jackc/pgx/v5     | 5.7.6     | 5.9.0 | CVE-2026-33816    | Same bump covers both CVEs.         |

### High (Go)

| Package                    | Installed | Fixed  | CVEs                                                                                                                                                                                                                                                             | Fix action                       |
|----------------------------|-----------|--------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------|
| github.com/go-jose/go-jose | 4.1.3     | 4.1.4  | CVE-2026-34986                                                                                                                                                                                                                                                   | Patch bump.                      |
| golang.org/x/crypto        | 0.46.0    | 0.52.0 | CVE-2026-39828, CVE-2026-39829, CVE-2026-39830, CVE-2026-39831, CVE-2026-39832, CVE-2026-39835, CVE-2026-42508, CVE-2026-46595, CVE-2026-46597                                                                                                                   | Bump indirect stdlib toolchain. |
| golang.org/x/mod           | 0.32.0    | 0.40.0 | CVE-2026-56864, CVE-2026-56865                                                                                                                                                                                                                                   | Follow-on x/mod bump.            |
| golang.org/x/net           | 0.48.0    | 0.55.0+ | CVE-2026-25681, CVE-2026-27136, CVE-2026-33814, CVE-2026-39821, CVE-2026-46600                                                                                                                                                                                  | Bump x/net.                      |
| golang.org/x/text          | 0.33.0    | 0.39.0 | CVE-2026-56852                                                                                                                                                                                                                                                   | Bump x/text.                     |

### High (npm)

| Package    | Installed | Fixed              | Advisory                       | Fix action                            |
|------------|-----------|--------------------|--------------------------------|---------------------------------------|
| axios      | 1.16.1    | 1.18.0 (or 0.33.0) | GHSA-gcfj-64vw-6mp9            | Bump axios to 1.18.0 in `web/`.       |
| form-data  | 4.0.5     | 4.0.6              | CVE-2026-12143                 | Bump form-data via transitive update. |

### Suggested follow-up

Open a Renovate-scoped dep-bump MR (or wait for the automated Renovate MRs to
land). All 20 findings resolve with dependency updates alone; no code changes
required.

## Trivy image scans

`trivy image --severity CRITICAL,HIGH --ignore-unfixed` against the built
service images (`SERVICE=server` and `SERVICE=web`).

Both reports include an OS packages result (`debian 12.15`) and a language
packages result (`app/kmt`, Go binary). No OS-level findings surfaced at
CRITICAL or HIGH with `--ignore-unfixed`. Every finding below is in the Go
binary layer.

### server (`debian:12-slim` + chromium)

3 Critical, 39 High.

| Package                  | Installed | Fixed                             | CVE              | Fix action                                |
|--------------------------|-----------|-----------------------------------|------------------|-------------------------------------------|
| github.com/jackc/pgx/v5  | 5.7.6     | 5.9.0                             | CVE-2026-33815   | Bump pgx (also fixes fs finding).          |
| github.com/jackc/pgx/v5  | 5.7.6     | 5.9.0                             | CVE-2026-33816   | Same bump.                                 |
| stdlib (Go)              | 1.25.1    | 1.24.13 / 1.25.7 / 1.26.0-rc.3    | CVE-2025-68121   | Bump `GO_VERSION` to 1.25.13 minimum.      |

The 39 High findings are all Go stdlib CVEs plus the same x/crypto, x/mod,
x/net, x/text, and go-jose entries already listed under Trivy fs. Every one
resolves with a Go toolchain bump plus the pending Renovate dependency MRs.

### web (`gcr.io/distroless/static-debian12:nonroot`)

1 Critical, 22 High. The web binary embeds fewer third-party libraries, so
its finding set is almost entirely Go stdlib.

| Package     | Installed | Fixed                          | CVE              | Fix action                       |
|-------------|-----------|--------------------------------|------------------|----------------------------------|
| stdlib (Go) | 1.25.1    | 1.24.13 / 1.25.7 / 1.26.0-rc.3 | CVE-2025-68121   | Bump `GO_VERSION` to 1.25.13.    |

The 22 High findings are all Go stdlib CVEs plus one x/text entry; all clear
with the Go toolchain bump.

## Semgrep SAST

`semgrep scan --config auto --severity ERROR` via the `security/sast`
component (Semgrep OSS 1.156.0).

Result: **0 findings**. 310 Community-origin rules loaded, 110 applicable to
this codebase (Go, TypeScript, JavaScript, JSON, YAML), scanned 637 tracked
files. 68 timeouts on Trail of Bits racy-* Go rules against large repository
files (`*/postgres/repository.go`); no missed findings, timeouts are a
known-slow-analysis limitation of those rules.

## Gitleaks

`gitleaks detect --source .` scans full history (17 commits at baseline).

Three matches were surfaced at baseline; all three are false positives from
the `generic-api-key` rule and are allowlisted via `.gitleaksignore`:

| File                                        | Line | Why it is safe                                                              |
|---------------------------------------------|-----:|-----------------------------------------------------------------------------|
| `deployments/local/docker-compose.yml`      |   86 | JDBC URL `jdbc:postgresql://keycloak-db-1:5432/keycloak`; hostname:port, not a secret. |
| `deployments/local/docker-compose.yml`      |  139 | Same JDBC URL pattern for `keycloak-db-2`.                                  |
| `docs/troubleshooting.md`                   |  398 | Documentation placeholder `must-be-at-least-32-characters-long`.            |

Real credentials in the docker-compose sample (`KEYCLOAK_ADMIN_PASSWORD: admin`,
`KC_DB_PASSWORD: keycloak`) are intentional local-dev defaults, not scanned as
secrets by gitleaks, and are documented in the deployment guide as needing to
be replaced before any non-local use.

## Hadolint

`hadolint --format json build/Dockerfile`

Four best-practice findings, none security-critical:

| Code   | Line | Level   | Rule                                                              | Decision                                                                       |
|--------|-----:|---------|-------------------------------------------------------------------|--------------------------------------------------------------------------------|
| DL3018 |   42 | warning | Pin versions in `apk add`.                                        | Accepted risk. Pinning Alpine `git` and `build-base` gets noisy; upstream repo is trusted. |
| DL3008 |   83 | warning | Pin versions in `apt-get install`.                                | Accepted risk. Chromium version tracks the Debian slim base; pinning defeats the base image update cadence. |
| DL3015 |   83 | info    | Use `--no-install-recommends`.                                    | Fix candidate for follow-up. Chromium recommends bring in fonts we need for PDF rendering; verify before applying. |
| DL3006 |  148 | warning | Always tag the version of an image explicitly (`FROM runtime-${SERVICE}`). | False positive. `runtime-${SERVICE}` is an internal build stage, not a remote image tag. |

## Out of scope

- `.trivyignore` / `.semgrepignore` tuning beyond the false positives above.
  Real findings should be fixed or explicitly accepted, not ignored.
- Third-party downloads at container startup (GeoIP databases, Keycloak
  themes). Not scanned here because they are runtime data, not build-time
  artifacts.
- Turning the security stage from warn-only to blocking. Follow-up work after
  one to two weeks of clean pipelines and a triaged baseline.
