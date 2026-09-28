package amfacheck

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// alertID builds a stable, unique alert ID from the given parts by joining them
// with ":" and hashing with sha256. The parts always start with a per-rule
// prefix (e.g. "amfa-risk-rejected") so IDs never collide across rules.
func alertID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(sum[:])
}
