package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/mysqldrv"
	"github.com/qiezi999/sql-cli/internal/output"
)

// HandleDescribe executes the DESCRIBE <database>.<table> or DESCRIBE <table> command.
// It validates the identifier, connects to the MySQL server,
// executes the query with proper escaping, and writes the result as a JSON envelope.
//
// Identifier formats:
//   - database.table: Explicit database and table
//   - table: Uses database from DSN (if specified)
//
// Returns exit code 0 on success, or appropriate error code on failure:
//   - 1: QUERY_ERROR (table doesn't exist or SQL execution error)
//   - 2: CONFIG_ERROR (missing, invalid, or malformed identifier)
//   - 3: CONNECTION_ERROR (connection failed)
//   - 4: AUTH_ERROR (authentication failed)
//   - 6: TIMEOUT (operation timed out)
//   - 7: PERMISSION_DENIED (insufficient privileges)
//   - 99: INTERNAL_ERROR (unexpected error)
func HandleDescribe(dsn string, args []string) int {
	// Track elapsed time from handler entry
	startTime := time.Now()

	// Validate table argument
	if len(args) < 1 {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"table identifier required: sql-cli --dsn <dsn> describe <table> or describe <database.table>",
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	identifier := args[0]

	// Convert mysql:// URL to driver DSN format
	driverDSN, errEnvelope := config.MySQLURLToDriverDSNOrError(dsn)
	if errEnvelope != nil {
		_ = output.WriteError(errEnvelope)
		return errEnvelope.Error.Code.ExitCode()
	}

	// Extract database from DSN for fallback
	dsnDatabase, err := config.ExtractDatabaseFromURL(dsn)
	if err != nil {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("failed to parse DSN: %v", err),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	var database, table string

	// Parse identifier: database.table or just table
	parts := strings.Split(identifier, ".")
	switch len(parts) {
	case 1:
		// Single identifier: use as table name, get database from DSN
		table = parts[0]
		if dsnDatabase == "" {
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				"database required: specify as database.table or include database in DSN",
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		}
		database = dsnDatabase
	case 2:
		// database.table format
		database = parts[0]
		table = parts[1]
	default:
		// Too many dots
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid identifier format: %s (expected table or database.table)", identifier),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Validate both parts against whitelist
	if !validIdentifier.MatchString(database) {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid database identifier: %s", database),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	if !validIdentifier.MatchString(table) {
		errEnvelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			fmt.Sprintf("invalid table identifier: %s", table),
			nil,
		)
		_ = output.WriteError(errEnvelope)
		return output.ErrorCodeConfigError.ExitCode()
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

	// Build query with backtick escaping for both database and table
	// The whitelist validation ensures both parts contain only [A-Za-z0-9_],
	// which cannot interfere with backtick delimiters or SQL syntax.
	// Using SHOW FULL COLUMNS instead of DESCRIBE to include Comment column.
	query := fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`.`%s`", database, table)

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

	// Query table comment from INFORMATION_SCHEMA
	tableComment := ""
	tableCommentQuery := fmt.Sprintf(
		"SELECT TABLE_COMMENT FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
	)
	err = db.QueryRowContext(ctx, tableCommentQuery, database, table).Scan(&tableComment)
	if err != nil {
		// Ignore errors for table comment query - it's optional metadata
		tableComment = ""
	}

	// Convert [][]any to []any for envelope
	rowsAsAny := make([]any, len(rowData))
	for i, row := range rowData {
		rowsAsAny[i] = row
	}

	// Calculate elapsed time (wall-clock from handler entry to last row read)
	elapsedMs := time.Since(startTime).Milliseconds()

	// Write success envelope to stdout with table comment
	err = output.WriteSuccessWithTableComment(os.Stdout, columns, rowsAsAny, len(rowData), elapsedMs, tableComment)
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
