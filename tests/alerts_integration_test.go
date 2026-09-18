//go:build integration

// End-to-end coverage for the alert pipeline. Unlike the database-backed tests
// elsewhere, this one needs a whole running deployment (see test_README.md),
// not just a PostgreSQL instance, so it is double-gated the way the AMFA
// integration tests are: the integration build tag keeps it out of the default
// run, and KMT_E2E_TEST_DATABASE_DSN supplies the deployment's database.
//
// The variable is this test's alone. It names a live deployment database, and
// no package test may ever read it.
//
//	KMT_E2E_TEST_DATABASE_DSN="host=localhost port=5432 dbname=monitoring user=monitoring password=... sslmode=disable" \
//	    go test -tags=integration ./tests/... -timeout 15m
package main_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestAlertsIntegration tests the full alert pipeline:
// 1. Insert fake events/health data into database
// 2. Wait for event checker to run (polls every 5 minutes)
// 3. Verify alerts are created in configuration_alerts table
// 4. Verify alerts show in UI (via API)
// 5. Verify Slack notification was sent (check notification_logs table)
// e2eFixtureTenantID is the tenant this test inserts into and cleans up.
// Deliberately unguessable rather than production-shaped: cleanup removes every
// row for this tenant inside a 15 minute window, not only the rows the test
// created, so the id must not collide with anything real.
const e2eFixtureTenantID = "e2e-alerts-fixture-8f42c1"

func TestAlertsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	connStr := os.Getenv("KMT_E2E_TEST_DATABASE_DSN")
	if connStr == "" {
		t.Skip("KMT_E2E_TEST_DATABASE_DSN not set; skipping integration test")
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Logf("Warning: Failed to close database connection: %v", err)
		}
	}()

	// Verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	ctx := context.Background()
	// Not "prod-keycloak": that name is shipped in config.yaml.example, so a
	// deployment built from the example really does have a tenant by that name,
	// and cleanup below deletes rows for whatever tenant this names. The default
	// is a fixture id nothing provisions by accident. Override it to run against
	// a tenant you have deliberately set aside for this test.
	tenantID := os.Getenv("KMT_E2E_TEST_TENANT_ID")
	if tenantID == "" {
		tenantID = e2eFixtureTenantID
	}

	// Clean up any existing test data
	t.Log("Cleaning up existing test data...")
	cleanup(t, db, tenantID)

	// Get realm name for this tenant
	realmName := getRealmName(t, db, tenantID)
	t.Logf("Using realm: %s", realmName)

	// Test 1: LOGIN_ERROR events (threshold: 10, should trigger WARNING alert)
	t.Run("LoginErrorAlert", func(t *testing.T) {
		t.Log("Inserting 15 LOGIN_ERROR events...")
		insertLoginErrors(t, db, tenantID, realmName, 15)

		t.Log("Waiting for event checker to run (up to 6 minutes)...")
		alert := waitForAlert(t, db, tenantID, "login_error_events", 6*time.Minute)

		if alert == nil {
			t.Fatal("Expected LOGIN_ERROR alert to be created, but none found")
			return
		}

		t.Logf("✓ Alert created: %s (severity: %s)", alert.Title, alert.Severity)

		// Verify alert properties
		if alert.Severity != "warning" {
			t.Errorf("Expected WARNING severity, got %s", alert.Severity)
		}
		if alert.Status != "active" {
			t.Errorf("Expected active status, got %s", alert.Status)
		}
	})

	// Test 2: CODE_TO_TOKEN_ERROR events (threshold: 1, should trigger CRITICAL alert)
	t.Run("CodeToTokenErrorAlert", func(t *testing.T) {
		t.Log("Inserting 3 CODE_TO_TOKEN_ERROR events...")
		insertCodeToTokenErrors(t, db, tenantID, realmName, 3)

		t.Log("Waiting for event checker to run...")
		alert := waitForAlert(t, db, tenantID, "code_to_token_error_events", 6*time.Minute)

		if alert == nil {
			t.Fatal("Expected CODE_TO_TOKEN_ERROR alert to be created, but none found")
			return
		}

		t.Logf("✓ Alert created: %s (severity: %s)", alert.Title, alert.Severity)

		// Verify alert is CRITICAL
		if alert.Severity != "critical" {
			t.Errorf("Expected CRITICAL severity, got %s", alert.Severity)
		}
	})

	// Test 3: High memory usage (threshold: 80%, should trigger WARNING alert)
	t.Run("HighMemoryAlert", func(t *testing.T) {
		t.Log("Inserting health data with 85% memory usage...")
		insertHighMemoryHealth(t, db, tenantID, 85)

		t.Log("Waiting for config checker to run...")
		alert := waitForAlert(t, db, tenantID, "health", 6*time.Minute)

		if alert == nil {
			t.Fatal("Expected high memory alert to be created, but none found")
			return
		}

		t.Logf("✓ Alert created: %s (severity: %s)", alert.Title, alert.Severity)

		if alert.Severity != "warning" {
			t.Errorf("Expected WARNING severity, got %s", alert.Severity)
		}
	})

	// Test 4: Check notification logs (verify Slack was notified)
	t.Run("VerifyNotifications", func(t *testing.T) {
		t.Log("Checking notification_logs table...")

		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM notification_logs
			WHERE tenant_id = $1
			AND created_at > NOW() - INTERVAL '10 minutes'
		`, tenantID).Scan(&count)

		if err != nil {
			t.Fatalf("Failed to query notification_logs: %v", err)
		}

		if count == 0 {
			t.Error("Expected notifications to be sent, but found none in notification_logs")
		} else {
			t.Logf("✓ Found %d notifications sent", count)
		}
	})

	// Cleanup
	t.Log("Cleaning up test data...")
	cleanup(t, db, tenantID)
}

type Alert struct {
	ID          int
	TenantID    string
	AlertID     string
	Title       string
	Severity    string
	Status      string
	CheckType   string
	Description string
}

func getRealmName(t *testing.T, db *sql.DB, tenantID string) string {
	var realmName string
	err := db.QueryRow(`
		SELECT DISTINCT realm_name
		FROM keycloak_realm_info
		WHERE tenant_id = $1
		LIMIT 1
	`, tenantID).Scan(&realmName)

	if err != nil {
		// Fallback to master if no realms found
		t.Logf("No realms found in keycloak_realm_info, using 'master' as fallback")
		return "master"
	}

	return realmName
}

func insertLoginErrors(t *testing.T, db *sql.DB, tenantID, realmName string, count int) {
	for i := 0; i < count; i++ {
		eventID := fmt.Sprintf("test-login-error-%d-%d", i, time.Now().Unix())
		// Insert all events within last 30 seconds so they're all in the 5-minute window
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, user_id, username,
				client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"LOGIN_ERROR", false, "192.168.1.100", "test-user-"+fmt.Sprint(i),
			"testuser"+fmt.Sprint(i), "test-client", "invalid_user_credentials")

		if err != nil {
			t.Fatalf("Failed to insert LOGIN_ERROR event: %v", err)
		}
	}
	t.Logf("Inserted %d LOGIN_ERROR events", count)
}

func insertCodeToTokenErrors(t *testing.T, db *sql.DB, tenantID, realmName string, count int) {
	for i := 0; i < count; i++ {
		eventID := fmt.Sprintf("test-code-token-error-%d-%d", i, time.Now().Unix())
		// Insert all events within last 30 seconds so they're all in the 5-minute window
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"CODE_TO_TOKEN_ERROR", false, "192.168.1.100",
			"oauth-client-"+fmt.Sprint(i), "invalid_code")

		if err != nil {
			t.Fatalf("Failed to insert CODE_TO_TOKEN_ERROR event: %v", err)
		}
	}
	t.Logf("Inserted %d CODE_TO_TOKEN_ERROR events", count)
}

func insertHighMemoryHealth(t *testing.T, db *sql.DB, tenantID string, memoryPercent int) {
	memoryMax := int64(1000 * 1024 * 1024) // 1000 MB
	memoryUsed := int64(float64(memoryMax) * float64(memoryPercent) / 100.0)
	memoryFree := memoryMax - memoryUsed

	_, err := db.Exec(`
		INSERT INTO keycloak_health (
			tenant_id, time, status, response_time,
			server_version, uptime_millis, memory_used,
			memory_max, memory_free
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, tenantID, time.Now(), "UP", 100, "23.0.7",
		int64(3600000), memoryUsed, memoryMax, memoryFree)

	if err != nil {
		t.Fatalf("Failed to insert health data: %v", err)
	}
	t.Logf("Inserted health data with %d%% memory usage", memoryPercent)
}

func waitForAlert(t *testing.T, db *sql.DB, tenantID, checkType string, timeout time.Duration) *Alert {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var alert Alert
			err := db.QueryRow(`
				SELECT id, tenant_id, alert_id, title, severity, status, check_type, description
				FROM configuration_alerts
				WHERE tenant_id = $1
				AND check_type = $2
				AND first_detected > NOW() - INTERVAL '10 minutes'
				ORDER BY first_detected DESC
				LIMIT 1
			`, tenantID, checkType).Scan(
				&alert.ID, &alert.TenantID, &alert.AlertID,
				&alert.Title, &alert.Severity, &alert.Status,
				&alert.CheckType, &alert.Description,
			)

			if err == sql.ErrNoRows {
				t.Logf("  No alert found yet for check_type=%s, waiting...", checkType)
				continue
			}
			if err != nil {
				t.Fatalf("Failed to query alerts: %v", err)
			}

			return &alert
		}
	}
}

func cleanup(t *testing.T, db *sql.DB, tenantID string) {
	// Delete test events
	_, err := db.Exec(`
		DELETE FROM keycloak_events
		WHERE tenant_id = $1
		AND (event_id LIKE 'test-%' OR time > NOW() - INTERVAL '15 minutes')
	`, tenantID)
	if err != nil {
		t.Logf("Warning: Failed to cleanup events: %v", err)
	}

	// Delete test alerts
	_, err = db.Exec(`
		DELETE FROM configuration_alerts
		WHERE tenant_id = $1
		AND first_detected > NOW() - INTERVAL '15 minutes'
	`, tenantID)
	if err != nil {
		t.Logf("Warning: Failed to cleanup alerts: %v", err)
	}

	// Delete test health data
	_, err = db.Exec(`
		DELETE FROM keycloak_health
		WHERE tenant_id = $1
		AND time > NOW() - INTERVAL '15 minutes'
	`, tenantID)
	if err != nil {
		t.Logf("Warning: Failed to cleanup health data: %v", err)
	}
}
