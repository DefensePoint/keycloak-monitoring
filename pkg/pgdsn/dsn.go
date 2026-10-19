package pgdsn

import (
	"fmt"
	"strings"
)

const DefaultSSLMode = "require"

func Format(host string, port int, database, user, password, sslmode string) string {
	if sslmode == "" {
		sslmode = DefaultSSLMode
	}
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		quoteValue(host), port, quoteValue(database), quoteValue(user), quoteValue(password), quoteValue(sslmode),
	)
}

// quoteValue quotes a keyword/value connection-string value when pgx would
// misread it. After '=', pgx trims whitespace and then takes the next token,
// so an unquoted empty password consumes the following keyword. The missing
// sslmode then defaults to prefer.
func quoteValue(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\n\r\v\f'\\") {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 2)
	b.WriteByte('\'')
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' || v[i] == '\'' {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('\'')
	return b.String()
}
