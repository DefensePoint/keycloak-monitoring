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

Three unique Critical CVEs surfaced at baseline; all cleared by dependency
bumps landed alongside this MR.

| CVE            | Component               | 5.7.6 -> 5.9.0 / etc              | Where it surfaced       | Fix                                                    |
|----------------|-------------------------|-----------------------------------|-------------------------|--------------------------------------------------------|
| CVE-2026-33815 | github.com/jackc/pgx/v5 | 5.7.6 -> 5.9.0                    | Trivy fs, server image  | `go get github.com/jackc/pgx/v5@v5.9.0`.               |
| CVE-2026-33816 | github.com/jackc/pgx/v5 | 5.7.6 -> 5.9.0                    | Trivy fs, server image  | Same pgx bump.                                         |
| CVE-2025-68121 | Go stdlib               | 1.25.1 -> 1.25.14                 | Server image, web image | `GO_VERSION` bumped in `.gitlab-ci.yml` and `build/Dockerfile`. |

Post-bump local scan totals: Trivy fs 0 Critical / 20 High; Trivy image
(server) 0 Critical / 18 High; Trivy image (web) 0 Critical / 1 High.
Remaining High findings are informational and continue to be tracked by
Renovate.

## Per-scanner roll-up

| Scanner              | Critical | High | Medium | Info | Status                                          |
|----------------------|---------:|-----:|-------:|-----:|-------------------------------------------------|
| Trivy fs             |        0 |   20 |      0 |    0 | High findings are Go/npm dep bumps.             |
| Trivy image (server) |        0 |   18 |      0 |    0 | go-jose + x/{crypto,mod,net,text}.              |
| Trivy image (web)    |        0 |    1 |      0 |    0 | Single golang.org/x/text CVE.                   |
| Semgrep (SAST)       |        0 |    0 |      0 |    0 | 110 rules ran, no ERROR findings.               |
| Gitleaks             |        0 |    0 |      0 |    0 | 3 false positives allowlisted in `.gitleaksignore`. |
| Hadolint             |        0 |    3 |      0 |    3 | Best-practice warnings, not blocking.           |

Counts are per-scanner row totals; a CVE that appears in both Trivy fs and
Trivy image is counted in each. The **Critical: 0** at the top is the
deduplicated unique-CVE count after the dependency bumps.

## Local reproduction

```
make security-scan
```

Runs the same tools CI runs and writes JSON reports to `reports/security/`.
Per-tool sub-targets: `make sec-hadolint`, `make sec-trivy-fs`,
`make sec-gitleaks`, `make sec-semgrep`, `make sec-container`. Tool
prerequisites are checked by `make sec-preflight`.

## Dependency scan

CI runs Grype via the `security/dependencies` component; `make sec-trivy-fs`
runs Trivy filesystem mode locally. Both tools consume the same lockfile
manifests (`go.mod`, `web/package-lock.json`) and read from equivalent NVD /
GHSA feeds, so the CVE lists line up. The findings below were captured with
`trivy fs --scanners vuln --severity CRITICAL,HIGH --ignore-unfixed .`;
reconcile against the CI `grype-report.json` artifact if numbers diverge.

### Critical

None. The two pgx/v5 CVEs from the initial baseline (CVE-2026-33815 and
CVE-2026-33816) are fixed in this MR by bumping to 5.9.0.

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

0 Critical, 18 High after the pgx + Go toolchain bumps. Every remaining
High is a Go module still shy of the latest patched version:

- `github.com/go-jose/go-jose/v4`
- `golang.org/x/crypto`
- `golang.org/x/mod`
- `golang.org/x/net`
- `golang.org/x/text`

Renovate is expected to open individual patch MRs for each. None block
merge or deploy under the current gate.

### web (`gcr.io/distroless/static-debian12:nonroot`)

0 Critical, 1 High: `golang.org/x/text v0.33.0` needs 0.39.0
(CVE-2026-56852, denial of service via invalid UTF-8 input). Cleared by the
pending Renovate bump.

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
