# Deployment Guide

Complete guide for deploying the Keycloak Monitoring Tool to production environments.

## Table of Contents

- [Overview](#overview)
- [Deployment Options](#deployment-options)
- [Docker Compose Deployment](#docker-compose-deployment)
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
  server_url: "https://keycloak.example.com"
  # Use environment variables for credentials

auth:
  simple:
    enabled: false
  oauth2:
    enabled: true
    provider_url: "https://keycloak.example.com/realms/production"
    client_id: "monitoring-dashboard"
    # Use environment variable for secret

logging:
  level: "info"
  format: "json"
```

### Step 2: Create Environment File

```bash
# Create .env file for secrets
cat > .env <<EOF
MONITORING_DATABASE_PASSWORD=your_secure_db_password
MONITORING_KEYCLOAK_CLIENT_SECRET=keycloak_client_secret
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
curl http://localhost:7888/api/health

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
    image: timescale/timescaledb:latest-pg16
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
      - MONITORING_KEYCLOAK_CLIENT_SECRET=${MONITORING_KEYCLOAK_CLIENT_SECRET}
      - MONITORING_AUTH_SESSION_SECRET=${MONITORING_AUTH_SESSION_SECRET}
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    networks:
      - monitoring_network
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:7888/api/health"]
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
      - "7880:7880"
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

`MONITORING_*` environment variables override `config.yaml` only when the matching key exists in the file. Keep secret-bearing keys present and empty in the ConfigMap, and inject the real values as env vars from a Secret.

Required secrets (32+ character session secret, AES encryption key, database password, tenant client secret):

- `MONITORING_AUTH_SESSION_SECRET`
- `MONITORING_SECURITY_ENCRYPTION_KEY`
- `MONITORING_DATABASE_PASSWORD`
- tenant client secret (the `MONITORING_KEYCLOAK_TENANTS_<NAME>_CLIENT_SECRET` key that matches your tenant block)

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
  image: timescale/timescaledb:latest-pg16
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
          image: timescale/timescaledb:latest-pg16
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
            - name: MONITORING_KEYCLOAK_TENANTS_DEMO_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: monitoring-secrets
                  key: keycloak-client-secret
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

Rename `MONITORING_KEYCLOAK_TENANTS_DEMO_CLIENT_SECRET` to match the tenant name in `config.yaml`.

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
Environment="MONITORING_DATABASE_PASSWORD=your_password"
Environment="MONITORING_KEYCLOAK_CLIENT_SECRET=keycloak_client_secret"

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
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
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
pg_dump -U monitoring monitoring | gzip > $BACKUP_DIR/monitoring_$DATE.sql.gz

# Keep only last 30 days
find $BACKUP_DIR -name "monitoring_*.sql.gz" -mtime +30 -delete
```

Add to crontab:

```bash
0 2 * * * /usr/local/bin/backup-monitoring-db.sh
```

## Security Hardening

### 1. Firewall Rules

```bash
# Allow only necessary ports
sudo ufw allow 443/tcp  # HTTPS
sudo ufw allow 22/tcp   # SSH
sudo ufw deny 7888/tcp  # Block direct API access
sudo ufw deny 7880/tcp  # Block direct web access
sudo ufw deny 5432/tcp  # Block direct database access
sudo ufw enable
```

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
