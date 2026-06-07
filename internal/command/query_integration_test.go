//go:build integration

package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

// TestQuerySubcommand_Integration tests the query subcommand end-to-end
// through the CLI binary with containerized MySQL.
//
// This test verifies:
// 1. Success: SELECT 1 returns result, exit 0
// 2. Safety blocked: UPDATE blocked, SAFETY_BLOCKED exit 5
// 3. Syntax error: invalid SQL → QUERY_ERROR exit 1, mysql_error_code 1064
// 4. Empty SQL: CONFIG_ERROR exit 2
// 5. Stdin read: query - reads from stdin successfully
func TestQuerySubcommand_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Setup MySQL container with DSN
	_, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL container: %v", err)
	}
	defer teardown()

	// Build CLI binary (ensure it exists)
	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	buildCmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Test Scenario 1: Success - SELECT 1 returns result
	t.Run("Success_SELECT1_ReturnsResult", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "query", "SELECT 1 AS value")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, stderr, exitCode := runQueryCommand(cmd)

		// Verify exit code 0
		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d\nStderr: %s", exitCode, stderr)
		}

		// Verify stderr is clean
		if len(stderr) > 0 {
			t.Errorf("Expected clean stderr, got: %s", stderr)
		}

		// Parse stdout as JSON envelope
		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}

		// Verify success envelope
		if !envelope.Ok {
			t.Errorf("Expected ok=true, got ok=false with error: %+v", envelope.Error)
		}

		// Verify columns structure
		if len(envelope.Columns) != 1 {
			t.Errorf("Expected 1 column, got %d columns", len(envelope.Columns))
		} else {
			if envelope.Columns[0].Name != "value" {
				t.Errorf("Expected column name 'value', got '%s'", envelope.Columns[0].Name)
			}
			if envelope.Columns[0].Type == "" {
				t.Errorf("Expected column type to be non-empty")
			}
		}

		// Verify row count
		if envelope.RowCount != 1 {
			t.Errorf("Expected row_count=1, got %d", envelope.RowCount)
		}

		// Verify row data
		if len(envelope.Rows) != 1 {
			t.Errorf("Expected 1 row, got %d rows", len(envelope.Rows))
		} else {
			rowArray, ok := envelope.Rows[0].([]any)
			if !ok {
				t.Errorf("Expected row to be []any, got %T", envelope.Rows[0])
			} else if len(rowArray) != 1 {
				t.Errorf("Expected row to have 1 element, got %d", len(rowArray))
			}
		}

		// Verify elapsed time is reasonable
		if envelope.Elapsed <= 0 {
			t.Errorf("Expected positive elapsed_ms, got %d", envelope.Elapsed)
		}
	})

	// Test Scenario 2: Safety blocked - UPDATE is blocked
	t.Run("SafetyBlocked_UPDATE_BlockedExit5", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "query", "UPDATE users SET name = 'hacked'")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runQueryCommand(cmd)

		// Verify exit code 5 (SAFETY_BLOCKED)
		if exitCode != 5 {
			t.Errorf("Expected exit code 5 (SAFETY_BLOCKED), got %d", exitCode)
		}

		// Parse stdout as JSON envelope
		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}

		// Verify error envelope
		if envelope.Ok {
			t.Error("Expected ok=false (error envelope), got ok=true (success envelope)")
		}
		if envelope.Error == nil {
			t.Fatal("Expected error field to be populated, got nil")
		}
		if envelope.Error.Code != output.ErrorCodeSafetyBlocked {
			t.Errorf("Expected error code SAFETY_BLOCKED, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}
		if !containsQueryString(envelope.Error.Message, "query blocked by safety scanner") {
			t.Errorf("Expected message to contain 'query blocked by safety scanner', got '%s'", envelope.Error.Message)
		}

		// Verify details map contains keywords
		if envelope.Error.Details == nil {
			t.Fatal("Expected details map to be populated for SAFETY_BLOCKED")
		}

		// Verify keywords field exists and contains UPDATE
		keywordsRaw, ok := envelope.Error.Details["keywords"]
		if !ok {
			t.Error("Expected details to contain 'keywords' field")
		} else {
			// Keywords should be an array of strings
			keywords, ok := keywordsRaw.([]any)
			if !ok {
				t.Errorf("Expected keywords to be []any, got %T", keywordsRaw)
			} else {
				// Check if UPDATE is in keywords
				foundUpdate := false
				for _, kw := range keywords {
					if kw == "UPDATE" {
						foundUpdate = true
						break
					}
				}
				if !foundUpdate {
					t.Errorf("Expected keywords to contain 'UPDATE', got: %v", keywords)
				}
			}
		}

		// Verify sql field exists in details
		sqlRaw, ok := envelope.Error.Details["sql"]
		if !ok {
			t.Error("Expected details to contain 'sql' field")
		} else {
			sqlStr, ok := sqlRaw.(string)
			if !ok {
				t.Errorf("Expected sql to be string, got %T", sqlRaw)
			} else if !containsQueryString(sqlStr, "UPDATE") {
				t.Errorf("Expected sql to contain 'UPDATE', got '%s'", sqlStr)
			}
		}
	})

	// Test Scenario 3: Syntax error - invalid SQL
	t.Run("SyntaxError_InvalidSQL_QUERY_ERROR_Exit1_MySqlErrorCode1064", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "query", "SELCT 1") // Intentional typo
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runQueryCommand(cmd)

		// Verify exit code 1 (QUERY_ERROR)
		if exitCode != 1 {
			t.Errorf("Expected exit code 1 (QUERY_ERROR), got %d", exitCode)
		}

		// Parse stdout as JSON envelope
		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}

		// Verify error envelope
		if envelope.Ok {
			t.Error("Expected ok=false (error envelope), got ok=true (success envelope)")
		}
		if envelope.Error == nil {
			t.Fatal("Expected error field to be populated, got nil")
		}
		if envelope.Error.Code != output.ErrorCodeQueryError {
			t.Errorf("Expected error code QUERY_ERROR, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}

		// Verify details map contains mysql_error_code 1064
		if envelope.Error.Details == nil {
			t.Fatal("Expected details map to be populated for QUERY_ERROR")
		}

		mysqlErrorCodeRaw, ok := envelope.Error.Details["mysql_error_code"]
		if !ok {
			t.Error("Expected details to contain 'mysql_error_code' field")
		} else {
			// mysql_error_code should be a number (JSON unmarshals as float64 for numbers)
			mysqlErrorCode, ok := mysqlErrorCodeRaw.(float64)
			if !ok {
				t.Errorf("Expected mysql_error_code to be numeric, got %T", mysqlErrorCodeRaw)
			} else if uint16(mysqlErrorCode) != 1064 {
				t.Errorf("Expected mysql_error_code=1064, got %d", uint16(mysqlErrorCode))
			}
		}

		// Verify sql field exists in details
		sqlRaw, ok := envelope.Error.Details["sql"]
		if !ok {
			t.Error("Expected details to contain 'sql' field")
		} else {
			sqlStr, ok := sqlRaw.(string)
			if !ok {
				t.Errorf("Expected sql to be string, got %T", sqlRaw)
			} else if sqlStr != "SELCT 1" {
				t.Errorf("Expected sql='SELCT 1', got '%s'", sqlStr)
			}
		}
	})

	// Test Scenario 4: Empty SQL - CONFIG_ERROR
	t.Run("EmptySQL_CONFIG_ERROR_Exit2", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "query", "")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runQueryCommand(cmd)

		// Verify exit code 2 (CONFIG_ERROR)
		if exitCode != 2 {
			t.Errorf("Expected exit code 2 (CONFIG_ERROR), got %d", exitCode)
		}

		// Parse stdout as JSON envelope
		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}

		// Verify error envelope
		if envelope.Ok {
			t.Error("Expected ok=false (error envelope), got ok=true (success envelope)")
		}
		if envelope.Error == nil {
			t.Fatal("Expected error field to be populated, got nil")
		}
		if envelope.Error.Code != output.ErrorCodeConfigError {
			t.Errorf("Expected error code CONFIG_ERROR, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}
		if !containsQueryString(envelope.Error.Message, "no SQL query provided") {
			t.Errorf("Expected message to contain 'no SQL query provided', got '%s'", envelope.Error.Message)
		}

		// Verify no details map for CONFIG_ERROR
		if envelope.Error.Details != nil && len(envelope.Error.Details) > 0 {
			t.Errorf("Expected no details for CONFIG_ERROR, got: %v", envelope.Error.Details)
		}
	})

	// Test Scenario 5: Stdin read - query - reads from stdin
	t.Run("StdinRead_QueryDash_ReadsFromStdin", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "query", "-")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		// Create stdin pipe with SQL query
		stdinPipe, err := cmd.StdinPipe()
		if err != nil {
			t.Fatalf("Failed to create stdin pipe: %v", err)
		}

		// Write SQL to stdin in a goroutine
		go func() {
			defer stdinPipe.Close()
			_, _ = io.WriteString(stdinPipe, "SELECT 2 AS stdin_value")
		}()

		stdout, stderr, exitCode := runQueryCommand(cmd)

		// Verify exit code 0
		if exitCode != 0 {
			t.Errorf("Expected exit code 0, got %d\nStderr: %s", exitCode, stderr)
		}

		// Verify stderr is clean
		if len(stderr) > 0 {
			t.Errorf("Expected clean stderr, got: %s", stderr)
		}

		// Parse stdout as JSON envelope
		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}

		// Verify success envelope
		if !envelope.Ok {
			t.Errorf("Expected ok=true, got ok=false with error: %+v", envelope.Error)
		}

		// Verify columns structure
		if len(envelope.Columns) != 1 {
			t.Errorf("Expected 1 column, got %d columns", len(envelope.Columns))
		} else {
			if envelope.Columns[0].Name != "stdin_value" {
				t.Errorf("Expected column name 'stdin_value', got '%s'", envelope.Columns[0].Name)
			}
		}

		// Verify row count
		if envelope.RowCount != 1 {
			t.Errorf("Expected row_count=1, got %d", envelope.RowCount)
		}

		// Verify elapsed time is reasonable
		if envelope.Elapsed <= 0 {
			t.Errorf("Expected positive elapsed_ms, got %d", envelope.Elapsed)
		}
	})
}

// runQueryCommand executes a command and returns stdout, stderr, and exit code
func runQueryCommand(cmd *exec.Cmd) ([]byte, []byte, int) {
	// Capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Run the command
	err := cmd.Run()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitCode
}

// containsQueryString checks if a string contains a substring (case-sensitive)
func containsQueryString(s, substr string) bool {
	return strings.Contains(s, substr)
}