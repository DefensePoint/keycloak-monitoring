package config

import (
	"errors"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// MCPConfig configures the MCP (Model Context Protocol) server binary.
type MCPConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Port    int  `mapstructure:"port"`

	// DefaultPageSize is the page size applied when a tool call does not
	// request one; MaxPageSize caps any page size a call does request.
	DefaultPageSize int `mapstructure:"default_page_size"`
	MaxPageSize     int `mapstructure:"max_page_size"`

	// CursorHMACKey signs the pagination cursors MCP tools hand to clients.
	// Required, minimum 32 bytes: a shorter key fails startup rather than
	// falling back to a process-random one whose cursors stop verifying
	// across a restart.
	CursorHMACKey string `mapstructure:"cursor_hmac_key"`

	// OriginAllowlist lists the exact Origin header values accepted on MCP
	// requests. Requests carrying an Origin not in this list are rejected;
	// an empty list rejects every Origin-bearing request (browsers cannot
	// call the MCP endpoint), which is the MCP-spec DNS-rebinding defense.
	OriginAllowlist []string `mapstructure:"origin_allowlist"`

	Metrics MetricsConfig `mapstructure:"metrics"`

	Rate MCPRateConfig `mapstructure:"rate"`

	// MaxResponseBytes caps the serialized size of any tool result. An
	// oversize result is replaced with ErrResultTooLarge. 0 or less falls
	// back to the 1 MiB default; the cap cannot be disabled.
	MaxResponseBytes int `mapstructure:"max_response_bytes"`

	// Database is the read-only role the MCP server connects with. It is
	// required: there is no fallback to the main database block, which names
	// the read-write application role, so a nil block fails startup.
	Database *DatabaseConfig `mapstructure:"database"`

	// RemovedKeys names the mcp.* keys of removedMCPKeys the config file still
	// carries. Not a setting: LoadConfig fills it in, because Unmarshal has no
	// field for such a key and drops it without a word.
	RemovedKeys []string `mapstructure:"-"`
}

// removedMCPKeys maps an mcp.* key this schema no longer reads to the sentence
// naming what replaced it.
var removedMCPKeys = map[string]string{
	"mcp.absolute_max_page_size": "mcp.absolute_max_page_size was removed: mcp.default_page_size is now the page size a tool call gets when it requests none, and mcp.max_page_size is now the cap on one it does request",
}

// removedMCPKeysInFile returns the removed mcp.* keys the config file itself
// carries, in a stable order.
func removedMCPKeysInFile(v *viper.Viper) []string {
	var found []string
	for key := range removedMCPKeys {
		if v.InConfig(key) {
			found = append(found, key)
		}
	}
	sort.Strings(found)
	return found
}

// RemovedKeyError reports the removed mcp.* keys the config file still carries.
// A config left on the old page size names keeps its values under the new
// meanings, where max_page_size caps what it used to default, so the server
// fails startup instead of silently throttling every page.
func (c MCPConfig) RemovedKeyError() error {
	if len(c.RemovedKeys) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(c.RemovedKeys))
	for _, key := range c.RemovedKeys {
		msgs = append(msgs, removedMCPKeys[key])
	}
	return errors.New("CONFIG ERROR: " + strings.Join(msgs, "; "))
}

// MCPRateConfig configures the MCP token-bucket rate limiters. The per-user
// budget is shared by all of a user's API tokens; the per-IP budget applies
// before authentication, keyed by the connection's direct remote address.
// Burst is the instantaneous allowance of both buckets. A value of 0 or less
// disables that limiter.
type MCPRateConfig struct {
	PerUserPerMinute int `mapstructure:"per_user_per_minute"`
	PerIPPerMinute   int `mapstructure:"per_ip_per_minute"`
	Burst            int `mapstructure:"burst"`
}

// setMCPDefaults registers mcp.* defaults. Viper's AutomaticEnv only binds a
// MONITORING_MCP_* env var once its key is registered here or present in
// config.yaml. mcp.database.* keys are deliberately not registered: a default
// would make the block always non-nil, so a config naming no read-only role
// would pass the startup check that exists to catch exactly that.
func setMCPDefaults(v *viper.Viper) {
	v.SetDefault("mcp.enabled", false)
	v.SetDefault("mcp.port", 7889)
	v.SetDefault("mcp.default_page_size", 50)
	v.SetDefault("mcp.max_page_size", 500)
	v.SetDefault("mcp.cursor_hmac_key", "")
	v.SetDefault("mcp.origin_allowlist", []string{})
	v.SetDefault("mcp.metrics.enabled", false)
	v.SetDefault("mcp.metrics.auth_token", "")
	v.SetDefault("mcp.rate.per_user_per_minute", 120)
	v.SetDefault("mcp.rate.per_ip_per_minute", 300)
	v.SetDefault("mcp.rate.burst", 30)
	v.SetDefault("mcp.max_response_bytes", 1<<20)
}
