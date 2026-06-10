package command

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/mysqldrv"
	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/safety"
)

// HandleQuery executes a SQL query after running safety checks.
// It supports reading SQL from stdin via `-` or `--stdin` flags.
//
// Returns exit code 0 on success, or appropriate error code on failure:
//   - 1: QUERY_ERROR (SQL syntax/execution error)
//   - 2: CONFIG_ERROR (empty SQL)
//   - 3: CONNECTION_ERROR (connection failed)
//   - 4: AUTH_ERROR (authentication failed)
//   - 5: SAFETY_BLOCKED (write keyword detected)
//   - 6: TIMEOUT (stdin read or execution timeout)
//   - 7: PERMISSION_DENIED (insufficient privileges)
//   - 99: INTERNAL_ERROR (unexpected error)
func HandleQuery(dsn string, args []string) int {
	// Track elapsed time from handler entry
	startTime := time.Now()

	// Parse arguments to get SQL
	sql, errCode := parseQueryArgs(args)
	if errCode != 0 {
		// Empty SQL → CONFIG_ERROR (exit 2)
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"no SQL query provided",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Convert mysql:// URL to driver DSN format
	driverDSN, errEnvelope := config.MySQLURLToDriverDSNOrError(dsn)
	if errEnvelope != nil {
		_ = output.WriteError(errEnvelope)
		return errEnvelope.Error.Code.ExitCode()
	}

	// Run safety scanner first
	keywords := safety.ScanKeywords(strings.TrimSpace(sql))
	if len(keywords) > 0 {
		// Write keyword detected → SAFETY_BLOCKED (exit 5)
		details := map[string]any{
			"keywords": keywords,
			"sql":      sql,
		}
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeSafetyBlocked,
			fmt.Sprintf("query blocked by safety scanner: detected write keyword(s): %s", strings.Join(keywords, ", ")),
			details,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeSafetyBlocked.ExitCode()
	}

	// Apply LIMIT guard: inject default LIMIT if SELECT has none
	maxRows := getMaxRowsFromEnv()
	sql = safety.AddLimitIfNeeded(sql, maxRows)

	// Create context with 30-second timeout for query execution
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to database
	db, err := mysqldrv.Open(ctx, driverDSN)
	if err != nil {
		// Classify the error and emit appropriate error envelope
		errCode, message, details := mysqldrv.ClassifyError(err, sql)
		errEnvelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(errEnvelope)
		return errCode.ExitCode()
	}
	defer db.Close()

	// Execute query
	rows, err := db.QueryContext(ctx, sql)
	if err != nil {
		// Classify the error and emit appropriate error envelope
		errCode, message, details := mysqldrv.ClassifyError(err, sql)
		errEnvelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(errEnvelope)
		return errCode.ExitCode()
	}
	defer rows.Close()

	// Convert rows to JSON-ready format
	columns, rowData, err := output.ConvertRows(rows)
	if err != nil {
		// Query conversion error
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeQueryError,
			fmt.Sprintf("failed to convert result rows: %v", err),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeQueryError.ExitCode()
	}

	// Convert [][]any to []any for envelope
	rowsAsAny := make([]any, len(rowData))
	for i, row := range rowData {
		rowsAsAny[i] = row
	}

	// Calculate elapsed time (wall-clock from handler entry to last row read)
	elapsedMs := time.Since(startTime).Milliseconds()

	// Write success envelope to stdout
	err = output.WriteSuccess(os.Stdout, columns, rowsAsAny, len(rowData), elapsedMs)
	if err != nil {
		// Write error for JSON marshal failure
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeInternalError,
			fmt.Sprintf("failed to write output: %v", err),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeInternalError.ExitCode()
	}

	return 0
}

// parseQueryArgs parses command arguments to extract SQL query.
// Returns the SQL string and an error code (0 for success, 2 for CONFIG_ERROR).
//
// Argument patterns:
//   - args[0] == "-" → read from stdin with 30s timeout
//   - args[0] == "--stdin" → read from stdin with 30s timeout
//   - otherwise → args[0] is the SQL query
func parseQueryArgs(args []string) (string, int) {
	if len(args) == 0 {
		// No arguments provided → CONFIG_ERROR
		return "", output.ErrorCodeConfigError.ExitCode()
	}

	// Check for stdin flags
	if args[0] == "-" || args[0] == "--stdin" {
		// Read from stdin with 30-second timeout
		sql, err := readStdinWithTimeout(30 * time.Second)
		if err != nil {
			// Stdin read timeout or error → TIMEOUT (exit 6)
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeTimeout,
				fmt.Sprintf("failed to read SQL from stdin: %v", err),
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return "", output.ErrorCodeTimeout.ExitCode()
		}

		// Check if stdin delivered empty SQL
		if strings.TrimSpace(sql) == "" {
			// Empty SQL from stdin → CONFIG_ERROR (exit 2)
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				"empty SQL query from stdin",
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return "", output.ErrorCodeConfigError.ExitCode()
		}

		return sql, 0
	}

	// Use args[0] as SQL query
	sql := args[0]

	// Check if SQL is empty
	if strings.TrimSpace(sql) == "" {
		// Empty SQL → CONFIG_ERROR (exit 2)
		return "", output.ErrorCodeConfigError.ExitCode()
	}

	return sql, 0
}

// readStdinWithTimeout reads from stdin with a timeout.
// Uses goroutine + channel pattern to enforce timeout.
//
// Returns the SQL string read from stdin, or an error on timeout/read failure.
func readStdinWithTimeout(timeout time.Duration) (string, error) {
	// Create channel for result
	resultCh := make(chan struct {
		data string
		err  error
	})

	// Spawn goroutine to read stdin
	go func() {
		data, err := io.ReadAll(os.Stdin)
		resultCh <- struct {
			data string
			err  error
		}{string(data), err}
	}()

	// Wait for result or timeout
	select {
	case result := <-resultCh:
		// Read completed before timeout
		return result.data, result.err

	case <-time.After(timeout):
		// Timeout occurred
		return "", fmt.Errorf("stdin read timeout after %v", timeout)
	}
}

// getMaxRowsFromEnv reads SQL_CLI_MAX_ROWS environment variable and returns the value.
// Returns default 1000 if not set or invalid.
func getMaxRowsFromEnv() int {
	const defaultMaxRows = 1000
	env := os.Getenv("SQL_CLI_MAX_ROWS")
	if env == "" {
		return defaultMaxRows
	}
	// Parse the value; if invalid, use default
	n := 0
	for _, c := range env {
		if c < '0' || c > '9' {
			return defaultMaxRows
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return defaultMaxRows
	}
	return n
}
