package output

import (
	"encoding/json"
	"testing"
)

func TestErrorCodeExitCode(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected int
	}{
		{ErrorCodeConfigError, 2},
		{ErrorCodeConnectionError, 3},
		{ErrorCodeAuthError, 4},
		{ErrorCodePermissionDenied, 7},
		{ErrorCodeSafetyBlocked, 5},
		{ErrorCodeQueryError, 1},
		{ErrorCodeTimeout, 6},
		{ErrorCodeInternalError, 99},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if got := tt.code.ExitCode(); got != tt.expected {
				t.Errorf("ErrorCode.ExitCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSuccessEnvelopeJSON(t *testing.T) {
	columns := []Column{
		{Name: "id", Type: "int"},
		{Name: "name", Type: "varchar"},
	}
	rows := []any{
		map[string]any{"id": 1, "name": "Alice"},
		map[string]any{"id": 2, "name": "Bob"},
	}

	envelope := NewSuccessEnvelope(columns, rows, 150)

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal success envelope: %v", err)
	}

	// Verify the JSON structure
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check ok field
	if ok, _ := result["ok"].(bool); !ok {
		t.Error("Expected ok to be true")
	}

	// Check row_count
	if rc, _ := result["row_count"].(float64); int(rc) != 2 {
		t.Errorf("Expected row_count to be 2, got %v", rc)
	}

	// Check elapsed_ms
	if elapsed, _ := result["elapsed_ms"].(float64); int64(elapsed) != 150 {
		t.Errorf("Expected elapsed_ms to be 150, got %v", elapsed)
	}

	// Verify error field is nil/not present
	if _, hasError := result["error"]; hasError {
		t.Error("Success envelope should not have error field")
	}
}

func TestErrorEnvelopeJSON(t *testing.T) {
	details := map[string]any{
		"query":  "SELECT * FROM users",
		"line":   42,
		"hint":   "Check syntax near 'SELEC'",
	}

	envelope := NewErrorEnvelope(ErrorCodeQueryError, "SQL syntax error", details)

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal error envelope: %v", err)
	}

	// Verify the JSON structure
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check ok field
	if ok, _ := result["ok"].(bool); ok {
		t.Error("Expected ok to be false")
	}

	// Check error object
	errorObj, _ := result["error"].(map[string]any)
	if errorObj == nil {
		t.Fatal("Expected error object to be present")
	}

	if code, _ := errorObj["code"].(string); code != "QUERY_ERROR" {
		t.Errorf("Expected error code 'QUERY_ERROR', got %v", code)
	}

	if msg, _ := errorObj["message"].(string); msg != "SQL syntax error" {
		t.Errorf("Expected message 'SQL syntax error', got %v", msg)
	}

	// Check details map
	errorDetails, _ := errorObj["details"].(map[string]any)
	if errorDetails == nil {
		t.Fatal("Expected details to be present")
	}

	if query, _ := errorDetails["query"].(string); query != "SELECT * FROM users" {
		t.Errorf("Expected query in details, got %v", query)
	}

	// Verify success fields are absent
	if _, hasColumns := result["columns"]; hasColumns {
		t.Error("Error envelope should not have columns field")
	}
	if _, hasRows := result["rows"]; hasRows {
		t.Error("Error envelope should not have rows field")
	}
}

func TestErrorEnvelopeWithoutDetails(t *testing.T) {
	envelope := NewErrorEnvelope(ErrorCodeConnectionError, "Failed to connect", nil)

	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Failed to marshal error envelope: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	errorObj, _ := result["error"].(map[string]any)
	if errorObj == nil {
		t.Fatal("Expected error object to be present")
	}

	// Check that details field is omitted when nil
	if _, hasDetails := errorObj["details"]; hasDetails {
		t.Error("Details field should be omitted when nil")
	}
}