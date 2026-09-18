package http

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/go-playground/validator/v10"
)

// keycloakRealmPattern matches characters that are safe to interpolate into a
// Keycloak admin URL path. Anything outside this set (slash, dot, query
// markers, encoded sequences) opens the door to path traversal in URLs of the
// form .../realms/{realm}/protocol/...
var keycloakRealmPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validate is a singleton validator instance.
var validate = func() *validator.Validate {
	v := validator.New()
	// keycloak_realm rejects realm names containing characters that could
	// be used for path traversal when interpolated into a Keycloak URL.
	_ = v.RegisterValidation("keycloak_realm", func(fl validator.FieldLevel) bool {
		s := fl.Field().String()
		if s == "" {
			return true // 'omitempty' handles the empty case
		}
		return keycloakRealmPattern.MatchString(s)
	})
	return v
}()

// ValidateAndParse parses JSON body and validates using struct tags.
// Returns the parsed struct and true if successful, nil and false otherwise.
func ValidateAndParse[T any](w http.ResponseWriter, r *http.Request, log *logger.Logger) (*T, bool) {
	var req T

	// Parse JSON body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("Invalid JSON request body", logger.Err(err))
		RespondValidationError(w, log, "Invalid JSON request body", nil)
		return nil, false
	}

	// Validate struct tags
	if err := validate.Struct(&req); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			details := formatValidationErrors(validationErrors)
			RespondValidationError(w, log, "Validation failed", details)
			return nil, false
		}
		RespondValidationError(w, log, err.Error(), nil)
		return nil, false
	}

	return &req, true
}

// RespondValidationError writes a validation error response.
func RespondValidationError(w http.ResponseWriter, log *logger.Logger, message string, details map[string]string) {
	response := map[string]interface{}{
		"error": message,
	}
	if details != nil {
		response["details"] = details
	}
	RespondJSON(w, log, http.StatusBadRequest, response)
}

// formatValidationErrors converts validator errors to a readable map.
func formatValidationErrors(errors validator.ValidationErrors) map[string]string {
	result := make(map[string]string)
	for _, e := range errors {
		result[e.Field()] = formatFieldError(e)
	}
	return result
}

// formatFieldError formats a single field validation error.
func formatFieldError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "This field is required"
	case "email":
		return "Invalid email format"
	case "min":
		return "Minimum length is " + e.Param()
	case "max":
		return "Maximum length is " + e.Param()
	case "url":
		return "Invalid URL format"
	case "oneof":
		return "Must be one of: " + e.Param()
	case "uuid":
		return "Invalid UUID format"
	case "alphanum":
		return "Must contain only alphanumeric characters"
	case "keycloak_realm":
		return "Must contain only letters, digits, underscore, or hyphen"
	case "required_if":
		return "This field is required"
	case "gte":
		return "Must be greater than or equal to " + e.Param()
	case "lte":
		return "Must be less than or equal to " + e.Param()
	default:
		return "Invalid value"
	}
}

// GetIntQuery extracts an integer query parameter with a default value.
// If the parameter is missing or invalid, returns the default value.
func GetIntQuery(r *http.Request, key string, defaultValue int) int {
	valueStr := r.URL.Query().Get(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// GetIntQueryWithBounds extracts an integer query parameter with bounds checking.
func GetIntQueryWithBounds(r *http.Request, key string, defaultValue, min, max int) int {
	value := GetIntQuery(r, key, defaultValue)

	if value < min {
		return min
	}
	if max > 0 && value > max {
		return max
	}

	return value
}

// GetStringQuery extracts a string query parameter with a default value.
func GetStringQuery(r *http.Request, key string, defaultValue string) string {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetBoolQuery extracts a boolean query parameter.
func GetBoolQuery(r *http.Request, key string, defaultValue bool) bool {
	valueStr := r.URL.Query().Get(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// RequireMethod checks if the request method matches the expected method.
// If not, it writes a 405 Method Not Allowed response and returns false.
func RequireMethod(w http.ResponseWriter, r *http.Request, method string, log interface{}) bool {
	if r.Method != method {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// ListOptions defines filtering and pagination options for queries.
type ListOptions struct {
	Limit  int
	Offset int
}
