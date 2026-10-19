package fx

import (
	"context"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/alerts"
	"github.com/DefensePoint/keycloak-monitoring/internal/config"
	"github.com/DefensePoint/keycloak-monitoring/internal/domain"
	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// A rule written as alert_type: "CLIENT_LOGIN_ERROR", the shape the example
// configuration uses, names the Keycloak event. An alert raised from a single
// event has the generic type "event", so the rule can only match through the
// event type recorded in its metadata.
func TestConvertEvent_AnEventTypeRuleSilencesTheEventsAlert(t *testing.T) {
	sm := alerts.NewSilenceManager(logger.NewNoop())
	if err := sm.LoadSilences(context.Background(), config.AlertSilencesConfig{{
		ID: "maintenance", Enabled: true,
		Matchers: config.AlertSilenceMatchersConfig{AlertType: "CLIENT_LOGIN_ERROR", ClientID: "production-app"},
	}}); err != nil {
		t.Fatalf("LoadSilences: %v", err)
	}

	// No repository: a silenced event must return before it is stored, and
	// would panic here if it were not.
	c := &eventAlertConverterImpl{silenceManager: sm, log: logger.NewNoop()}
	err := c.ConvertEvent(context.Background(), &domain.KeycloakEvent{
		TenantID: "t1", EventID: "e1", EventType: "CLIENT_LOGIN_ERROR",
		RealmName: "master", ClientID: "production-app",
	})
	if err != nil {
		t.Fatalf("ConvertEvent: %v", err)
	}
	if m := sm.GetMetrics(); m.TotalSilencedAlerts != 1 {
		t.Errorf("silenced %d alerts, want the one", m.TotalSilencedAlerts)
	}
}
