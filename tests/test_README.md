# Integration Tests

This directory contains end-to-end integration tests for the Keycloak Monitoring Tool.

## Prerequisites

1. **Docker Compose running**: The tests connect to the real PostgreSQL database
   ```bash
   cd deployments/local
   docker compose up -d
   ```

2. **Server running**: The alert checkers need to be running
   ```bash
   # From project root
   go run cmd/server/main.go
   ```

3. **Config**: Make sure `config.yaml` has:
   - A tenant set aside for this test, configured and enabled. The test
     defaults to `e2e-alerts-fixture-8f42c1`; set `KMT_E2E_TEST_TENANT_ID`
     to use a different one. Do not point it at a tenant carrying data you
     need, for the reason in the warning below.
   - Event checker enabled with reasonable poll intervals
   - Slack notifications enabled (or check notification_logs table instead)

## Running the Tests

The test is double-gated so it stays out of default runs: it needs the
`integration` build tag, and it reads the deployment's database from
`KMT_E2E_TEST_DATABASE_DSN`, skipping when that is unset.

> **This test deletes recent rows from the database you point it at.** Its
> `cleanup` helper removes, for the tenant under test, every `keycloak_events`
> row from the last 15 minutes (not only the ones it created, because the
> predicate is `event_id LIKE 'test-%' OR time > NOW() - INTERVAL '15 minutes'`),
> and every `configuration_alerts` and `keycloak_health` row from the same
> window with no test-marker filter at all. On a monitored deployment that is
> exactly the window you care about. The tenant id defaults to a fixture
> value nothing provisions by accident, which is the main thing keeping this
> off real rows; if you override `KMT_E2E_TEST_TENANT_ID`, pick a tenant
> whose last 15 minutes are expendable.
>
> Never export `KMT_E2E_TEST_DATABASE_DSN` into a shell that later runs the
> package tests: `KMT_TEST_DATABASE_DSN` and
> `KMT_DESTRUCTIVE_TEST_DATABASE_DSN` are separate variables precisely so
> that a deployment DSN cannot reach a helper that drops the schema. Set it
> inline on the one command below, not with `export`.

The DSN below is the local stack from step 1, which publishes PostgreSQL on
host port 5433 (`deployments/local/docker-compose.yml`). A server deployment
publishes the same database on 5432.

```bash
# From project root
KMT_E2E_TEST_DATABASE_DSN="host=localhost port=5433 dbname=monitoring user=monitoring password=<password> sslmode=disable" \
  go test -v -tags=integration ./tests/... -timeout 15m

# Or with coverage
KMT_E2E_TEST_DATABASE_DSN="host=localhost port=5433 dbname=monitoring user=monitoring password=<password> sslmode=disable" \
  go test -v -tags=integration ./tests/... -timeout 15m -cover
```

## What the Tests Do

### `TestAlertsIntegration`

This test verifies the complete alert pipeline:

1. **Insert fake events** into `keycloak_events` table
   - 15 `LOGIN_ERROR` events (threshold: 10)
   - 3 `CODE_TO_TOKEN_ERROR` events (threshold: 1)

2. **Insert fake health data** into `keycloak_health` table
   - 85% memory usage (threshold: 80%)

3. **Wait for checkers to run** (polls every 10 seconds, timeout 6 minutes)
   - Event checker polls every 5 minutes
   - Config checker polls every 5 minutes

4. **Verify alerts created** in `configuration_alerts` table
   - Login error alert (WARNING severity)
   - Code-to-token error alert (CRITICAL severity)
   - High memory alert (WARNING severity)

5. **Verify notifications sent** in `notification_logs` table
   - Checks that Slack notifications were triggered

6. **Cleanup** test data after completion

## Expected Results

```
=== RUN   TestAlertsIntegration
=== RUN   TestAlertsIntegration/LoginErrorAlert
    Inserting 15 LOGIN_ERROR events...
    Inserted 15 LOGIN_ERROR events
    Waiting for event checker to run (up to 6 minutes)...
    No alert found yet for check_type=login_error_events, waiting...
    No alert found yet for check_type=login_error_events, waiting...
    ✓ Alert created: Excessive Failed Login Attempts (severity: warning)
=== RUN   TestAlertsIntegration/CodeToTokenErrorAlert
    Inserting 3 CODE_TO_TOKEN_ERROR events...
    Inserted 3 CODE_TO_TOKEN_ERROR events
    Waiting for event checker to run...
    ✓ Alert created: OAuth Code Exchange Failures (severity: critical)
=== RUN   TestAlertsIntegration/HighMemoryAlert
    Inserting health data with 85% memory usage...
    Inserted health data with 85% memory usage
    Waiting for config checker to run...
    ✓ Alert created: High Memory Usage (severity: warning)
=== RUN   TestAlertsIntegration/VerifyNotifications
    Checking notification_logs table...
    ✓ Found 3 notifications sent
--- PASS: TestAlertsIntegration (370.45s)
PASS
```

## Troubleshooting

### Tests timeout waiting for alerts

**Cause**: Event checker or config checker not running, or poll intervals too long

**Fix**:
1. Check server is running: `ps aux | grep kmt`
2. Check server logs for checker activity
3. Verify config.yaml poll intervals (should be 5m or less)
4. Check database connection

### Alerts not created

**Cause**: Checkers not configured or disabled

**Fix**:
1. Check `config.yaml` - ensure event_checker and config_checker are enabled
2. Verify the fixture tenant (`KMT_E2E_TEST_TENANT_ID`, default
   `e2e-alerts-fixture-8f42c1`) is enabled
3. Check server logs for errors

### Notifications not sent

**Cause**: Slack configuration missing or severity threshold too high

**Fix**:
1. Check `config.yaml` slack configuration
2. Verify `min_severity` allows warning/critical alerts
3. Check `notification_logs` table for errors

## Manual Testing

You can also manually insert data and check the results:

```bash
# 1. Insert test events
docker compose -f deployments/local/docker-compose.yml exec postgres psql -U monitoring -d monitoring -c "
INSERT INTO keycloak_events (tenant_id, event_id, time, realm_id, realm_name, event_type, success, ip_address, user_id, username, client_id)
SELECT 'e2e-alerts-fixture-8f42c1', 'manual-test-' || generate_series, NOW() - interval '1 minute', 'master', 'master', 'LOGIN_ERROR', false, '192.168.1.100', 'user-' || generate_series, 'user' || generate_series, 'test-client'
FROM generate_series(1, 15);
"

# 2. Wait 5 minutes for event checker to run

# 3. Check for alerts
docker compose -f deployments/local/docker-compose.yml exec postgres psql -U monitoring -d monitoring -c "
SELECT title, severity, check_type, first_detected
FROM configuration_alerts
WHERE tenant_id = 'e2e-alerts-fixture-8f42c1'
ORDER BY first_detected DESC
LIMIT 5;
"

# 4. Check Slack channel for notifications
```
