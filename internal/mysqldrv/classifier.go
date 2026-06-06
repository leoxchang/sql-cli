package mysqldrv

import (
	"context"
	"errors"

	"github.com/go-sql-driver/mysql"
	"github.com/qiezi999/sql-cli/internal/output"
)

// ClassifyError analyzes an error and returns the appropriate error code,
// message, and details map based on the error type.
//
// MySQL error mapping:
//   - 1045, 1044 → AUTH_ERROR (authentication/access denied)
//   - 1142 → PERMISSION_DENIED (insufficient privileges)
//   - 1146 → QUERY_ERROR (table doesn't exist)
//   - 1064 → QUERY_ERROR (syntax error)
//   - 3024 → QUERY_ERROR (query execution interrupted)
//   - context.DeadlineExceeded → TIMEOUT
//   - All other errors → INTERNAL_ERROR
//
// Details map population:
//   - Non-INTERNAL_ERROR: includes mysql_error_code (uint16) and optionally sql (string)
//   - INTERNAL_ERROR: returns nil/empty map (per failure-envelope schema)
//
// The sql parameter is included in details for debugging purposes when available.
func ClassifyError(err error, sql string) (output.ErrorCode, string, map[string]any) {
	if err == nil {
		return output.ErrorCodeInternalError, "unexpected nil error", nil
	}

	// Check for context timeout
	if errors.Is(err, context.DeadlineExceeded) {
		details := map[string]any{
			"mysql_error_code": uint16(0), // 0 indicates non-MySQL error
		}
		if sql != "" {
			details["sql"] = sql
		}
		return output.ErrorCodeTimeout, "operation timed out", details
	}

	// Check for MySQL errors
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return classifyMySQLError(mysqlErr, sql)
	}

	// Default to internal error for unknown error types
	return output.ErrorCodeInternalError, err.Error(), nil
}

// classifyMySQLError maps MySQL error numbers to error codes.
func classifyMySQLError(err *mysql.MySQLError, sql string) (output.ErrorCode, string, map[string]any) {
	details := map[string]any{
		"mysql_error_code": err.Number,
	}
	if sql != "" {
		details["sql"] = sql
	}

	switch err.Number {
	case 1045, 1044:
		// 1045: Access denied for user
		// 1044: Access denied for database
		return output.ErrorCodeAuthError, err.Message, details

	case 1142:
		// 1142: SELECT command denied to user
		return output.ErrorCodePermissionDenied, err.Message, details

	case 1146:
		// 1146: Table doesn't exist
		return output.ErrorCodeQueryError, err.Message, details

	case 1064:
		// 1064: SQL syntax error
		return output.ErrorCodeQueryError, err.Message, details

	case 3024:
		// 3024: Query execution was interrupted
		return output.ErrorCodeQueryError, err.Message, details

	default:
		// Unknown MySQL error - treat as internal error
		// Per spec: INTERNAL_ERROR must return nil/empty details
		return output.ErrorCodeInternalError, err.Message, nil
	}
}