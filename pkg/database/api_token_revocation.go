package database

import (
	"time"

	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// cutOffTokenRevocation revokes, once, the live API tokens of users who were
// cut off before the release that revokes tokens at the moment of the cut-off.
//
// From that release on, deactivating or blocking a user revokes their tokens
// in the same transaction, and so does reactivating or unblocking them. A user
// cut off by an earlier release kept their tokens unrevoked: refused while the
// user stays cut off, and revoked only once the user is restored. This revokes
// them at the upgrade instead, so the token list stops showing them as live
// and an account cut off before the upgrade ends up exactly as one cut off
// after it.
//
// It is the backstop, not the guarantee: restoring a user revokes whatever is
// still live, which also covers a user cut off by an older release after this
// ran, during a rollback or by an older replica in a rolling deploy. So each
// runs once, marked in data_migrations, and never again.
//
// Deactivation and blocking have a marker each, because they shipped in
// separate releases: a database that already ran the first still needs the
// second.
type cutOffTokenRevocation struct {
	// key marks the revocation as applied.
	key string
	// userCondition selects the cut-off users, over users aliased u.
	userCondition string
	// what names the users in the log line.
	what string
}

var cutOffTokenRevocations = []cutOffTokenRevocation{
	{
		key:           "api_tokens.revoke_inactive_users",
		userCondition: "u.is_active = false",
		what:          "inactive users",
	},
	{
		key:           "api_tokens.revoke_blocked_users",
		userCondition: "u.is_blocked = true",
		what:          "blocked users",
	},
}

func (c *Client) revokeCutOffUsersTokens() error {
	for _, rev := range cutOffTokenRevocations {
		if err := c.applyCutOffTokenRevocation(rev); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyCutOffTokenRevocation(rev cutOffTokenRevocation) error {
	applied, err := dataMigrationApplied(c.db, rev.key)
	if err != nil || applied {
		return err
	}

	var ran bool
	var rows int64
	err = c.db.Transaction(func(tx *gorm.DB) error {
		// Held until this transaction ends, so a second startup waits and
		// then sees the marker instead of revoking again.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?)::bigint)`, rev.key).Error; err != nil {
			return err
		}
		applied, err := dataMigrationApplied(tx, rev.key)
		if err != nil || applied {
			return err
		}
		res := tx.Exec(`
			UPDATE api_tokens AS t
			SET revoked_at = now()
			FROM users AS u
			WHERE t.user_id = u.id
			  AND t.revoked_at IS NULL
			  AND (t.expires_at IS NULL OR t.expires_at > now())
			  AND ` + rev.userCondition)
		if res.Error != nil {
			return res.Error
		}
		if err := tx.Create(&DataMigration{Key: rev.key, AppliedAt: time.Now()}).Error; err != nil {
			return err
		}
		ran, rows = true, res.RowsAffected
		return nil
	})
	if err != nil || !ran {
		return err
	}
	if rows > 0 {
		c.logger.Info("Revoked the API tokens of users cut off before tokens were revoked with the account",
			logger.Str("users", rev.what),
			logger.Int64("tokens", rows))
	}
	return nil
}
