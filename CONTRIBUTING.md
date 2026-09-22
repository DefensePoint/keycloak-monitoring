# Contributing to KMT

Thank you for your interest in contributing!

## Getting Started

1. Fork the repository and create a branch from `main`.
2. Follow the setup instructions in the README to run the platform locally.
3. Make your changes, add tests where applicable, and ensure all existing tests pass.
4. Open a pull request with a clear description of the change.

## Development Setup

KMT is a Go API server plus a React frontend, backed by PostgreSQL with
TimescaleDB.

```bash
# Install toolchains: Go 1.25+, Node 20+, Docker
make install-frontend

# Run the whole stack in Docker
cp config.yaml.example config.yaml   # then edit it
make docker-build
make docker-up
```

To iterate on the frontend against a running API server:

```bash
make dev-frontend
```

## Running Tests

```bash
make test-backend    # Go tests, skips integration tests
make test-frontend   # Vitest
make test-all        # both
```

Before opening a pull request, run the full gate:

```bash
make check           # tidy, fmt, vet, lint, test-all
```

Tests that need a live PostgreSQL skip silently unless you point them at one,
so `make test-backend` passing does not mean they ran. Set
`KMT_TEST_DATABASE_DSN` and run them whenever you touch repository code. A
second set drops the schema and needs both `KMT_DESTRUCTIVE_TEST_DATABASE_DSN`
and a run without `-short`; aim that one at a throwaway database only.
`docs/engineering-guide.md` has the full table.

## Code Style

- **Go 1.25+.** `gofmt` is enforced by the pre-commit hook. Lint with
  `golangci-lint`.
- **TypeScript + React** in `web/`, with Vite, MUI, and Recharts. ESLint and
  Prettier are enforced.
- New Keycloak configuration checks go in `configcheck/`, event-pattern checks
  in `eventscheck/`, and Adaptive MFA risk checks in `amfacheck/`.
- Access to Adaptive MFA's database is **read-only by construction**. Models in
  `amfa/postgres` carry the `gorm:"->"` tag on every field and `AutoMigrate` is
  never called on that connection. Keep it that way: KMT observes AMFA, it never
  writes to it.
- All new code needs corresponding tests.

## Commit Messages

This repository uses [Conventional Commits](https://www.conventionalcommits.org),
enforced by commitlint via a Husky hook. Allowed types: `feat`, `fix`, `docs`,
`style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.

```
feat(configcheck): flag realms without brute force protection
```

## Pull Request Guidelines

- Keep pull requests focused on a single concern
- Include a test for any bug fix or new feature
- Update relevant documentation in `docs/` if behavior changes

## Developer Certificate of Origin (DCO)

This project requires a DCO sign-off on every commit. By signing off, you
certify that you wrote the change or otherwise have the right to submit it
under the project's open-source license.

Add a `Signed-off-by` line to each commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

Git can do this automatically with `git commit -s`.

By signing off you agree to the
[Developer Certificate of Origin](https://developercertificate.org/).

## Reporting Issues

Open an issue with a clear description, steps to reproduce, and the versions you
are running, including your Keycloak version.

For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of
opening a public issue.
