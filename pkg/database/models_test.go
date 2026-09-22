package database

import (
	"testing"
	"time"
)

func TestEvent_TableName(t *testing.T) {
	event := &Event{}
	expected := "events"

	if event.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, event.TableName())
	}
}

func TestUser_TableName(t *testing.T) {
	user := &User{}
	expected := "users"

	if user.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, user.TableName())
	}
}

func TestKeycloakEvent_TableName(t *testing.T) {
	event := &KeycloakEvent{}
	expected := "keycloak_events"

	if event.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, event.TableName())
	}
}

func TestKeycloakMetrics_TableName(t *testing.T) {
	metrics := &KeycloakMetrics{}
	expected := "keycloak_metrics"

	if metrics.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, metrics.TableName())
	}
}

func TestKeycloakHealth_TableName(t *testing.T) {
	health := &KeycloakHealth{}
	expected := "keycloak_health"

	if health.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, health.TableName())
	}
}

func TestKeycloakRealmInfo_TableName(t *testing.T) {
	realmInfo := &KeycloakRealmInfo{}
	expected := "keycloak_realms"

	if realmInfo.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, realmInfo.TableName())
	}
}

func TestConfigurationAlert_TableName(t *testing.T) {
	alert := &ConfigurationAlert{}
	expected := "configuration_alerts"

	if alert.TableName() != expected {
		t.Errorf("Expected table name %s, got %s", expected, alert.TableName())
	}
}

func TestEvent_FieldMapping(t *testing.T) {
	now := time.Now()
	event := &Event{
		ID:           1,
		EventID:      "test-id",
		Type:         "login",
		Category:     "authentication",
		Severity:     "info",
		Description:  "User logged in",
		Source:       "keycloak",
		SourceIP:     "192.168.1.1",
		SourceSystem: "prod",
		UserID:       "user-123",
		Username:     "testuser",
		Email:        "test@example.com",
		Location:     "US",
		RawData:      []byte(`{"key":"value"}`),
		Status:       "active",
		Timestamp:    now,
	}

	// Verify all fields are properly set
	if event.ID != 1 {
		t.Errorf("ID not set correctly")
	}
	if event.EventID != "test-id" {
		t.Errorf("EventID not set correctly")
	}
	if event.Type != "login" {
		t.Errorf("Type not set correctly")
	}
	if event.Category != "authentication" {
		t.Errorf("Category not set correctly")
	}
	if event.Severity != "info" {
		t.Errorf("Severity not set correctly")
	}
	if event.Description != "User logged in" {
		t.Errorf("Description not set correctly")
	}
	if event.Source != "keycloak" {
		t.Errorf("Source not set correctly")
	}
	if event.SourceIP != "192.168.1.1" {
		t.Errorf("SourceIP not set correctly")
	}
	if event.SourceSystem != "prod" {
		t.Errorf("SourceSystem not set correctly")
	}
	if event.UserID != "user-123" {
		t.Errorf("UserID not set correctly")
	}
	if event.Username != "testuser" {
		t.Errorf("Username not set correctly")
	}
	if event.Email != "test@example.com" {
		t.Errorf("Email not set correctly")
	}
	if event.Location != "US" {
		t.Errorf("Location not set correctly")
	}
	if string(event.RawData) != `{"key":"value"}` {
		t.Errorf("RawData not set correctly")
	}
	if event.Status != "active" {
		t.Errorf("Status not set correctly")
	}
	if !event.Timestamp.Equal(now) {
		t.Errorf("Timestamp not set correctly")
	}
}

func TestUser_FieldMapping(t *testing.T) {
	now := time.Now()
	user := &User{
		ID:                1,
		Subject:           "sub-456",
		Email:             "test@example.com",
		EmailVerified:     true,
		Name:              "Test User",
		GivenName:         "Test",
		FamilyName:        "User",
		PreferredUsername: "testuser",
		Locale:            "en-US",
		Username:          "testuser",
		PasswordHash:      "hashed",
		AuthMethod:        "simple",
		IsActive:          true,
		IsBlocked:         false,
		LastLoginAt:       &now,
		LastLoginIP:       "192.168.1.1",
		LoginCount:        5,
		LastAccessedAt:    now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Verify all fields are properly set
	if user.ID != 1 {
		t.Errorf("ID not set correctly")
	}
	if user.Subject != "sub-456" {
		t.Errorf("Subject not set correctly")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email not set correctly")
	}
	if user.EmailVerified != true {
		t.Errorf("EmailVerified not set correctly")
	}
	if user.Username != "testuser" {
		t.Errorf("Username not set correctly")
	}
	if user.IsBlocked != false {
		t.Errorf("IsBlocked not set correctly")
	}
	if !user.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt not set correctly")
	}
	if !user.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt not set correctly")
	}
}

func TestKeycloakEvent_FieldMapping(t *testing.T) {
	now := time.Now()
	event := &KeycloakEvent{
		Time:       now,
		EventID:    "kc-event-123",
		RealmID:    "realm-id",
		RealmName:  "test-realm",
		ClientID:   "client-101",
		SessionID:  "session-789",
		IPAddress:  "192.168.1.1",
		EventType:  "LOGIN",
		EventError: "",
		UserID:     "user-456",
		Username:   "testuser",
		Email:      "test@example.com",
		Details:    []byte(`{"username":"testuser"}`),
		Success:    true,
	}

	// Verify all fields are properly set
	if !event.Time.Equal(now) {
		t.Errorf("Time not set correctly")
	}
	if event.EventID != "kc-event-123" {
		t.Errorf("EventID not set correctly")
	}
	if event.RealmName != "test-realm" {
		t.Errorf("RealmName not set correctly")
	}
	if event.EventType != "LOGIN" {
		t.Errorf("EventType not set correctly")
	}
	if event.UserID != "user-456" {
		t.Errorf("UserID not set correctly")
	}
	if event.SessionID != "session-789" {
		t.Errorf("SessionID not set correctly")
	}
	if event.IPAddress != "192.168.1.1" {
		t.Errorf("IPAddress not set correctly")
	}
	if event.ClientID != "client-101" {
		t.Errorf("ClientID not set correctly")
	}
	if string(event.Details) != `{"username":"testuser"}` {
		t.Errorf("Details not set correctly")
	}
	if event.Success != true {
		t.Errorf("Success not set correctly")
	}
}

func TestKeycloakMetrics_FieldMapping(t *testing.T) {
	now := time.Now()
	metrics := &KeycloakMetrics{
		Time:              now,
		RealmName:         "test-realm",
		TotalUsers:        500,
		EnabledUsers:      450,
		DisabledUsers:     50,
		ActiveSessions:    100,
		OfflineSessions:   20,
		TotalClients:      25,
		LoginEvents:       1000,
		LogoutEvents:      800,
		FailedLoginEvents: 50,
		RegisterEvents:    200,
	}

	// Verify key fields are properly set
	if !metrics.Time.Equal(now) {
		t.Errorf("Time not set correctly")
	}
	if metrics.RealmName != "test-realm" {
		t.Errorf("RealmName not set correctly")
	}
	if metrics.TotalUsers != 500 {
		t.Errorf("TotalUsers not set correctly")
	}
	if metrics.ActiveSessions != 100 {
		t.Errorf("ActiveSessions not set correctly")
	}
	if metrics.LoginEvents != 1000 {
		t.Errorf("LoginEvents not set correctly")
	}
}

func TestKeycloakHealth_FieldMapping(t *testing.T) {
	now := time.Now()
	health := &KeycloakHealth{
		Time:          now,
		Status:        "UP",
		ResponseTime:  150,
		ServerVersion: "21.0.0",
		UptimeMillis:  86400000,
		MemoryUsed:    512000000,
		MemoryMax:     1024000000,
		MemoryFree:    512000000,
		ErrorMessage:  "",
	}

	// Verify all fields are properly set
	if !health.Time.Equal(now) {
		t.Errorf("Time not set correctly")
	}
	if health.Status != "UP" {
		t.Errorf("Status not set correctly")
	}
	if health.ResponseTime != 150 {
		t.Errorf("ResponseTime not set correctly")
	}
	if health.ServerVersion != "21.0.0" {
		t.Errorf("ServerVersion not set correctly")
	}
	if health.UptimeMillis != 86400000 {
		t.Errorf("UptimeMillis not set correctly")
	}
	if health.MemoryUsed != 512000000 {
		t.Errorf("MemoryUsed not set correctly")
	}
}

func TestKeycloakRealmInfo_FieldMapping(t *testing.T) {
	now := time.Now()
	realmInfo := &KeycloakRealmInfo{
		ID:                        1,
		RealmID:                   "realm-123",
		RealmName:                 "test-realm",
		DisplayName:               "Test Realm",
		Enabled:                   true,
		SslRequired:               "external",
		RegistrationAllowed:       false,
		LoginWithEmailAllowed:     true,
		DuplicateEmailsAllowed:    false,
		ResetPasswordAllowed:      true,
		EditUsernameAllowed:       false,
		BruteForceProtected:       true,
		EventsEnabled:             true,
		EventsListeners:           `["listener1","listener2"]`,
		EnabledEventTypes:         `["LOGIN","LOGOUT"]`,
		AdminEventsEnabled:        true,
		AdminEventsDetailsEnabled: false,
		LastChecked:               now,
		IsHealthy:                 true,
		HealthMessage:             "OK",
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}

	// Verify key fields are properly set
	if realmInfo.ID != 1 {
		t.Errorf("ID not set correctly")
	}
	if realmInfo.RealmID != "realm-123" {
		t.Errorf("RealmID not set correctly")
	}
	if realmInfo.RealmName != "test-realm" {
		t.Errorf("RealmName not set correctly")
	}
	if realmInfo.DisplayName != "Test Realm" {
		t.Errorf("DisplayName not set correctly")
	}
	if realmInfo.Enabled != true {
		t.Errorf("Enabled not set correctly")
	}
	if realmInfo.BruteForceProtected != true {
		t.Errorf("BruteForceProtected not set correctly")
	}
	if !realmInfo.LastChecked.Equal(now) {
		t.Errorf("LastChecked not set correctly")
	}
}

func TestEvent_EmptyRawData(t *testing.T) {
	event := &Event{
		ID:      1,
		EventID: "test",
		RawData: []byte{},
	}

	if event.RawData == nil {
		t.Error("RawData should not be nil")
	}
	if len(event.RawData) != 0 {
		t.Error("RawData should be empty")
	}
}

func TestKeycloakEvent_EmptyDetails(t *testing.T) {
	event := &KeycloakEvent{
		EventID: "test",
		Details: []byte{},
	}

	if event.Details == nil {
		t.Error("Details should not be nil")
	}
	if len(event.Details) != 0 {
		t.Error("Details should be empty")
	}
}

func TestKeycloakHealth_EmptyRawData(t *testing.T) {
	health := &KeycloakHealth{
		Time:    time.Now(),
		RawData: []byte{},
	}

	if health.RawData == nil {
		t.Error("RawData should not be nil")
	}
	if len(health.RawData) != 0 {
		t.Error("RawData should be empty")
	}
}

func TestKeycloakMetrics_EmptyRawMetrics(t *testing.T) {
	metrics := &KeycloakMetrics{
		Time:       time.Now(),
		RealmName:  "test",
		RawMetrics: []byte{},
	}

	if metrics.RawMetrics == nil {
		t.Error("RawMetrics should not be nil")
	}
	if len(metrics.RawMetrics) != 0 {
		t.Error("RawMetrics should be empty")
	}
}

func TestKeycloakRealmInfo_EmptyRawData(t *testing.T) {
	realmInfo := &KeycloakRealmInfo{
		ID:      1,
		RawData: []byte{},
	}

	if realmInfo.RawData == nil {
		t.Error("RawData should not be nil")
	}
	if len(realmInfo.RawData) != 0 {
		t.Error("RawData should be empty")
	}
}

func TestConfigurationAlert_FieldMapping(t *testing.T) {
	now := time.Now()
	resolvedAt := time.Now().Add(1 * time.Hour)
	acknowledgedAt := time.Now().Add(30 * time.Minute)

	alert := &ConfigurationAlert{
		ID:             1,
		AlertID:        "alert-123",
		Type:           "identity_provider",
		Severity:       "warning",
		Status:         "active",
		Title:          "Test Alert",
		Description:    "Test Description",
		Recommendation: "Test Recommendation",
		ResourceType:   "identity_provider",
		ResourceID:     "idp-123",
		ResourceName:   "test-idp",
		RealmName:      "test-realm",
		CheckType:      "metadata_check",
		Metadata:       []byte(`{"key":"value"}`),
		FirstDetected:  now,
		LastSeen:       now,
		ResolvedAt:     &resolvedAt,
		AcknowledgedAt: &acknowledgedAt,
		AcknowledgedBy: "admin@example.com",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Verify all fields are properly set
	if alert.ID != 1 {
		t.Errorf("ID not set correctly")
	}
	if alert.AlertID != "alert-123" {
		t.Errorf("AlertID not set correctly")
	}
	if alert.Type != "identity_provider" {
		t.Errorf("Type not set correctly")
	}
	if alert.Severity != "warning" {
		t.Errorf("Severity not set correctly")
	}
	if alert.Status != "active" {
		t.Errorf("Status not set correctly")
	}
	if alert.Title != "Test Alert" {
		t.Errorf("Title not set correctly")
	}
	if alert.Description != "Test Description" {
		t.Errorf("Description not set correctly")
	}
	if alert.Recommendation != "Test Recommendation" {
		t.Errorf("Recommendation not set correctly")
	}
	if alert.ResourceType != "identity_provider" {
		t.Errorf("ResourceType not set correctly")
	}
	if alert.ResourceID != "idp-123" {
		t.Errorf("ResourceID not set correctly")
	}
	if alert.ResourceName != "test-idp" {
		t.Errorf("ResourceName not set correctly")
	}
	if alert.RealmName != "test-realm" {
		t.Errorf("RealmName not set correctly")
	}
	if alert.CheckType != "metadata_check" {
		t.Errorf("CheckType not set correctly")
	}
	if string(alert.Metadata) != `{"key":"value"}` {
		t.Errorf("Metadata not set correctly")
	}
	if !alert.FirstDetected.Equal(now) {
		t.Errorf("FirstDetected not set correctly")
	}
	if !alert.LastSeen.Equal(now) {
		t.Errorf("LastSeen not set correctly")
	}
	if alert.ResolvedAt == nil {
		t.Error("ResolvedAt should be set")
	} else if !alert.ResolvedAt.Equal(resolvedAt) {
		t.Error("ResolvedAt not set correctly")
	}
	if alert.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt should be set")
	} else if !alert.AcknowledgedAt.Equal(acknowledgedAt) {
		t.Error("AcknowledgedAt not set correctly")
	}
	if alert.AcknowledgedBy != "admin@example.com" {
		t.Errorf("AcknowledgedBy not set correctly")
	}
	if !alert.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt not set correctly")
	}
	if !alert.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt not set correctly")
	}
}

func TestConfigurationAlert_EmptyMetadata(t *testing.T) {
	alert := &ConfigurationAlert{
		ID:       1,
		AlertID:  "test",
		Metadata: []byte{},
	}

	if alert.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
	if len(alert.Metadata) != 0 {
		t.Error("Metadata should be empty")
	}
}
