package mysqldrv

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/qiezi999/sql-cli/internal/output"
)

// ClassifyError analyzes an error and returns the appropriate error code,
// message, and details map based on the error type.
//
// MySQL error mapping:
//   - 1045 → AUTH_ERROR (authentication failed)
//   - 1044, 1142 → PERMISSION_DENIED (insufficient privileges)
//   - 1146, 1064 → QUERY_ERROR (table doesn't exist, syntax error)
//   - 3024, context.DeadlineExceeded → TIMEOUT
//   - MySQL client library errors (2000-2999) → QUERY_ERROR
//   - Network errors → CONNECTION_ERROR
//   - All other errors → INTERNAL_ERROR
//
// Details map population:
//   - TIMEOUT: includes only sql field (no mysql_error_code)
//   - Non-TIMEOUT, non-INTERNAL_ERROR: includes mysql_error_code (uint16) and optionally sql (string)
//   - INTERNAL_ERROR: returns nil/empty map (per failure-envelope schema)
//
// The sql parameter is included in details for debugging purposes when available.
func ClassifyError(err error, sql string) (output.ErrorCode, string, map[string]any) {
	if err == nil {
		return output.ErrorCodeInternalError, "unexpected nil error", nil
	}

	// Check for context timeout first
	if errors.Is(err, context.DeadlineExceeded) {
		details := map[string]any{}
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

	// Check for network/connection errors
	if isConnectionError(err) {
		return output.ErrorCodeConnectionError, err.Error(), nil
	}

	// Default to internal error for unknown error types
	return output.ErrorCodeInternalError, err.Error(), nil
}

// isConnectionError detects network-level connection failures.
func isConnectionError(err error) bool {
	// Check for net.Error interface
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for common connection error patterns in error message
	errMsg := strings.ToLower(err.Error())
	connectionKeywords := []string{"connection", "network", "timeout", "refused"}
	for _, keyword := range connectionKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// classifyMySQLError maps MySQL error numbers to error codes.
func classifyMySQLError(err *mysql.MySQLError, sql string) (output.ErrorCode, string, map[string]any) {
	switch err.Number {
	case 1045:
		// 1045: Access denied for user (authentication failed)
		details := map[string]any{
			"mysql_error_code": err.Number,
		}
		if sql != "" {
			details["sql"] = sql
		}
		return output.ErrorCodeAuthError, err.Message, details

	case 1044, 1142:
		// 1044: Access denied for database
		// 1142: SELECT command denied to user
		details := map[string]any{
			"mysql_error_code": err.Number,
		}
		if sql != "" {
			details["sql"] = sql
		}
		return output.ErrorCodePermissionDenied, err.Message, details

	case 3024:
		// 3024: Query execution was interrupted (timeout)
		details := map[string]any{}
		if sql != "" {
			details["sql"] = sql
		}
		return output.ErrorCodeTimeout, err.Message, details

	case 1146, 1064:
		// 1146: Table doesn't exist
		// 1064: SQL syntax error
		details := map[string]any{
			"mysql_error_code": err.Number,
		}
		if sql != "" {
			details["sql"] = sql
		}
		return output.ErrorCodeQueryError, err.Message, details

	default:
		// MySQL client library errors (2000-2999) map to QUERY_ERROR
		if err.Number >= 2000 && err.Number < 3000 {
			details := map[string]any{
				"mysql_error_code": err.Number,
			}
			if sql != "" {
				details["sql"] = sql
			}
			return output.ErrorCodeQueryError, err.Message, details
		}

		// Unknown MySQL error - treat as internal error
		// Per spec: INTERNAL_ERROR must return nil/empty details
		return output.ErrorCodeInternalError, err.Message, nil
	}
}