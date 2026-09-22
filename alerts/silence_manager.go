// Package alerts provides the alerts domain model and services.
package alerts

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// SilenceManager defines the interface for alert silence management.
type SilenceManager interface {
	// LoadSilences loads silence rules from configuration.
	LoadSilences(ctx context.Context, cfg config.AlertSilencesConfig) error

	// ShouldSilence checks if an alert should be silenced based on loaded rules.
	// Returns (shouldSilence, matchedRuleID).
	ShouldSilence(alert *domain.Alert) (bool, string)

	// IsSilenced is a convenience method that just returns whether the alert should be silenced.
	IsSilenced(tenantID, alertType, resourceType, resourceID string) bool

	// GetActiveSilences returns currently active silence rules.
	GetActiveSilences() []config.AlertSilenceConfig

	// GetMetrics returns silence metrics.
	GetMetrics() SilenceMetrics
}

// SilenceMetrics tracks silence statistics.
type SilenceMetrics struct {
	TotalSilencedAlerts int64            `json:"total_silenced_alerts"`
	SilencesByRule      map[string]int64 `json:"silences_by_rule"`
	SilencesByTenant    map[string]int64 `json:"silences_by_tenant"`
	SilencesByType      map[string]int64 `json:"silences_by_type"`
}

// activeSilence represents a loaded and active silence rule.
type activeSilence struct {
	config.AlertSilenceConfig
	startsAt time.Time
	endsAt   time.Time
}

// silenceManager implements SilenceManager interface.
type silenceManager struct {
	silences       []activeSilence
	mu             sync.RWMutex
	logger         *logger.Logger
	metricsTracker *silenceMetricsTracker
}

// silenceMetricsTracker tracks silence statistics internally.
type silenceMetricsTracker struct {
	mu                  sync.RWMutex
	totalSilencedAlerts int64
	silencesByRule      map[string]int64
	silencesByTenant    map[string]int64
	silencesByType      map[string]int64
}

// NewSilenceManager creates a new silence manager.
func NewSilenceManager(log *logger.Logger) SilenceManager {
	return &silenceManager{
		silences: make([]activeSilence, 0),
		logger:   log.WithComponent("silence_manager"),
		metricsTracker: &silenceMetricsTracker{
			silencesByRule:   make(map[string]int64),
			silencesByTenant: make(map[string]int64),
			silencesByType:   make(map[string]int64),
		},
	}
}

// LoadSilences loads silence rules from configuration.
func (sm *silenceManager) LoadSilences(ctx context.Context, cfg config.AlertSilencesConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.silences = make([]activeSilence, 0, len(cfg))
	now := time.Now()

	for _, silenceCfg := range cfg {
		if !silenceCfg.Enabled {
			continue
		}

		silence := activeSilence{
			AlertSilenceConfig: silenceCfg,
		}

		// Parse start time
		if silenceCfg.StartsAt != "" {
			startsAt, err := time.Parse(time.RFC3339, silenceCfg.StartsAt)
			if err != nil {
				sm.logger.Error("Invalid starts_at time for silence",
					logger.Str("silence_id", silenceCfg.ID),
					logger.Err(err))
				continue
			}
			silence.startsAt = startsAt
		} else {
			silence.startsAt = now
		}

		// Parse duration and compute end time
		if silenceCfg.Duration != "" {
			duration, err := time.ParseDuration(silenceCfg.Duration)
			if err != nil {
				sm.logger.Error("Invalid duration for silence",
					logger.Str("silence_id", silenceCfg.ID),
					logger.Err(err))
				continue
			}
			silence.endsAt = silence.startsAt.Add(duration)
		} else if silenceCfg.EndsAt != "" {
			endsAt, err := time.Parse(time.RFC3339, silenceCfg.EndsAt)
			if err != nil {
				sm.logger.Error("Invalid ends_at time for silence",
					logger.Str("silence_id", silenceCfg.ID),
					logger.Err(err))
				continue
			}
			silence.endsAt = endsAt
		}
		// else: permanent silence (zero endsAt)

		sm.silences = append(sm.silences, silence)
		sm.logger.Info("Loaded alert silence",
			logger.Str("silence_id", silenceCfg.ID),
			logger.Str("name", silenceCfg.Name),
			logger.Bool("permanent", silence.endsAt.IsZero()))
	}

	sm.logger.Info("Loaded alert silences from configuration",
		logger.Int("count", len(sm.silences)))

	return nil
}

// ShouldSilence checks if an alert should be silenced based on loaded rules.
func (sm *silenceManager) ShouldSilence(alert *domain.Alert) (bool, string) {
	sm.mu.RLock()
	now := time.Now()
	var matchedSilenceID string

	for _, silence := range sm.silences {
		// Check if silence is currently active (time-based)
		if now.Before(silence.startsAt) {
			continue
		}
		if !silence.endsAt.IsZero() && now.After(silence.endsAt) {
			continue
		}

		// Check if silence applies to this tenant
		if silence.TenantID != "" && alert.TenantID != silence.TenantID {
			continue
		}

		// Match against silence rules
		if sm.matchesSilence(alert, &silence) {
			matchedSilenceID = silence.ID
			break
		}
	}
	sm.mu.RUnlock()

	// Record metrics outside of read lock to avoid lock contention
	if matchedSilenceID != "" {
		sm.recordSilencedAlert(matchedSilenceID, alert)
		return true, matchedSilenceID
	}

	return false, ""
}

// IsSilenced is a convenience method for checking if an alert should be silenced.
// This creates a minimal alert for checking against silence rules.
func (sm *silenceManager) IsSilenced(tenantID, alertType, resourceType, resourceID string) bool {
	alert := &domain.Alert{
		TenantID:     tenantID,
		Type:         domain.AlertType(alertType),
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}
	silenced, _ := sm.ShouldSilence(alert)
	return silenced
}

// matchesSilence checks if an alert matches a silence rule's matchers.
func (sm *silenceManager) matchesSilence(alert *domain.Alert, silence *activeSilence) bool {
	matchers := silence.Matchers

	// Match alert type
	if matchers.AlertType != "" {
		if !matchPattern(string(alert.Type), matchers.AlertType) {
			return false
		}
	}

	// Match source
	if matchers.Source != "" {
		if string(alert.Source) != matchers.Source {
			return false
		}
	}

	// Match severity
	if matchers.Severity != "" {
		if string(alert.Severity) != matchers.Severity {
			return false
		}
	}

	// Match realm name
	if matchers.RealmName != "" {
		if !matchPattern(alert.RealmName, matchers.RealmName) {
			return false
		}
	}

	// Parse metadata for extended matching
	var metadata map[string]string
	if alert.Metadata != "" {
		if err := json.Unmarshal([]byte(alert.Metadata), &metadata); err != nil {
			sm.logger.Debug("Failed to parse alert metadata for matching",
				logger.Str("silence_id", silence.ID),
				logger.Err(err))
			metadata = make(map[string]string)
		}
	} else {
		metadata = make(map[string]string)
	}

	// Match client ID (from metadata or resource fields)
	if matchers.ClientID != "" {
		clientID := metadata["client_id"]
		if clientID == "" && alert.ResourceType == "client" {
			clientID = alert.ResourceID
		}
		if clientID == "" || !matchPattern(clientID, matchers.ClientID) {
			return false
		}
	}

	// Match username (from metadata)
	if matchers.Username != "" {
		username := metadata["username"]
		if username == "" || !matchPattern(username, matchers.Username) {
			return false
		}
	}

	// Match additional fields from metadata
	if len(matchers.FieldMatches) > 0 {
		for field, pattern := range matchers.FieldMatches {
			value := metadata[field]
			if value == "" || !matchPattern(value, pattern) {
				return false
			}
		}
	}

	return true
}

// matchPattern performs wildcard pattern matching.
// Supports * as wildcard.
func matchPattern(value, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == value {
		return true
	}

	// Simple wildcard matching using filepath.Match
	matched, _ := filepath.Match(pattern, value)
	return matched
}

// recordSilencedAlert records that an alert was silenced for metrics.
func (sm *silenceManager) recordSilencedAlert(silenceID string, alert *domain.Alert) {
	sm.metricsTracker.mu.Lock()
	defer sm.metricsTracker.mu.Unlock()

	sm.metricsTracker.totalSilencedAlerts++
	sm.metricsTracker.silencesByRule[silenceID]++
	sm.metricsTracker.silencesByTenant[alert.TenantID]++
	sm.metricsTracker.silencesByType[string(alert.Type)]++
}

// GetMetrics returns silence metrics.
func (sm *silenceManager) GetMetrics() SilenceMetrics {
	sm.metricsTracker.mu.RLock()
	defer sm.metricsTracker.mu.RUnlock()

	return SilenceMetrics{
		TotalSilencedAlerts: sm.metricsTracker.totalSilencedAlerts,
		SilencesByRule:      copyInt64Map(sm.metricsTracker.silencesByRule),
		SilencesByTenant:    copyInt64Map(sm.metricsTracker.silencesByTenant),
		SilencesByType:      copyInt64Map(sm.metricsTracker.silencesByType),
	}
}

// GetActiveSilences returns currently active silences.
func (sm *silenceManager) GetActiveSilences() []config.AlertSilenceConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	now := time.Now()
	active := make([]config.AlertSilenceConfig, 0)

	for _, silence := range sm.silences {
		// Check if silence is currently active
		if now.Before(silence.startsAt) {
			continue
		}
		if !silence.endsAt.IsZero() && now.After(silence.endsAt) {
			continue
		}
		active = append(active, silence.AlertSilenceConfig)
	}

	return active
}

// copyInt64Map creates a copy of an int64 map.
func copyInt64Map(m map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
