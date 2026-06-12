package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestWriteSuccess(t *testing.T) {
	columns := []Column{
		{Name: "id", Type: "int"},
		{Name: "name", Type: "varchar"},
	}
	rows := []any{
		map[string]any{"id": 1, "name": "Alice"},
		map[string]any{"id": 2, "name": "Bob"},
	}

	tests := []struct {
		name       string
		columns    []Column
		rows       []any
		rowCount   int
		elapsedMs  int64
		wantOutput bool
	}{
		{
			name:      "basic success envelope",
			columns:   columns,
			rows:      rows,
			rowCount:  2,
			elapsedMs: 150,
			wantOutput: true,
		},
		{
			name:      "empty result set",
			columns:   []Column{},
			rows:      []any{},
			rowCount:  0,
			elapsedMs: 25,
			wantOutput: true,
		},
		{
			name:      "nil rows",
			columns:   columns,
			rows:      nil,
			rowCount:  0,
			elapsedMs: 10,
			wantOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteSuccess(&buf, tt.columns, tt.rows, tt.rowCount, tt.elapsedMs)

			if err != nil {
				t.Fatalf("WriteSuccess returned error: %v", err)
			}

			// Verify the JSON structure
			var result map[string]any
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatalf("Failed to unmarshal output: %v", err)
			}

			// Check ok field
			if ok, _ := result["ok"].(bool); !ok {
				t.Error("Expected ok to be true")
			}

			// Check elapsed_ms
			if elapsed, _ := result["elapsed_ms"].(float64); int64(elapsed) != tt.elapsedMs {
				t.Errorf("Expected elapsed_ms to be %d, got %v", tt.elapsedMs, elapsed)
			}

			// Verify error field is not present
			if _, hasError := result["error"]; hasError {
				t.Error("Success envelope should not have error field")
			}
		})
	}
}

func TestWriteFailure(t *testing.T) {
	tests := []struct {
		name     string
		code     ErrorCode
		message  string
		details  map[string]any
		wantCode string
	}{
		{
			name:    "error with details",
			code:    ErrorCodeQueryError,
			message: "SQL syntax error",
			details: map[string]any{
				"mysql_error_code": uint16(1064),
				"sql":              "SELECT * FORM users",
				"hint":             "Check syntax near 'FORM'",
			},
			wantCode: "QUERY_ERROR",
		},
		{
			name:     "error without details",
			code:     ErrorCodeConnectionError,
			message:  "Failed to connect to database",
			details:  nil,
			wantCode: "CONNECTION_ERROR",
		},
		{
			name:     "error with empty details",
			code:     ErrorCodeInternalError,
			message:  "Unexpected error occurred",
			details:  map[string]any{},
			wantCode: "INTERNAL_ERROR",
		},
		{
			name:    "auth error with mysql code",
			code:    ErrorCodeAuthError,
			message: "Access denied for user",
			details: map[string]any{
				"mysql_error_code": uint16(1045),
			},
			wantCode: "AUTH_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteFailure(&buf, tt.code, tt.message, tt.details)

			if err != nil {
				t.Fatalf("WriteFailure returned error: %v", err)
			}

			// Verify the JSON structure
			var result map[string]any
			if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
				t.Fatalf("Failed to unmarshal output: %v", err)
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

			if code, _ := errorObj["code"].(string); code != tt.wantCode {
				t.Errorf("Expected error code '%s', got %v", tt.wantCode, code)
			}

			if msg, _ := errorObj["message"].(string); msg != tt.message {
				t.Errorf("Expected message '%s', got %v", tt.message, msg)
			}

			// Check details
			if tt.details != nil && len(tt.details) > 0 {
				errorDetails, _ := errorObj["details"].(map[string]any)
				if errorDetails == nil {
					t.Fatal("Expected details to be present")
				}

				// Verify specific detail fields
				if mysqlCode, ok := tt.details["mysql_error_code"]; ok {
					if got, _ := errorDetails["mysql_error_code"].(float64); uint16(got) != mysqlCode.(uint16) {
						t.Errorf("Expected mysql_error_code %d, got %v", mysqlCode, got)
					}
				}

				if sql, ok := tt.details["sql"]; ok {
					if got, _ := errorDetails["sql"].(string); got != sql.(string) {
						t.Errorf("Expected sql '%s', got %v", sql, got)
					}
				}
			} else {
				// Verify details field is omitted when nil/empty
				if _, hasDetails := errorObj["details"]; hasDetails {
					t.Error("Details field should be omitted when nil/empty")
				}
			}

			// Verify success fields are absent
			if _, hasColumns := result["columns"]; hasColumns {
				t.Error("Error envelope should not have columns field")
			}
			if _, hasRows := result["rows"]; hasRows {
				t.Error("Error envelope should not have rows field")
			}
		})
	}
}

// mockWriter is a test helper that simulates write errors
type mockWriter struct {
	err error
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	if m.err != nil {
		return 0, m.err
	}
	return len(p), nil
}

func TestWriteSuccessWriteError(t *testing.T) {
	mockErr := errors.New("write error")
	mock := &mockWriter{err: mockErr}

	err := WriteSuccess(mock, []Column{}, []any{}, 0, 0)
	if err == nil {
		t.Error("Expected error from failed write, got nil")
	}
	if err != mockErr {
		t.Errorf("Expected error '%v', got '%v'", mockErr, err)
	}
}

func TestWriteFailureWriteError(t *testing.T) {
	mockErr := errors.New("write error")
	mock := &mockWriter{err: mockErr}

	err := WriteFailure(mock, ErrorCodeInternalError, "test", nil)
	if err == nil {
		t.Error("Expected error from failed write, got nil")
	}
	if err != mockErr {
		t.Errorf("Expected error '%v', got '%v'", mockErr, err)
	}
}

func TestWriteSuccessJSONFormat(t *testing.T) {
	columns := []Column{
		{Name: "id", Type: "int"},
		{Name: "name", Type: "varchar"},
	}
	rows := []any{
		map[string]any{"id": 1, "name": "Alice"},
	}

	var buf bytes.Buffer
	err := WriteSuccess(&buf, columns, rows, 1, 100)
	if err != nil {
		t.Fatalf("WriteSuccess returned error: %v", err)
	}

	// Verify JSON is properly formatted: trailing newline ensures terminal/pipe
	// consumers see a complete line. JSON parser ignores trailing whitespace.
	output := buf.String()
	if len(output) == 0 {
		t.Fatal("Expected non-empty output")
	}
	if output[len(output)-1] != '\n' {
		t.Errorf("Expected trailing newline, got %q", output[len(output)-1:])
	}
	// JSON portion (without the trailing newline) must parse.
	var result Envelope
	if err := json.Unmarshal([]byte(output[:len(output)-1]), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Verify envelope structure
	if !result.Ok {
		t.Error("Expected Ok to be true")
	}
	if len(result.Columns) != 2 {
		t.Errorf("Expected 2 columns, got %d", len(result.Columns))
	}
	if result.Elapsed != 100 {
		t.Errorf("Expected elapsed_ms 100, got %d", result.Elapsed)
	}
}

func TestWriteFailureJSONFormat(t *testing.T) {
	details := map[string]any{
		"mysql_error_code": uint16(1064),
		"sql":              "SELECT * FORM users",
	}

	var buf bytes.Buffer
	err := WriteFailure(&buf, ErrorCodeQueryError, "Syntax error", details)
	if err != nil {
		t.Fatalf("WriteFailure returned error: %v", err)
	}

	// Verify JSON is properly formatted
	output := buf.String()
	if len(output) == 0 {
		t.Fatal("Expected non-empty output")
	}

	// Verify it's valid JSON
	var result Envelope
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Verify envelope structure
	if result.Ok {
		t.Error("Expected Ok to be false")
	}
	if result.Error == nil {
		t.Fatal("Expected Error to be present")
	}
	if result.Error.Code != ErrorCodeQueryError {
		t.Errorf("Expected code QUERY_ERROR, got %s", result.Error.Code)
	}
	if result.Error.Message != "Syntax error" {
		t.Errorf("Expected message 'Syntax error', got %s", result.Error.Message)
	}
	if result.Error.Details == nil {
		t.Fatal("Expected Details to be present")
	}
}