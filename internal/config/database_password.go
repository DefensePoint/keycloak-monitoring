package config

import (
	"fmt"
	"strings"
)

// ExampleDatabasePassword is the database password this project's local
// stacks and docs publish for throwaway development databases.
const ExampleDatabasePassword = "monitoring_password"

// ValidateDatabasePassword refuses a database password that is empty or a
// ${NAME} placeholder the config loader left unexpanded: the server must not
// start without a password set for it. There is no built-in password to fall
// back to. It reports, without refusing, the password the local stacks and
// docs publish, which is fine on a throwaway database and wrong anywhere else.
// key names the setting, such as database.password, for the error.
func ValidateDatabasePassword(key, password string) (published bool, err error) {
	envVar := "MONITORING_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	howTo := fmt.Sprintf("Set %s in config.yaml, or the %s environment variable.", key, envVar)
	if strings.HasPrefix(key, "mcp.database.") {
		// mcp.database.* keys have no defaults, so that a missing block still
		// fails startup, and the loader binds their environment variables only
		// for keys config.yaml already has.
		howTo = fmt.Sprintf("Set %s in config.yaml, or set the %s environment variable and keep a "+
			"password key, which may be empty, under mcp.database in config.yaml: the variable is "+
			"ignored without it.", key, envVar)
	}
	p := strings.TrimSpace(password)
	if p == "" {
		return false, fmt.Errorf("SECURITY ERROR: %s is not set, and there is no built-in password to fall back to. %s", key, howTo)
	}
	// The loader keeps ${NAME} as literal text when NAME is unset.
	if strings.Contains(p, "${") {
		return false, fmt.Errorf("SECURITY ERROR: %s is %q, a placeholder whose environment variable is not set. "+
			"Set that variable. %s", key, p, howTo)
	}
	return strings.EqualFold(p, ExampleDatabasePassword), nil
}
