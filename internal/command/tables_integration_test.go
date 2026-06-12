//go:build integration

package command_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

// TestTablesSubcommand_Integration tests the tables subcommand end-to-end
// through the CLI binary with containerized MySQL.
//
// This test verifies:
// 1. CLI binary can be executed via os/exec
// 2. Tables lists seeded tables from existing database
// 3. Missing database argument returns CONFIG_ERROR (exit 2)
// 4. Injection attempt "app; DROP" returns CONFIG_ERROR (exit 2)
// 5. Non-existent database returns QUERY_ERROR (exit 1) with mysql_error_code
// 6. Exit codes match error envelope codes
// 7. stderr is clean on success, contains error on failure
// 8. JSON envelope structure is valid for all scenarios
func TestTablesSubcommand_Integration(t *testing.T) {
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

	// Seed testdb with sample tables
	seedTestTables(t, dsn)

	// Build CLI binary (ensure it exists)
	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	buildCmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Test Scenario 1: Success - tables lists seeded tables
	t.Run("Success_ListsSeededTables", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "tables", "testdb")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, stderr, exitCode := runCommand(cmd)

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

		// Verify columns structure: now two columns — table name and TABLE_COMMENT.
		if len(envelope.Columns) != 2 {
			t.Errorf("Expected 2 columns (table name + comment), got %d columns", len(envelope.Columns))
		} else {
			if envelope.Columns[0].Name != fmt.Sprintf("Tables_in_testdb") {
				t.Errorf("Expected column[0] name 'Tables_in_testdb', got '%s'", envelope.Columns[0].Name)
			}
			if envelope.Columns[1].Name != "Table_comment" {
				t.Errorf("Expected column[1] name 'Table_comment', got '%s'", envelope.Columns[1].Name)
			}
		}

		// Verify at least 2 seeded tables exist
		if envelope.RowCount < 2 {
			t.Errorf("Expected at least 2 tables, got %d", envelope.RowCount)
		}

		// Verify seeded tables are in the list, and the users table carries its 中文 comment.
		foundUsers := false
		foundProducts := false
		usersComment := ""
		for _, row := range envelope.Rows {
			rowArray, ok := row.([]any)
			if !ok {
				t.Errorf("Expected row to be []any, got %T", row)
				continue
			}
			if len(rowArray) != 2 {
				t.Errorf("Expected row to have 2 elements (name + comment), got %d", len(rowArray))
				continue
			}
			tableName, ok := rowArray[0].(string)
			if !ok {
				t.Errorf("Expected table name to be string, got %T", rowArray[0])
				continue
			}
			// comment is a string column; may be empty for tables without COMMENT clause.
			comment, _ := rowArray[1].(string)
			if tableName == "users" {
				foundUsers = true
				usersComment = comment
			}
			if tableName == "products" {
				foundProducts = true
			}
		}

		if !foundUsers {
			t.Errorf("Expected to find 'users' table in list")
			t.Logf("Available tables: %v", envelope.Rows)
		}
		if foundUsers && usersComment != "用户表" {
			t.Errorf("Expected users table COMMENT='用户表', got %q", usersComment)
		}
		if !foundProducts {
			t.Errorf("Expected to find 'products' table in list")
			t.Logf("Available tables: %v", envelope.Rows)
		}

		// Verify elapsed time is reasonable
		if envelope.Elapsed <= 0 {
			t.Errorf("Expected positive elapsed_ms, got %d", envelope.Elapsed)
		}
	})

	// Test Scenario 2: Missing argument AND DSN without database path → CONFIG_ERROR (exit 2)
	t.Run("MissingArgument_CONFIG_ERROR", func(t *testing.T) {
		// Strip the trailing /<db> from the DSN so the handler cannot fall back
		// to the URL path. New behaviour: positional arg OR DSN path is required.
		noDBDSN := strings.TrimSuffix(dsn, "/"+extractDBName(dsn))
		cmd := exec.Command(binaryPath, "--dsn", noDBDSN, "tables")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runCommand(cmd)

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
		if !containsString(envelope.Error.Message, "database name required") {
			t.Errorf("Expected message to contain 'database name required', got '%s'", envelope.Error.Message)
		}
		if !containsString(envelope.Error.Message, "DSN URL") {
			t.Errorf("Expected message to mention DSN URL fallback, got '%s'", envelope.Error.Message)
		}
	})

	// Test Scenario 2b: Missing argument BUT DSN contains database path → SUCCESS (fallback)
	t.Run("MissingArgument_FallsBackToDSN", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "tables")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runCommand(cmd)

		// Exit code 0 — handler should not require positional arg when DSN provides one.
		if exitCode != 0 {
			t.Errorf("Expected exit code 0 (DSN database fallback), got %d", exitCode)
		}

		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdout)
		}
		if !envelope.Ok {
			t.Errorf("Expected success envelope via DSN fallback, got error: %+v", envelope.Error)
		}
	})

	// Test Scenario 3: Injection attempt → CONFIG_ERROR (exit 2)
	t.Run("InjectionAttempt_CONFIG_ERROR", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "tables", "app; DROP TABLE users; --")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runCommand(cmd)

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
		if !containsString(envelope.Error.Message, "invalid database identifier") {
			t.Errorf("Expected message to contain 'invalid database identifier', got '%s'", envelope.Error.Message)
		}

		// Verify no details map for CONFIG_ERROR
		if envelope.Error.Details != nil && len(envelope.Error.Details) > 0 {
			t.Errorf("Expected no details for CONFIG_ERROR, got: %v", envelope.Error.Details)
		}
	})

	// Test Scenario 4: Non-existent database → QUERY_ERROR (exit 1) with mysql_error_code
	t.Run("NonExistentDatabase_QUERY_ERROR", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "tables", "nonexistent_db")
		cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

		stdout, _, exitCode := runCommand(cmd)

		// Verify exit code 1
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

		// Verify mysql_error_code in details
		if envelope.Error.Details == nil {
			t.Fatal("Expected details map to be populated for QUERY_ERROR, got nil")
		}
		mysqlErrorCode, ok := envelope.Error.Details["mysql_error_code"]
		if !ok {
			t.Error("Expected 'mysql_error_code' in details map")
			t.Logf("Available details: %v", envelope.Error.Details)
		} else {
			// Verify mysql_error_code is numeric
			switch v := mysqlErrorCode.(type) {
			case float64:
				if v < 1000 || v > 9999 {
					t.Errorf("Expected mysql_error_code to be 4-digit number, got %v", v)
				}
			case uint16:
				if v < 1000 || v > 9999 {
					t.Errorf("Expected mysql_error_code to be 4-digit number, got %v", v)
				}
			default:
				t.Errorf("Expected mysql_error_code to be numeric, got %T", mysqlErrorCode)
			}
		}

		// Verify sql field in details (optional but expected for QUERY_ERROR)
		if _, ok := envelope.Error.Details["sql"]; !ok {
			t.Log("Note: 'sql' field not present in details (optional for QUERY_ERROR)")
		}
	})
}

// seedTestTables creates sample tables in the testdb database for testing
func seedTestTables(t *testing.T, mysqlURL string) {
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

	// Drop tables first so a re-run picks up schema changes (notably users.COMMENT
	// added in the new INFORMATION_SCHEMA-based output). IF EXISTS keeps the
	// first run green.
	if _, err := db.Exec("DROP TABLE IF EXISTS users"); err != nil {
		t.Fatalf("Failed to drop users table: %v", err)
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS products"); err != nil {
		t.Fatalf("Failed to drop products table: %v", err)
	}

	// Create users table with a TABLE_COMMENT so the new tables output column
	// has something to display. products is left without a comment to exercise
	// the "table has no COMMENT" path (column populated with "").
	createUsersTable := `
		CREATE TABLE users (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) COMMENT='用户表'
	`
	if _, err := db.Exec(createUsersTable); err != nil {
		t.Fatalf("Failed to create users table: %v", err)
	}

	// Create products table
	createProductsTable := `
		CREATE TABLE products (
			id INT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL,
			price DECIMAL(10, 2) NOT NULL,
			description TEXT
		)
	`
	if _, err := db.Exec(createProductsTable); err != nil {
		t.Fatalf("Failed to create products table: %v", err)
	}

	// Verify tables were created
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'testdb'").Scan(&tableCount)
	if err != nil {
		t.Fatalf("Failed to verify table creation: %v", err)
	}
	if tableCount < 2 {
		t.Errorf("Expected at least 2 tables to be created, got %d", tableCount)
	}

	t.Logf("Seeded testdb with %d tables", tableCount)
}

// runCommand executes a command and returns stdout, stderr, and exit code
func runCommand(cmd *exec.Cmd) ([]byte, []byte, int) {
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
	stdout, err := readAll(stdoutPipe)
	if err != nil {
		return nil, []byte(fmt.Sprintf("Failed to read stdout: %v", err)), -1
	}

	// Read stderr
	stderr, err := readAll(stderrPipe)
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

// containsString checks if a string contains a substring (case-sensitive)
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// extractDBName extracts the trailing database segment from a mysql:// DSN.
// Input:  mysql://user:pass@host:port/testdb
// Output: testdb
//
// Returns empty string if there is no database segment.
func extractDBName(mysqlURL string) string {
	// Find the last '/' before any query string.
	q := strings.Index(mysqlURL, "?")
	path := mysqlURL
	if q >= 0 {
		path = mysqlURL[:q]
	}
	i := strings.LastIndex(path, "/")
	if i < 0 || i == len(path)-1 {
		return ""
	}
	return path[i+1:]
}

// Note: readAll function is defined in databases_integration_test.go (same package)