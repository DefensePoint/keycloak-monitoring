package keycloakadmin

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// replyFieldMax caps each Keycloak error field quoted in an error message.
const replyFieldMax = 200

// replyLogMax caps the raw reply written to the debug log.
const replyLogMax = 2048

// replyDetail describes a non-2xx reply for an error message, and logs the raw
// reply at debug level.
//
// Error messages reach API callers: Test Connection returns them, and a
// tenant's health and last error store them for anyone who can read tenants.
// The server URL is chosen by whoever creates the tenant, so quoting the reply
// body would hand them whatever that server returns. Only Keycloak's own error
// fields are quoted (error, error_description, and the Admin API's
// errorMessage), each trimmed and capped: they are what explains a failure,
// such as a wrong secret or an unknown realm. Anything else stays in the log.
//
// The result is empty or starts with ", error: ", so it can follow the status
// in a message.
func (c *Client) replyDetail(op string, status int, body []byte) string {
	if c.logger != nil {
		raw := string(body)
		if len(raw) > replyLogMax {
			raw = raw[:replyLogMax] + "..."
		}
		c.logger.Debug("Keycloak returned an error reply", "operation", op, "status", status, "body", raw)
	}

	var reply struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		ErrorMessage     string `json:"errorMessage"`
	}
	if json.Unmarshal(body, &reply) != nil {
		return ""
	}
	var parts []string
	for _, field := range []string{reply.Error, reply.ErrorDescription, reply.ErrorMessage} {
		if f := cleanReplyField(field); f != "" {
			parts = append(parts, f)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf(", error: %s", strings.Join(parts, " - "))
}

// cleanReplyField trims a field, drops control characters, and cuts it to
// replyFieldMax runes.
func cleanReplyField(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(s))
	if runes := []rune(s); len(runes) > replyFieldMax {
		s = string(runes[:replyFieldMax]) + "..."
	}
	return strings.TrimSpace(s)
}
