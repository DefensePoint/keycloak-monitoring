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
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/DefensePoint/keycloak-monitoring/tests/alertscleanup"
)

// TestAlertsIntegration tests the full alert pipeline:
// 1. Insert fake events/health data into database
// 2. Wait for event checker to run (polls every 5 minutes)
// 3. Verify alerts are created in configuration_alerts table
// 4. Verify alerts show in UI (via API)
// 5. Verify Slack notification was sent (check notification_logs table)
// e2eFixtureTenantID is the tenant this test inserts into. Deliberately
// unguessable rather than production-shaped. Cleanup deletes this run's
// events and the health row it stamped. It does not delete alerts.
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
	// Close is registered first so it runs last. t.Cleanup is LIFO, and a
	// defer would close the database before any cleanup registered later.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	ctx := context.Background()
	// Not "prod-keycloak": that name is shipped in config.yaml.example, so a
	// deployment built from the example really does have a tenant by that name.
	// The default is a fixture id nothing provisions by accident. Override it
	// only for a tenant set aside for this test.
	tenantID := os.Getenv("KMT_E2E_TEST_TENANT_ID")
	if tenantID == "" {
		tenantID = e2eFixtureTenantID
	}
	eventLike := newEventLike(t)
	var started time.Time
	if err := db.QueryRow(`SELECT NOW()`).Scan(&started); err != nil {
		t.Fatalf("read database clock: %v", err)
	}

	t.Cleanup(func() {
		if err := alertscleanup.Cleanup(db, tenantID, eventLike); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})

	t.Log("Cleaning up a leftover health row from an earlier run...")
	if err := alertscleanup.Cleanup(db, tenantID, eventLike); err != nil {
		t.Fatalf("pre-run cleanup: %v", err)
	}

	// Get realm name for this tenant
	realmName := getRealmName(t, db, tenantID)
	t.Logf("Using realm: %s", realmName)

	// Test 1: LOGIN_ERROR events (threshold: 10, should trigger WARNING alert)
	t.Run("LoginErrorAlert", func(t *testing.T) {
		t.Log("Inserting 15 LOGIN_ERROR events...")
		insertLoginErrors(t, db, tenantID, realmName, eventLike, 15)

		t.Log("Waiting for event checker to run (up to 6 minutes)...")
		alert := waitForAlert(t, db, tenantID, "login_error_events", started, 6*time.Minute)

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
		insertCodeToTokenErrors(t, db, tenantID, realmName, eventLike, 3)

		t.Log("Waiting for event checker to run...")
		alert := waitForAlert(t, db, tenantID, "code_to_token_error_events", started, 6*time.Minute)

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
		alert := waitForAlert(t, db, tenantID, "health", started, 6*time.Minute)

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

func newEventLike(t *testing.T) string {
	t.Helper()
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("event prefix: %v", err)
	}
	return "test-" + hex.EncodeToString(b[:]) + "-%"
}

func getRealmName(t *testing.T, db *sql.DB, tenantID string) string {
	var realmName string
	err := db.QueryRow(`
		SELECT realm_name
		FROM keycloak_realms
		WHERE tenant_id = $1
		LIMIT 1
	`, tenantID).Scan(&realmName)
	if err == sql.ErrNoRows {
		t.Logf("No realms for tenant %s, using master", tenantID)
		return "master"
	}
	if err != nil {
		t.Fatalf("lookup realm: %v", err)
	}
	return realmName
}

func insertLoginErrors(t *testing.T, db *sql.DB, tenantID, realmName, eventLike string, count int) {
	prefix := strings.TrimSuffix(eventLike, "%")
	for i := 0; i < count; i++ {
		eventID := fmt.Sprintf("%slogin-error-%d-%d", prefix, i, time.Now().Unix())
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

func insertCodeToTokenErrors(t *testing.T, db *sql.DB, tenantID, realmName, eventLike string, count int) {
	prefix := strings.TrimSuffix(eventLike, "%")
	for i := 0; i < count; i++ {
		eventID := fmt.Sprintf("%scode-token-error-%d-%d", prefix, i, time.Now().Unix())
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
	`, tenantID, time.Now(), "UP", 100, alertscleanup.HealthMarker,
		int64(3600000), memoryUsed, memoryMax, memoryFree)

	if err != nil {
		t.Fatalf("Failed to insert health data: %v", err)
	}
	t.Logf("Inserted health data with %d%% memory usage", memoryPercent)
}

func waitForAlert(t *testing.T, db *sql.DB, tenantID, checkType string, since time.Time, timeout time.Duration) *Alert {
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
				AND last_seen >= $3
				ORDER BY last_seen DESC
				LIMIT 1
			`, tenantID, checkType, since).Scan(
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
