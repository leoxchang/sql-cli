package command

import "fmt"

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

// buildIndexSQL returns the probe SQL (always the same) and the main query SQL
// selected based on which optional columns the server exposes.
//
//   - hasIsVisible=true, hasExpression=true  → 8.0.13+: full 14 columns
//   - hasIsVisible=true, hasExpression=false → 8.0.0-8.0.12: 12 + IS_VISIBLE + '' AS EXPRESSION
//   - hasIsVisible=false, hasExpression=false → 5.7: 12 + '' AS IS_VISIBLE + '' AS EXPRESSION
func buildIndexSQL(database, table string, hasIsVisible, hasExpression bool) (probe string, main string, err error) {
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

	main = fmt.Sprintf(
		"SELECT %s FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX",
		joinColumns(cols),
	)

	return probeSQL, main, nil
}

// joinColumns joins column expressions with ", ".
func joinColumns(cols []string) string {
	result := ""
	for i, c := range cols {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}