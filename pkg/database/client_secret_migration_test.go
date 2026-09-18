package database

import (
	"encoding/base64"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/secretcrypto"
)

// testTenantIDs are the tenant_id values these tests create and clean up. Kept
// distinctive so a run against a shared database can't disturb real tenants.
var testTenantIDs = []string{
	"test-secretmig-live",
	"test-secretmig-softdeleted",
}

// newTenantMigrationTestClient opens a postgres-backed client with the
// keycloak_tenants table migrated. The DSN comes from KMT_TEST_DATABASE_DSN
// and the test is skipped when it isn't set. That is the non-destructive
// variable: this helper only removes the fixtures in testTenantIDs, so unlike
// newTestClient it does not need a throwaway database.
func newTenantMigrationTestClient(t *testing.T) *Client {
	t.Helper()

	dsn := os.Getenv("KMT_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("KMT_TEST_DATABASE_DSN not set; skipping integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	if err := db.AutoMigrate(&KeycloakTenant{}); err != nil {
		t.Fatalf("failed to auto-migrate keycloak_tenants: %v", err)
	}

	cleanup := func() {
		// Unscoped, or the soft-deleted fixture would survive between runs
		// and collide with the unique index on tenant_id.
		db.Unscoped().Where("tenant_id IN ?", testTenantIDs).Delete(&KeycloakTenant{})
	}
	cleanup()
	t.Cleanup(cleanup)

	return &Client{db: db, logger: logger.New(zerolog.New(io.Discard))}
}

// newTestEncryptor builds an Encryptor from a valid 32-byte AES-256 key.
func newTestEncryptor(t *testing.T) *secretcrypto.Encryptor {
	t.Helper()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := secretcrypto.New(key)
	if err != nil {
		t.Fatalf("failed to build test encryptor: %v", err)
	}
	return enc
}

// storedSecret reads client_secret straight out of the row, including
// soft-deleted rows, without going through any conversion logic.
func storedSecret(t *testing.T, c *Client, tenantID string) string {
	t.Helper()

	var got KeycloakTenant
	if err := c.db.Unscoped().Where("tenant_id = ?", tenantID).First(&got).Error; err != nil {
		t.Fatalf("failed to read back tenant %s: %v", tenantID, err)
	}
	return got.ClientSecret
}

// TestMigrateEncryptClientSecrets_EncryptsSoftDeletedTenant covers the rows
// that matter most to the threat model this feature exists for. A soft-deleted
// tenant keeps its client_secret (Delete only sets deleted_at), that secret is
// still live at Keycloak, the tenant can be brought back via RestoreTenant, and
// the row is present in any database dump. Leaving it as plaintext means the
// migration reports success while the credential it was meant to protect is
// still readable.
func TestMigrateEncryptClientSecrets_EncryptsSoftDeletedTenant(t *testing.T) {
	c := newTenantMigrationTestClient(t)
	enc := newTestEncryptor(t)

	const tenantID = "test-secretmig-softdeleted"
	const plaintext = "soft-deleted-plaintext-secret"

	tenant := KeycloakTenant{
		TenantID:     tenantID,
		Name:         "Soft Deleted Tenant",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "client-softdeleted",
		ClientSecret: plaintext,
	}
	if err := c.db.Create(&tenant).Error; err != nil {
		t.Fatalf("failed to seed tenant: %v", err)
	}
	if err := c.db.Delete(&tenant).Error; err != nil {
		t.Fatalf("failed to soft-delete tenant: %v", err)
	}

	if err := c.MigrateEncryptClientSecrets(enc); err != nil {
		t.Fatalf("migration returned error: %v", err)
	}

	stored := storedSecret(t, c, tenantID)
	if !secretcrypto.IsEncrypted(stored) {
		t.Fatalf("soft-deleted tenant's client_secret was left unencrypted: %q", stored)
	}

	// Encrypted is not enough: it has to still be the original secret, or the
	// tenant is broken rather than protected.
	decrypted, err := enc.Decrypt(stored)
	if err != nil {
		t.Fatalf("stored ciphertext failed to decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted secret = %q, want %q", decrypted, plaintext)
	}
}

// TestMigrateEncryptClientSecrets_EncryptsLiveTenant guards the pre-existing
// behavior against a regression while the soft-delete scoping changes.
func TestMigrateEncryptClientSecrets_EncryptsLiveTenant(t *testing.T) {
	c := newTenantMigrationTestClient(t)
	enc := newTestEncryptor(t)

	const tenantID = "test-secretmig-live"
	const plaintext = "live-plaintext-secret"

	tenant := KeycloakTenant{
		TenantID:     tenantID,
		Name:         "Live Tenant",
		ServerURL:    "https://keycloak.example.com",
		AdminRealm:   "master",
		ClientID:     "client-live",
		ClientSecret: plaintext,
	}
	if err := c.db.Create(&tenant).Error; err != nil {
		t.Fatalf("failed to seed tenant: %v", err)
	}

	if err := c.MigrateEncryptClientSecrets(enc); err != nil {
		t.Fatalf("migration returned error: %v", err)
	}

	stored := storedSecret(t, c, tenantID)
	decrypted, err := enc.Decrypt(stored)
	if err != nil {
		t.Fatalf("stored ciphertext failed to decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted secret = %q, want %q", decrypted, plaintext)
	}
}
