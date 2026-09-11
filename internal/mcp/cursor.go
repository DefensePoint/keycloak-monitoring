package mcp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/events"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

const (
	// cursorDomainLabel domain-separates cursor MACs from any other HMAC use
	// of the same key. The version suffix also retires the cursors of an
	// earlier payload shape: they fail verification instead of decoding into
	// a payload whose missing fields would read as a zero window.
	cursorDomainLabel = "mcp/list_events/v2"

	// MinCursorKeyLen is the shortest mcp.cursor_hmac_key the server accepts.
	MinCursorKeyLen = 32

	maxCursorLen = 512
)

// cursorSigner mints and verifies the opaque keyset-pagination cursors handed
// to MCP clients. A cursor carries the keyset position and the window the
// first page resolved, so a listing that let the window default keeps
// querying that same window instead of one sliding with wall-clock time. The
// HMAC-SHA256 signature binds both, plus the id of the API token the cursor
// was minted for and a digest of the filter parameters, so a cursor neither
// validates under another token nor resumes a query with different filters.
// Tenant and realm scope are never read from a cursor: every page re-derives
// them from the caller's token.
type cursorSigner struct {
	key []byte
}

// newCursorSigner builds the signer for a configured key. The assembly
// refuses to start on a key shorter than MinCursorKeyLen, so the
// process-random fallback below is reachable only from a test server: it
// signs cursors that stop verifying across a restart.
func newCursorSigner(configuredKey string, log *logger.Logger) *cursorSigner {
	if len(configuredKey) >= MinCursorKeyLen {
		return &cursorSigner{key: []byte(configuredKey)}
	}
	if configuredKey != "" {
		log.Warn("mcp.cursor_hmac_key is shorter than 32 bytes and was ignored; cursors are signed with a process-random key and stop verifying across a restart")
	}
	return &cursorSigner{key: []byte(rand.Text())}
}

type cursorPayload struct {
	Timestamp int64  `json:"t"`
	ID        uint64 `json:"i"`
	From      int64  `json:"f"`
	To        int64  `json:"e"`
	Signature []byte `json:"s"`
}

// cursorState is what a verified cursor resolves to: the keyset position the
// next page resumes from and the window that page must query.
type cursorState struct {
	Position events.KeysetPosition
	From     time.Time
	To       time.Time
}

func (s *cursorSigner) mac(p *cursorPayload, tokenID uint, filterDigest []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(cursorDomainLabel))
	var buf [8]byte
	for _, field := range []uint64{uint64(p.Timestamp), p.ID, uint64(p.From), uint64(p.To), uint64(tokenID)} {
		binary.BigEndian.PutUint64(buf[:], field)
		m.Write(buf[:])
	}
	m.Write(filterDigest)
	return m.Sum(nil)
}

// Encode signs a keyset position and the window it belongs to into an opaque
// cursor. Timestamps are carried as microseconds, matching both Postgres
// timestamp resolution and the truncation ListKeyset applies, so the position
// round-trips exactly.
func (s *cursorSigner) Encode(pos events.KeysetPosition, from, to time.Time, tokenID uint, filterDigest []byte) string {
	p := cursorPayload{
		Timestamp: pos.Timestamp.UnixMicro(),
		ID:        uint64(pos.ID),
		From:      from.UnixMicro(),
		To:        to.UnixMicro(),
	}
	p.Signature = s.mac(&p, tokenID, filterDigest)
	raw, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Decode verifies a cursor against the caller's token and filter digest and
// returns the position and window it carries. Every failure maps to the
// generic invalid-input error so a client cannot distinguish tampering from a
// stale or malformed cursor.
func (s *cursorSigner) Decode(cursor string, tokenID uint, filterDigest []byte) (*cursorState, error) {
	if len(cursor) > maxCursorLen {
		return nil, fmt.Errorf("%w: oversized cursor", ErrInvalidInput)
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidInput)
	}
	var p cursorPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidInput)
	}
	if !hmac.Equal(p.Signature, s.mac(&p, tokenID, filterDigest)) {
		return nil, fmt.Errorf("%w: cursor signature mismatch", ErrInvalidInput)
	}
	return &cursorState{
		Position: events.KeysetPosition{
			Timestamp: time.UnixMicro(p.Timestamp).UTC(),
			ID:        uint(p.ID),
		},
		From: time.UnixMicro(p.From).UTC(),
		To:   time.UnixMicro(p.To).UTC(),
	}, nil
}
