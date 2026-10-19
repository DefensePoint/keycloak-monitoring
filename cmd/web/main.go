// Web Frontend Server
package main

import (
	"context"
	"encoding/json"
	"errors"
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
	for _, w := range cfg.PlaceholderWarnings {
		log.Printf("WARNING: %s", w)
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
		Handler:      withHSTS(cfg.HTTP.SecurityHeaders, mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: writeTimeoutFor(upstreamTimeout),
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

// withHSTS adds the configured Strict-Transport-Security header to every
// response, the application page included: the API sets it on its own
// responses, but the page the browser loads first comes from here.
func withHSTS(headers config.SecurityHeadersConfig, next http.Handler) http.Handler {
	if !headers.Enabled || headers.HSTS == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", headers.HSTS)
		next.ServeHTTP(w, r)
	})
}

// upstreamTimeout is how long the proxy waits for the API to answer.
const upstreamTimeout = 30 * time.Second

// writeMargin is how much longer than the upstream timeout the server keeps a
// response writable. With the two equal, the proxy's timeout error was written
// after the write deadline, so the connection closed with no response and the
// browser silently retried the GET, running the slow request again.
const writeMargin = 10 * time.Second

// writeTimeoutFor is the server's write deadline for a given upstream timeout.
func writeTimeoutFor(upstream time.Duration) time.Duration {
	return upstream + writeMargin
}

// createProxyHandler creates a reverse proxy handler for API requests
func createProxyHandler(backendURL string) http.Handler {
	return newProxyHandler(backendURL, upstreamTimeout)
}

// newProxyHandler is createProxyHandler with the upstream timeout given.
func newProxyHandler(backendURL string, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Build target URL
		targetURL := backendURL + r.URL.Path
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		// Create new request. It carries the browser's context, so the API
		// stops working on it once the browser has gone.
		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
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
		// The backend sees this proxy's Host, so pass on the host the browser
		// addressed. Logout compares it with Origin to refuse requests from
		// another site. An outer proxy that already set it wins.
		if proxyReq.Header.Get("X-Forwarded-Host") == "" {
			proxyReq.Header.Set("X-Forwarded-Host", r.Host)
		}
		// The API server's cross-origin check compares Origin with the Host it
		// receives, so it gets the browser's Host too.
		proxyReq.Host = r.Host

		// Execute request
		// Don't follow redirects - pass them through to the client
		client := &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(proxyReq)
		if err != nil {
			log.Printf("Failed to proxy request to %s: %v", targetURL, err)
			switch {
			case r.Context().Err() != nil:
				// The browser left; nobody reads an answer.
			case isTimeout(err):
				writeJSONError(w, http.StatusGatewayTimeout, "The API did not answer in time")
			default:
				http.Error(w, "Bad gateway", http.StatusBadGateway)
			}
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Failed to close response body: %v", err)
			}
		}()

		// Copy response headers. The backend's replace this server's own,
		// so a header both set, such as HSTS, is sent once.
		for key, values := range resp.Header {
			w.Header().Del(key)
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

// isTimeout reports whether err is the proxy giving up on the API.
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}

// writeJSONError answers with the {"error": ...} body the API's own errors
// have, which the web client reads.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Printf("Failed to write error response: %v", err)
	}
}

// Cache-Control values for the files the web server serves. Vite names the
// build files under assets/ after their content, so a new build gives them
// new names and a cached copy can be kept for good. Everything else keeps its
// name from build to build: index.html above all, which names the build files
// of its own build. Without a Cache-Control a browser could reuse an
// index.html from before a deploy without asking, and run the old build, or
// show a blank page once the old build files were gone. Those files are
// revalidated on every load instead, which costs a 304 when they are
// unchanged.
const (
	cacheRevalidate = "no-cache"
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheNever      = "no-store"
)

// createSPAHandler creates a handler for static files with SPA routing fallback
func createSPAHandler(staticDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(staticDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, r.URL.Path)
		// Check if file exists
		info, err := os.Stat(path)
		if err == nil {
			// Only the files are named after their content, not the
			// directory, which the file server lists.
			buildFile := strings.HasPrefix(r.URL.Path, "/assets/") && !info.IsDir()
			// File exists, serve it
			if buildFile {
				w.Header().Set("Cache-Control", cacheImmutable)
			} else {
				w.Header().Set("Cache-Control", cacheRevalidate)
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist, check if it's an asset request (has extension)
		if strings.Contains(filepath.Base(r.URL.Path), ".") {
			// It's a file request but file doesn't exist - 404. A page from
			// before a deploy asks for build files the deploy removed; the
			// answer must not be kept, as a later deploy may bring them back.
			w.Header().Set("Cache-Control", cacheNever)
			http.NotFound(w, r)
			return
		}

		// It's a SPA route (no extension), serve index.html
		indexPath := filepath.Join(staticDir, "index.html")
		w.Header().Set("Cache-Control", cacheRevalidate)
		http.ServeFile(w, r, indexPath)
	})
}
