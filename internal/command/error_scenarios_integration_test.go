//go:build integration

package command_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// TestErrorScenarios_Integration tests error handling through the CLI binary.
// This test verifies the error classifier (D2, D10) and one-to-one exit code mapping.
//
// Test scenarios:
// 1. Bad credentials → AUTH_ERROR (exit 4)
// 2. Permission denied → PERMISSION_DENIED (exit 7)
// 3. Bad host → CONNECTION_ERROR (exit 3)
func TestErrorScenarios_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Build CLI binary (ensure it exists)
	binaryPath := "/tmp/sql-cli-test-error"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	buildCmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Test Scenario 1: Bad credentials → AUTH_ERROR (exit 4)
	t.Run("BadCredentials_AUTH_ERROR_Exit4", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create MySQL container
		mysqlC, err := mysql.Run(ctx,
			"mysql:8.0",
			mysql.WithUsername("test"),
			mysql.WithPassword("test"),
			mysql.WithDatabase("testdb"),
		)
		if err != nil {
			t.Fatalf("Failed to create MySQL container: %v", err)
		}
		defer func() {
			if err := mysqlC.Terminate(ctx); err != nil {
				t.Logf("Warning: failed to terminate container: %v", err)
			}
		}()

		// Get connection string
		connStr, err := mysqlC.ConnectionString(ctx)
		if err != nil {
			t.Fatalf("Failed to get connection string: %v", err)
		}

		// Create DSN with WRONG password
		// Original: test:test@localhost:PORT/testdb
		// Modified: test:WRONG_PASSWORD@localhost:PORT/testdb
		badDSN := "mysql://" + connStr
		badDSN = replacePassword(badDSN, "test", "WRONG_PASSWORD")

		// Execute query with bad credentials
		cmd := exec.Command(binaryPath, "--dsn", badDSN, "query", "SELECT 1")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runErrorCommand(cmd)

		// Verify exit code 4 (AUTH_ERROR)
		if exitCode != 4 {
			t.Errorf("Expected exit code 4 (AUTH_ERROR), got %d", exitCode)
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
		if envelope.Error.Code != output.ErrorCodeAuthError {
			t.Errorf("Expected error code AUTH_ERROR, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}

		// Verify details map contains mysql_error_code 1045
		if envelope.Error.Details == nil {
			t.Fatal("Expected details map to be populated for AUTH_ERROR")
		}

		mysqlErrorCodeRaw, ok := envelope.Error.Details["mysql_error_code"]
		if !ok {
			t.Error("Expected details to contain 'mysql_error_code' field")
		} else {
			mysqlErrorCode, ok := mysqlErrorCodeRaw.(float64)
			if !ok {
				t.Errorf("Expected mysql_error_code to be numeric, got %T", mysqlErrorCodeRaw)
			} else if uint16(mysqlErrorCode) != 1045 {
				t.Errorf("Expected mysql_error_code=1045, got %d", uint16(mysqlErrorCode))
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
			} else if sqlStr != "SELECT 1" {
				t.Errorf("Expected sql='SELECT 1', got '%s'", sqlStr)
			}
		}
	})

	// Test Scenario 2: Permission denied → PERMISSION_DENIED (exit 7)
	t.Run("PermissionDenied_PERMISSION_DENIED_Exit7", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Create MySQL container
		mysqlC, err := mysql.Run(ctx,
			"mysql:8.0",
			mysql.WithUsername("restricted"),
			mysql.WithPassword("restricted"),
			mysql.WithDatabase("testdb"),
		)
		if err != nil {
			t.Fatalf("Failed to create MySQL container: %v", err)
		}
		defer func() {
			if err := mysqlC.Terminate(ctx); err != nil {
				t.Logf("Warning: failed to terminate container: %v", err)
			}
		}()

		// Get container connection details
		// Connect as root to create restricted user
		port, err := mysqlC.MappedPort(ctx, "3306")
		if err != nil {
			t.Fatalf("Failed to get mapped port: %v", err)
		}

		host, err := mysqlC.Host(ctx)
		if err != nil {
			t.Fatalf("Failed to get host: %v", err)
		}

		// Create connection string for root access
		rootDSN := fmt.Sprintf("mysql://root:test@%s:%s/testdb", host, port.Port())

		// Create restricted user with NO privileges
		createUserCmd := exec.Command(binaryPath, "--dsn", rootDSN, "query",
			"CREATE USER IF NOT EXISTS 'noperm'@'%' IDENTIFIED BY 'noperm'")
		createUserCmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"
		if err := createUserCmd.Run(); err != nil {
			t.Logf("Warning: failed to create user: %v", err)
		}

		// Try to query with restricted user (should fail with permission denied)
		restrictedDSN := fmt.Sprintf("mysql://noperm:noperm@%s:%s/testdb", host, port.Port())

		cmd := exec.Command(binaryPath, "--dsn", restrictedDSN, "query", "SELECT * FROM secrets")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runErrorCommand(cmd)

		// Verify exit code 7 (PERMISSION_DENIED)
		if exitCode != 7 {
			t.Errorf("Expected exit code 7 (PERMISSION_DENIED), got %d", exitCode)
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
		if envelope.Error.Code != output.ErrorCodePermissionDenied {
			t.Errorf("Expected error code PERMISSION_DENIED, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}

		// Verify details map contains mysql_error_code (1044 or 1142)
		if envelope.Error.Details == nil {
			t.Fatal("Expected details map to be populated for PERMISSION_DENIED")
		}

		mysqlErrorCodeRaw, ok := envelope.Error.Details["mysql_error_code"]
		if !ok {
			t.Error("Expected details to contain 'mysql_error_code' field")
		} else {
			mysqlErrorCode, ok := mysqlErrorCodeRaw.(float64)
			if !ok {
				t.Errorf("Expected mysql_error_code to be numeric, got %T", mysqlErrorCodeRaw)
			} else {
				code := uint16(mysqlErrorCode)
				// MySQL may return 1044 (access denied for database) or 1142 (SELECT command denied)
				if code != 1044 && code != 1142 {
					t.Errorf("Expected mysql_error_code=1044 or 1142, got %d", code)
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
			} else if sqlStr != "SELECT * FROM secrets" {
				t.Errorf("Expected sql='SELECT * FROM secrets', got '%s'", sqlStr)
			}
		}
	})

	// Test Scenario 3: Bad host → CONNECTION_ERROR (exit 3)
	t.Run("BadHost_CONNECTION_ERROR_Exit3", func(t *testing.T) {
		// Use invalid host that doesn't exist
		badDSN := "mysql://test:test@invalid-host-that-does-not-exist.local:3306/testdb"

		cmd := exec.Command(binaryPath, "--dsn", badDSN, "query", "SELECT 1")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runErrorCommand(cmd)

		// Verify exit code 3 (CONNECTION_ERROR)
		if exitCode != 3 {
			t.Errorf("Expected exit code 3 (CONNECTION_ERROR), got %d", exitCode)
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
		if envelope.Error.Code != output.ErrorCodeConnectionError {
			t.Errorf("Expected error code CONNECTION_ERROR, got %s", envelope.Error.Code)
		}
		if envelope.Error.Message == "" {
			t.Error("Expected non-empty error message")
		}

		// Verify NO mysql_error_code for connection errors (network-level error)
		if envelope.Error.Details != nil {
			if _, ok := envelope.Error.Details["mysql_error_code"]; ok {
				t.Error("Expected no mysql_error_code for CONNECTION_ERROR (network-level error)")
			}
		}

		// Connection errors may or may not have details depending on error type
		// (net.OpError vs generic connection error)
	})
}

// runErrorCommand executes a command and returns stdout, stderr, and exit code
func runErrorCommand(cmd *exec.Cmd) ([]byte, []byte, int) {
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

// replacePassword replaces the password in a mysql:// DSN
// Format: mysql://user:password@host:port/database
func replacePassword(dsn, oldPass, newPass string) string {
	// Simple replacement: mysql://user:oldPass@ → mysql://user:newPass@
	oldPattern := ":" + oldPass + "@"
	newPattern := ":" + newPass + "@"
	return replaceString(dsn, oldPattern, newPattern)
}

// replaceString replaces a substring in a string
func replaceString(s, old, new string) string {
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}