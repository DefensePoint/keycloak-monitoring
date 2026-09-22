package postgres

import (
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// TestCheckAmfaSchema_EmptyExpectedNoLogger verifies the early-return path when
// no expected version is provided. Exercised without a database — the function
// must not touch db when expected=="".
func TestCheckAmfaSchema_EmptyExpectedNoLogger(t *testing.T) {
	// A nil *gorm.DB is acceptable here because the function should short-circuit
	// before any DB call when expected is empty.
	if err := CheckAmfaSchema(nil, "", nil); err != nil {
		t.Errorf("expected nil error with empty expected version, got %v", err)
	}
}

// TestCheckAmfaSchema_EmptyExpectedWithLogger verifies the nil-logger tolerance
// is symmetric: passing a real logger with an empty expected version also
// returns nil without touching the database.
func TestCheckAmfaSchema_EmptyExpectedWithLogger(t *testing.T) {
	log := logger.NewNoop()
	if err := CheckAmfaSchema(nil, "", log); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
