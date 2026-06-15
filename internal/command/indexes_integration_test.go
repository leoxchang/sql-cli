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

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

const indexesBinaryPath = "/tmp/sql-cli-test-indexes"

func TestIndexesSubcommand_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL container: %v", err)
	}
	defer teardown()

	seedIndexesTestTable(t, dsn)

	buildCmd := exec.Command("go", "build", "-o", indexesBinaryPath, "./cmd/sql-cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v\n%s", err, out)
	}

	t.Run("Success_ReturnsIndexMetadata", func(t *testing.T) {
		stdout, stderr, exitCode := runIndexesCmd(t, dsn, "indexes", "testdb.idx_test")
		if exitCode != 0 {
			t.Fatalf("Expected exit 0, got %d\nStderr: %s\nStdout: %s", exitCode, stderr, stdout)
		}
		if len(stderr) > 0 {
			t.Errorf("Expected clean stderr, got: %s", stderr)
		}

		var env output.Envelope
		if err := json.Unmarshal(stdout, &env); err != nil {
			t.Fatalf("Failed to parse envelope: %v\nOutput: %s", err, stdout)
		}
		if !env.Ok {
			t.Fatalf("Expected ok=true, got error: %s", env.Error.Message)
		}

		if env.RowCount != 4 {
			t.Errorf("Expected 4 rows, got %d", env.RowCount)
		}
		if len(env.Columns) != 14 {
			t.Errorf("Expected 14 columns, got %d", len(env.Columns))
		}

		expectedNames := []string{
			"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
			"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
			"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
			"IS_VISIBLE", "EXPRESSION",
		}
		for i, col := range env.Columns {
			if col.Name != expectedNames[i] {
				t.Errorf("Column %d: expected %q, got %q", i, expectedNames[i], col.Name)
			}
		}

		// On 8.0.13+: IS_VISIBLE = "YES", EXPRESSION = "" (non-expression index)
		assertColumnValue(t, env, "IS_VISIBLE", 0, "YES")
		assertColumnValue(t, env, "EXPRESSION", 0, "")
	})

	t.Run("NonExistentTable_EmptyResult", func(t *testing.T) {
		stdout, _, exitCode := runIndexesCmd(t, dsn, "indexes", "testdb.nonexistent")
		if exitCode != 0 {
			t.Fatalf("Expected exit 0 for non-existent table, got %d", exitCode)
		}

		var env output.Envelope
		if err := json.Unmarshal(stdout, &env); err != nil {
			t.Fatalf("Failed to parse envelope: %v", err)
		}
		if !env.Ok {
			t.Fatalf("Expected ok=true for non-existent table")
		}
		if env.RowCount != 0 {
			t.Errorf("Expected row_count=0, got %d", env.RowCount)
		}
	})

	t.Run("Aliases_IdenticalOutput", func(t *testing.T) {
		commands := []string{"indexes", "idx", "keys"}
		outputs := make([]string, len(commands))
		for i, sub := range commands {
			stdout, _, exitCode := runIndexesCmd(t, dsn, sub, "testdb.idx_test")
			if exitCode != 0 {
				t.Fatalf("%s: expected exit 0, got %d", sub, exitCode)
			}
			outputs[i] = string(stdout)
		}
		for i := 1; i < len(outputs); i++ {
			if outputs[i] != outputs[0] {
				t.Errorf("Alias %q output differs from indexes", commands[i])
			}
		}
	})

	t.Run("TableOnlyForm_DSNFallback", func(t *testing.T) {
		stdout, _, exitCode := runIndexesCmd(t, dsn, "indexes", "idx_test")
		if exitCode != 0 {
			t.Fatalf("Expected exit 0, got %d", exitCode)
		}

		var env output.Envelope
		if err := json.Unmarshal(stdout, &env); err != nil {
			t.Fatalf("Failed to parse envelope: %v", err)
		}
		if !env.Ok || env.RowCount != 4 {
			t.Errorf("Expected 4 rows via DSN fallback, got ok=%v rows=%d", env.Ok, env.RowCount)
		}
	})

	t.Run("ParameterValidation", func(t *testing.T) {
		cases := []struct {
			name string
			arg  string
		}{
			{"three segments", "a.b.c"},
			{"dot only", "."},
			{"trailing dot", "a."},
			{"leading dot", ".a"},
			{"double dot", "a..b"},
			{"injection", "testdb.idx_test; DROP TABLE x"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, _, exitCode := runIndexesCmd(t, dsn, "indexes", tc.arg)
				if exitCode != 2 {
					t.Errorf("Expected exit 2 for %q, got %d", tc.name, exitCode)
				}
			})
		}
	})
}

func TestIndexesSubcommand_Integration_ExpressionIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL container: %v", err)
	}
	defer teardown()

	db, err := sql.Open("mysql", mustDriverDSN(t, dsn))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS testdb",
		`CREATE TABLE IF NOT EXISTS testdb.t_expr (
			a INT, b INT,
			INDEX idx_expr ((a + b))
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("Seed failed: %v\nSQL: %s", err, s)
		}
	}

	buildCmd := exec.Command("go", "build", "-o", indexesBinaryPath, "./cmd/sql-cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v\n%s", err, out)
	}

	stdout, _, exitCode := runIndexesCmd(t, dsn, "indexes", "testdb.t_expr")
	if exitCode != 0 {
		t.Fatalf("Expected exit 0, got %d\nStdout: %s", exitCode, stdout)
	}

	var env output.Envelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}
	if !env.Ok || env.RowCount != 1 {
		t.Fatalf("Expected 1 row, got ok=%v rows=%d", env.Ok, env.RowCount)
	}

	// EXPRESSION should contain "(a + b)" for the expression index
	exprVal := getColumnValue(t, env, "EXPRESSION", 0)
	if exprVal == "" {
		t.Errorf("Expected non-empty EXPRESSION for expression index, got empty string")
	}
	if !strings.Contains(exprVal, "a") || !strings.Contains(exprVal, "b") {
		t.Errorf("Expected EXPRESSION to contain a and b, got %q", exprVal)
	}

	// COLUMN_NAME should be empty for expression indexes
	colNameVal := getColumnValue(t, env, "COLUMN_NAME", 0)
	if colNameVal != "" {
		t.Errorf("Expected empty COLUMN_NAME for expression index, got %q", colNameVal)
	}
}

func TestIndexesSubcommand_Integration_MySQL57(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if !mySQL57ImageAvailable() {
		t.Skip("mysql:5.7 image not available; pull with: docker pull mysql:5.7")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQL57ContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL 5.7 container: %v", err)
	}
	defer teardown()

	seedIndexesTestTable(t, dsn)

	buildCmd := exec.Command("go", "build", "-o", indexesBinaryPath, "./cmd/sql-cli")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v\n%s", err, out)
	}

	stdout, stderr, exitCode := runIndexesCmd(t, dsn, "indexes", "testdb.idx_test")
	if exitCode != 0 {
		t.Fatalf("Expected exit 0 on 5.7, got %d\nStderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}

	var env output.Envelope
	if err := json.Unmarshal(stdout, &env); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}
	if !env.Ok {
		t.Fatalf("Expected ok=true on 5.7")
	}
	if env.RowCount != 4 {
		t.Errorf("Expected 4 rows on 5.7, got %d", env.RowCount)
	}
	if len(env.Columns) != 14 {
		t.Errorf("Expected 14 columns on 5.7, got %d", len(env.Columns))
	}

	// On 5.7: IS_VISIBLE and EXPRESSION are both "" (compatibility projection)
	assertColumnValue(t, env, "IS_VISIBLE", 0, "")
	assertColumnValue(t, env, "EXPRESSION", 0, "")
}

// --- helpers ---

func seedIndexesTestTable(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("mysql", mustDriverDSN(t, dsn))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS testdb",
		`CREATE TABLE IF NOT EXISTS testdb.idx_test (
			tenant_id INT NOT NULL,
			id INT NOT NULL,
			name VARCHAR(100),
			PRIMARY KEY (tenant_id, id),
			INDEX idx_name (tenant_id, name)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("Seed failed: %v\nSQL: %s", err, s)
		}
	}
}

func runIndexesCmd(t *testing.T, dsn, subcommand, arg string) ([]byte, []byte, int) {
	t.Helper()
	cmd := exec.Command(indexesBinaryPath, "--dsn", dsn, subcommand, arg)
	stdout, err := cmd.Output()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stdout, exitErr.Stderr, exitErr.ExitCode()
	}
	if err != nil {
		return stdout, []byte(fmt.Sprintf("exec error: %v", err)), -1
	}
	return stdout, nil, 0
}

func assertColumnValue(t *testing.T, env output.Envelope, colName string, rowIdx int, expected string) {
	t.Helper()
	actual := getColumnValue(t, env, colName, rowIdx)
	if actual != expected {
		t.Errorf("Column %s row %d: expected %q, got %q", colName, rowIdx, expected, actual)
	}
}

func getColumnValue(t *testing.T, env output.Envelope, colName string, rowIdx int) string {
	t.Helper()
	colIdx := -1
	for i, c := range env.Columns {
		if c.Name == colName {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		t.Fatalf("Column %q not found in envelope", colName)
	}
	if rowIdx >= len(env.Rows) {
		t.Fatalf("Row index %d out of range (have %d rows)", rowIdx, len(env.Rows))
	}
	row, ok := env.Rows[rowIdx].([]any)
	if !ok {
		t.Fatalf("Row %d is not []any: %T", rowIdx, env.Rows[rowIdx])
	}
	return fmt.Sprintf("%v", row[colIdx])
}

func mustDriverDSN(t *testing.T, mysqlURL string) string {
	t.Helper()
	dsn, err := config.MySQLURLToDriverDSN(mysqlURL)
	if err != nil {
		t.Fatalf("Failed to convert DSN: %v", err)
	}
	return dsn
}

func mySQL57ImageAvailable() bool {
	cmd := exec.Command("docker", "image", "inspect", "mysql:5.7")
	return cmd.Run() == nil
}
