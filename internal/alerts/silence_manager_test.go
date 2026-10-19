package alerts

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func silenceWith(t *testing.T, m config.AlertSilenceMatchersConfig) SilenceManager {
	t.Helper()
	sm := NewSilenceManager(logger.NewNoop())
	if err := sm.LoadSilences(context.Background(), config.AlertSilencesConfig{
		{ID: "rule", Enabled: true, Matchers: m},
	}); err != nil {
		t.Fatalf("LoadSilences: %v", err)
	}
	return sm
}

func TestSilenceMatching(t *testing.T) {
	// Shaped like the alerts the threshold checks raise: numbers and flags
	// next to the names a rule matches on.
	threshold := &domain.Alert{
		TenantID: "t1", Type: domain.AlertTypeSecurity, RealmName: "master",
		ResourceType: "amfa_client", ResourceID: "test-app",
		Metadata: `{"event_type":"LOGIN_ERROR","client":"test-app","count":12,"threshold":5,"is_vpn":true,"nested":{"a":1}}`,
	}
	// Shaped like the alerts raised from single events, which already matched.
	event := &domain.Alert{
		TenantID: "t1", Type: domain.AlertTypeEvent, RealmName: "master", ResourceType: "event",
		Metadata: `{"client_id":"web","username":"alice","ip_address":"10.0.0.1"}`,
	}

	cases := []struct {
		name  string
		m     config.AlertSilenceMatchersConfig
		alert *domain.Alert
		want  bool
	}{
		{"event type through alert_type", config.AlertSilenceMatchersConfig{AlertType: "LOGIN_*"}, threshold, true},
		{"alert type itself still matches", config.AlertSilenceMatchersConfig{AlertType: "security"}, threshold, true},
		{"another event type", config.AlertSilenceMatchersConfig{AlertType: "CLIENT_LOGIN_ERROR"}, threshold, false},
		{"client recorded as client", config.AlertSilenceMatchersConfig{ClientID: "test-*"}, threshold, true},
		{"another client", config.AlertSilenceMatchersConfig{ClientID: "prod-*"}, threshold, false},
		{"a number", config.AlertSilenceMatchersConfig{FieldMatches: map[string]string{"threshold": "5"}}, threshold, true},
		{"a flag", config.AlertSilenceMatchersConfig{FieldMatches: map[string]string{"is_vpn": "true"}}, threshold, true},
		{"a nested value matches nothing", config.AlertSilenceMatchersConfig{FieldMatches: map[string]string{"nested": "*"}}, threshold, false},
		{"event alert client", config.AlertSilenceMatchersConfig{ClientID: "web"}, event, true},
		{"event alert username", config.AlertSilenceMatchersConfig{Username: "ali*"}, event, true},
		{"event alert field", config.AlertSilenceMatchersConfig{FieldMatches: map[string]string{"ip_address": "10.0.0.*"}}, event, true},
		{"event alert type", config.AlertSilenceMatchersConfig{AlertType: "event"}, event, true},
		{"unreadable metadata matches no metadata rule",
			config.AlertSilenceMatchersConfig{ClientID: "*"}, &domain.Alert{Metadata: "{not json"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := silenceWith(t, c.m).ShouldSilence(c.alert)
			if got != c.want {
				t.Errorf("ShouldSilence = %v, want %v", got, c.want)
			}
		})
	}
}
