package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

const (
	dbHost     = "localhost"
	dbPort     = 5432
	dbUser     = "monitoring"
	dbPassword = "monitoring_password"
	dbName     = "monitoring"
)

func main() {
	// Connect to database
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Warning: Failed to close database connection: %v", err)
		}
	}()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	tenantID := "prod-keycloak"
	realmName := "master"

	log.Println("Inserting test events to trigger all 5 alert types...")
	log.Println("All events will be within the last 30 seconds (same 5-minute window)")

	// 1. LOGIN_ERROR events (threshold: 10, WARNING)
	log.Println("Inserting 15 LOGIN_ERROR events...")
	for i := 0; i < 15; i++ {
		eventID := fmt.Sprintf("demo-login-error-%d-%d", i, time.Now().Unix())
		// All events within last 30 seconds
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, user_id, username,
				client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"LOGIN_ERROR", false, "192.168.1.100", "demo-user-"+fmt.Sprint(i),
			"demouser"+fmt.Sprint(i), "demo-client", "invalid_user_credentials")

		if err != nil {
			log.Printf("Failed to insert LOGIN_ERROR event: %v", err)
		}
	}
	log.Println("✓ Inserted 15 LOGIN_ERROR events")

	// 2. IDENTITY_PROVIDER_LOGIN_ERROR events (threshold: 1, WARNING)
	log.Println("Inserting 3 IDENTITY_PROVIDER_LOGIN_ERROR events...")
	for i := 0; i < 3; i++ {
		eventID := fmt.Sprintf("demo-idp-error-%d-%d", i, time.Now().Unix())
		// All events within last 30 seconds
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, user_id, username,
				client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"IDENTITY_PROVIDER_LOGIN_ERROR", false, "192.168.1.101",
			"demo-idp-user-"+fmt.Sprint(i), "idpuser"+fmt.Sprint(i),
			"demo-client", "identity_provider_login_failure")

		if err != nil {
			log.Printf("Failed to insert IDP_ERROR event: %v", err)
		}
	}
	log.Println("✓ Inserted 3 IDENTITY_PROVIDER_LOGIN_ERROR events")

	// 3. CODE_TO_TOKEN_ERROR events (threshold: 1, CRITICAL)
	log.Println("Inserting 5 CODE_TO_TOKEN_ERROR events...")
	for i := 0; i < 5; i++ {
		eventID := fmt.Sprintf("demo-code-token-error-%d-%d", i, time.Now().Unix())
		// All events within last 30 seconds
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"CODE_TO_TOKEN_ERROR", false, "192.168.1.102",
			"demo-oauth-client-"+fmt.Sprint(i), "invalid_code")

		if err != nil {
			log.Printf("Failed to insert CODE_TO_TOKEN_ERROR event: %v", err)
		}
	}
	log.Println("✓ Inserted 5 CODE_TO_TOKEN_ERROR events")

	// 4. CLIENT_LOGIN_ERROR events (threshold: 10, WARNING)
	log.Println("Inserting 12 CLIENT_LOGIN_ERROR events...")
	for i := 0; i < 12; i++ {
		eventID := fmt.Sprintf("demo-client-error-%d-%d", i, time.Now().Unix())
		// All events within last 30 seconds
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"CLIENT_LOGIN_ERROR", false, "192.168.1.103",
			"demo-bad-client-"+fmt.Sprint(i), "invalid_client_credentials")

		if err != nil {
			log.Printf("Failed to insert CLIENT_LOGIN_ERROR event: %v", err)
		}
	}
	log.Println("✓ Inserted 12 CLIENT_LOGIN_ERROR events")

	// 5. Common errors: REGISTER_ERROR (threshold: 50, WARNING)
	log.Println("Inserting 55 REGISTER_ERROR events...")
	for i := 0; i < 55; i++ {
		eventID := fmt.Sprintf("demo-register-error-%d-%d", i, time.Now().Unix())
		// All events within last 55 seconds (still within 5-minute window)
		eventTime := time.Now().Add(-time.Duration(i) * time.Second)

		_, err := db.Exec(`
			INSERT INTO keycloak_events (
				tenant_id, event_id, time, realm_id, realm_name,
				event_type, success, ip_address, user_id, username,
				client_id, event_error
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, tenantID, eventID, eventTime, realmName, realmName,
			"REGISTER_ERROR", false, "192.168.1.104",
			"demo-register-user-"+fmt.Sprint(i), "reguser"+fmt.Sprint(i),
			"demo-client", "registration_failed")

		if err != nil {
			log.Printf("Failed to insert REGISTER_ERROR event: %v", err)
		}
	}
	log.Println("✓ Inserted 55 REGISTER_ERROR events")

	log.Println("\n========================================")
	log.Println("All test events inserted successfully!")
	log.Println("========================================")
	log.Println("")
	log.Println("All events are within the last 60 seconds (same 5-minute window)")
	log.Println("Wait up to 5 minutes for the event checker to run.")
	log.Println("")
	log.Println("You should see 5 alerts in Slack:")
	log.Println("  1. LOGIN_ERROR - WARNING (15 events, threshold 10)")
	log.Println("  2. IDENTITY_PROVIDER_LOGIN_ERROR - WARNING (3 events, threshold 1)")
	log.Println("  3. CODE_TO_TOKEN_ERROR - CRITICAL (5 events, threshold 1)")
	log.Println("  4. CLIENT_LOGIN_ERROR - WARNING (12 events, threshold 10)")
	log.Println("  5. REGISTER_ERROR - WARNING (55 events, threshold 50)")
	log.Println("")
	log.Println("View alerts in UI: http://localhost:7880/prod-keycloak/alerts")
	log.Println("")
	log.Println("To clean up test events:")
	log.Println("  docker compose -f deployments/local/docker-compose.yml exec postgres psql -U monitoring -d monitoring -c \"DELETE FROM keycloak_events WHERE tenant_id = 'prod-keycloak' AND event_id LIKE 'demo-%';\"")
}
