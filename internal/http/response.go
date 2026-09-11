package http

import (
	"encoding/json"
	"net/http"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

// RespondJSON writes a JSON response with the given status code and data.
func RespondJSON(w http.ResponseWriter, log *logger.Logger, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Error("Failed to encode JSON response", logger.Err(err))
	}
}

// RespondSuccess writes a successful JSON response (200 OK).
func RespondSuccess(w http.ResponseWriter, log *logger.Logger, data interface{}) {
	RespondJSON(w, log, http.StatusOK, data)
}

// RespondCreated writes a created response (201 Created).
func RespondCreated(w http.ResponseWriter, log *logger.Logger, data interface{}) {
	RespondJSON(w, log, http.StatusCreated, data)
}

// RespondError writes an error response with appropriate status code.
func RespondError(w http.ResponseWriter, log *logger.Logger, statusCode int, message string) {
	RespondJSON(w, log, statusCode, map[string]interface{}{
		"error": message,
	})
}

// RespondBadRequest writes a 400 Bad Request error.
func RespondBadRequest(w http.ResponseWriter, log *logger.Logger, message string) {
	RespondError(w, log, http.StatusBadRequest, message)
}

// RespondUnauthorized writes a 401 Unauthorized error.
func RespondUnauthorized(w http.ResponseWriter, log *logger.Logger, message string) {
	RespondError(w, log, http.StatusUnauthorized, message)
}

// RespondForbidden writes a 403 Forbidden error.
func RespondForbidden(w http.ResponseWriter, log *logger.Logger, message string) {
	RespondError(w, log, http.StatusForbidden, message)
}

// RespondNotFound writes a 404 Not Found error.
func RespondNotFound(w http.ResponseWriter, log *logger.Logger, message string) {
	RespondError(w, log, http.StatusNotFound, message)
}

// RespondInternalError writes a 500 Internal Server Error.
func RespondInternalError(w http.ResponseWriter, log *logger.Logger, message string) {
	RespondError(w, log, http.StatusInternalServerError, message)
}

// RespondMethodNotAllowed writes a 405 Method Not Allowed error.
func RespondMethodNotAllowed(w http.ResponseWriter, log *logger.Logger) {
	RespondError(w, log, http.StatusMethodNotAllowed, "Method not allowed")
}

// ParseJSONBody parses a JSON request body into the target struct.
// Returns true if parsing succeeded, false otherwise (and writes error response).
func ParseJSONBody(w http.ResponseWriter, r *http.Request, log *logger.Logger, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		log.Warn("Invalid JSON request body", logger.Err(err))
		RespondBadRequest(w, log, "Invalid JSON request body")
		return false
	}
	return true
}
