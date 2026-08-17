# Contributing to KMT

Thank you for your interest in contributing!

## Getting Started

1. Fork the repository and create a branch from `main`.
2. Follow the setup instructions in the README to run the platform locally.
3. Make your changes, add tests where applicable, and ensure all existing tests pass.
4. Open a merge request with a clear description of the change.

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

Before opening a merge request, run the full gate. It is the same set of checks
CI runs:

```bash
make check           # tidy, fmt, vet, lint, test-all
```

Integration tests that need a live PostgreSQL are excluded from `make
test-backend` by the `-short` flag. Run them with `go test ./...` against a
database when you touch repository code.

## Code Style

- **Go 1.25+.** `gofmt` is enforced in CI and by the pre-commit hook. Lint with
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

## Merge Request Guidelines

- Keep merge requests focused on a single concern
- Include a test for any bug fix or new feature
- Update relevant documentation in `docs/` if behavior changes

## Reporting Issues

Open an issue with a clear description, steps to reproduce, and the versions you
are running, including your Keycloak version and whether it is upstream Keycloak
or Red Hat build of Keycloak.

For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of
opening a public issue.
