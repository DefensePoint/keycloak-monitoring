// Monitoring Dashboard API Server (Fx version)
// This is the new entry point using Uber Fx for dependency injection.

//	@title			Keycloak Monitoring Tool API
//	@version		1.0
//	@description	API for monitoring Keycloak instances, managing alerts, and security operations
//	@termsOfService	https://defensepoint.com/terms

//	@contact.name	DefensePoint Support
//	@contact.email	support@defensepoint.com

//	@license.name	Apache 2.0
//	@license.url	https://www.apache.org/licenses/LICENSE-2.0.html

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
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	appfx "github.com/DefensePoint/keycloak-monitoring/internal/fx"
	"github.com/DefensePoint/keycloak-monitoring/internal/healthcheck"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "", "Path to configuration file")
	healthFlag := flag.Bool("healthcheck", false, "Probe the local health endpoint and exit (container healthcheck)")
	flag.Parse()

	if *healthFlag {
		cfg, err := config.LoadConfigQuiet(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
			os.Exit(1)
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.HTTP.Server.Port)
		if err := healthcheck.Probe(url, 3*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		return
	}

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
