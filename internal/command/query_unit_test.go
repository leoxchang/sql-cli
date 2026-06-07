package command

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
)

// TestParseQueryArgs tests argument parsing logic (unit test, no containers)
func TestParseQueryArgs(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		expectedSQL     string
		expectedErrCode int
	}{
		{
			name:            "direct_sql",
			args:            []string{"SELECT 1"},
			expectedSQL:     "SELECT 1",
			expectedErrCode: 0,
		},
		{
			name:            "empty_args",
			args:            []string{},
			expectedSQL:     "",
			expectedErrCode: 2,
		},
		{
			name:            "empty_sql_whitespace",
			args:            []string{"   "},
			expectedSQL:     "",
			expectedErrCode: 2,
		},
		{
			name:            "empty_sql_empty_string",
			args:            []string{""},
			expectedSQL:     "",
			expectedErrCode: 2,
		},
		{
			name:            "sql_with_whitespace",
			args:            []string{"  SELECT 1  "},
			expectedSQL:     "  SELECT 1  ", // Original preserved
			expectedErrCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: stdin tests ("-", "--stdin") require mocking and are handled separately
			if tt.name == "stdin_dash" || tt.name == "stdin_explicit" {
				t.Skip("Stdin tests require mocking")
			}

			sql, errCode := parseQueryArgs(tt.args)

			if errCode != tt.expectedErrCode {
				t.Errorf("Expected error code %d, got %d", tt.expectedErrCode, errCode)
			}

			if errCode == 0 && sql != tt.expectedSQL {
				t.Errorf("Expected SQL '%s', got '%s'", tt.expectedSQL, sql)
			}
		})
	}
}

// TestParseQueryArgsStdinSuccess tests successful stdin reading
func TestParseQueryArgsStdinSuccess(t *testing.T) {
	tests := []struct {
		name string
		flag string
		sql  string
	}{
		{
			name: "stdin_dash",
			flag: "-",
			sql:  "SELECT 1",
		},
		{
			name: "stdin_explicit",
			flag: "--stdin",
			sql:  "SELECT 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock stdin
			oldStdin := os.Stdin
			r, w, _ := os.Pipe()
			os.Stdin = r

			// Write SQL to stdin
			go func() {
				w.Write([]byte(tt.sql))
				w.Close()
			}()

			// Mock stdout to capture error messages
			oldStdout := os.Stdout
			rOut, wOut, _ := os.Pipe()
			os.Stdout = wOut

			sql, errCode := parseQueryArgs([]string{tt.flag})

			// Restore stdin and stdout
			wOut.Close()
			os.Stdin = oldStdin
			os.Stdout = oldStdout

			// Read captured output (discard)
			var buf bytes.Buffer
			buf.ReadFrom(rOut)

			if errCode != 0 {
				t.Errorf("Expected error code 0, got %d", errCode)
			}

			if sql != tt.sql {
				t.Errorf("Expected SQL '%s', got '%s'", tt.sql, sql)
			}
		})
	}
}

// TestParseQueryArgsStdinEmpty tests empty stdin
func TestParseQueryArgsStdinEmpty(t *testing.T) {
	tests := []struct {
		name string
		flag string
	}{
		{
			name: "stdin_dash_empty",
			flag: "-",
		},
		{
			name: "stdin_explicit_empty",
			flag: "--stdin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock stdin with empty content
			oldStdin := os.Stdin
			r, w, _ := os.Pipe()
			os.Stdin = r

			// Write empty content
			go func() {
				w.Write([]byte("   "))
				w.Close()
			}()

			// Mock stdout to capture error message
			oldStdout := os.Stdout
			rOut, wOut, _ := os.Pipe()
			os.Stdout = wOut

			sql, errCode := parseQueryArgs([]string{tt.flag})

			// Restore stdin and stdout
			wOut.Close()
			os.Stdin = oldStdin
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			buf.ReadFrom(rOut)

			// Should return CONFIG_ERROR (exit 2)
			if errCode != 2 {
				t.Errorf("Expected error code 2 (CONFIG_ERROR), got %d", errCode)
			}

			if sql != "" {
				t.Errorf("Expected empty SQL, got '%s'", sql)
			}

			// Verify error envelope
			var envelope output.Envelope
			if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
				t.Fatalf("Failed to parse envelope: %v", err)
			}

			if envelope.Ok {
				t.Error("Expected error envelope, got success")
			}

			if envelope.Error.Code != output.ErrorCodeConfigError {
				t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
			}
		})
	}
}

// TestReadStdinWithTimeout tests stdin reading with timeout (unit test)
func TestReadStdinWithTimeout(t *testing.T) {
	// Test successful read
	t.Run("successful_read", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r

		// Write data immediately
		go func() {
			w.Write([]byte("SELECT 1"))
			w.Close()
		}()

		sql, err := readStdinWithTimeout(5 * time.Second)
		os.Stdin = oldStdin

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if sql != "SELECT 1" {
			t.Errorf("Expected 'SELECT 1', got '%s'", sql)
		}
	})

	// Test timeout
	t.Run("timeout", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r

		// Don't write anything - let it timeout
		// Cleanup writer after timeout
		go func() {
			time.Sleep(200 * time.Millisecond)
			w.Close()
		}()

		// Use very short timeout for test
		sql, err := readStdinWithTimeout(100 * time.Millisecond)
		os.Stdin = oldStdin

		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Expected timeout error message, got: %v", err)
		}
		if sql != "" {
			t.Errorf("Expected empty SQL on timeout, got '%s'", sql)
		}
	})

	// Test large content
	t.Run("large_content", func(t *testing.T) {
		oldStdin := os.Stdin
		r, w, _ := os.Pipe()
		os.Stdin = r

		// Write large SQL
		largeSQL := strings.Repeat("SELECT * FROM users WHERE id = 1; ", 1000)
		go func() {
			w.Write([]byte(largeSQL))
			w.Close()
		}()

		sql, err := readStdinWithTimeout(5 * time.Second)
		os.Stdin = oldStdin

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if sql != largeSQL {
			t.Errorf("Expected large SQL, got truncated content")
		}
	})
}

// TestHandleQueryEmptyArgs tests HandleQuery with empty arguments
func TestHandleQueryEmptyArgs(t *testing.T) {
	// Mock stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HandleQuery with empty args
	exitCode := HandleQuery("mysql://root:test@localhost:3306/test", []string{})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify exit code (CONFIG_ERROR = 2)
	if exitCode != 2 {
		t.Errorf("Expected exit code 2, got %d", exitCode)
	}

	// Parse envelope
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}

	// Verify error envelope
	if envelope.Ok {
		t.Error("Expected error envelope, got success")
	}

	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}

	if !strings.Contains(envelope.Error.Message, "no SQL query provided") {
		t.Errorf("Expected message 'no SQL query provided', got '%s'", envelope.Error.Message)
	}
}

// TestHandleQueryEmptySQL tests HandleQuery with empty SQL string
func TestHandleQueryEmptySQL(t *testing.T) {
	// Mock stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call HandleQuery with empty SQL
	exitCode := HandleQuery("mysql://root:test@localhost:3306/test", []string{"   "})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify exit code (CONFIG_ERROR = 2)
	if exitCode != 2 {
		t.Errorf("Expected exit code 2, got %d", exitCode)
	}

	// Parse envelope
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}

	// Verify error envelope
	if envelope.Ok {
		t.Error("Expected error envelope, got success")
	}

	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestHandleQuerySafetyBlocked tests HandleQuery safety scanner blocking
func TestHandleQuerySafetyBlocked(t *testing.T) {
	tests := []struct {
		name        string
		sql         string
		expectedKW  []string
	}{
		{
			name:       "insert_blocked",
			sql:        "INSERT INTO users VALUES (1)",
			expectedKW: []string{"INSERT"},
		},
		{
			name:       "update_blocked",
			sql:        "UPDATE users SET name = 'test'",
			expectedKW: []string{"UPDATE"},
		},
		{
			name:       "delete_blocked",
			sql:        "DELETE FROM users",
			expectedKW: []string{"DELETE"},
		},
		{
			name:       "drop_blocked",
			sql:        "DROP TABLE users",
			expectedKW: []string{"DROP"},
		},
		{
			name:       "multiple_keywords",
			sql:        "CREATE TABLE t; INSERT INTO t VALUES (1)",
			expectedKW: []string{"CREATE", "INSERT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call HandleQuery with write keyword
			exitCode := HandleQuery("mysql://root:test@localhost:3306/test", []string{tt.sql})

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			buf.ReadFrom(r)

			// Verify exit code (SAFETY_BLOCKED = 5)
			if exitCode != 5 {
				t.Errorf("Expected exit code 5, got %d", exitCode)
			}

			// Parse envelope
			var envelope output.Envelope
			if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
				t.Fatalf("Failed to parse envelope: %v", err)
			}

			// Verify error envelope
			if envelope.Ok {
				t.Error("Expected error envelope, got success")
			}

			if envelope.Error.Code != output.ErrorCodeSafetyBlocked {
				t.Errorf("Expected error code SAFETY_BLOCKED, got %s", envelope.Error.Code)
			}

			// Verify keywords in details
			if keywords, ok := envelope.Error.Details["keywords"].([]interface{}); ok {
				if len(keywords) != len(tt.expectedKW) {
					t.Errorf("Expected %d keywords, got %d", len(tt.expectedKW), len(keywords))
				}

				// Check each expected keyword is present
				for _, expectedKW := range tt.expectedKW {
					found := false
					for _, kw := range keywords {
						if kw == expectedKW {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected keyword '%s' not found in %v", expectedKW, keywords)
					}
				}
			} else {
				t.Error("Expected keywords in details")
			}

			// Verify SQL in details
			if sqlInDetails, ok := envelope.Error.Details["sql"].(string); ok {
				if sqlInDetails != tt.sql {
					t.Errorf("Expected SQL '%s' in details, got '%s'", tt.sql, sqlInDetails)
				}
			} else {
				t.Error("Expected sql in details")
			}
		})
	}
}