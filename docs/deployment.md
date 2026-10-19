# Deployment Guide

Complete guide for deploying the Keycloak Monitoring Tool to production environments.

## Table of Contents

- [Overview](#overview)
- [Deployment Options](#deployment-options)
- [Docker Compose Deployment](#docker-compose-deployment)
- [Pinned Images](#pinned-images)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Manual Deployment](#manual-deployment)
- [Systemd Services](#systemd-services)
- [Reverse Proxy Setup](#reverse-proxy-setup)
- [SSL/TLS Configuration](#ssltls-configuration)
- [Database Setup](#database-setup)
- [Monitoring and Logging](#monitoring-and-logging)
- [Security Hardening](#security-hardening)
- [Performance Tuning](#performance-tuning)

## Overview

The Keycloak Monitoring Tool can be deployed in various environments. This guide covers production-ready deployment strategies with emphasis on security, reliability, and performance.

### Deployment Architecture

```bash
                          ┌─────────────────┐
                          │   Load Balancer │
                          │  (nginx/traefik)│
                          └────────┬────────┘
                                   │ HTTPS
                ┌──────────────────┼─────────────────┐
                │                  │                 │
                ▼                  ▼                 ▼
         ┌──────────┐       ┌──────────┐      ┌──────────┐
         │   Web    │       │   Web    │      │   Web    │
         │  Server  │       │  Server  │      │  Server  │
         │  (7880)  │       │  (7880)  │      │  (7880)  │
         └─────┬────┘       └─────┬────┘      └─────┬────┘
               │                  │                 │
               └──────────────────┼─────────────────┘
                                  │ Internal
                ┌─────────────────┼─────────────────┐
                │                 │                 │
                ▼                 ▼                 ▼
         ┌──────────┐      ┌──────────┐     ┌──────────┐
         │   API    │      │   API    │     │   API    │
         │  Server  │      │  Server  │     │  Server  │
         │  (7888)  │      │  (7888)  │     │  (7888)  │
         └─────┬────┘      └─────┬────┘     └─────┬────┘
               │                 │                │
               └─────────────────┼────────────────┘
                                 │
                                 ▼
                          ┌──────────────┐
                          │  PostgreSQL  │
                          │ (TimescaleDB)│
                          │    Cluster   │
                          └──────────────┘
```

## Deployment Options

### 1. Docker Compose (Recommended for Small-Medium)

**Pros**:

- Simple setup and management
- Good for single server deployments
- Easy configuration
- Built-in networking

**Cons**:

- Limited horizontal scaling
- Single point of failure without orchestration

**Best for**: Small to medium deployments, development, testing

### 2. Kubernetes (Recommended for Large Scale)

**Pros**:

- Horizontal scaling
- High availability
- Self-healing
- Rolling updates

**Cons**:

- Complex setup
- Higher resource overhead
- Steeper learning curve

**Best for**: Large-scale deployments, multi-environment setups

### 3. Manual Deployment

**Pros**:

- Full control
- Lightweight
- No container overhead

**Cons**:

- Manual management
- More complex updates
- Environment-specific configuration

**Best for**: Bare metal servers, custom infrastructure

## Docker Compose Deployment

### Prerequisites

- Docker 20.10+
- Docker Compose 2.0+
- 2GB+ RAM
- 10GB+ disk space

### Step 1: Prepare Configuration

```bash
# Clone repository
git clone https://github.com/DefensePoint/keycloak-monitoring.git
cd keycloak-monitoring

# Create configuration
cp config.yaml.example config.yaml

# Edit configuration
vim config.yaml
```

**Production config.yaml**:

```yaml
http:
  server:
    host: "0.0.0.0"
    port: 7888

web:
  server:
    host: "0.0.0.0"
    port: 7880
    api_host_url: "http://api-server:7888"

database:
  host: "kmt-postgres"
  port: 5432
  database: "monitoring"
  user: "monitoring"
  # Use environment variable
  ssl_mode: "require"
  max_conns: 50

keycloak:
  tenants:
    prod-keycloak:
      name: "Production Keycloak"
      server_url: "https://keycloak.example.com"
      admin_realm: "master"
      client_id: "monitoring-service"
      # Filled in from the PROD_KEYCLOAK_CLIENT_SECRET environment variable
      client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"

auth:
  simple:
    enabled: false
  oauth2:
    enabled: true
    provider_url: "https://keycloak.example.com/realms/production"
    client_id: "monitoring-dashboard"
    # Set by MONITORING_AUTH_OAUTH2_CLIENT_SECRET; the key must stay present
    # (empty is fine) or the override is ignored.
    client_secret: ""
    redirect_url: "https://monitoring.example.com/auth/callback"
    admin_users:
      - "operator@example.com"
  session:
    # Set by MONITORING_AUTH_SESSION_SECRET; the key must stay present
    # (empty is fine) or the override is ignored.
    secret: ""

logging:
  level: "info"
  format: "json"
```

### Step 2: Create Environment File

```bash
# Create .env file for secrets
cat > .env <<EOF
MONITORING_DATABASE_PASSWORD=your_secure_db_password
PROD_KEYCLOAK_CLIENT_SECRET=keycloak_client_secret
MONITORING_AUTH_OAUTH2_CLIENT_SECRET=oauth2_client_secret
MONITORING_AUTH_SESSION_SECRET=$(openssl rand -base64 32)
EOF

# Secure the file
chmod 600 .env
```

### Step 3: Build Images

```bash
# Build all Docker images
make docker-build

# Or build individually
docker build --build-arg SERVICE=server -t kmt-server:latest .
docker build --build-arg SERVICE=web -t kmt-web:latest .
```

The AMFA geolocation map draws from boundary data bundled in the web image, so
it needs no tile-service key and no network access to a third party. See
`web/public/geo/SOURCE.md` for the data's provenance and licence.

### Step 4: Start Services

```bash
# Start with docker-compose
docker-compose -f deployments/local/docker-compose.yml up -d

# Check status
docker-compose -f deployments/local/docker-compose.yml ps

# View logs
docker-compose -f deployments/local/docker-compose.yml logs -f
```

### Step 5: Verify Deployment

```bash
# Check API health
curl http://localhost:7888/health

# Check web server
curl http://localhost:7880

# Access dashboard
# Open browser to http://localhost:7880
```

### Production docker-compose.yml

```yaml
version: '3.8'

services:
  postgres:
    image: timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f
    container_name: kmt-postgres
    restart: unless-stopped
    environment:
      POSTGRES_DB: monitoring
      POSTGRES_USER: monitoring
      POSTGRES_PASSWORD: ${MONITORING_DATABASE_PASSWORD}
    volumes:
      - postgres_data:/var/lib/postgresql/data
    networks:
      - monitoring_network
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U monitoring"]
      interval: 10s
      timeout: 5s
      retries: 5

  api-server:
    image: kmt-server:latest
    container_name: kmt-server
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      - MONITORING_DATABASE_PASSWORD=${MONITORING_DATABASE_PASSWORD}
      - PROD_KEYCLOAK_CLIENT_SECRET=${PROD_KEYCLOAK_CLIENT_SECRET}
      - MONITORING_AUTH_OAUTH2_CLIENT_SECRET=${MONITORING_AUTH_OAUTH2_CLIENT_SECRET}
      - MONITORING_AUTH_SESSION_SECRET=${MONITORING_AUTH_SESSION_SECRET}
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    networks:
      - monitoring_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:7888/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  web:
    image: kmt-web:latest
    container_name: kmt-web
    restart: unless-stopped
    depends_on:
      api-server:
        condition: service_healthy
    ports:
      - "127.0.0.1:7880:7880"  # host only; put a reverse proxy in front
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    networks:
      - monitoring_network

volumes:
  postgres_data:
    driver: local

networks:
  monitoring_network:
    driver: bridge
```

### Compose servers (`deployments/server`)

`deployments/server/docker-compose.yml` is the server manifest. No pipeline deploys it. Set `KMT_API_IMAGE`, `KMT_WEB_IMAGE` and `KMT_MCP_IMAGE` to your image references (they default to the local `kmt-server:latest`, `kmt-web:latest` and `kmt-mcp-server:latest` tags), and put `config.yaml` next to it. The `api-server`, `mcp-server` and `web` services all load their environment from `kmt.env` in the same directory. Use it for the variables that `config.yaml`'s `${NAME}` placeholders reference, and for `MONITORING_*` overrides:

```bash
# On the server, in the deploy directory
sudo tee kmt.env >/dev/null <<'EOF'
PROD_KEYCLOAK_CLIENT_SECRET='value-with-$-kept'
MONITORING_AUTH_SIMPLE_DEFAULT_PASS='Init!2026-Qx7nK2'
EOF
sudo chown root:docker kmt.env
sudo chmod 640 kmt.env
```

- Create the file, even empty, before the first `docker compose up`: Compose refuses to start a service whose `env_file` is missing. After editing it, run `docker compose up -d --force-recreate`.
- Docker Compose substitutes `$` in env-file values: an unquoted or double-quoted `kc-$ecret` reaches the container as `kc-`. Single-quote any value that contains `$`, or write `$$`.
- All three services load the same `config.yaml`, which is why all three read the file. A placeholder whose variable is missing keeps its literal `${NAME}` text and is logged as a warning at startup.

The `postgres` service does not read `kmt.env`. Compose fills its `${POSTGRES_USER}` and `${POSTGRES_PASSWORD}` from the shell or from a `.env` file next to the compose file. Without them Postgres refuses to start ("You must specify POSTGRES_PASSWORD"). Create `.env` before the first start:

```bash
# On the server, in the deploy directory
sudo tee .env >/dev/null <<'EOF'
POSTGRES_USER=monitoring
POSTGRES_PASSWORD='a-strong-password'
EOF
sudo chown root:docker .env
sudo chmod 640 .env
```

Compose reads `.env` for every command, `ps` and `logs` included. Depending on its version, it reads `kmt.env` for some or all commands too, so the `docker` group needs to read both. Membership of that group already gives root on the host, so this exposes nothing new. An admin outside the group runs every `docker compose` command with `sudo`.

`config.yaml`'s `database.user` and `database.password` must match these. Supply the password to the application through `MONITORING_DATABASE_PASSWORD` in `kmt.env`, with the same value.

The Compose file runs the web server on 7880, the port it publishes and Traefik routes to, by setting `MONITORING_WEB_SERVER_PORT` on the `web` service; `config.yaml.example` defaults it to 3000.

Only Traefik is public. The file does not publish Postgres or the MCP server, and binds the API (7888) and the web server (7880) to `127.0.0.1`, so other machines can't reach them (see [Firewall Rules](#1-firewall-rules) for why ufw alone does not do this). On the server itself:

```bash
curl -fsS http://127.0.0.1:7888/health          # API health; /metrics is on the same port
curl -fsS -o /dev/null http://127.0.0.1:7880/   # web server, bypassing Traefik
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" monitoring'
```

The single quotes leave `$POSTGRES_USER` to the container, where it is set. Unquoted, the admin's own shell expands it, normally to nothing, and psql connects as `root` and fails.

A metrics scraper on the host reads `http://127.0.0.1:7888/metrics`. One in a container joins `kmt-network` and reads `http://api-server:7888/metrics`. An admin on another machine uses an SSH tunnel: `ssh -L 7888:127.0.0.1:7888 <server>`.

## Pinned Images

Every image this repository ships or tells you to run is pinned to a release tag and the digest that tag resolved to. The tag says which version it is, the digest is what actually gets pulled, so the same file gives the same software on any day and on any host. The pins live in `deployments/server/docker-compose.yml`, the three files under `deployments/local` and `build/Dockerfile`.

### Moving to a newer pinned version

1. Read the release notes for the version you are moving to.
2. Resolve its digest and confirm the index covers the platforms you build for:

   ```bash
   docker buildx imagetools inspect timescale/timescaledb:2.31.0-pg16
   ```

3. Replace both halves of the reference, the tag and the digest, everywhere the old one appears. A tag that no longer matches its digest is the one mistake this convention cannot catch for you:

   ```bash
   grep -rn "timescale/timescaledb:" deployments build docs
   ```

4. Open a pull request, so CI builds and tests on the new pin before anyone deploys it.

### Moving an existing server to a newer TimescaleDB

A PostgreSQL data directory is compatible across PostgreSQL 16 minor releases, so the database starts on the new image without `pg_upgrade`. The TimescaleDB extension is different: restarting on a newer image leaves the extension at the version the data was created with until you update it explicitly.

Never move a database onto a TimescaleDB older than the one its data was created with. It can refuse to start.

1. Check what the server runs today, before changing anything:

   ```bash
   docker compose exec postgres psql -U monitoring -d monitoring \
     -c "SELECT extversion FROM pg_extension WHERE extname='timescaledb'" \
     -c "SHOW server_version"
   ```

   If that extension version is newer than the one in the Compose file, stop. Pin the Compose file forward to at least what the server already runs.

2. Back up the database, then pull and restart on the pinned image:

   ```bash
   docker compose pull postgres
   docker compose up -d postgres
   ```

3. Update the extension in every database that has it. Run it as the first statement of a fresh session, which a standalone `-c` gives you:

   ```bash
   docker compose exec postgres psql -U monitoring -d monitoring \
     -c "ALTER EXTENSION timescaledb UPDATE"
   ```

4. Verify the new version and that the hypertables came through:

   ```bash
   docker compose exec postgres psql -U monitoring -d monitoring \
     -c "SELECT extversion FROM pg_extension WHERE extname='timescaledb'" \
     -c "SELECT hypertable_name FROM timescaledb_information.hypertables"
   ```

5. Restart the application services so they reconnect: `docker compose up -d`.

## Kubernetes Deployment

Use Helm if you already run charts. The kubectl samples below are the same shape without Helm. Both paths expect images you built and pushed (see `build/Dockerfile` and `make docker-build`).

### Workloads

| Workload | Port | Probe | Notes |
| --- | --- | --- | --- |
| API server | 7888 | liveness `/health`, readiness `/ready` | Owns schema migrations. Run **one** replica with `Recreate`. |
| Web | 7880 | `/healthz` | The only service behind Ingress. Proxies `/api/` and `/auth/` to the API server. |
| MCP server | 7889 | liveness `/health`, readiness `/ready` | Optional. Off by default. Keep ClusterIP; do not put it on the Ingress. |
| PostgreSQL | 5432 | `pg_isready` | TimescaleDB is required. A plain Postgres image will fail startup. |

Do not probe `/api/health`. That path is not a health endpoint.

`/ready` answers 503 while the server's database does not answer within 2 seconds, and 200 again once it does. `/health` answers 200 while the process serves requests, database or not. Keep it that way round: a liveness probe on `/ready` restarts every pod during a database outage, which does not bring the database back. A tenant's Keycloak or AMFA being unreachable does not affect either; the tenant's health alerts report that. The Compose files' container healthcheck (`-healthcheck`) probes `/ready`, so `docker compose ps` shows the API and MCP servers unhealthy while the database is unreachable.

Environment overrides, the keys a ConfigMap must keep present for values
injected from a Secret, secret requirements, and initial administrator password
rules are defined in the [configuration reference](configuration.md#deployment-variable-requirements).

### Helm (recommended)

This repository does not ship a Helm chart. Maintain a chart with this layout and values, then install it:

```text
charts/kmt/
  Chart.yaml
  values.yaml
  templates/
    configmap.yaml      # config.yaml mounted at /app/config.yaml
    secret.yaml         # or your secret manager
    postgres.yaml       # skip if you bring your own TimescaleDB
    server.yaml
    web.yaml
    mcp-server.yaml     # gated on mcp.enabled
    ingress.yaml        # fronts web only
```

**values.yaml** (trim and point images at your registry):

```yaml
server:
  replicas: 1
  image: your-registry/kmt-server:TAG
  strategy: Recreate
  startupProbe:
    httpGet: { path: /health, port: http }
    periodSeconds: 10
    failureThreshold: 30
  readinessProbe:
    httpGet: { path: /ready, port: http }
  livenessProbe:
    httpGet: { path: /health, port: http }

web:
  replicas: 2
  image: your-registry/kmt-web:TAG
  readinessProbe:
    httpGet: { path: /healthz, port: http }
  livenessProbe:
    httpGet: { path: /healthz, port: http }

mcp:
  enabled: false
  replicas: 0
  image: your-registry/kmt-mcp-server:TAG
  readinessProbe:
    httpGet: { path: /ready, port: http }
  livenessProbe:
    httpGet: { path: /health, port: http }

postgres:
  enabled: true
  image: timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f
  database: monitoring
  user: monitoring

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: monitoring.example.com
      serviceName: web
      servicePort: 7880

config:
  http:
    server: { host: "0.0.0.0", port: 7888 }
  web:
    server:
      host: "0.0.0.0"
      port: 7880
      api_host_url: "http://api-server:7888"
  mcp:
    enabled: false
    port: 7889
```

Render `config:` into the ConfigMap verbatim. When you enable MCP, set both `mcp.enabled` in values **and** `config.mcp.enabled`; the binary exits immediately if the config says disabled.

```bash
helm upgrade --install kmt ./charts/kmt \
  --namespace monitoring-platform --create-namespace \
  -f values.yaml
```

### kubectl

Same workloads, no chart. Replace `TAG` and the secret values.

```bash
kubectl create namespace monitoring-platform

kubectl create configmap monitoring-config \
  --from-file=config.yaml \
  -n monitoring-platform

kubectl create secret generic monitoring-secrets \
  --from-literal=database-password='...' \
  --from-literal=session-secret="$(openssl rand -base64 32)" \
  --from-literal=encryption-key="$(openssl rand -base64 32)" \
  --from-literal=keycloak-client-secret='...' \
  --from-literal=initial-admin-password='Init!2026-Qx7nK2' \
  -n monitoring-platform
```

**postgres.yaml**

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-pvc
  namespace: monitoring-platform
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 10Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: monitoring-platform
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
        - name: postgres
          image: timescale/timescaledb:2.30.2-pg16@sha256:6f139d56042989bd35f50ba5986e492cd32e35cc6778c50be2659add298dc09f
          env:
            - name: POSTGRES_DB
              value: monitoring
            - name: POSTGRES_USER
              value: monitoring
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: database-password
            # New volumes only. An existing cluster at the mount root
            # is invisible if PGDATA moves to a subdirectory.
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          ports:
            - containerPort: 5432
          volumeMounts:
            - name: postgres-storage
              mountPath: /var/lib/postgresql/data
      volumes:
        - name: postgres-storage
          persistentVolumeClaim:
            claimName: postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: monitoring-platform
spec:
  selector:
    app: postgres
  ports:
    - port: 5432
      targetPort: 5432
```

`PGDATA` in a subdirectory is for **new** volumes. Kubernetes PVCs on ext4 often have `lost+found` at the mount root, which makes initdb refuse the directory. An existing database already at `/var/lib/postgresql/data` becomes invisible if you change `PGDATA`; move the data or keep the old value.

**api-server.yaml**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: monitoring-platform
spec:
  replicas: 1
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
    spec:
      containers:
        - name: api-server
          image: your-registry/kmt-server:TAG
          env:
            - name: MONITORING_DATABASE_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: database-password
            - name: MONITORING_AUTH_SESSION_SECRET
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: session-secret
            - name: MONITORING_SECURITY_ENCRYPTION_KEY
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: encryption-key
            - name: PROD_KEYCLOAK_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: keycloak-client-secret
            - name: MONITORING_AUTH_SIMPLE_DEFAULT_USER
              value: operator
            - name: MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL
              value: operator@example.com
            - name: MONITORING_AUTH_SIMPLE_DEFAULT_PASS
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: initial-admin-password
          ports:
            - name: http
              containerPort: 7888
          startupProbe:
            httpGet:
              path: /health
              port: http
            periodSeconds: 10
            failureThreshold: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: http
          livenessProbe:
            httpGet:
              path: /health
              port: http
          volumeMounts:
            - name: config
              mountPath: /app/config.yaml
              subPath: config.yaml
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: monitoring-config
---
apiVersion: v1
kind: Service
metadata:
  name: api-server
  namespace: monitoring-platform
spec:
  selector:
    app: api-server
  ports:
    - port: 7888
      targetPort: http
```

`PROD_KEYCLOAK_CLIENT_SECRET` only reaches a tenant through a `client_secret: "${PROD_KEYCLOAK_CLIENT_SECRET}"` placeholder in its `config.yaml` block. See the [configuration reference](configuration.md#deployment-variable-requirements).

**web.yaml**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: monitoring-platform
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: web
          image: your-registry/kmt-web:TAG
          ports:
            - name: http
              containerPort: 7880
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
          volumeMounts:
            - name: config
              mountPath: /app/config.yaml
              subPath: config.yaml
              readOnly: true
      volumes:
        - name: config
          configMap:
            name: monitoring-config
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: monitoring-platform
spec:
  selector:
    app: web
  ports:
    - port: 7880
      targetPort: http
```

**ingress.yaml** (fronts web only)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: monitoring-ingress
  namespace: monitoring-platform
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [monitoring.example.com]
      secretName: monitoring-tls
  rules:
    - host: monitoring.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 7880
```

```bash
kubectl apply -f postgres.yaml -f api-server.yaml -f web.yaml -f ingress.yaml
kubectl get pods,svc,ingress -n monitoring-platform
```

Optional MCP: same shape as the API server, image `your-registry/kmt-mcp-server:TAG`, container port 7889, probes `/health` and `/ready`, Service ClusterIP only. Set `mcp.enabled: true` in `config.yaml` or the process exits.

## Manual Deployment

### Step 1: Build Binaries

```bash
# Build backend
make build-server
make build-web

# Build frontend
cd web && npm install && npm run build

# Verify binaries
ls -lh bin/
```

### Step 2: Install

```bash
# Create installation directory
sudo mkdir -p /opt/monitoring-platform
sudo mkdir -p /opt/monitoring-platform/bin
sudo mkdir -p /opt/monitoring-platform/web/dist

# Copy binaries
sudo cp bin/server /opt/monitoring-platform/bin/
sudo cp bin/web /opt/monitoring-platform/bin/
sudo cp -r web/dist/* /opt/monitoring-platform/web/dist/

# Copy configuration
sudo cp config.yaml /opt/monitoring-platform/

# Set permissions
sudo chmod +x /opt/monitoring-platform/bin/*
```

### Step 3: Create User

```bash
sudo useradd -r -s /bin/false -d /opt/monitoring-platform monitoring
sudo chown -R monitoring:monitoring /opt/monitoring-platform
```

## Systemd Services

### API Server Service

Unit files are world-readable. Put secrets in a 0600 EnvironmentFile. The web unit does not seed simple-auth.

```bash
sudo install -d -m 0750 -o monitoring -g monitoring /etc/monitoring-platform
sudo tee /etc/monitoring-platform/secrets.env >/dev/null <<'EOF'
MONITORING_DATABASE_PASSWORD=your_password
PROD_KEYCLOAK_CLIENT_SECRET=keycloak_client_secret
MONITORING_AUTH_SIMPLE_DEFAULT_PASS=Init!2026-Qx7nK2
EOF
sudo chmod 0600 /etc/monitoring-platform/secrets.env
sudo chown monitoring:monitoring /etc/monitoring-platform/secrets.env
```

**/etc/systemd/system/monitoring-api.service**:

```ini
[Unit]
Description=Keycloak Monitoring Tool API Server
After=network.target postgresql.service

[Service]
Type=simple
User=monitoring
Group=monitoring
WorkingDirectory=/opt/monitoring-platform
ExecStart=/opt/monitoring-platform/bin/server -config /opt/monitoring-platform/config.yaml
Restart=always
RestartSec=5s

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/monitoring-platform

# Environment
EnvironmentFile=/etc/monitoring-platform/secrets.env
Environment="MONITORING_AUTH_SIMPLE_DEFAULT_USER=operator"
Environment="MONITORING_AUTH_SIMPLE_DEFAULT_EMAIL=operator@example.com"

[Install]
WantedBy=multi-user.target
```

### Web Server Service

**/etc/systemd/system/monitoring-web.service**:

```ini
[Unit]
Description=Keycloak Monitoring Tool Web Server
After=network.target monitoring-api.service

[Service]
Type=simple
User=monitoring
Group=monitoring
WorkingDirectory=/opt/monitoring-platform
ExecStart=/opt/monitoring-platform/bin/web -config /opt/monitoring-platform/config.yaml
Restart=always
RestartSec=5s

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

### Enable and Start

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable services
sudo systemctl enable monitoring-api
sudo systemctl enable monitoring-web

# Start services
sudo systemctl start monitoring-api
sudo systemctl start monitoring-web

# Check status
sudo systemctl status monitoring-api
sudo systemctl status monitoring-web

# View logs
sudo journalctl -u monitoring-api -f
sudo journalctl -u monitoring-web -f
```

## Reverse Proxy Setup

### Nginx Configuration

**/etc/nginx/sites-available/monitoring**:

```nginx
upstream api_backend {
    server localhost:7888;
    keepalive 32;
}

upstream web_backend {
    server localhost:7880;
    keepalive 32;
}

server {
    listen 80;
    server_name monitoring.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name monitoring.example.com;

    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/monitoring.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/monitoring.example.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Security Headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # Logging
    access_log /var/log/nginx/monitoring-access.log;
    error_log /var/log/nginx/monitoring-error.log;

    # Proxy to web server
    location / {
        proxy_pass http://web_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        # $http_host keeps the port; $host drops it, and the API refuses a
        # browser write whose Origin port differs from Host.
        proxy_set_header Host $http_host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # chi RealIP prefers True-Client-IP over X-Real-IP. Clear it so a
        # client cannot set the logged address behind this proxy.
        proxy_set_header True-Client-IP "";
    }

    # Client max body size
    client_max_body_size 10M;
}
```

Enable and reload:

```bash
sudo ln -s /etc/nginx/sites-available/monitoring /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### Upgrade: nginx must forward the port in Host

The API refuses a POST, PUT or DELETE from a browser page on another origin. Over plain HTTP, browsers other than on localhost send no `Sec-Fetch-Site`, so the API compares the `Origin` header with `Host`, port included. nginx's `$host` drops the port: behind `proxy_set_header Host $host;` on a non-standard port, such as `http://kmt.example.com:8080`, the UI's own login, edits and logout are refused with 403 and logged as `Cross-origin request refused`.

Before upgrading, change the line to `proxy_set_header Host $http_host;` and reload nginx. A site on HTTPS, or on port 80 or 443, works either way, but `$http_host` is correct for both.

## SSL/TLS Configuration

### Using Let's Encrypt

```bash
# Install certbot
sudo apt-get install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d monitoring.example.com

# Auto-renewal
sudo certbot renew --dry-run
```

## Database Setup

### Upgrade: no built-in database password

There is no built-in `database.password` any more. A server whose config
sets none used to connect with `monitoring_password`, a password this project
publishes; it now refuses to start with `SECURITY ERROR: database.password is
not set`. Set the real password in `config.yaml` or through
`MONITORING_DATABASE_PASSWORD` before upgrading. The same applies to
`mcp.database.password`. A server still configured with `monitoring_password`
starts, and logs a warning to use a password of its own.

### Upgrade: database.ssl_mode defaults to require

If `database.ssl_mode` is omitted, the server now connects with `require`.
Before this, the omitted value was `disable`. A Postgres instance with TLS
off will refuse the connection, and the error mentions TLS rather than the
default. Set `ssl_mode: "disable"` for local and private-network databases
that do not speak TLS, or `MONITORING_DATABASE_SSL_MODE=disable`. Deployed
configs that already set `require` are unchanged.

### Upgrade: databases from early single-tenant releases

Those releases monitored a single Keycloak, configured under
`keycloak.server_url`, and their tables carry no tenant. For the first start
on such a database, configure that Keycloak as the only tenant under
`keycloak.tenants`. The server then:

- gives every realm, alert and event row to that tenant;
- removes the unique indexes that made a realm ID, realm name or alert ID
  unique across every tenant;
- renames `keycloak_events`, `keycloak_metrics` and `keycloak_health` to
  `<table>_pre_tenant` and creates them again per tenant. The old Keycloak
  event, metrics and health history stays in the `_pre_tenant` tables but is
  not shown; drop them once it is no longer wanted.

With no tenant or several tenants configured, the server refuses to start
and says so, rather than guess who the rows belong to. Further tenants can be
added after the first successful start. Databases created by any KMT release are not
affected.

### PostgreSQL with TimescaleDB

```bash
# Install PostgreSQL
sudo apt-get install postgresql-17

# Install TimescaleDB
sudo add-apt-repository ppa:timescale/timescaledb-ppa
sudo apt-get update
sudo apt-get install timescaledb-2-postgresql-17

# Configure
sudo timescaledb-tune

# Create database
sudo -u postgres psql
CREATE DATABASE monitoring;
CREATE USER monitoring WITH PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE monitoring TO monitoring;
\c monitoring
CREATE EXTENSION IF NOT EXISTS timescaledb;
\q
```

### Database Backup

```bash
# Backup script
#!/bin/bash
BACKUP_DIR="/var/backups/monitoring-db"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR
# Postgres is not published, so dump through the container
docker compose -f /opt/kmt/docker-compose.yml exec -T postgres \
  sh -c 'pg_dump -U "$POSTGRES_USER" monitoring' | gzip > $BACKUP_DIR/monitoring_$DATE.sql.gz

# Keep only last 30 days
find $BACKUP_DIR -name "monitoring_*.sql.gz" -mtime +30 -delete
```

Add to crontab:

```bash
0 2 * * * /usr/local/bin/backup-monitoring-db.sh
```

## Security Hardening

### 1. Firewall Rules

ufw does not protect ports that Docker publishes. Docker writes its own iptables rules for a published port, and they take effect before ufw's, so `ufw deny 5432/tcp` leaves a published `5432:5432` reachable from other machines.

Keep services private in the Compose file instead:

- Don't publish a port that only containers use. `deployments/server/docker-compose.yml` publishes nothing for Postgres or the MCP server.
- Bind a port that only the host needs to loopback, such as `"127.0.0.1:7888:7888"`. Other machines can't reach it.
- Let the reverse proxy (Traefik) be the only public entry point.

ufw still protects services that run on the host itself, so keep it for SSH and anything else outside Docker:

```bash
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 443/tcp  # HTTPS (Traefik)
sudo ufw allow 80/tcp   # HTTP, redirected to HTTPS
sudo ufw enable
```

If a port must be published on a public interface, publish it on IPv4 only and filter it in Docker's `DOCKER-USER` chain, which Docker evaluates before its own rules. Publish it as `"0.0.0.0:7888:7888"`, not `"7888:7888"`: the short form also listens on IPv6, and those connections never pass through `DOCKER-USER`, so the rule below would leave IPv6 open.

`--ctdir ORIGINAL` limits the rule to the client's packets; without it the rule also drops the replies, and even an allowed client is refused. `-i` limits it to connections arriving on the public interface; without it the rule also drops connections the server's containers open to port 7888 on other machines. For example, to allow only one subnet, replacing `eth0` with the server's public interface:

```bash
sudo iptables -I DOCKER-USER -i eth0 -p tcp -m conntrack --ctorigdstport 7888 --ctdir ORIGINAL ! -s 10.0.0.0/8 -j DROP
```

The rule is gone after a reboot: Docker creates `DOCKER-USER` empty when it starts. Add it again each time Docker starts with a systemd unit, using the same rule:

```ini
# /etc/systemd/system/kmt-firewall.service
[Unit]
Description=DOCKER-USER rule for Keycloak Monitoring Tool
After=docker.service
Requires=docker.service
PartOf=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'R="-i eth0 -p tcp -m conntrack --ctorigdstport 7888 --ctdir ORIGINAL ! -s 10.0.0.0/8 -j DROP"; iptables -C DOCKER-USER $R 2>/dev/null || iptables -I DOCKER-USER $R'

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now kmt-firewall.service
```

The `-C` check adds the rule only when it is missing, so running the unit again never stacks a second copy.

Check the result from other machines, not from the host: from an address outside the allowed subnet, `nc -zv <server> 7888` must fail, over IPv6 too if the server has an IPv6 address; from an address inside it, it must connect.

### 2. Fail2ban

```ini
# /etc/fail2ban/jail.local
[monitoring]
enabled = true
port = 443
filter = monitoring
logpath = /var/log/nginx/monitoring-access.log
maxretry = 5
bantime = 3600
```

### 3. SELinux/AppArmor

Enable and configure based on your distribution.

## Performance Tuning

### Database

```sql
-- Increase shared buffers
ALTER SYSTEM SET shared_buffers = '4GB';

-- Increase work memory
ALTER SYSTEM SET work_mem = '50MB';

-- Reload
SELECT pg_reload_conf();
```

### Connection Pooling

```yaml
database:
  max_conns: 100
  min_conns: 20
```
