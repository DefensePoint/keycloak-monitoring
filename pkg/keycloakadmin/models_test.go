package keycloakadmin

import (
	"encoding/json"
	"testing"
)

func TestTokenResponse_JSONMarshaling(t *testing.T) {
	token := &TokenResponse{
		AccessToken:      "access-token-value",
		ExpiresIn:        300,
		RefreshExpiresIn: 1800,
		RefreshToken:     "refresh-token-value",
		TokenType:        "Bearer",
		SessionState:     "session-state-value",
		Scope:            "openid profile email",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Failed to marshal TokenResponse: %v", err)
	}

	// Unmarshal back
	var unmarshaled TokenResponse
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal TokenResponse: %v", err)
	}

	// Verify fields
	if unmarshaled.AccessToken != token.AccessToken {
		t.Errorf("AccessToken mismatch: got %s, want %s", unmarshaled.AccessToken, token.AccessToken)
	}
	if unmarshaled.ExpiresIn != token.ExpiresIn {
		t.Errorf("ExpiresIn mismatch: got %d, want %d", unmarshaled.ExpiresIn, token.ExpiresIn)
	}
	if unmarshaled.TokenType != token.TokenType {
		t.Errorf("TokenType mismatch: got %s, want %s", unmarshaled.TokenType, token.TokenType)
	}
}

func TestRealmRepresentation_JSONMarshaling(t *testing.T) {
	realm := &RealmRepresentation{
		ID:                        "realm-id",
		Realm:                     "test-realm",
		DisplayName:               "Test Realm",
		Enabled:                   true,
		SslRequired:               "external",
		RegistrationAllowed:       false,
		LoginWithEmailAllowed:     true,
		BruteForceProtected:       true,
		EventsEnabled:             true,
		EventsListeners:           []string{"jboss-logging", "metrics-listener"},
		EnabledEventTypes:         []string{"LOGIN", "LOGOUT"},
		AdminEventsEnabled:        true,
		AdminEventsDetailsEnabled: false,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(realm)
	if err != nil {
		t.Fatalf("Failed to marshal RealmRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled RealmRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal RealmRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != realm.ID {
		t.Errorf("ID mismatch")
	}
	if unmarshaled.Realm != realm.Realm {
		t.Errorf("Realm mismatch")
	}
	if unmarshaled.Enabled != realm.Enabled {
		t.Errorf("Enabled mismatch")
	}
	if len(unmarshaled.EventsListeners) != len(realm.EventsListeners) {
		t.Errorf("EventsListeners length mismatch")
	}
}

func TestUserRepresentation_JSONMarshaling(t *testing.T) {
	user := &UserRepresentation{
		ID:               "user-id",
		CreatedTimestamp: 1609459200000,
		Username:         "testuser",
		Enabled:          true,
		EmailVerified:    true,
		FirstName:        "Test",
		LastName:         "User",
		Email:            "test@example.com",
		Attributes: map[string][]string{
			"custom": {"value1", "value2"},
		},
		RequiredActions: []string{"UPDATE_PASSWORD"},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("Failed to marshal UserRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled UserRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal UserRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != user.ID {
		t.Errorf("ID mismatch")
	}
	if unmarshaled.Username != user.Username {
		t.Errorf("Username mismatch")
	}
	if unmarshaled.Email != user.Email {
		t.Errorf("Email mismatch")
	}
	if len(unmarshaled.Attributes["custom"]) != 2 {
		t.Errorf("Attributes mismatch")
	}
}

func TestClientRepresentation_JSONMarshaling(t *testing.T) {
	client := &ClientRepresentation{
		ID:                      "client-uuid",
		ClientID:                "test-client",
		Name:                    "Test Client",
		Description:             "A test client",
		Enabled:                 true,
		ClientAuthenticatorType: "client-secret",
		RedirectUris:            []string{"http://localhost:8080/*"},
		WebOrigins:              []string{"http://localhost:8080"},
		StandardFlowEnabled:     true,
		PublicClient:            false,
		Protocol:                "openid-connect",
		Attributes: map[string]string{
			"custom-attr": "value",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("Failed to marshal ClientRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled ClientRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ClientRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.ID != client.ID {
		t.Errorf("ID mismatch")
	}
	if unmarshaled.ClientID != client.ClientID {
		t.Errorf("ClientID mismatch")
	}
	if unmarshaled.Protocol != client.Protocol {
		t.Errorf("Protocol mismatch")
	}
	if len(unmarshaled.RedirectUris) != 1 {
		t.Errorf("RedirectUris length mismatch")
	}
}

func TestEventRepresentation_JSONMarshaling(t *testing.T) {
	event := &EventRepresentation{
		Time:      1609459200000,
		Type:      "LOGIN",
		RealmID:   "realm-id",
		ClientID:  "client-id",
		UserID:    "user-id",
		SessionID: "session-id",
		IPAddress: "192.168.1.1",
		Error:     "",
		Details: map[string]string{
			"username":    "testuser",
			"auth_method": "password",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal EventRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled EventRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal EventRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.Time != event.Time {
		t.Errorf("Time mismatch")
	}
	if unmarshaled.Type != event.Type {
		t.Errorf("Type mismatch")
	}
	if unmarshaled.IPAddress != event.IPAddress {
		t.Errorf("IPAddress mismatch")
	}
	if len(unmarshaled.Details) != 2 {
		t.Errorf("Details length mismatch")
	}
}

func TestAdminEventRepresentation_JSONMarshaling(t *testing.T) {
	adminEvent := &AdminEventRepresentation{
		Time:    1609459200000,
		RealmID: "realm-id",
		AuthDetails: &AuthDetails{
			RealmID:   "realm-id",
			ClientID:  "admin-cli",
			UserID:    "admin-user-id",
			IPAddress: "10.0.0.1",
		},
		ResourceType:   "USER",
		OperationType:  "CREATE",
		ResourcePath:   "users/user-id",
		Representation: `{"username":"newuser"}`,
		Error:          "",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(adminEvent)
	if err != nil {
		t.Fatalf("Failed to marshal AdminEventRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled AdminEventRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal AdminEventRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.ResourceType != adminEvent.ResourceType {
		t.Errorf("ResourceType mismatch")
	}
	if unmarshaled.OperationType != adminEvent.OperationType {
		t.Errorf("OperationType mismatch")
	}
	if unmarshaled.AuthDetails == nil {
		t.Error("AuthDetails should not be nil")
	} else {
		if unmarshaled.AuthDetails.UserID != adminEvent.AuthDetails.UserID {
			t.Errorf("AuthDetails.UserID mismatch")
		}
	}
}

func TestServerInfoRepresentation_JSONMarshaling(t *testing.T) {
	serverInfo := &ServerInfoRepresentation{
		SystemInfo: &SystemInfo{
			Version:      "21.0.0",
			UptimeMillis: 86400000,
			JavaVersion:  "17.0.1",
			OSName:       "Linux",
			OSVersion:    "5.10.0",
		},
		MemoryInfo: &MemoryInfo{
			Total:          1073741824,
			Used:           536870912,
			Free:           536870912,
			FreePercentage: 50,
		},
		ProfileInfo: &ProfileInfo{
			Name:        "default",
			Description: "Default profile",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(serverInfo)
	if err != nil {
		t.Fatalf("Failed to marshal ServerInfoRepresentation: %v", err)
	}

	// Unmarshal back
	var unmarshaled ServerInfoRepresentation
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ServerInfoRepresentation: %v", err)
	}

	// Verify fields
	if unmarshaled.SystemInfo == nil {
		t.Error("SystemInfo should not be nil")
	} else {
		if unmarshaled.SystemInfo.Version != serverInfo.SystemInfo.Version {
			t.Errorf("SystemInfo.Version mismatch")
		}
	}
	if unmarshaled.MemoryInfo == nil {
		t.Error("MemoryInfo should not be nil")
	} else {
		if unmarshaled.MemoryInfo.Total != serverInfo.MemoryInfo.Total {
			t.Errorf("MemoryInfo.Total mismatch")
		}
	}
}

func TestErrorResponse_JSONMarshaling(t *testing.T) {
	errResp := &ErrorResponse{
		Error:            "invalid_grant",
		ErrorDescription: "Invalid username or password",
		ErrorMessage:     "Authentication failed",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("Failed to marshal ErrorResponse: %v", err)
	}

	// Unmarshal back
	var unmarshaled ErrorResponse
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ErrorResponse: %v", err)
	}

	// Verify fields
	if unmarshaled.Error != errResp.Error {
		t.Errorf("Error mismatch")
	}
	if unmarshaled.ErrorDescription != errResp.ErrorDescription {
		t.Errorf("ErrorDescription mismatch")
	}
	if unmarshaled.ErrorMessage != errResp.ErrorMessage {
		t.Errorf("ErrorMessage mismatch")
	}
}

func TestEmptyModels(t *testing.T) {
	tests := []struct {
		name  string
		model interface{}
	}{
		{"TokenResponse", &TokenResponse{}},
		{"RealmRepresentation", &RealmRepresentation{}},
		{"UserRepresentation", &UserRepresentation{}},
		{"ClientRepresentation", &ClientRepresentation{}},
		{"EventRepresentation", &EventRepresentation{}},
		{"AdminEventRepresentation", &AdminEventRepresentation{}},
		{"ServerInfoRepresentation", &ServerInfoRepresentation{}},
		{"ErrorResponse", &ErrorResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that empty models can be marshaled without error
			jsonData, err := json.Marshal(tt.model)
			if err != nil {
				t.Errorf("Failed to marshal empty %s: %v", tt.name, err)
			}

			// Test that marshaled data can be unmarshaled without error
			if err := json.Unmarshal(jsonData, tt.model); err != nil {
				t.Errorf("Failed to unmarshal empty %s: %v", tt.name, err)
			}
		})
	}
}
