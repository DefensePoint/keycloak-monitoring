# Keycloak Monitoring Tool

A comprehensive monitoring and security compliance platform for Keycloak identity and access management systems. Built with Go and React, this platform provides real-time monitoring, automated security configuration scanning, alert management, and integrated notifications for multi-tenant Keycloak deployments.

## Features

### Core Capabilities

- **Multi-Tenant Monitoring**: Monitor multiple Keycloak instances from a single platform with isolated tenant data
- **Real-Time Event Tracking**: Comprehensive logging of logins, logouts, registrations, and security events with time-based filtering
- **Security Configuration Scanning**: Automated detection of misconfigurations and security vulnerabilities across realms, clients, and identity providers
- **Centralized Alert Management**: Track, acknowledge, resolve, and ignore security alerts with severity-based filtering
- **Notification Integrations**: Send alerts to Slack, Email, and automatically create GitLab issues with configurable severity thresholds
- **Dashboard Analytics**: Interactive charts and statistics showing authentication patterns and trends
- **User & Client Management**: Browse users, clients, and sessions across all realms and tenants
- **TimescaleDB Integration**: Efficient time-series data storage for high-volume event data
- **RESTful API**: Full-featured API for integration, automation, and programmatic access

### Security & Compliance Features

- **Realm Security Checks**: SSL requirements, brute force protection, password policies, email verification, audit logging
- **Client Security Checks**: Redirect URI validation, public client configurations, deprecated OAuth2 flows, HTTP detection
- **Identity Provider Checks**: SAML metadata validation, certificate expiration monitoring, federation configuration
- **Alert Lifecycle Management**: Active, Acknowledged, Resolved, and Ignored status tracking with audit trail
- **Configurable Severity Levels**: Fine-grained control over what gets alerted where (info, warning, error, critical)
- **Automated Remediation Guidance**: Each alert includes detailed recommendations for fixing the issue

### Technical Features

- **Role-Based Access Control (RBAC)**: Fine-grained permissions with three default roles (Admin, Operator, Viewer) and support for custom roles
- **Dual Authentication**: Supports both simple username/password and OAuth2/OIDC (Keycloak SSO)
- **Auto-Detection**: Automatically detects Keycloak version (17+ vs legacy /auth prefix)
- **Health Monitoring**: Continuous health checks and server status monitoring for all tenants
- **Configurable Polling**: Customizable intervals for metrics, events, health checks, and configuration scans
- **Tenant Isolation**: Complete data separation between monitored Keycloak instances with tenant-scoped permissions
- **Docker Support**: Full containerization with Docker and docker-compose for easy deployment
- **Production Ready**: Built-in logging, error handling, graceful shutdown, and connection pooling
- **PostgreSQL Storage**: Reliable data persistence with TimescaleDB extensions for time-series data
- **Session Management**: Secure session handling with configurable cookies and CSRF protection

## Architecture

The platform consists of the following main components:

```bash
┌─────────────────┐      ┌─────────────────┐      ┌──────────────────┐
│  Web Frontend   │─────▶│   API Server    │─────▶│   PostgreSQL     │
│   (React/TS)    │      │    (Go API)     │      │  (TimescaleDB)   │
│   Port 7880     │      │   Port 7888     │      │   Port 5432      │
└─────────────────┘      └─────────────────┘      └──────────────────┘
                                  │                         ▲
                                  │                         │ read-only
                                  ▼                         │
                         ┌─────────────────┐      ┌──────────────────┐
                         │    Keycloak     │      │   MCP Server     │
                         │  (Monitored)    │      │   Port 7889      │
                         └─────────────────┘      └──────────────────┘
```

### Components

- **API Server** (`cmd/server`): Go-based REST API server handling authentication, data retrieval, and Keycloak monitoring
- **Web Server** (`cmd/web`): Serves the React frontend and proxies API requests
- **MCP Server** (`cmd/mcp-server`): Read-only Model Context Protocol (MCP) server exposing monitoring data to MCP clients, disabled by default
- **Frontend** (`web/`): React + TypeScript dashboard with Recharts for visualization
- **Database**: PostgreSQL 17 with TimescaleDB for time-series data

For detailed architecture information, see the [architecture docs](docs/architecture.md).

### MCP Server

`cmd/mcp-server` exposes tenants, Keycloak realms, alerts, events and Adaptive Multi-Factor Authentication (AMFA) statistics to MCP clients such as Claude Code and Claude Desktop. Every tool is read-only, it runs as its own process alongside the API server, and it is disabled by default: set `mcp.enabled` in `config.yaml` to turn it on. Clients authenticate with personal access tokens and see only what the token and the acting user's RBAC roles allow. See [cmd/mcp-server/README.md](cmd/mcp-server/README.md) for the token lifecycle, client setup and tool reference.

## Quick Start

The fastest way to get started is with Docker. See the **[Quick Start Guide](docs/quickstart.md)** for detailed step-by-step instructions.

### Docker Deployment (Recommended)

```bash
# 1. Clone the repository
git clone https://github.com/DefensePoint/keycloak-monitoring.git
cd keycloak-monitoring

# 2. Copy and edit configuration
cp config.yaml.example config.yaml
# Edit config.yaml and add your Keycloak connection details

# 3. Set required admin credentials via environment variables
export MONITORING_AUTH_SIMPLE_DEFAULT_USER="admin"
export MONITORING_AUTH_SIMPLE_DEFAULT_PASS="YourS3cur3P@ssw0rd!"  # REQUIRED: Use a strong password
export MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL="admin@example.com"

# 4. Build and start services
make docker-build
make docker-up

# 5. Access the dashboard at http://localhost:7880
# Login with your configured admin credentials
# IMPORTANT: Change the password after first login; the initial password remains valid until changed
```

### What You Get

After following the quick start:

- Complete monitoring platform running in Docker
- PostgreSQL database with TimescaleDB
- Real-time monitoring of your Keycloak instance
- Automated security configuration scanning
- Web dashboard with analytics and alerts

### Next Steps

1. **Add more Keycloak instances** via the Tenants page or configuration file
2. **Configure notifications** for Slack, Email, or GitLab (see [quickstart guide](docs/quickstart.md#setting-up-notifications))
3. **Review security alerts** on the Alerts page
4. **Customize configuration checks** in `config.yaml`

See the **[Quick Start Guide](docs/quickstart.md)** for comprehensive setup instructions, including notification configuration and security best practices.

## Documentation

Comprehensive documentation is available in the `docs/` directory:

### Getting Started

- **[Quick Start Guide](docs/quickstart.md)** - Get up and running in 10 minutes
- **[Keycloak Setup Guide](docs/keycloak-setup.md)** - How to configure Keycloak for monitoring

### Configuration & Deployment

- **[Configuration Guide](docs/configuration.md)** - Detailed configuration reference
- **[RBAC Guide](docs/rbac.md)** - Role-Based Access Control configuration and management
- **[Deployment Guide](docs/deployment.md)** - Production deployment instructions
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions

### Development & Architecture

- **[Architecture Guide](docs/architecture.md)** - System design and component overview
- **[Engineering Guide](docs/engineering-guide.md)** - Development setup and workflow
- **[Hacking Guide](docs/hacking-guide.md)** - Code structure and contribution guidelines

## Development

### Project Structure

```bash
.
├── cmd/
│   ├── mcp-server/      # MCP server entry point
│   ├── server/          # API server entry point
│   └── web/             # Web server entry point
├── internal/            # Internal packages (not exported)
├── pkg/                 # Public packages (can be imported)
├── web/                 # React frontend
│   ├── src/
│   │   ├── components/  # React components
│   │   ├── pages/       # Page components
│   │   ├── services/    # API services
│   │   └── types/       # TypeScript types
│   └── dist/            # Built frontend (generated)
├── deployments/         # Deployment configurations
│   ├── local/           # Local docker-compose
│   └── server/          # Production docker-compose
├── docs/                # Documentation
├── config.yaml.example  # Configuration template
├── Dockerfile           # Multi-stage Docker build
├── Makefile             # Build and development commands
└── README.md            # This file
```

### Make Commands

Run `make help` to see all available commands

### Running Tests

```bash
# Run all tests
make test-all

# Run backend tests with coverage
make test-backend

# Run frontend tests
make test-frontend

# Run specific test
go test -v ./pkg/keycloak/...
```

For the full testing reference — including running backend tests via Docker without a local Go install, env-gated DB tests (`KMT_TEST_DATABASE_DSN`), and the build-tagged AMFA integration tests (`-tags=integration`, `AMFA_TEST_DSN`) — see [docs/engineering-guide.md § Testing](docs/engineering-guide.md#testing).

## Deployment

### Docker Compose (Recommended)

The easiest way to deploy is using Docker Compose:

```bash
# 1. Copy configuration
cp config.yaml.example config.yaml

# 2. Edit config.yaml with your settings

# 3. Build images
make docker-build

# 4. Start services
docker-compose -f deployments/local/docker-compose.yml up -d

# 5. Check logs
docker-compose -f deployments/local/docker-compose.yml logs -f
```

### Manual Deployment

See [docs/deployment.md](docs/deployment.md) for detailed deployment instructions including:

- Kubernetes deployment
- Systemd service configuration
- Reverse proxy setup (nginx/traefik)
- SSL/TLS configuration
- Production best practices

## Configuration

The application is configured via `config.yaml`. Key configuration sections include:

### Multi-Tenant Keycloak Monitoring

```yaml
keycloak:
  # Global settings (inherited by all tenants)
  global:
    polling:
      metrics_interval: 5m
      events_interval: 30s
      health_interval: 1m

    # Automated configuration checking
    config_checker:
      poll_interval: 5m
      checks:
        realm_security:
          enabled: true
          check_ssl_required: true
          check_brute_force: true
          min_password_length: 12
        client_security:
          enabled: true
          check_redirect_uris: true
          production_environment: true

  # Monitored Keycloak instances
  tenants:
    prod-keycloak:
      enabled: true
      name: "Production Keycloak"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      client_secret: "${KEYCLOAK_CLIENT_SECRET}"
      tags: ["production", "critical"]
```

### Notification Integrations

```yaml
notifications:
  # Send critical alerts to Slack
  slack:
    enabled: true
    webhook_url: "${SLACK_WEBHOOK_URL}"
    min_severity: "warning"  # warning, error, critical

  # Email for errors and critical
  email:
    enabled: true
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    from: "alerts@example.com"
    to: ["security-team@example.com"]
    min_severity: "error"  # error, critical only

  # Create GitLab issues for critical alerts
  gitlab:
    enabled: true
    url: "https://gitlab.com"
    token: "${GITLAB_TOKEN}"
    project_id: "12345"
    min_severity: "critical"  # critical only
```

### Authentication

```yaml
auth:
  simple:
    enabled: true
    # SECURITY: Set credentials via environment variables, NOT in config files
    # Leave these empty and use:
    #   MONITORING_AUTH_SIMPLE_DEFAULT_USER=admin
    #   MONITORING_AUTH_SIMPLE_DEFAULT_PASS=<strong-password>  # Must meet complexity requirements
    #   MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL=admin@example.com
    default_user: ""
    default_pass: ""
    default_email: ""
  oauth2:
    enabled: false
    provider_url: "https://keycloak.example.com/realms/master"
    client_id: "monitoring-dashboard"
    client_secret: "your-secret"
```

**Security Requirements for Initial Admin Password:**

- Minimum 12 characters
- At least one uppercase letter, one lowercase letter, one number, and one special character
- Cannot be a common weak password (e.g., admin123, password123, etc.)

Change the password after first login. The platform does not rotate it automatically, so the initial password remains valid until changed.

For complete configuration reference, see the [configuration documents](docs/configuration.md) or review the [configuration example](config.yaml.example).

### Environment Variables

All configuration can be overridden with environment variables using the prefix `MONITORING_`:

```bash
export MONITORING_DATABASE_HOST=localhost
export MONITORING_DATABASE_PASSWORD=secure_password
export MONITORING_KEYCLOAK_SERVER_URL=https://keycloak.example.com
export MONITORING_AUTH_SESSION_SECRET=$(openssl rand -base64 32)
```
