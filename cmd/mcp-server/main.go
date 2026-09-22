// Keycloak Monitoring Tool MCP Server
// Read-only Model Context Protocol server exposing monitoring data to MCP
// clients, authenticated with personal access tokens.
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
	configPath := flag.String("config", "", "Path to configuration file")
	healthFlag := flag.Bool("healthcheck", false, "Probe the local health endpoint and exit (container healthcheck)")
	flag.Parse()

	if *healthFlag {
		cfg, err := config.LoadConfigQuiet(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.MCP.Port)
		if err := healthcheck.Probe(url, 3*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	if !cfg.MCP.Enabled {
		fmt.Println("MCP server is disabled (mcp.enabled=false), nothing to do - exiting")
		return
	}

	app := appfx.NewMCPApp(*configPath)

	app.Run()

	if err := app.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}
