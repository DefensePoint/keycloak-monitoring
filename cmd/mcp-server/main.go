// Keycloak Monitoring Tool MCP Server
// Read-only Model Context Protocol server exposing monitoring data to MCP
// clients, authenticated with personal access tokens.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	appfx "github.com/DefensePoint/keycloak-monitoring/internal/fx"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

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
