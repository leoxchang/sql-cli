package command

import (
	"fmt"
	"strings"

	"github.com/qiezi999/sql-cli/internal/config"
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