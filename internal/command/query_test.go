//go:build integration

package command

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/safety"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// TestHandleQuery_Success tests successful query execution
func TestHandleQuery_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	// Convert to mysql:// URL format
	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute handler with valid query
	exitCode := HandleQuery(mysqlURL, []string{"SELECT 1 AS value"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify exit code
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Parse envelope
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}

	// Verify success envelope
	if !envelope.Ok {
		t.Errorf("Expected success envelope, got error: %v", envelope.Error)
	}

	// Verify columns
	if len(envelope.Columns) != 1 {
		t.Errorf("Expected 1 column, got %d", len(envelope.Columns))
	}
	if envelope.Columns[0].Name != "value" {
		t.Errorf("Expected column name 'value', got '%s'", envelope.Columns[0].Name)
	}

	// Verify rows
	if envelope.RowCount != 1 {
		t.Errorf("Expected 1 row, got %d", envelope.RowCount)
	}

	// Verify elapsed time exists
	if envelope.Elapsed <= 0 {
		t.Errorf("Expected positive elapsed time, got %d", envelope.Elapsed)
	}
}

// TestHandleQuery_SyntaxError tests query with syntax error
func TestHandleQuery_SyntaxError(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute handler with invalid query
	exitCode := HandleQuery(mysqlURL, []string{"SELECT INVALID SYNTAX HERE"})

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)

	// Verify exit code (QUERY_ERROR = 1)
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
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

	// Verify error code
	if envelope.Error.Code != output.ErrorCodeQueryError {
		t.Errorf("Expected error code QUERY_ERROR, got %s", envelope.Error.Code)
	}

	// Verify mysql_error_code is 1064 (syntax error)
	if mysqlCode, ok := envelope.Error.Details["mysql_error_code"].(float64); ok {
		if int(mysqlCode) != 1064 {
			t.Errorf("Expected mysql_error_code 1064, got %d", int(mysqlCode))
		}
	} else {
		t.Error("Expected mysql_error_code in details")
	}
}

// TestHandleQuery_SafetyBlocked tests query blocked by safety scanner
func TestHandleQuery_SafetyBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute handler with write keyword
	exitCode := HandleQuery(mysqlURL, []string{"UPDATE users SET name = 'test'"})

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

	// Verify error code
	if envelope.Error.Code != output.ErrorCodeSafetyBlocked {
		t.Errorf("Expected error code SAFETY_BLOCKED, got %s", envelope.Error.Code)
	}

	// Verify keywords in details
	if keywords, ok := envelope.Error.Details["keywords"].([]interface{}); ok {
		if len(keywords) == 0 {
			t.Error("Expected keywords in details")
		}
		// Check that UPDATE is in keywords
		found := false
		for _, kw := range keywords {
			if kw == "UPDATE" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected UPDATE in keywords")
		}
	} else {
		t.Error("Expected keywords in details")
	}
}

// TestHandleQuery_EmptySQL tests empty SQL query
func TestHandleQuery_EmptySQL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute handler with empty SQL
	exitCode := HandleQuery(mysqlURL, []string{"   "})

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

	// Verify error code
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestHandleQuery_NoArgs tests no arguments provided
func TestHandleQuery_NoArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Execute handler with no args
	exitCode := HandleQuery(mysqlURL, []string{})

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

	// Verify error code
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestHandleQuery_StdinFlag tests reading SQL from stdin via "-" flag
func TestHandleQuery_StdinFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Create mock stdin with SQL query
	sqlQuery := "SELECT 1 AS stdin_value"
	oldStdin := os.Stdin
	rIn, wIn, _ := os.Pipe()
	os.Stdin = rIn

	// Write SQL to stdin pipe
	go func() {
		wIn.Write([]byte(sqlQuery))
		wIn.Close()
	}()

	// Capture stdout
	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	// Execute handler with "-" flag
	exitCode := HandleQuery(mysqlURL, []string{"-"})

	// Restore stdin and stdout
	wOut.Close()
	os.Stdin = oldStdin
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(rOut)

	// Verify exit code
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Parse envelope
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}

	// Verify success envelope
	if !envelope.Ok {
		t.Errorf("Expected success envelope, got error: %v", envelope.Error)
	}

	// Verify columns
	if len(envelope.Columns) != 1 {
		t.Errorf("Expected 1 column, got %d", len(envelope.Columns))
	}
	if envelope.Columns[0].Name != "stdin_value" {
		t.Errorf("Expected column name 'stdin_value', got '%s'", envelope.Columns[0].Name)
	}
}

// TestHandleQuery_StdinFlagExplicit tests reading SQL from stdin via "--stdin" flag
func TestHandleQuery_StdinFlagExplicit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Create mock stdin with SQL query
	sqlQuery := "SELECT 2 AS explicit_stdin"
	oldStdin := os.Stdin
	rIn, wIn, _ := os.Pipe()
	os.Stdin = rIn

	// Write SQL to stdin pipe
	go func() {
		wIn.Write([]byte(sqlQuery))
		wIn.Close()
	}()

	// Capture stdout
	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	// Execute handler with "--stdin" flag
	exitCode := HandleQuery(mysqlURL, []string{"--stdin"})

	// Restore stdin and stdout
	wOut.Close()
	os.Stdin = oldStdin
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(rOut)

	// Verify exit code
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Parse envelope
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}

	// Verify success envelope
	if !envelope.Ok {
		t.Errorf("Expected success envelope, got error: %v", envelope.Error)
	}

	// Verify columns
	if len(envelope.Columns) != 1 {
		t.Errorf("Expected 1 column, got %d", len(envelope.Columns))
	}
	if envelope.Columns[0].Name != "explicit_stdin" {
		t.Errorf("Expected column name 'explicit_stdin', got '%s'", envelope.Columns[0].Name)
	}
}

// TestHandleQuery_StdinEmpty tests empty SQL from stdin
func TestHandleQuery_StdinEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start MySQL container
	container, err := mysql.Run(ctx, "mysql:8.0")
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Get connection string
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatalf("Failed to get connection string: %v", err)
	}

	mysqlURL := convertDriverDSNToMySQLURL(dsn)

	// Create mock stdin with empty content
	oldStdin := os.Stdin
	rIn, wIn, _ := os.Pipe()
	os.Stdin = rIn

	// Write empty content to stdin pipe
	go func() {
		wIn.Write([]byte("   "))
		wIn.Close()
	}()

	// Capture stdout
	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	// Execute handler with "-" flag
	exitCode := HandleQuery(mysqlURL, []string{"-"})

	// Restore stdin and stdout
	wOut.Close()
	os.Stdin = oldStdin
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(rOut)

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

	// Verify error code
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

// TestParseQueryArgs tests argument parsing logic
func TestParseQueryArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedSQL    string
		expectedErrCode int
	}{
		{
			name:           "direct_sql",
			args:           []string{"SELECT 1"},
			expectedSQL:    "SELECT 1",
			expectedErrCode: 0,
		},
		{
			name:           "empty_args",
			args:           []string{},
			expectedSQL:    "",
			expectedErrCode: 2,
		},
		{
			name:           "empty_sql",
			args:           []string{"   "},
			expectedSQL:    "",
			expectedErrCode: 2,
		},
		{
			name:           "stdin_dash",
			args:           []string{"-"},
			expectedSQL:    "", // Will be read from stdin in actual test
			expectedErrCode: 0, // Needs stdin mock, tested separately
		},
		{
			name:           "stdin_explicit",
			args:           []string{"--stdin"},
			expectedSQL:    "", // Will be read from stdin in actual test
			expectedErrCode: 0, // Needs stdin mock, tested separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip stdin tests here (they need mocking)
			if tt.name == "stdin_dash" || tt.name == "stdin_explicit" {
				t.Skip("Stdin tests handled separately")
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

// TestReadStdinWithTimeout tests stdin reading with timeout
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
		// Close writer after test to cleanup
		go func() {
			time.Sleep(35 * time.Second) // Wait longer than test timeout
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
}

// TestSafetyScannerIntegration tests safety scanner integration
func TestSafetyScannerIntegration(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "safe_select",
			sql:      "SELECT * FROM users",
			expected: []string{},
		},
		{
			name:     "insert_blocked",
			sql:      "INSERT INTO users VALUES (1)",
			expected: []string{"INSERT"},
		},
		{
			name:     "update_blocked",
			sql:      "UPDATE users SET name = 'test'",
			expected: []string{"UPDATE"},
		},
		{
			name:     "delete_blocked",
			sql:      "DELETE FROM users",
			expected: []string{"DELETE"},
		},
		{
			name:     "drop_blocked",
			sql:      "DROP TABLE users",
			expected: []string{"DROP"},
		},
		{
			name:     "multiple_keywords",
			sql:      "CREATE TABLE t; INSERT INTO t VALUES (1)",
			expected: []string{"CREATE", "INSERT"},
		},
		{
			name:     "keyword_in_comment",
			sql:      "SELECT * FROM users -- UPDATE is commented",
			expected: []string{},
		},
		{
			name:     "keyword_in_string",
			sql:      "SELECT 'UPDATE statement' AS text",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := safety.ScanKeywords(strings.TrimSpace(tt.sql))

			if len(keywords) != len(tt.expected) {
				t.Errorf("Expected %d keywords, got %d: %v", len(tt.expected), len(keywords), keywords)
			}

			// Check each expected keyword
			for _, expectedKW := range tt.expected {
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
		})
	}
}