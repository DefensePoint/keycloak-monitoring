package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestNewBcryptPasswordHasher(t *testing.T) {
	tests := []struct {
		name         string
		cost         int
		expectedCost int
	}{
		{
			name:         "Valid cost",
			cost:         12,
			expectedCost: 12,
		},
		{
			name:         "Cost below minimum defaults to DefaultCost",
			cost:         2,
			expectedCost: bcrypt.DefaultCost,
		},
		{
			name:         "Zero cost defaults to DefaultCost",
			cost:         0,
			expectedCost: bcrypt.DefaultCost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasher := NewBcryptPasswordHasher(tt.cost)
			if hasher == nil {
				t.Fatal("NewBcryptPasswordHasher returned nil")
				return
			}
			if hasher.cost != tt.expectedCost {
				t.Errorf("expected cost %d, got %d", tt.expectedCost, hasher.cost)
			}
		})
	}
}

func TestBcryptPasswordHasher_HashPassword(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost) // Use MinCost for faster tests

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Hash simple password",
			password: "password123",
			wantErr:  false,
		},
		{
			name:     "Hash complex password",
			password: "P@ssw0rd!#$%^&*()",
			wantErr:  false,
		},
		{
			name:     "Hash empty password",
			password: "",
			wantErr:  false,
		},
		{
			name:     "Hash long password within bcrypt limit",
			password: "this-is-a-long-password-but-within-the-72-byte-bcrypt-limit",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashed, err := hasher.HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(hashed) == 0 {
					t.Error("HashPassword() returned empty hash")
				}
				// Verify the hash can be used to verify the password
				if !hasher.VerifyPassword(hashed, tt.password) {
					t.Error("Generated hash does not verify the original password")
				}
			}
		})
	}
}

func TestBcryptPasswordHasher_HashPassword_ProducesUniqueHashes(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost) // Use MinCost for faster tests
	password := "samepassword"

	hash1, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("First hash failed: %v", err)
	}

	hash2, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("Second hash failed: %v", err)
	}

	// Bcrypt should produce different hashes for the same password due to salt
	if hash1 == hash2 {
		t.Error("Expected different hashes for same password, got identical hashes")
	}

	// But both should verify the password
	if !hasher.VerifyPassword(hash1, password) {
		t.Error("First hash does not verify password")
	}
	if !hasher.VerifyPassword(hash2, password) {
		t.Error("Second hash does not verify password")
	}
}

func TestBcryptPasswordHasher_VerifyPassword(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost) // Use MinCost for faster tests
	password := "correctpassword"
	hashed, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	tests := []struct {
		name           string
		hashedPassword string
		password       string
		expected       bool
	}{
		{
			name:           "Correct password",
			hashedPassword: hashed,
			password:       password,
			expected:       true,
		},
		{
			name:           "Incorrect password",
			hashedPassword: hashed,
			password:       "wrongpassword",
			expected:       false,
		},
		{
			name:           "Empty password against hash",
			hashedPassword: hashed,
			password:       "",
			expected:       false,
		},
		{
			name:           "Invalid hash format",
			hashedPassword: "not-a-valid-hash",
			password:       password,
			expected:       false,
		},
		{
			name:           "Empty hash",
			hashedPassword: "",
			password:       password,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasher.VerifyPassword(tt.hashedPassword, tt.password)
			if result != tt.expected {
				t.Errorf("VerifyPassword() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestBcryptPasswordHasher_VerifyPassword_CaseSensitive(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost) // Use MinCost for faster tests
	password := "Password123"
	hashed, err := hasher.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Verify that password verification is case-sensitive
	if hasher.VerifyPassword(hashed, "password123") {
		t.Error("VerifyPassword should be case-sensitive")
	}
	if hasher.VerifyPassword(hashed, "PASSWORD123") {
		t.Error("VerifyPassword should be case-sensitive")
	}
	if !hasher.VerifyPassword(hashed, password) {
		t.Error("VerifyPassword failed for correct password")
	}
}

func TestBcryptPasswordHasher_DifferentCosts(t *testing.T) {
	// Test that different costs produce valid but different hashes
	// Use lower costs (4 and 5) for faster tests
	password := "testpassword"

	hasher1 := NewBcryptPasswordHasher(4)
	hash1, err := hasher1.HashPassword(password)
	if err != nil {
		t.Fatalf("Hasher with cost 4 failed: %v", err)
	}

	hasher2 := NewBcryptPasswordHasher(5)
	hash2, err := hasher2.HashPassword(password)
	if err != nil {
		t.Fatalf("Hasher with cost 5 failed: %v", err)
	}

	// Hashes should be different
	if hash1 == hash2 {
		t.Error("Expected different hashes for different costs")
	}

	// Both hashers should verify both hashes (bcrypt is backward compatible)
	if !hasher1.VerifyPassword(hash1, password) {
		t.Error("Hasher1 failed to verify its own hash")
	}
	if !hasher2.VerifyPassword(hash2, password) {
		t.Error("Hasher2 failed to verify its own hash")
	}
	if !hasher1.VerifyPassword(hash2, password) {
		t.Error("Hasher1 failed to verify hash from different cost")
	}
	if !hasher2.VerifyPassword(hash1, password) {
		t.Error("Hasher2 failed to verify hash from different cost")
	}
}

func TestBcryptPasswordHasher_Hash_Interface(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost)

	password := "testpassword"
	hash, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	if len(hash) == 0 {
		t.Error("Hash() returned empty string")
	}

	// Verify using interface method
	err = hasher.Compare(password, hash)
	if err != nil {
		t.Errorf("Compare() failed for correct password: %v", err)
	}
}

func TestBcryptPasswordHasher_Compare_Interface(t *testing.T) {
	hasher := NewBcryptPasswordHasher(bcrypt.MinCost)

	password := "testpassword"
	hash, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}

	// Test correct password
	err = hasher.Compare(password, hash)
	if err != nil {
		t.Errorf("Compare() failed for correct password: %v", err)
	}

	// Test incorrect password
	err = hasher.Compare("wrongpassword", hash)
	if err == nil {
		t.Error("Compare() should fail for incorrect password")
	}

	authErr, ok := err.(*AuthError)
	if !ok {
		t.Error("Error should be of type *Error")
	} else if authErr.Code != "INVALID_PASSWORD" {
		t.Errorf("Expected error code INVALID_PASSWORD, got %s", authErr.Code)
	}
}

func TestValidatePassword(t *testing.T) {
	requirements := DefaultPasswordRequirements()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "Valid password",
			password: "SecureP@ss123!",
			wantErr:  false,
		},
		{
			name:     "Too short",
			password: "Sh0rt!",
			wantErr:  true,
		},
		{
			name:     "No uppercase",
			password: "nouppercase123!",
			wantErr:  true,
		},
		{
			name:     "No lowercase",
			password: "NOLOWERCASE123!",
			wantErr:  true,
		},
		{
			name:     "No number",
			password: "NoNumberHere!!",
			wantErr:  true,
		},
		{
			name:     "No special character",
			password: "NoSpecialChar123",
			wantErr:  true,
		},
		{
			name:     "Common weak password",
			password: "Password123!@#",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password, requirements)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultPasswordRequirements(t *testing.T) {
	requirements := DefaultPasswordRequirements()

	if requirements.MinLength != 12 {
		t.Errorf("Expected MinLength 12, got %d", requirements.MinLength)
	}
	if !requirements.RequireUpper {
		t.Error("Expected RequireUpper to be true")
	}
	if !requirements.RequireLower {
		t.Error("Expected RequireLower to be true")
	}
	if !requirements.RequireNumber {
		t.Error("Expected RequireNumber to be true")
	}
	if !requirements.RequireSpecial {
		t.Error("Expected RequireSpecial to be true")
	}
}

func TestPasswordValidationError(t *testing.T) {
	err := &PasswordValidationError{
		Violations: []string{"too short", "no uppercase"},
	}

	expected := "password validation failed: too short; no uppercase"
	if err.Error() != expected {
		t.Errorf("Error() = %s, expected %s", err.Error(), expected)
	}
}

func TestGetPasswordRequirementsDescription(t *testing.T) {
	requirements := DefaultPasswordRequirements()

	description := GetPasswordRequirementsDescription(requirements)

	if description == "" {
		t.Error("GetPasswordRequirementsDescription() returned empty string")
	}

	// Check that description contains expected parts
	expectedParts := []string{
		"at least 12 characters",
		"one uppercase letter",
		"one lowercase letter",
		"one number",
		"one special character",
	}

	for _, part := range expectedParts {
		if !contains(description, part) {
			t.Errorf("Description missing: %s", part)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
