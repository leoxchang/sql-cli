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

// probeSQL queries INFORMATION_SCHEMA.COLUMNS to detect which optional columns
// exist on INFORMATION_SCHEMA.STATISTICS. IS_VISIBLE was added in MySQL 8.0.0,
// EXPRESSION in MySQL 8.0.13. MySQL 5.7 has neither.
const probeSQL = "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = 'information_schema' AND TABLE_NAME = 'statistics' AND COLUMN_NAME IN ('is_visible', 'expression')"

// baseColumns are the 12 columns present on all supported MySQL versions.
var baseColumns = []string{
	"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
	"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
	"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
}

// buildIndexSQL returns the main query SQL selected based on which optional columns
// the server exposes.
//
//   - hasIsVisible=true, hasExpression=true  → 8.0.13+: full 14 columns
//   - hasIsVisible=true, hasExpression=false → 8.0.0-8.0.12: 12 + IS_VISIBLE + '' AS EXPRESSION
//   - hasIsVisible=false, hasExpression=false → 5.7: 12 + '' AS IS_VISIBLE + '' AS EXPRESSION
func buildIndexSQL(hasIsVisible, hasExpression bool) string {
	cols := make([]string, len(baseColumns))
	copy(cols, baseColumns)

	if hasIsVisible {
		cols = append(cols, "IS_VISIBLE")
	} else {
		cols = append(cols, "'' AS IS_VISIBLE")
	}

	if hasExpression {
		cols = append(cols, "EXPRESSION")
	} else {
		cols = append(cols, "'' AS EXPRESSION")
	}

	return fmt.Sprintf(
		"SELECT %s FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX",
		strings.Join(cols, ", "),
	)
}

// parseIndexesArgs parses the single positional argument for the indexes command.
// Returns (database, table, error).
//
// Two valid forms:
//   - "<db>.<table>" — exactly one dot, both segments match validIdentifier
//   - "<table>" — database taken from DSN URL path; error if DSN has no database
//
// Rejected: empty string, 0 or 3+ dot-segments, any segment with empty or
// non-whitelist characters.
func parseIndexesArgs(arg string, dsn string) (database string, table string, err error) {
	if arg == "" {
		return "", "", fmt.Errorf("table identifier required")
	}

	parts := strings.Split(arg, ".")

	// Count non-empty segments. Any empty segment (from ".", "a.", ".a", "a..b")
	// is rejected — the split must produce exactly 1 or 2 non-empty parts with
	// no empty parts mixed in.
	nonEmpty := 0
	for _, p := range parts {
		if p != "" {
			nonEmpty++
		}
	}

	switch {
	case nonEmpty == 0 || len(parts) > 2:
		return "", "", fmt.Errorf("invalid identifier format: expected table or database.table")
	case len(parts) == 2 && (parts[0] == "" || parts[1] == ""):
		// "a." → ["a", ""] or ".a" → ["", "a"]
		return "", "", fmt.Errorf("invalid identifier format: expected table or database.table")
	case len(parts) == 2:
		database = parts[0]
		table = parts[1]
	case len(parts) == 1:
		table = parts[0]
		dsnDB, dsErr := config.ExtractDatabaseFromURL(dsn)
		if dsErr != nil || dsnDB == "" {
			return "", "", fmt.Errorf("database required: specify as database.table or include database in DSN URL (--dsn mysql://user:pass@host:port/<db>)")
		}
		database = dsnDB
	}

	if !validIdentifier.MatchString(database) {
		return "", "", fmt.Errorf("invalid database identifier: %s", database)
	}
	if !validIdentifier.MatchString(table) {
		return "", "", fmt.Errorf("invalid table identifier: %s", table)
	}

	return database, table, nil
}

// HandleIndexes executes the indexes subcommand: queries INFORMATION_SCHEMA.STATISTICS
// for the given <db>.<table> and writes a 14-column JSON envelope.
func HandleIndexes(dsn string, args []string) int {
	startTime := time.Now()

	if len(args) < 1 {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"table identifier required: sql-cli --dsn <dsn> indexes <table> or indexes <database.table>",
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	database, table, err := parseIndexesArgs(args[0], dsn)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	driverDSN, errEnvelope := config.MySQLURLToDriverDSNOrError(dsn)
	if errEnvelope != nil {
		_ = output.WriteError(errEnvelope)
		return errEnvelope.Error.Code.ExitCode()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mysqldrv.Open(ctx, driverDSN)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, "")
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}
	defer db.Close()

	// Probe: detect IS_VISIBLE and EXPRESSION column existence
	probeRows, err := db.QueryContext(ctx, probeSQL)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, probeSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}

	hasIsVisible := false
	hasExpression := false
	for probeRows.Next() {
		var colName string
		if err := probeRows.Scan(&colName); err != nil {
			probeRows.Close()
			envelope := output.NewErrorEnvelope(
				output.ErrorCodeInternalError,
				fmt.Sprintf("failed to scan probe result: %v", err),
				nil,
			)
			_ = output.WriteError(envelope)
			return output.ErrorCodeInternalError.ExitCode()
		}
		switch colName {
		case "is_visible":
			hasIsVisible = true
		case "expression":
			hasExpression = true
		}
	}
	probeRows.Close()
	if err := probeRows.Err(); err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, probeSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}

	// Build main query based on probe results
	mainSQL := buildIndexSQL(hasIsVisible, hasExpression)

	rows, err := db.QueryContext(ctx, mainSQL, database, table)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, mainSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}
	defer rows.Close()

	columns, rowData, err := output.ConvertRows(rows)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeQueryError,
			fmt.Sprintf("failed to convert result rows: %v", err),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeQueryError.ExitCode()
	}

	rowsAsAny := make([]any, len(rowData))
	for i, row := range rowData {
		rowsAsAny[i] = row
	}

	elapsedMs := time.Since(startTime).Milliseconds()

	err = output.WriteSuccess(os.Stdout, columns, rowsAsAny, len(rowData), elapsedMs)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeInternalError,
			fmt.Sprintf("failed to write output: %v", err),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeInternalError.ExitCode()
	}

	return 0
}