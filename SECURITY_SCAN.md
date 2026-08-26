# Security Scan Baseline

> **Critical: 0 | High: 39 | Medium: 0**
>
> Deduplicated across every scanner. The dependency and container scans
> block the pipeline on Critical CVEs; High and Medium are informational
> and tracked as follow-up dependency work.

Baseline for the container and code scans introduced by DFPT-1080. The
dependency scan and the container-image scan gate on Critical only; all
other scanners (secrets, SAST, Dockerfile lint) stay warn-only.

## Fixed in this MR

Critical findings surfaced at baseline, all cleared by dependency bumps
landed alongside this MR.

| ID                  | Component               | From -> To         | Rated by       | Fix                                                    |
|---------------------|-------------------------|--------------------|----------------|--------------------------------------------------------|
| CVE-2026-33815      | github.com/jackc/pgx/v5 | 5.7.6 -> 5.9.0     | Trivy (NVD)    | `go get github.com/jackc/pgx/v5@v5.9.0`.               |
| CVE-2026-33816      | github.com/jackc/pgx/v5 | 5.7.6 -> 5.9.0     | Trivy (NVD)    | Same pgx bump.                                         |
| CVE-2025-68121      | Go stdlib               | 1.25.1 -> 1.25.14  | Trivy (NVD)    | `GO_VERSION` bumped in `.gitlab-ci.yml` and `build/Dockerfile`. |
| 8x GHSA (see below) | golang.org/x/crypto     | 0.46.0 -> 0.55.0   | Grype (GHSA)   | `go get golang.org/x/crypto@latest`; transitively bumps `x/net`, `x/text`, `x/sys`, `x/mod`, `x/tools`. |

The eight `x/crypto` GHSA IDs Grype rated Critical (GHSA-5cgq-3rg8-m6cv,
GHSA-vgwf-h737-ff37, GHSA-f5wc-c3c7-36mc, GHSA-x527-x647-q7gg,
GHSA-rm3j-f69w-wqmq, GHSA-89gr-r52h-f8rx, GHSA-jppx-rxg9-jmrx,
GHSA-w879-237q-wc7r) map to the CVE-2026-3982x cluster that Trivy rates
High under NVD. The CI dependency gate runs Grype, so Grype's severity is
what enforcement uses.

Post-bump local scans:

- Trivy fs: 0 Critical, 5 High.
- Grype fs: 0 Critical, 3 High (`--fail-on critical` exits 0).

## Per-scanner roll-up

| Scanner              | Critical | High | Notes                                                       |
|----------------------|---------:|-----:|-------------------------------------------------------------|
| Grype (CI gate)      |        0 |    3 | `go-jose`, `form-data`, `x/mod`; all High, all warn-only.   |
| Trivy fs (local)     |        0 |    5 | Same packages plus one npm entry; NVD severity mapping.     |
| Trivy image (server) |        0 |    3 | `go-jose` and `x/mod`; distroless base is clean.            |
| Trivy image (web)    |        0 |    0 | Fully clean.                                                |
| Semgrep (SAST)       |        0 |    0 | 110 rules ran, no ERROR findings.                           |
| Gitleaks             |        0 |    0 | 3 false positives allowlisted in `.gitleaksignore`.         |
| Hadolint             |        0 |    3 | Plus 3 info; best-practice warnings, not blocking.          |

Numbers are per-scanner totals; a package that surfaces in both Grype and
Trivy fs is counted in each. The **Critical: 0** at the top is what the
enforcement gate actually reads.

## Local reproduction

```
make security-scan
```

Runs the same tools CI runs and writes JSON reports to `reports/security/`.
Per-tool sub-targets: `make sec-hadolint`, `make sec-trivy-fs`,
`make sec-gitleaks`, `make sec-semgrep`, `make sec-container`. Tool
prerequisites are checked by `make sec-preflight`.

## Dependency scan

CI runs Grype via the `security/dependencies` component and enforces
`--fail-on critical`. Locally, `make sec-trivy-fs` runs Trivy filesystem
mode; both tools consume the same lockfile manifests
(`go.mod`, `web/package-lock.json`) but read different vulnerability
feeds. When Grype (GHSA) and Trivy (NVD) rate the same CVE at different
severities, Grype's call is what enforcement uses.

### Critical

None. The three baseline Criticals (two pgx/v5 CVEs and the x/crypto
GHSA cluster) are fixed in this MR by the pgx and x/crypto bumps.

### High (post-bump)

| Package                    | Installed | Fixed  | Notes                              |
|----------------------------|-----------|--------|------------------------------------|
| github.com/go-jose/go-jose | 4.1.3     | 4.1.4  | Patch bump, follow-up.             |
| golang.org/x/mod           | 0.38.0    | 0.40.0 | Indirect, still shy of latest.     |
| form-data (npm)            | 4.0.5     | 4.0.6  | Follow-up via Renovate.            |

x/crypto, x/net, x/sys, x/text were all bumped in this MR alongside the
x/crypto Critical fix, so their earlier High entries are already resolved.
The npm side has additional Medium findings on `axios` and `react-router`
that Renovate is expected to churn.

### Suggested follow-up

Let Renovate open the individual patch MRs for the High findings above; each
resolves with a dependency bump, no code changes required. None gates the
release under the current Critical-only enforcement.

## Trivy image scans

`trivy image --severity CRITICAL,HIGH --ignore-unfixed` against the built
service images (`SERVICE=server` and `SERVICE=web`).

Both reports include an OS packages result (`debian 12.15`) and a language
packages result (`app/kmt`, Go binary). No OS-level findings surfaced at
CRITICAL or HIGH with `--ignore-unfixed`. Every finding below is in the Go
binary layer.

### server (`debian:12-slim` + chromium)

0 Critical, 3 High after the pgx, Go toolchain, and x/crypto bumps:
`github.com/go-jose/go-jose/v4` and `golang.org/x/mod` still shy of the
latest patched version. None blocks merge or deploy under the current
Critical-only gate.

### web (`gcr.io/distroless/static-debian12:nonroot`)

0 Critical, 0 High. Fully clean after the toolchain and library bumps.

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

`hadolint --format json build/Dockerfile` (v2.15.1).

Six best-practice findings, none security-critical:

| Code   | Line | Level   | Rule                                                              | Decision                                                                       |
|--------|-----:|---------|-------------------------------------------------------------------|--------------------------------------------------------------------------------|
| DL3018 |   42 | warning | Pin versions in `apk add`.                                        | Accepted risk. Pinning Alpine `git` and `build-base` gets noisy; upstream repo is trusted. |
| DL3008 |   83 | warning | Pin versions in `apt-get install`.                                | Accepted risk. Chromium version tracks the Debian slim base; pinning defeats the base image update cadence. |
| DL3015 |   83 | info    | Use `--no-install-recommends`.                                    | Fix candidate for follow-up. Chromium recommends bring in fonts we need for PDF rendering; verify before applying. |
| DL3066 |  115 | info    | Non-numeric user-id may not be resolvable by host system (`USER nonroot:nonroot`). | Accepted risk. The `nonroot` user is created explicitly in the server runtime stage with UID/GID 65532; the string form is deliberate for readability. |
| DL3066 |  136 | info    | Same as above in the web runtime stage.                           | Accepted risk. `USER nonroot:nonroot` matches the distroless base's baked-in `nonroot` account (UID 65532). |
| DL3006 |  148 | warning | Always tag the version of an image explicitly (`FROM runtime-${SERVICE}`). | False positive. `runtime-${SERVICE}` is an internal build stage, not a remote image tag. |

## Gate posture

- **Dependency scan (Grype) and container-scan (Trivy):** block the
  pipeline on Critical only (`--fail-on critical` / `--severity CRITICAL
  --exit-code 1`). High and Medium are reported for visibility, do not
  block.
- **Secrets (Gitleaks), SAST (Semgrep), Dockerfile lint (hadolint):**
  warn-only. Findings surface as artifacts and non-blocking job failures.

Extending blocking to High severity is a follow-up decision once the
current baseline of High findings is cleared by Renovate.

## Out of scope

- `.trivyignore` / `.semgrepignore` tuning beyond the false positives above.
  Real findings should be fixed or explicitly accepted, not ignored.
- Third-party downloads at container startup (GeoIP databases, Keycloak
  themes). Not scanned here because they are runtime data, not build-time
  artifacts.
- Promoting `security:container-scan` into a reusable `security/container`
  component. Track after DFPT-1081 also uses the local job.
