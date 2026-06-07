package command

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/mysqldrv"
	"github.com/qiezi999/sql-cli/internal/output"
)

// validIdentifier matches legal MySQL identifiers: alphanumeric and underscore only.
// This whitelist prevents SQL injection by rejecting any characters that could
// be used to escape the backtick delimiter in SQL construction.
var validIdentifier = regexp.MustCompile("^[A-Za-z0-9_]+$")

// HandleTables executes the SHOW TABLES FROM <database> command.
// It validates the database identifier, connects to the MySQL server,
// executes the query with proper escaping, and writes the result as a JSON envelope.
//
// Returns exit code 0 on success, or appropriate error code on failure:
//   - 1: QUERY_ERROR (database doesn't exist or SQL execution error)
//   - 2: CONFIG_ERROR (missing or invalid database identifier)
//   - 3: CONNECTION_ERROR (connection failed)
//   - 4: AUTH_ERROR (authentication failed)
//   - 6: TIMEOUT (operation timed out)
//   - 7: PERMISSION_DENIED (insufficient privileges)
//   - 99: INTERNAL_ERROR (unexpected error)
func HandleTables(dsn string, args []string) int {
	// Track elapsed time from handler entry
	startTime := time.Now()

	// Validate database argument
	if len(args) < 1 {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"database name required: sql-cli --dsn <dsn> tables <database>",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	database := args[0]

	// Validate identifier against whitelist
	if !validIdentifier.MatchString(database) {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid database identifier: %s", database),
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

	// Build query with backtick escaping
	// The whitelist validation ensures database contains only [A-Za-z0-9_],
	// which cannot interfere with backtick delimiters or SQL syntax.
	query := fmt.Sprintf("SHOW TABLES FROM `%s`", database)

	// Execute query
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