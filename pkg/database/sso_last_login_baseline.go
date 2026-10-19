package database

import (
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const ssoLastLoginBaselineSQL = `
	UPDATE users
	SET last_login_at = now()
	WHERE auth_method = 'oauth'
	  AND last_login_at IS NULL
	  AND deleted_at IS NULL`

// baselineSSOLastLogin starts the SSO sign-in clock for SSO users who have no
// recorded sign-in.
//
// SSO sign-ins did not record last_login_at until the release that added the
// MCP sign-in limit, so every SSO user upgrades with NULL. The limit lets a
// user with no recorded sign-in through, so without a time here someone who
// left before the upgrade, and can no longer sign in, would keep their tokens
// until they expire. Stamping the startup time gives each of them the full
// limit, and then they age out like everyone else.
//
// It runs on every startup and only touches NULL rows, so a row it stamped is
// left alone afterwards. An SSO account that appears later without a sign-in
// gets its clock started at the next startup.
//
// It runs after migrateUnprovenEmailVerified. That migration clears
// email_verified for SSO users whose last_login_at is NULL or before its
// cutoff, so on the upgrade boot it clears as before, and once the stamp moves
// last_login_at past the cutoff later boots leave the flag alone.
func (c *Client) baselineSSOLastLogin() error {
	res := c.db.Exec(ssoLastLoginBaselineSQL)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		c.logger.Info("Started the SSO sign-in clock for SSO users with no recorded sign-in",
			logger.Int64("rows", res.RowsAffected))
	}
	return nil
}
