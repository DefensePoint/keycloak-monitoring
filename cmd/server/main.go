// Monitoring Dashboard API Server (Fx version)
// This is the new entry point using Uber Fx for dependency injection.

//	@title			Point Monitoring Platform API
//	@version		1.0
//	@description	API for monitoring Keycloak instances, managing alerts, and security operations
//	@termsOfService	https://defensepoint.com/terms

//	@contact.name	DefensePoint Support
//	@contact.url	https://defensepoint.com/support
//	@contact.email	support@defensepoint.com

//	@license.name	Proprietary
//	@license.url	https://defensepoint.com/license

//	@host
//	@BasePath	/
//	@schemes	http https

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token

package main

import (
	"flag"
	"fmt"
	"os"

	appfx "repos.defensepoint.com/iam/oss-pmt/kmt/internal/fx"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Create and run the Fx application
	app := appfx.NewStandaloneApp(*configPath)

	// Run the application (blocks until stopped)
	app.Run()

	// Check for startup errors
	if err := app.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}
