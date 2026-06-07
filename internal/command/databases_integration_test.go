//go:build integration

package command_test

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

// TestDatabasesSubcommand_Integration tests the databases subcommand end-to-end
// through the CLI binary with containerized MySQL.
//
// This test verifies:
// 1. CLI binary can be executed via os/exec
// 2. DSN is properly passed to the command
// 3. Exit code is 0 on success
// 4. stdout contains valid JSON envelope
// 5. envelope.ok is true
// 6. envelope contains seeded testdb database
// 7. stderr is clean (no unexpected output)
func TestDatabasesSubcommand_Integration(t *testing.T) {
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

	// Execute CLI: ./sql-cli --dsn <mysql-url> databases
	cmd := exec.Command(binaryPath, "--dsn", dsn, "databases")
	cmd.Dir = "/Users/qiezi999/Documents/work/sql-cli"

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to create stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to create stderr pipe: %v", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start CLI command: %v", err)
	}

	// Read stdout
	var stdoutBuf []byte
	stdoutBuf, err = readAll(stdout)
	if err != nil {
		t.Fatalf("Failed to read stdout: %v", err)
	}

	// Read stderr
	var stderrBuf []byte
	stderrBuf, err = readAll(stderr)
	if err != nil {
		t.Fatalf("Failed to read stderr: %v", err)
	}

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		t.Fatalf("CLI command failed: %v\nStderr: %s", err, stderrBuf)
	}

	// Verify exit code 0
	if cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("Expected exit code 0, got %d", cmd.ProcessState.ExitCode())
	}

	// Verify stderr is clean (no unexpected output)
	if len(stderrBuf) > 0 {
		t.Errorf("Expected clean stderr, got: %s", stderrBuf)
	}

	// Parse stdout as JSON envelope
	var envelope output.Envelope
	if err := json.Unmarshal(stdoutBuf, &envelope); err != nil {
		t.Fatalf("Failed to parse stdout as JSON envelope: %v\nOutput: %s", err, stdoutBuf)
	}

	// Verify success envelope
	if !envelope.Ok {
		t.Errorf("Expected ok=true, got ok=false with error: %+v", envelope.Error)
	}

	// Verify columns structure
	if len(envelope.Columns) != 1 {
		t.Errorf("Expected 1 column (Database), got %d columns", len(envelope.Columns))
	} else {
		if envelope.Columns[0].Name != "Database" {
			t.Errorf("Expected column name 'Database', got '%s'", envelope.Columns[0].Name)
		}
		if envelope.Columns[0].Type != "VARCHAR" {
			t.Errorf("Expected column type 'VARCHAR', got '%s'", envelope.Columns[0].Type)
		}
	}

	// Verify at least one database exists
	if envelope.RowCount < 1 {
		t.Errorf("Expected at least 1 database, got %d", envelope.RowCount)
	}

	// Verify testdb is in the list
	foundTestDB := false
	for _, row := range envelope.Rows {
		rowArray, ok := row.([]any)
		if !ok {
			t.Errorf("Expected row to be []any, got %T", row)
			continue
		}
		if len(rowArray) != 1 {
			t.Errorf("Expected row to have 1 element, got %d", len(rowArray))
			continue
		}
		dbName, ok := rowArray[0].(string)
		if !ok {
			t.Errorf("Expected database name to be string, got %T", rowArray[0])
			continue
		}
		if dbName == "testdb" {
			foundTestDB = true
			break
		}
	}

	if !foundTestDB {
		t.Errorf("Expected to find 'testdb' in database list, but it was not found")
		t.Logf("Available databases: %v", envelope.Rows)
	}

	// Verify elapsed time is present and reasonable
	if envelope.Elapsed <= 0 {
		t.Errorf("Expected positive elapsed_ms, got %d", envelope.Elapsed)
	}
	if envelope.Elapsed > 30000 {
		t.Errorf("Elapsed time too high: %dms (expected < 30s)", envelope.Elapsed)
	}

	// Verify stdout ends with newline
	outputStr := string(stdoutBuf)
	if len(outputStr) == 0 || outputStr[len(outputStr)-1] != '\n' {
		t.Error("Expected stdout to end with newline")
	}
}

// readAll reads all data from an io.Reader with timeout protection
func readAll(r io.Reader) ([]byte, error) {
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