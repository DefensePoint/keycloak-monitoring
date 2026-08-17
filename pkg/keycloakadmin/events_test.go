package keycloakadmin

import (
	"testing"
	"time"
)

func TestFormatKeycloakTime(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "Standard date",
			input:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			expected: "2024-01-15",
		},
		{
			name:     "Single digit day and month",
			input:    time.Date(2024, 3, 5, 14, 22, 0, 0, time.UTC),
			expected: "2024-03-05",
		},
		{
			name:     "End of year",
			input:    time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC),
			expected: "2023-12-31",
		},
		{
			name:     "Start of year",
			input:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "2024-01-01",
		},
		{
			name:     "Leap year date",
			input:    time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			expected: "2024-02-29",
		},
		{
			name:     "Zero time",
			input:    time.Time{},
			expected: "0001-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatKeycloakTime(tt.input)
			if result != tt.expected {
				t.Errorf("formatKeycloakTime() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatKeycloakTime_Consistency(t *testing.T) {
	// Test that the same time always produces the same output
	now := time.Now()
	result1 := formatKeycloakTime(now)
	result2 := formatKeycloakTime(now)

	if result1 != result2 {
		t.Errorf("formatKeycloakTime is not consistent: %s != %s", result1, result2)
	}
}

func TestFormatKeycloakTime_Format(t *testing.T) {
	// Test that the format is exactly "YYYY-MM-DD"
	testTime := time.Date(2024, 5, 15, 10, 30, 0, 0, time.UTC)
	result := formatKeycloakTime(testTime)

	// Check length
	if len(result) != 10 {
		t.Errorf("Expected length 10, got %d", len(result))
	}

	// Check format (YYYY-MM-DD)
	if result[4] != '-' || result[7] != '-' {
		t.Errorf("Expected format YYYY-MM-DD, got %s", result)
	}
}

func TestEventQueryOptions_EmptyFields(t *testing.T) {
	options := &EventQueryOptions{}

	// Test that empty options don't cause issues
	if len(options.Types) != 0 {
		t.Error("Expected empty Types slice")
	}
	if options.Client != "" {
		t.Error("Expected empty Client string")
	}
	if !options.DateFrom.IsZero() {
		t.Error("Expected zero DateFrom")
	}
	if !options.DateTo.IsZero() {
		t.Error("Expected zero DateTo")
	}
	if options.First != 0 {
		t.Error("Expected zero First")
	}
	if options.Max != 0 {
		t.Error("Expected zero Max")
	}
}

func TestEventQueryOptions_PopulatedFields(t *testing.T) {
	now := time.Now()
	options := &EventQueryOptions{
		Types:     []string{"LOGIN", "LOGOUT"},
		Client:    "test-client",
		User:      "test-user",
		IPAddress: "192.168.1.1",
		DateFrom:  now.Add(-24 * time.Hour),
		DateTo:    now,
		First:     0,
		Max:       100,
	}

	if len(options.Types) != 2 {
		t.Errorf("Expected 2 types, got %d", len(options.Types))
	}
	if options.Types[0] != "LOGIN" {
		t.Errorf("Expected first type to be LOGIN, got %s", options.Types[0])
	}
	if options.Client != "test-client" {
		t.Errorf("Expected Client to be test-client, got %s", options.Client)
	}
	if options.Max != 100 {
		t.Errorf("Expected Max to be 100, got %d", options.Max)
	}
}

func TestAdminEventQueryOptions_EmptyFields(t *testing.T) {
	options := &AdminEventQueryOptions{}

	// Test that empty options don't cause issues
	if len(options.OperationTypes) != 0 {
		t.Error("Expected empty OperationTypes slice")
	}
	if options.ResourcePath != "" {
		t.Error("Expected empty ResourcePath string")
	}
	if !options.DateFrom.IsZero() {
		t.Error("Expected zero DateFrom")
	}
	if !options.DateTo.IsZero() {
		t.Error("Expected zero DateTo")
	}
}

func TestAdminEventQueryOptions_PopulatedFields(t *testing.T) {
	now := time.Now()
	options := &AdminEventQueryOptions{
		OperationTypes: []string{"CREATE", "UPDATE", "DELETE"},
		ResourcePath:   "/users/user-id",
		DateFrom:       now.Add(-24 * time.Hour),
		DateTo:         now,
		First:          0,
		Max:            50,
	}

	if len(options.OperationTypes) != 3 {
		t.Errorf("Expected 3 operation types, got %d", len(options.OperationTypes))
	}
	if options.OperationTypes[0] != "CREATE" {
		t.Errorf("Expected first operation type to be CREATE, got %s", options.OperationTypes[0])
	}
	if options.ResourcePath != "/users/user-id" {
		t.Errorf("Expected ResourcePath to be /users/user-id, got %s", options.ResourcePath)
	}
	if options.Max != 50 {
		t.Errorf("Expected Max to be 50, got %d", options.Max)
	}
}
