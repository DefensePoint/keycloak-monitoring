package auth

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestTimeFromUnix(t *testing.T) {
	tests := []struct {
		name      string
		timestamp int64
		expected  time.Time
	}{
		{
			name:      "Zero timestamp",
			timestamp: 0,
			expected:  time.Unix(0, 0),
		},
		{
			name:      "Positive timestamp",
			timestamp: 1609459200, // 2021-01-01 00:00:00 UTC
			expected:  time.Unix(1609459200, 0),
		},
		{
			name:      "Negative timestamp",
			timestamp: -1000,
			expected:  time.Unix(-1000, 0),
		},
		{
			name:      "Current time",
			timestamp: time.Now().Unix(),
			expected:  time.Unix(time.Now().Unix(), 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := timeFromUnix(tt.timestamp)
			if !result.Equal(tt.expected) {
				t.Errorf("timeFromUnix(%d) = %v, expected %v", tt.timestamp, result, tt.expected)
			}
		})
	}
}

func TestGenerateRandomState(t *testing.T) {
	t.Run("Generates non-empty state", func(t *testing.T) {
		state, err := GenerateRandomState()
		if err != nil {
			t.Fatalf("GenerateRandomState() failed: %v", err)
		}
		if state == "" {
			t.Error("GenerateRandomState() returned empty string")
		}
	})

	t.Run("Generates valid base64 URL encoded string", func(t *testing.T) {
		state, err := GenerateRandomState()
		if err != nil {
			t.Fatalf("GenerateRandomState() failed: %v", err)
		}

		// Should be valid base64 URL encoding
		_, err = base64.URLEncoding.DecodeString(state)
		if err != nil {
			t.Errorf("Generated state is not valid base64 URL encoding: %v", err)
		}
	})

	t.Run("Generates unique states", func(t *testing.T) {
		states := make(map[string]bool)
		iterations := 100

		for i := 0; i < iterations; i++ {
			state, err := GenerateRandomState()
			if err != nil {
				t.Fatalf("GenerateRandomState() failed on iteration %d: %v", i, err)
			}

			if states[state] {
				t.Errorf("GenerateRandomState() generated duplicate state: %s", state)
			}
			states[state] = true
		}

		if len(states) != iterations {
			t.Errorf("Expected %d unique states, got %d", iterations, len(states))
		}
	})

	t.Run("Generates state of expected length", func(t *testing.T) {
		state, err := GenerateRandomState()
		if err != nil {
			t.Fatalf("GenerateRandomState() failed: %v", err)
		}

		// 32 bytes encoded in base64 should produce a string longer than 40 chars
		if len(state) < 40 {
			t.Errorf("Generated state is too short: %d characters", len(state))
		}
	})

	t.Run("State contains no whitespace", func(t *testing.T) {
		state, err := GenerateRandomState()
		if err != nil {
			t.Fatalf("GenerateRandomState() failed: %v", err)
		}

		if strings.ContainsAny(state, " \t\n\r") {
			t.Error("Generated state contains whitespace")
		}
	})
}

func TestGenerateSessionID(t *testing.T) {
	t.Run("Generates non-empty session ID", func(t *testing.T) {
		sessionID, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() failed: %v", err)
		}
		if sessionID == "" {
			t.Error("GenerateSessionID() returned empty string")
		}
	})

	t.Run("Generates valid base64 URL encoded string", func(t *testing.T) {
		sessionID, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() failed: %v", err)
		}

		// Should be valid base64 URL encoding
		_, err = base64.URLEncoding.DecodeString(sessionID)
		if err != nil {
			t.Errorf("Generated session ID is not valid base64 URL encoding: %v", err)
		}
	})

	t.Run("Generates unique session IDs", func(t *testing.T) {
		sessionIDs := make(map[string]bool)
		iterations := 100

		for i := 0; i < iterations; i++ {
			sessionID, err := GenerateSessionID()
			if err != nil {
				t.Fatalf("GenerateSessionID() failed on iteration %d: %v", i, err)
			}

			if sessionIDs[sessionID] {
				t.Errorf("GenerateSessionID() generated duplicate ID: %s", sessionID)
			}
			sessionIDs[sessionID] = true
		}

		if len(sessionIDs) != iterations {
			t.Errorf("Expected %d unique session IDs, got %d", iterations, len(sessionIDs))
		}
	})

	t.Run("Generates session ID of expected length", func(t *testing.T) {
		sessionID, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() failed: %v", err)
		}

		// 32 bytes encoded in base64 should produce a string longer than 40 chars
		if len(sessionID) < 40 {
			t.Errorf("Generated session ID is too short: %d characters", len(sessionID))
		}
	})

	t.Run("Session ID contains no whitespace", func(t *testing.T) {
		sessionID, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() failed: %v", err)
		}

		if strings.ContainsAny(sessionID, " \t\n\r") {
			t.Error("Generated session ID contains whitespace")
		}
	})

	t.Run("Session ID and State use same algorithm", func(t *testing.T) {
		sessionID, err := GenerateSessionID()
		if err != nil {
			t.Fatalf("GenerateSessionID() failed: %v", err)
		}

		state, err := GenerateRandomState()
		if err != nil {
			t.Fatalf("GenerateRandomState() failed: %v", err)
		}

		// Both should have similar length (same byte size)
		if len(sessionID) != len(state) {
			t.Errorf("Session ID and state have different lengths: %d vs %d", len(sessionID), len(state))
		}
	})
}

func TestGenerateRandomState_Cryptographic(t *testing.T) {
	t.Run("Uses crypto/rand for randomness", func(t *testing.T) {
		// Generate multiple states and check they have high entropy
		states := make([]string, 10)
		for i := 0; i < 10; i++ {
			state, err := GenerateRandomState()
			if err != nil {
				t.Fatalf("Failed to generate state: %v", err)
			}
			states[i] = state
		}

		// Check that no two states are similar (this is probabilistic)
		for i := 0; i < len(states); i++ {
			for j := i + 1; j < len(states); j++ {
				if states[i] == states[j] {
					t.Error("Found identical states, crypto/rand may not be working")
				}
				// Check that states don't share common prefixes (would indicate low entropy)
				if len(states[i]) > 10 && states[i][:10] == states[j][:10] {
					t.Error("States share common prefix, may indicate low entropy")
				}
			}
		}
	})
}

func TestGenerateSessionID_Cryptographic(t *testing.T) {
	t.Run("Uses crypto/rand for randomness", func(t *testing.T) {
		// Generate multiple session IDs and check they have high entropy
		sessionIDs := make([]string, 10)
		for i := 0; i < 10; i++ {
			sessionID, err := GenerateSessionID()
			if err != nil {
				t.Fatalf("Failed to generate session ID: %v", err)
			}
			sessionIDs[i] = sessionID
		}

		// Check that no two session IDs are similar (this is probabilistic)
		for i := 0; i < len(sessionIDs); i++ {
			for j := i + 1; j < len(sessionIDs); j++ {
				if sessionIDs[i] == sessionIDs[j] {
					t.Error("Found identical session IDs, crypto/rand may not be working")
				}
				// Check that IDs don't share common prefixes (would indicate low entropy)
				if len(sessionIDs[i]) > 10 && sessionIDs[i][:10] == sessionIDs[j][:10] {
					t.Error("Session IDs share common prefix, may indicate low entropy")
				}
			}
		}
	})
}
