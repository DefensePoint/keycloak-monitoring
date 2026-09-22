// Web Frontend Server
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
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
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		// Unlike the api and mcp servers, web binds cfg host, which may be a
		// specific interface where loopback would refuse the probe.
		host := cfg.Web.Server.Host
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		url := fmt.Sprintf("http://%s/healthz", net.JoinHostPort(host, strconv.Itoa(cfg.Web.Server.Port)))
		if err := healthcheck.Probe(url, 3*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "unhealthy: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Verify static directory exists
	if _, err := os.Stat(cfg.Web.Server.StaticDir); os.IsNotExist(err) {
		log.Fatalf("Static directory does not exist: %s", cfg.Web.Server.StaticDir)
	}

	// Setup handlers
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"status":"healthy","service":"web-frontend"}`); err != nil {
			log.Printf("Failed to write health check response: %v", err)
		}
	})

	// Forward /api and /auth requests to backend
	mux.Handle("/api/", createProxyHandler(cfg.Web.Server.APIHostURL))
	mux.Handle("/auth/", createProxyHandler(cfg.Web.Server.APIHostURL))

	// Static files and SPA routing
	mux.Handle("/", createSPAHandler(cfg.Web.Server.StaticDir))

	addr := fmt.Sprintf("%s:%d", cfg.Web.Server.Host, cfg.Web.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("Starting web frontend server on %s", addr)
	log.Printf("Serving files from: %s", cfg.Web.Server.StaticDir)
	log.Printf("Proxying /api and /auth to: %s", cfg.Web.Server.APIHostURL)

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errCh:
		log.Fatalf("Server error: %v", err)
	case <-sigCh:
		log.Println("Shutdown signal received, stopping server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		log.Println("Server stopped gracefully")
	}
}

// createProxyHandler creates a reverse proxy handler for API requests
func createProxyHandler(backendURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Build target URL
		targetURL := backendURL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		// Create new request
		proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
		if err != nil {
			log.Printf("Failed to create proxy request: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Copy headers
		for key, values := range r.Header {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}

		// Execute request
		// Don't follow redirects - pass them through to the client
		client := &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(proxyReq)
		if err != nil {
			log.Printf("Failed to proxy request to %s: %v", targetURL, err)
			http.Error(w, "Bad gateway", http.StatusBadGateway)
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Failed to close response body: %v", err)
			}
		}()

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Copy status code and body
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("Failed to copy response body: %v", err)
		}
	})
}

// createSPAHandler creates a handler for static files with SPA routing fallback
func createSPAHandler(staticDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(staticDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, r.URL.Path)

		// Check if file exists
		_, err := os.Stat(path)
		if err == nil {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist, check if it's an asset request (has extension)
		if strings.Contains(filepath.Base(r.URL.Path), ".") {
			// It's a file request but file doesn't exist - 404
			http.NotFound(w, r)
			return
		}

		// It's a SPA route (no extension), serve index.html
		indexPath := filepath.Join(staticDir, "index.html")
		http.ServeFile(w, r, indexPath)
	})
}
