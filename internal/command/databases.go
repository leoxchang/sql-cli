package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/mysqldrv"
	"github.com/qiezi999/sql-cli/internal/output"
)

// HandleDatabases executes the SHOW DATABASES command.
// It connects to the MySQL server, executes SHOW DATABASES,
// and writes the result as a JSON envelope to stdout.
//
// Returns exit code 0 on success, or appropriate error code on failure:
//   - 1: QUERY_ERROR (SQL syntax/execution error)
//   - 2: CONFIG_ERROR (invalid DSN)
//   - 3: CONNECTION_ERROR (connection failed)
//   - 4: AUTH_ERROR (authentication failed)
//   - 6: TIMEOUT (operation timed out)
//   - 99: INTERNAL_ERROR (unexpected error)
func HandleDatabases(dsn string, args []string) int {
	// Track elapsed time from handler entry
	startTime := time.Now()

	// Convert mysql:// URL to driver DSN format
	driverDSN, errEnvelope := config.MySQLURLToDriverDSNOrError(dsn)
	if errEnvelope != nil {
		_ = output.WriteError(errEnvelope)
		return errEnvelope.Error.Code.ExitCode()
	}

	// Create context with 30-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to database
	db, err := mysqldrv.Open(ctx, driverDSN)
	if err != nil {
		// Classify the error and emit appropriate error envelope
		errCode, message, details := mysqldrv.ClassifyError(err, "")
		errEnvelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(errEnvelope)
		return errCode.ExitCode()
	}
	defer db.Close()

	// Execute SHOW DATABASES query
	query := "SHOW DATABASES"
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// Classify the error and emit appropriate error envelope
		errCode, message, details := mysqldrv.ClassifyError(err, query)
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
	// Each row ([]any) becomes an element in the outer array
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