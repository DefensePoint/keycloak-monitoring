package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
)

func TestRespondJSON(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	data := map[string]string{"message": "test"}
	RespondJSON(w, log, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["message"] != "test" {
		t.Errorf("Expected message 'test', got '%s'", response["message"])
	}
}

func TestRespondSuccess(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	data := map[string]interface{}{
		"success": true,
		"data":    "test data",
	}

	RespondSuccess(w, log, data)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if success, ok := response["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}
}

func TestRespondCreated(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	data := map[string]string{"id": "123"}
	RespondCreated(w, log, data)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["id"] != "123" {
		t.Errorf("Expected id '123', got '%s'", response["id"])
	}
}

func TestRespondError(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondError(w, log, http.StatusBadRequest, "Invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if message, ok := response["error"].(string); !ok || message != "Invalid input" {
		t.Errorf("Expected error 'Invalid input', got '%s'", message)
	}
}

func TestRespondBadRequest(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondBadRequest(w, log, "Bad request message")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Bad request message" {
		t.Errorf("Expected error 'Bad request message', got '%v'", response["error"])
	}
}

func TestRespondUnauthorized(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondUnauthorized(w, log, "Authentication required")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Authentication required" {
		t.Errorf("Expected error 'Authentication required', got '%v'", response["error"])
	}
}

func TestRespondForbidden(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondForbidden(w, log, "Access denied")

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Access denied" {
		t.Errorf("Expected error 'Access denied', got '%v'", response["error"])
	}
}

func TestRespondNotFound(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondNotFound(w, log, "Resource not found")

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Resource not found" {
		t.Errorf("Expected error 'Resource not found', got '%v'", response["error"])
	}
}

func TestRespondInternalError(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondInternalError(w, log, "Internal server error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Internal server error" {
		t.Errorf("Expected error 'Internal server error', got '%v'", response["error"])
	}
}

func TestRespondMethodNotAllowed(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	RespondMethodNotAllowed(w, log)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Method not allowed" {
		t.Errorf("Expected error 'Method not allowed', got '%v'", response["error"])
	}
}

func TestParseJSONBody_Success(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	jsonBody := `{"name": "test", "value": 123}`
	r := httptest.NewRequest("POST", "/test", strings.NewReader(jsonBody))

	var target TestStruct
	result := ParseJSONBody(w, r, log, &target)

	if !result {
		t.Error("ParseJSONBody() should return true for valid JSON")
	}

	if target.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", target.Name)
	}

	if target.Value != 123 {
		t.Errorf("Expected value 123, got %d", target.Value)
	}
}

func TestParseJSONBody_InvalidJSON(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	type TestStruct struct {
		Name string `json:"name"`
	}

	invalidJSON := `{"name": invalid}`
	r := httptest.NewRequest("POST", "/test", strings.NewReader(invalidJSON))

	var target TestStruct
	result := ParseJSONBody(w, r, log, &target)

	if result {
		t.Error("ParseJSONBody() should return false for invalid JSON")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["error"] != "Invalid JSON request body" {
		t.Errorf("Expected error message about invalid JSON, got '%v'", response["error"])
	}
}

func TestParseJSONBody_EmptyBody(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	type TestStruct struct {
		Name string `json:"name"`
	}

	r := httptest.NewRequest("POST", "/test", bytes.NewReader([]byte{}))

	var target TestStruct
	result := ParseJSONBody(w, r, log, &target)

	// Empty body should fail to parse
	if result {
		t.Error("ParseJSONBody() should return false for empty body")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestParseJSONBody_TypeMismatch(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	type TestStruct struct {
		Value int `json:"value"`
	}

	// String instead of int
	jsonBody := `{"value": "not a number"}`
	r := httptest.NewRequest("POST", "/test", strings.NewReader(jsonBody))

	var target TestStruct
	result := ParseJSONBody(w, r, log, &target)

	if result {
		t.Error("ParseJSONBody() should return false for type mismatch")
	}

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRespondJSON_EncodingError(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	// Create a channel which cannot be marshaled to JSON
	data := map[string]interface{}{
		"channel": make(chan int),
	}

	// This should not panic even though encoding fails
	RespondJSON(w, log, http.StatusOK, data)

	// Status code should still be set
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d even on encoding error, got %d", http.StatusOK, w.Code)
	}
}

func TestRespondError_WithSpecialCharacters(t *testing.T) {
	log := logger.NewJSON("info")
	w := httptest.NewRecorder()

	message := "Error with \"quotes\" and <html> tags & special chars"
	RespondError(w, log, http.StatusBadRequest, message)

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// JSON encoding should properly escape special characters
	if response["error"] != message {
		t.Errorf("Message not properly preserved: got '%v'", response["error"])
	}
}
