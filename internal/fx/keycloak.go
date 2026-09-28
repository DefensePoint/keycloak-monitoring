package fx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/fx"
	"gorm.io/gorm"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	chihttp "github.com/DefensePoint/keycloak-monitoring/internal/http/chi"
	"github.com/DefensePoint/keycloak-monitoring/internal/keycloak"
	kcpostgres "github.com/DefensePoint/keycloak-monitoring/internal/keycloak/postgres"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/DefensePoint/keycloak-monitoring/pkg/keycloakadmin"
	"github.com/DefensePoint/keycloak-monitoring/pkg/saferequest"
)

// KeycloakModule provides keycloak domain components
var KeycloakModule = fx.Module("keycloak",
	fx.Provide(
		// Repositories - direct GORM implementation
		provideKeycloakEventRepo,
		provideKeycloakMetricsRepo,
		provideKeycloakHealthRepo,
		provideKeycloakRealmRepo,

		// Service
		provideKeycloakService,

		// Connection tester for tenant handlers
		provideKeycloakConnectionTester,
	),
)

// Repository providers - use direct GORM implementation
func provideKeycloakEventRepo(db *gorm.DB) keycloak.EventRepository {
	return kcpostgres.NewEventRepository(db)
}

func provideKeycloakMetricsRepo(db *gorm.DB) keycloak.MetricsRepository {
	return kcpostgres.NewMetricsRepository(db)
}

func provideKeycloakHealthRepo(db *gorm.DB) keycloak.HealthRepository {
	return kcpostgres.NewHealthRepository(db)
}

func provideKeycloakRealmRepo(db *gorm.DB) keycloak.RealmRepository {
	return kcpostgres.NewRealmRepository(db)
}

// Service
func provideKeycloakService(
	eventRepo keycloak.EventRepository,
	metricsRepo keycloak.MetricsRepository,
	healthRepo keycloak.HealthRepository,
	realmRepo keycloak.RealmRepository,
) keycloak.Service {
	// ClientProvider and VersionChecker are optional - pass nil for now
	// These will be provided by the monitor pool when live Keycloak access is needed
	return keycloak.NewService(eventRepo, metricsRepo, healthRepo, realmRepo, nil, nil)
}

// ============================================================================
// KEYCLOAK CONNECTION TESTER
// ============================================================================

// clientCreationError represents an error during Keycloak client creation.
// This error type signals that the client could not be created (e.g., invalid URL, config).
type clientCreationError struct {
	err error
}

func (e *clientCreationError) Error() string {
	return e.err.Error()
}

func (e *clientCreationError) Unwrap() error {
	return e.err
}

func (e *clientCreationError) IsClientCreationError() bool {
	return true
}

// keycloakConnectionTester implements chihttp.KeycloakConnectionTester
type keycloakConnectionTester struct {
	log    *logger.Logger
	policy saferequest.Policy
}

// testConnectionTimeout bounds the whole "Test Connection" request (client
// creation, base-URL auto-detection, and the health check) so a user gets
// feedback quickly even when the target host hangs instead of responding.
const testConnectionTimeout = 10 * time.Second

// provideKeycloakConnectionTester creates a connection tester for tenant handlers.
func provideKeycloakConnectionTester(cfg *config.AppConfig, log *logger.Logger) chihttp.KeycloakConnectionTester {
	return &keycloakConnectionTester{
		log:    log,
		policy: keycloakSafeRequestPolicy(cfg),
	}
}

// keycloakSafeRequestPolicy derives the SSRF policy from app configuration.
// AllowPrivateRanges defaults to true (set in setDefaults) so on-prem
// deployments keep working out of the box.
func keycloakSafeRequestPolicy(cfg *config.AppConfig) saferequest.Policy {
	conn := cfg.Keycloak.Global.Connection
	timeout := conn.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return saferequest.Policy{
		AllowPrivateRanges: conn.AllowPrivateRanges,
		Timeout:            timeout,
		SkipTLSVerify:      conn.SkipTLSVerify,
	}
}

// ValidateServerURL checks that serverURL is acceptable under the active
// SSRF policy. Used by create/update tenant handlers to reject malicious
// URLs before they are persisted.
func (t *keycloakConnectionTester) ValidateServerURL(ctx context.Context, serverURL string) error {
	return saferequest.ValidateURLContext(ctx, serverURL, t.policy)
}

// TestConnection tests connectivity to a Keycloak instance.
// Returns ClientCreationError if client creation fails (should result in 400 Bad Request).
// Returns regular error if authentication fails (should result in 200 OK with error status).
func (t *keycloakConnectionTester) TestConnection(
	ctx context.Context,
	serverURL, adminRealm, clientID, clientSecret string,
) error {
	// Bound the whole test: a host that accepts the TCP connection but never
	// responds is otherwise limited only by the HTTP client's timeout, which
	// NewClient can hit twice (base-URL auto-detection retries once with the
	// /auth prefix) — up to a minute of a hung request with no feedback.
	ctx, cancel := context.WithTimeout(ctx, testConnectionTimeout)
	defer cancel()

	// SSRF guard: validate the target before constructing any HTTP client.
	// We surface a generic error to the handler; the detailed reason stays in
	// the server log so an attacker cannot use the response to map internal
	// network topology.
	if err := saferequest.ValidateURLContext(ctx, serverURL, t.policy); err != nil {
		t.log.Warn("Rejected outbound URL in test-connection",
			logger.Str("server_url", serverURL),
			logger.Err(err))
		return &clientCreationError{err: errors.New("connection failed")}
	}

	// Create a keycloakadmin.ClientConfig for testing
	clientConfig := &keycloakadmin.ClientConfig{
		ServerURL:     serverURL,
		AdminRealm:    adminRealm,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		Timeout:       30 * time.Second,
		SkipTLSVerify: t.policy.SkipTLSVerify,
		MaxRetries:    2,
		RetryBackoff:  3 * time.Second,
		HTTPClient:    saferequest.HTTPClient(t.policy),
	}

	// Create logger adapter for keycloakadmin
	loggerAdapter := &connectionTesterLoggerAdapter{log: t.log}

	// Create a temporary Keycloak client for testing
	client, err := keycloakadmin.NewClient(ctx, clientConfig, loggerAdapter)
	if err != nil {
		// Wrap as clientCreationError to signal 400 Bad Request, with a
		// generic message so we don't echo "blocked: link-local" back.
		if errors.Is(err, saferequest.ErrBlockedAddress) {
			t.log.Warn("Blocked dial in test-connection",
				logger.Str("server_url", serverURL),
				logger.Err(err))
			return &clientCreationError{err: errors.New("connection failed")}
		}
		return &clientCreationError{err: err}
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.log.Warn("Failed to close test Keycloak client", logger.Err(closeErr))
		}
	}()

	// Test authentication by checking if we can authenticate
	if !client.IsHealthy(ctx) {
		// Return regular error for authentication failure (200 OK with error status)
		return fmt.Errorf("authentication failed")
	}

	return nil
}

// connectionTesterLoggerAdapter adapts *logger.Logger to keycloakadmin.Logger
type connectionTesterLoggerAdapter struct {
	log *logger.Logger
}

func (a *connectionTesterLoggerAdapter) Info(msg string, fields ...any) {
	a.log.Info(msg, convertToLoggerFields(fields)...)
}
func (a *connectionTesterLoggerAdapter) Error(msg string, fields ...any) {
	a.log.Error(msg, convertToLoggerFields(fields)...)
}
func (a *connectionTesterLoggerAdapter) Warn(msg string, fields ...any) {
	a.log.Warn(msg, convertToLoggerFields(fields)...)
}
func (a *connectionTesterLoggerAdapter) Debug(msg string, fields ...any) {
	a.log.Debug(msg, convertToLoggerFields(fields)...)
}

// convertToLoggerFields converts key-value pairs to logger.Field slice
func convertToLoggerFields(fields []any) []logger.Field {
	if len(fields) == 0 {
		return nil
	}
	result := make([]logger.Field, 0, len(fields)/2)
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		value := fields[i+1]
		switch v := value.(type) {
		case string:
			result = append(result, logger.Str(key, v))
		case int:
			result = append(result, logger.Int(key, v))
		case int64:
			result = append(result, logger.Int(key, int(v)))
		case error:
			result = append(result, logger.Err(v))
		default:
			result = append(result, logger.Any(key, v))
		}
	}
	return result
}
