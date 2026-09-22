package auth

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordHasher handles password hashing and verification using bcrypt.
// It implements the PasswordHasher interface.
type BcryptPasswordHasher struct {
	cost int
}

// NewBcryptPasswordHasher creates a new password hasher with the specified cost.
// The cost determines how computationally expensive the hashing is.
// Default bcrypt cost is 10, higher values are more secure but slower.
func NewBcryptPasswordHasher(cost int) *BcryptPasswordHasher {
	if cost < bcrypt.MinCost {
		cost = bcrypt.DefaultCost
	}
	return &BcryptPasswordHasher{cost: cost}
}

// HashPassword hashes a plaintext password using bcrypt.
func (h *BcryptPasswordHasher) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", &AuthError{
			Code:    "HASH_FAILED",
			Message: "failed to hash password",
			Err:     err,
		}
	}
	return string(hashedBytes), nil
}

// VerifyPassword checks if a plaintext password matches a hashed password.
func (h *BcryptPasswordHasher) VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// Hash implements PasswordHasher interface.
func (h *BcryptPasswordHasher) Hash(password string) (string, error) {
	return h.HashPassword(password)
}

// Compare implements PasswordHasher interface.
func (h *BcryptPasswordHasher) Compare(password, hash string) error {
	if !h.VerifyPassword(hash, password) {
		return &AuthError{
			Code:    "INVALID_PASSWORD",
			Message: "password does not match",
		}
	}
	return nil
}

// PasswordRequirements defines the requirements for a valid password.
type PasswordRequirements struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireNumber  bool
	RequireSpecial bool
}

// DefaultPasswordRequirements returns the default password requirements.
func DefaultPasswordRequirements() PasswordRequirements {
	return PasswordRequirements{
		MinLength:      12,
		RequireUpper:   true,
		RequireLower:   true,
		RequireNumber:  true,
		RequireSpecial: true,
	}
}

// PasswordValidationError represents a password validation error.
type PasswordValidationError struct {
	Violations []string
}

func (e *PasswordValidationError) Error() string {
	return fmt.Sprintf("password validation failed: %s", strings.Join(e.Violations, "; "))
}

// ValidatePassword validates a password against the given requirements.
func ValidatePassword(password string, requirements PasswordRequirements) error {
	violations := []string{}

	// Check minimum length
	if len(password) < requirements.MinLength {
		violations = append(violations, fmt.Sprintf("must be at least %d characters long", requirements.MinLength))
	}

	// Check for uppercase letter
	if requirements.RequireUpper && !hasUppercase(password) {
		violations = append(violations, "must contain at least one uppercase letter")
	}

	// Check for lowercase letter
	if requirements.RequireLower && !hasLowercase(password) {
		violations = append(violations, "must contain at least one lowercase letter")
	}

	// Check for number
	if requirements.RequireNumber && !hasNumber(password) {
		violations = append(violations, "must contain at least one number")
	}

	// Check for special character
	if requirements.RequireSpecial && !hasSpecialChar(password) {
		violations = append(violations, "must contain at least one special character")
	}

	// Check for common weak passwords
	if isCommonWeakPassword(password) {
		violations = append(violations, "password is too common or weak")
	}

	if len(violations) > 0 {
		return &PasswordValidationError{Violations: violations}
	}

	return nil
}

func hasUppercase(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func hasLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func hasNumber(s string) bool {
	for _, r := range s {
		if unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func hasSpecialChar(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// isCommonWeakPassword checks if the password is in a list of common weak passwords.
func isCommonWeakPassword(password string) bool {
	lower := strings.ToLower(password)

	commonPasswords := []string{
		"password", "password123", "password1234",
		"admin", "admin123", "admin1234",
		"welcome", "welcome123",
		"letmein", "letmein123",
		"qwerty", "qwerty123",
		"123456", "12345678", "1234567890",
		"secret", "secret123",
		"changeme", "changeme123",
		"default", "default123",
	}

	for _, weak := range commonPasswords {
		if lower == weak || strings.Contains(lower, weak) {
			return true
		}
	}

	return false
}

// GetPasswordRequirementsDescription returns a human-readable description of password requirements.
func GetPasswordRequirementsDescription(requirements PasswordRequirements) string {
	parts := []string{}

	parts = append(parts, fmt.Sprintf("at least %d characters", requirements.MinLength))

	if requirements.RequireUpper {
		parts = append(parts, "one uppercase letter")
	}
	if requirements.RequireLower {
		parts = append(parts, "one lowercase letter")
	}
	if requirements.RequireNumber {
		parts = append(parts, "one number")
	}
	if requirements.RequireSpecial {
		parts = append(parts, "one special character")
	}

	return "Password must contain " + strings.Join(parts, ", ")
}
