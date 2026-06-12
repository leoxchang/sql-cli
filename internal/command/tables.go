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

// validIdentifier matches legal MySQL identifiers: alphanumeric, underscore, and dash.
// MySQL permits dashes inside backtick-quoted identifiers (e.g., `ry-vue`).
// The whitelist still rejects backticks, NUL bytes, and other meta-characters that
// could escape the backtick delimiter or introduce SQL syntax during construction.
var validIdentifier = regexp.MustCompile("^[A-Za-z0-9_-]+$")

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

	// Resolve database identifier: positional argument, or fall back to DSN URL path.
	// The DSN path is preferred when the user is already in the database they want
	// to inspect — typing `tables` again would be busywork.
	var database string
	if len(args) >= 1 {
		database = args[0]
	} else {
		// DSN must be a syntactically valid mysql:// URL before we can extract the
		// database segment. ValidateMySQLURL also rejects \n/\r, so it is safe to
		// call before MySQLURLToDriverDSN.
		if err := config.ValidateMySQLURL(dsn); err != nil {
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				"database name required: provide as positional argument or in DSN URL (--dsn mysql://user:pass@host:port/<db>)",
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		}
		dsnDB, err := config.ExtractDatabaseFromURL(dsn)
		if err != nil || dsnDB == "" {
			errEnvelope := output.NewErrorEnvelope(
				output.ErrorCodeConfigError,
				"database name required: provide as positional argument or in DSN URL (--dsn mysql://user:pass@host:port/<db>)",
				nil,
			)
			_ = output.WriteError(errEnvelope)
			return output.ErrorCodeConfigError.ExitCode()
		}
		database = dsnDB
	}

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

	// Build query against INFORMATION_SCHEMA so the response carries both the
	// table name and the MySQL TABLE_COMMENT (set via `COMMENT='...'` in CREATE
	// TABLE). The dbName placeholder is bound, not interpolated; the whitelist
	// check above already excludes anything but [A-Za-z0-9_-], so the binding
	// is belt-and-suspenders.
	query := "SELECT TABLE_NAME, TABLE_COMMENT FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? ORDER BY TABLE_NAME"

	// Execute query
	rows, err := db.QueryContext(ctx, query, database)
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
