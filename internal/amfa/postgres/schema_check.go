package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// schemaCheckTimeout bounds the alembic_version probe so a slow or hung AMFA
// PostgreSQL does not block KMT's boot indefinitely. The check is best-effort
// (logged, never fatal), so a short deadline is fine.
const schemaCheckTimeout = 5 * time.Second

// CheckAmfaSchema reads AMFA's alembic_version table and logs a warning if it
// differs from the expected version this build was tested against. Never
// fatal: schema drift logs a WARN and returns nil so KMT continues to boot.
// A nil logger is tolerated. The probe carries its own short deadline
// (schemaCheckTimeout) so a hung database can never block startup.
func CheckAmfaSchema(db *gorm.DB, expected string, log *logger.Logger) error {
	if expected == "" {
		if log != nil {
			log.Debug("AMFA expected_schema_version not set; skipping schema check")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), schemaCheckTimeout)
	defer cancel()
	var actual string
	if err := db.WithContext(ctx).Raw("SELECT version_num FROM alembic_version").Scan(&actual).Error; err != nil {
		if log != nil {
			log.Warn("Could not read AMFA alembic_version table; schema verification skipped",
				logger.Err(err))
		}
		return nil
	}
	if actual != expected {
		if log != nil {
			log.Warn("AMFA schema version drift detected; KMT may need updating",
				logger.Str("expected", expected),
				logger.Str("actual", actual))
		}
		return nil
	}
	if log != nil {
		log.Info("AMFA schema version matches", logger.Str("version", actual))
	}
	return nil
}
