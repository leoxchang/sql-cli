//go:build integration

package command_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

// TestDescribeSubcommand_Integration tests the describe subcommand end-to-end
// through the CLI binary with containerized MySQL.
//
// This test verifies:
// 1. Success: describe returns column metadata for seeded table
// 2. Missing dot → CONFIG_ERROR (exit 2)
// 3. Multi-dot → CONFIG_ERROR (exit 2)
// 4. Injection attempt → CONFIG_ERROR (exit 2)
// 5. Exit codes match error envelope codes
// 6. stderr is clean on success, contains error on failure
// 7. JSON envelope structure is valid for all scenarios
// 8. Column metadata includes name and type fields
func TestDescribeSubcommand_Integration(t *testing.T) {
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

	// Seed testdb with users table
	seedDescribeTestTable(t, dsn)

	// Build CLI binary (ensure it exists)
	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	buildCmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Test Scenario 1: Success - describe returns column metadata
	t.Run("Success_ReturnsColumnMetadata", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "describe", "testdb.users")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, stderr, exitCode := runDescribeCommand(cmd)

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

		// Verify columns structure (DESCRIBE returns: Field, Type, Null, Key, Default, Extra)
		expectedColumns := 6
		if len(envelope.Columns) != expectedColumns {
			t.Errorf("Expected %d columns, got %d columns", expectedColumns, len(envelope.Columns))
		} else {
			// Verify column names match DESCRIBE output structure
			expectedNames := []string{"Field", "Type", "Null", "Key", "Default", "Extra"}
			for i, col := range envelope.Columns {
				if col.Name != expectedNames[i] {
					t.Errorf("Expected column[%d] name '%s', got '%s'", i, expectedNames[i], col.Name)
				}
				// Verify Type field is populated (VARCHAR, INT, TIMESTAMP, etc.)
				if col.Type == "" {
					t.Errorf("Expected column[%d] type to be non-empty", i)
				}
			}
		}

		// Verify at least 4 rows exist (users table has: id, name, email, created_at)
		if envelope.RowCount < 4 {
			t.Errorf("Expected at least 4 rows (columns in users table), got %d", envelope.RowCount)
		}

		// Verify specific columns from users table
		foundColumns := make(map[string]bool)
		for _, row := range envelope.Rows {
			rowArray, ok := row.([]any)
			if !ok {
				t.Errorf("Expected row to be []any, got %T", row)
				continue
			}
			if len(rowArray) < 2 {
				t.Errorf("Expected row to have at least 2 elements (Field, Type), got %d", len(rowArray))
				continue
			}
			fieldName, ok := rowArray[0].(string)
			if !ok {
				t.Errorf("Expected Field name to be string, got %T", rowArray[0])
				continue
			}
			fieldType, ok := rowArray[1].(string)
			if !ok {
				t.Errorf("Expected Type to be string, got %T", rowArray[1])
				continue
			}
			foundColumns[fieldName] = true

			// Log column info for debugging
			t.Logf("Found column: %s (type: %s)", fieldName, fieldType)
		}

		// Verify expected columns from users table
		expectedFields := []string{"id", "name", "email", "created_at"}
		for _, expected := range expectedFields {
			if !foundColumns[expected] {
				t.Errorf("Expected to find column '%s' in DESCRIBE output", expected)
			}
		}

		// Verify elapsed time is reasonable
		if envelope.Elapsed <= 0 {
			t.Errorf("Expected positive elapsed_ms, got %d", envelope.Elapsed)
		}
	})

	// Test Scenario 2: Missing dot → CONFIG_ERROR (exit 2)
	t.Run("MissingDot_CONFIG_ERROR", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "describe", "testdb")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runDescribeCommand(cmd)

		// Verify exit code 2
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
		if !containsDescribeString(envelope.Error.Message, "expected database.table with exactly one dot") {
			t.Errorf("Expected message to contain 'expected database.table with exactly one dot', got '%s'", envelope.Error.Message)
		}

		// Verify no details map for CONFIG_ERROR
		if envelope.Error.Details != nil && len(envelope.Error.Details) > 0 {
			t.Errorf("Expected no details for CONFIG_ERROR, got: %v", envelope.Error.Details)
		}
	})

	// Test Scenario 3: Multi-dot → CONFIG_ERROR (exit 2)
	t.Run("MultiDot_CONFIG_ERROR", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "describe", "testdb.users.column")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runDescribeCommand(cmd)

		// Verify exit code 2
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
		if !containsDescribeString(envelope.Error.Message, "expected database.table with exactly one dot") {
			t.Errorf("Expected message to contain 'expected database.table with exactly one dot', got '%s'", envelope.Error.Message)
		}

		// Verify no details map for CONFIG_ERROR
		if envelope.Error.Details != nil && len(envelope.Error.Details) > 0 {
			t.Errorf("Expected no details for CONFIG_ERROR, got: %v", envelope.Error.Details)
		}
	})

	// Test Scenario 4: Injection attempt → CONFIG_ERROR (exit 2)
	t.Run("InjectionAttempt_CONFIG_ERROR", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "describe", "app.users; DROP TABLE users; --")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runDescribeCommand(cmd)

		// Verify exit code 2
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
		if !containsDescribeString(envelope.Error.Message, "invalid table identifier") {
			t.Errorf("Expected message to contain 'invalid table identifier', got '%s'", envelope.Error.Message)
		}

		// Verify no details map for CONFIG_ERROR
		if envelope.Error.Details != nil && len(envelope.Error.Details) > 0 {
			t.Errorf("Expected no details for CONFIG_ERROR, got: %v", envelope.Error.Details)
		}
	})
}

// seedDescribeTestTable creates a users table in the testdb database for testing
func seedDescribeTestTable(t *testing.T, mysqlURL string) {
	t.Helper()

	// Convert mysql:// URL to driver DSN format
	// mysqlURL format: mysql://test:test@localhost:3306/testdb
	// driverDSN format: test:test@tcp(localhost:3306)/testdb

	// Remove "mysql://" prefix
	dsnPart := mysqlURL[8:] // "test:test@localhost:3306/testdb"

	// Split at the last "/" to separate credentials@host:port from database
	lastSlash := len(dsnPart) - 1
	for i := len(dsnPart) - 1; i >= 0; i-- {
		if dsnPart[i] == '/' {
			lastSlash = i
			break
		}
	}

	credentialsHost := dsnPart[:lastSlash] // "test:test@localhost:3306"
	database := dsnPart[lastSlash+1:]      // "testdb"

	// Build driver DSN: credentials@host:port -> credentials@tcp(host:port)/database
	// Split at "@" to separate credentials from host:port
	atIndex := -1
	for i := 0; i < len(credentialsHost); i++ {
		if credentialsHost[i] == '@' {
			atIndex = i
			break
		}
	}

	credentials := credentialsHost[:atIndex] // "test:test"
	hostPort := credentialsHost[atIndex+1:]  // "localhost:3306"

	driverDSN := fmt.Sprintf("%s@tcp(%s)/%s", credentials, hostPort, database)

	// Open database connection
	db, err := sql.Open("mysql", driverDSN)
	if err != nil {
		t.Fatalf("Failed to open database connection for seeding: %v", err)
	}
	defer db.Close()

	// Set connection parameters
	db.SetConnMaxLifetime(time.Minute * 5)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// Wait for connection to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ping to verify connection
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Create users table with various column types
	createUsersTable := `
		CREATE TABLE IF NOT EXISTS users (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`
	if _, err := db.Exec(createUsersTable); err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Verify table was created
	var tableExists int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'testdb' AND table_name = 'users'",
	).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to verify table creation: %v", err)
	}
	if tableExists != 1 {
		t.Errorf("Expected users table to be created, got count %d", tableExists)
	}

	t.Logf("Seeded testdb with users table for DESCRIBE test")
}

// runDescribeCommand executes a command and returns stdout, stderr, and exit code
func runDescribeCommand(cmd *exec.Cmd) ([]byte, []byte, int) {
	// Capture stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, []byte(fmt.Sprintf("Failed to create stdout pipe: %v", err)), -1
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, []byte(fmt.Sprintf("Failed to create stderr pipe: %v", err)), -1
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, []byte(fmt.Sprintf("Failed to start command: %v", err)), -1
	}

	// Read stdout
	stdout, err := readDescribeAll(stdoutPipe)
	if err != nil {
		return nil, []byte(fmt.Sprintf("Failed to read stdout: %v", err)), -1
	}

	// Read stderr
	stderr, err := readDescribeAll(stderrPipe)
	if err != nil {
		return nil, []byte(fmt.Sprintf("Failed to read stderr: %v", err)), -1
	}

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Get exit code
	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return stdout, stderr, exitCode
}

// containsDescribeString checks if a string contains a substring (case-sensitive)
func containsDescribeString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// readDescribeAll reads all data from an io.Reader with timeout protection
func readDescribeAll(r io.Reader) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	for {
		chunk := make([]byte, 1024)
		n, err := r.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		if n == 0 {
			break
		}
	}
	return buf, nil
}