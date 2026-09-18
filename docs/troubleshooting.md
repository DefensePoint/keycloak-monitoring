# Troubleshooting Guide

Common issues and solutions for the Keycloak Monitoring Tool.

## Table of Contents

- [Database Issues](#database-issues)
- [Keycloak Connection Issues](#keycloak-connection-issues)
- [Authentication Issues](#authentication-issues)
- [Docker Issues](#docker-issues)
- [Performance Issues](#performance-issues)
- [Frontend Issues](#frontend-issues)
- [Logging and Debugging](#logging-and-debugging)
- [Common Error Messages](#common-error-messages)

## Database Issues

### Cannot Connect to Database

**Symptoms**:

```bash
Failed to connect to database: connection refused
FATAL: password authentication failed for user "monitoring"
```

**Solutions**:

1. **Check if PostgreSQL is running**:

   ```bash
   # Check Docker container
   docker ps | grep postgres

   # Check system service
   sudo systemctl status postgresql

   # Start if not running
   docker start postgres
   # or
   sudo systemctl start postgresql
   ```

2. **Verify connection settings**:

   ```yaml
   # config.yaml
   database:
     host: "localhost"  # or "kmt-postgres" for Docker
     port: 5432
     database: "monitoring"
     user: "monitoring"
     password: "correct_password"
   ```

3. **Test connection manually**:

   ```bash
   psql -h localhost -U monitoring -d monitoring
   # Enter password when prompted
   ```

4. **Check network connectivity**:

   ```bash
   # Test port is open
   nc -zv localhost 5432
   telnet localhost 5432
   ```

5. **Verify password**:

   ```bash
   # Use environment variable
   export MONITORING_DATABASE_PASSWORD=correct_password
   ```

### TimescaleDB Extension Not Found

**Symptoms**:

```bash
ERROR: extension "timescaledb" does not exist
```

**Solutions**:

1. **Install TimescaleDB extension**:

   ```bash
   # Connect to PostgreSQL
   sudo -u postgres psql

   # Install extension in database
   \c monitoring
   CREATE EXTENSION IF NOT EXISTS timescaledb;
   \q
   ```

2. **For Docker, use TimescaleDB image**:

   ```yaml
   # docker-compose.yml
   postgres:
     image: timescale/timescaledb:latest-pg16  # Not postgres:17
   ```

### Database Migrations Failed

**Symptoms**:

```bash
Failed to run migrations: table already exists
```

**Solutions**:

1. **Check migration status**:

   ```bash
   # Connect to database
   psql -h localhost -U monitoring -d monitoring

   # Check tables
   \dt

   # Check specific table
   SELECT * FROM events LIMIT 1;
   ```

2. **Reset database** (development only):

   ```bash
   # Drop and recreate database
   sudo -u postgres psql -c "DROP DATABASE monitoring;"
   sudo -u postgres psql -c "CREATE DATABASE monitoring;"

   # Restart application to run migrations
   make run
   ```

### Database Connection Pool Exhausted

**Symptoms**:

```bash
Error: sorry, too many clients already
Failed to acquire connection from pool
```

**Solutions**:

1. **Increase connection pool size**:

   ```yaml
   # config.yaml
   database:
     max_conns: 50  # Increase from 25
     min_conns: 10
   ```

2. **Check for connection leaks**:

   ```sql
   -- View active connections
   SELECT count(*) FROM pg_stat_activity;

   -- View connections by database
   SELECT datname, count(*) FROM pg_stat_activity GROUP BY datname;
   ```

3. **Increase PostgreSQL max connections**:

   ```bash
   # Edit postgresql.conf
   max_connections = 200  # Default is 100

   # Restart PostgreSQL
   sudo systemctl restart postgresql
   ```

## Keycloak Connection Issues

### Cannot Connect to Keycloak

**Symptoms**:

```bash
Failed to authenticate with Keycloak
connection refused
dial tcp: lookup keycloak.example.com: no such host
```

**Solutions**:

1. **Verify Keycloak URL**:

   ```yaml
   # config.yaml
   keycloak:
     server_url: "https://keycloak.example.com"  # Correct URL
     # or
     server_url: "http://localhost:8080"  # For local Keycloak
   ```

2. **Test Keycloak accessibility**:

   ```bash
   # Test connectivity
   curl https://keycloak.example.com

   # Test Keycloak API
   curl https://keycloak.example.com/realms/master
   ```

3. **Check network/firewall**:

   ```bash
   # Test DNS resolution
   nslookup keycloak.example.com

   # Test port connectivity
   nc -zv keycloak.example.com 443

   # Check firewall rules
   sudo ufw status
   ```

4. **Verify credentials**:

   ```bash
   # Test authentication manually (client_credentials grant)
   curl -X POST "https://keycloak.example.com/realms/master/protocol/openid-connect/token" \
     -H "Content-Type: application/x-www-form-urlencoded" \
     -d "grant_type=client_credentials" \
     -d "client_id=monitoring-service" \
     -d "client_secret=your_client_secret"
   ```

### TLS Certificate Errors

**Symptoms**:

```bash
x509: certificate signed by unknown authority
x509: certificate has expired
```

**Solutions**:

1. **Add CA certificate to system trust store**:

   ```bash
   # Ubuntu/Debian
   sudo cp keycloak-ca.crt /usr/local/share/ca-certificates/
   sudo update-ca-certificates

   # RHEL/CentOS
   sudo cp keycloak-ca.crt /etc/pki/ca-trust/source/anchors/
   sudo update-ca-trust
   ```

2. **For development only - skip TLS verification**:

   ```yaml
   # config.yaml - DO NOT USE IN PRODUCTION
   keycloak:
     connection:
       skip_tls_verify: true
   ```

3. **Use correct certificate**:
   - Ensure certificate is not expired
   - Verify certificate matches domain
   - Check certificate chain is complete

### Keycloak Authentication Failed

**Symptoms**:

```bash
authentication failed: unauthorized_client / invalid_client
401 Unauthorized
403 Forbidden  (token issued, but Admin API call rejected)
```

**Solutions**:

1. **Verify client credentials**:

   ```yaml
   # config.yaml
   keycloak:
     client_id: "monitoring-service"   # confidential client
     client_secret: "your-client-secret"
   ```

2. **Check the client configuration in Keycloak**:
   - Client exists in the admin realm and is enabled
   - **Client authentication** is ON (confidential) and **Service accounts** are enabled
   - The client secret matches (regenerate on the Credentials tab if unsure)

3. **If the token issues but Admin API calls return 403**, the service account
   is missing roles:
   - Assign the master `admin` role (cross-realm), or the scoped read roles
     (`view-users`, `view-events`, …) per realm
   - See [keycloak-setup.md](keycloak-setup.md#step-4-assign-service-account-roles)

### No Events Being Collected

**Symptoms**:

- Dashboard shows 0 events
- Events page is empty
- Event statistics show 0

**Solutions**:

1. **Enable event logging in Keycloak**:
   - Go to Realm → Events → Config
   - Enable "Save Events"
   - Click "Save"

2. **Verify events are being generated**:

   ```bash
   # Perform a test login in Keycloak
   # Check Keycloak Admin Console → Events → Login Events
   ```

3. **Check event configuration**:

   ```yaml
   # config.yaml
   keycloak:
     polling:
       events_interval: 30s  # Not too long
     events:
       types: []  # Empty = collect all types
       max_events_per_poll: 1000
   ```

4. **Check logs for errors**:

   ```bash
   # Look for event collection errors
   docker-compose logs api-server | grep -i event
   ```

5. **Verify realm is being monitored**:

   ```yaml
   # config.yaml
   keycloak:
     realms:
       - "master"  # Include your realm
   ```

## Authentication Issues

### Cannot Login to Dashboard

**Symptoms**:

```bash
Invalid username or password
401 Unauthorized
Session expired
```

**Solutions**:

1. **Check credentials**:

   ```yaml
   # Default credentials in config.yaml
   auth:
     simple:
       default_user: "admin"
       default_pass: "admin123"
   ```

2. **Verify authentication is enabled**:

   ```yaml
   # config.yaml
   auth:
     simple:
       enabled: true  # Must be true
   ```

3. **Check session configuration**:

   ```yaml
   # config.yaml
   auth:
     session:
       secret: "must-be-at-least-32-characters-long"
       max_age: 24h
   ```

4. **Clear browser cookies**:
   - Clear cookies for the monitoring site
   - Try incognito/private browsing mode

5. **Check browser console for errors**:
   - Open browser developer tools (F12)
   - Look for authentication errors

### Session Keeps Expiring

**Symptoms**:

- Logged out frequently
- Session expired message

**Solutions**:

1. **Increase session duration**:

   ```yaml
   # config.yaml
   auth:
     session:
       max_age: 72h  # Increase from 24h
   ```

2. **Check session secret**:

   ```yaml
   # Must be consistent across restarts
   auth:
     session:
       secret: "same-secret-every-time-32-chars-minimum"
   ```

3. **Verify cookie settings**:

   ```yaml
   # config.yaml
   auth:
     session:
       secure: false  # Use false for HTTP (development)
       same_site: "lax"
   ```

### OAuth2 Login Not Working

**Symptoms**:

- Redirected to Keycloak but login fails
- OAuth callback errors

**Solutions**:

1. **Verify OAuth2 configuration**:

   ```yaml
   # config.yaml
   auth:
     oauth2:
       enabled: true
       provider_url: "https://keycloak.example.com/realms/myrealm"
       client_id: "monitoring-dashboard"
       client_secret: "correct-secret"
       redirect_url: "https://monitoring.example.com/auth/callback"
   ```

2. **Check Keycloak client configuration**:
   - Client ID matches
   - Client secret matches
   - Redirect URL is in "Valid Redirect URIs"
   - Client is enabled

3. **Check logs**:

   ```bash
   docker-compose logs api-server | grep -i oauth
   ```

## Docker Issues

### Container Won't Start

**Symptoms**:

```bash
Error starting container
Container exits immediately
```

**Solutions**:

1. **Check container logs**:

   ```bash
   docker-compose logs api-server
   docker-compose logs web
   docker-compose logs postgres
   ```

2. **Verify image exists**:

   ```bash
   docker images | grep kmt
   ```

3. **Rebuild images**:

   ```bash
   make docker-build
   docker-compose up -d
   ```

4. **Check resource limits**:

   ```bash
   # Check Docker resources
   docker stats

   # Check disk space
   df -h
   ```

### Cannot Access Services

**Symptoms**:

- Cannot access web UI
- API not responding
- Connection refused

**Solutions**:

1. **Check containers are running**:

   ```bash
   docker-compose ps
   ```

2. **Verify port mappings**:

   ```bash
   # Check which ports are mapped
   docker-compose ps

   # Should show:
   # web: 7880:7880
   # api-server: 7888:7888 (usually not exposed)
   # postgres: 5432:5432 (usually not exposed)
   ```

3. **Check firewall**:

   ```bash
   sudo ufw allow 7880/tcp
   ```

4. **Test port accessibility**:

   ```bash
   curl http://localhost:7880
   curl http://localhost:7888/health
   ```

### Docker Network Issues

**Symptoms**:

```bash
web cannot connect to api-server
api-server cannot connect to postgres
```

**Solutions**:

1. **Check Docker network**:

   ```bash
   docker network ls
   docker network inspect monitoring_network
   ```

2. **Verify service names in config**:

   ```yaml
   # config.yaml
   database:
     host: "kmt-postgres"  # Docker service name

   web:
     server:
       api_host_url: "http://api-server:7888"  # Docker service name
   ```

3. **Recreate network**:

   ```bash
   docker-compose down
   docker network prune
   docker-compose up -d
   ```

## Performance Issues

### Slow Dashboard Loading

**Solutions**:

1. **Check database performance**:

   ```sql
   -- Check slow queries
   SELECT query, mean_exec_time, calls
   FROM pg_stat_statements
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```

2. **Add database indexes**:

   ```sql
   -- Events table indexes
   CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
   CREATE INDEX IF NOT EXISTS idx_events_realm ON events(realm);
   CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
   ```

3. **Increase database connection pool**:

   ```yaml
   # config.yaml
   database:
     max_conns: 50
   ```

4. **Optimize polling intervals**:

   ```yaml
   # config.yaml
   keycloak:
     polling:
       metrics_interval: 10m  # Reduce frequency
       events_interval: 1m
   ```

### High Memory Usage

**Solutions**:

1. **Check memory usage**:

   ```bash
   docker stats
   ```

2. **Limit event collection**:

   ```yaml
   # config.yaml
   keycloak:
     events:
       max_events_per_poll: 500  # Reduce from 1000
   ```

3. **Set container memory limits**:

   ```yaml
   # docker-compose.yml
   api-server:
     mem_limit: 512m
   ```

### Database Growing Too Large

**Solutions**:

1. **Check database size**:

   ```sql
   SELECT pg_size_pretty(pg_database_size('monitoring'));
   ```

2. **Set up data retention policy**:

   ```sql
   -- Delete events older than 30 days
   DELETE FROM events WHERE timestamp < NOW() - INTERVAL '30 days';
   ```

3. **Use TimescaleDB retention policy**:

   ```sql
   -- Automatically drop old data
   SELECT add_retention_policy('events', INTERVAL '30 days');
   ```

4. **Enable compression** (TimescaleDB):

   ```sql
   ALTER TABLE events SET (
     timescaledb.compress,
     timescaledb.compress_segmentby = 'realm'
   );

   SELECT add_compression_policy('events', INTERVAL '7 days');
   ```

## Frontend Issues

### Frontend Won't Load

**Symptoms**:

- Blank page
- 404 errors
- Bundle not found

**Solutions**:

1. **Build frontend**:

   ```bash
   cd web
   npm install
   npm run build
   ```

2. **Check static files exist**:

   ```bash
   ls -la web/dist/
   ```

3. **Verify web server configuration**:

   ```yaml
   # config.yaml
   web:
     server:
       static_dir: "/app/web/dist"  # For Docker
       # or
       static_dir: "./web/dist"  # For local
   ```

### API Calls Failing from Frontend

**Symptoms**:

- Network errors in browser console
- CORS errors
- 404 on API calls

**Solutions**:

1. **Check API proxy configuration**:

   ```yaml
   # config.yaml
   web:
     server:
       api_host_url: "http://api-server:7888"  # Docker
       # or
       api_host_url: "http://localhost:7888"  # Local
   ```

2. **Verify API is accessible**:

   ```bash
   curl http://localhost:7888/api/health
   ```

3. **Check browser console** (F12):
   - Look for CORS errors
   - Check network tab for failed requests

## Logging and Debugging

### Enable Debug Logging

```yaml
# config.yaml
logging:
  level: "debug"  # Change from "info"
  format: "console"  # More readable than JSON
  output:
    stdout:
      enabled: true
      pretty: true
```

### View Logs

```bash
# Docker logs
docker-compose logs -f api-server
docker-compose logs -f web
docker-compose logs -f postgres

# System logs
sudo journalctl -u monitoring-api -f
sudo journalctl -u monitoring-web -f

# Application logs
tail -f /var/log/monitoring/api.log
```

### Common Debug Commands

```bash
# Check API health
curl http://localhost:7888/health

# Check version
curl http://localhost:7888/api/version

# Test authentication
curl -X POST http://localhost:7888/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# Check Keycloak connectivity
curl http://localhost:7888/api/keycloak/health
```

## Common Error Messages

### "Failed to load configuration"

**Cause**: Config file missing or invalid

**Solution**:

```bash
# Create config file
cp config.yaml.example config.yaml

# Validate YAML syntax
python3 -c "import yaml; yaml.safe_load(open('config.yaml'))"
```

### "Port already in use"

**Cause**: Another service is using the port

**Solution**:

```bash
# Find process using port
sudo lsof -i :7888
sudo lsof -i :7880

# Kill process or change port in config.yaml
```

### "Permission denied"

**Cause**: Insufficient file permissions

**Solution**:

```bash
# Fix permissions
chmod 644 config.yaml
chmod 755 bin/server bin/web

# For Docker
sudo chown -R 1000:1000 .
```
