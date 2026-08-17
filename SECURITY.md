# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Reporting a Vulnerability

Please do **not** open a public issue for security vulnerabilities.

Report security issues by emailing **security@defensepoint.com**. Include:

- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Any suggested fix (optional)

We will acknowledge receipt within 48 hours and aim to release a fix within 14 days for critical issues.

## Scope

KMT connects to systems that hold identity data. When reporting, it helps to
say which trust boundary is involved:

- The Keycloak Admin API client credentials KMT uses to monitor a tenant
- The read-only connection KMT opens to an Adaptive MFA database
- The KMT web session, its RBAC model, and tenant isolation between monitored
  Keycloak instances
- Notification delivery (Slack, email, GitLab issues), which can carry alert
  content off the platform
