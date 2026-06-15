package output

// ErrorCode represents a standardized error code with a corresponding exit code.
// Each error code maps one-to-one with a specific exit code for CLI behavior.
type ErrorCode string

const (
	// ConfigError indicates a problem with configuration files, flags, or environment.
	// Exit code: 2
	ErrorCodeConfigError ErrorCode = "CONFIG_ERROR"

	// ConnectionError indicates a failure to connect to the database.
	// Exit code: 3
	ErrorCodeConnectionError ErrorCode = "CONNECTION_ERROR"

	// AuthError indicates authentication or credential issues.
	// Exit code: 4
	ErrorCodeAuthError ErrorCode = "AUTH_ERROR"

	// PermissionDenied indicates the user lacks necessary permissions.
	// Exit code: 7
	ErrorCodePermissionDenied ErrorCode = "PERMISSION_DENIED"

	// SafetyBlocked indicates a query was blocked by safety mechanisms.
	// Exit code: 5
	ErrorCodeSafetyBlocked ErrorCode = "SAFETY_BLOCKED"

	// QueryError indicates a SQL syntax or execution error.
	// Exit code: 1
	ErrorCodeQueryError ErrorCode = "QUERY_ERROR"

	// Timeout indicates an operation exceeded its time limit.
	// Exit code: 6
	ErrorCodeTimeout ErrorCode = "TIMEOUT"

	// InternalError indicates an unexpected internal failure.
	// Exit code: 99
	ErrorCodeInternalError ErrorCode = "INTERNAL_ERROR"
)

// ExitCode returns the numeric exit code for a given ErrorCode.
// This provides the one-to-one mapping between error codes and exit codes.
func (ec ErrorCode) ExitCode() int {
	switch ec {
	case ErrorCodeConfigError:
		return 2
	case ErrorCodeConnectionError:
		return 3
	case ErrorCodeAuthError:
		return 4
	case ErrorCodePermissionDenied:
		return 7
	case ErrorCodeSafetyBlocked:
		return 5
	case ErrorCodeQueryError:
		return 1
	case ErrorCodeTimeout:
		return 6
	case ErrorCodeInternalError:
		return 99
	default:
		return 99 // Default to internal error exit code
	}
}

// Column represents a result set column with its name and type.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ErrorDetail contains detailed information about an error.
type ErrorDetail struct {
	Code    ErrorCode      `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"` // Optional additional context
}

// Envelope represents the standard output format for all operations.
// It can represent either a success or failure response.
//
// Success envelope:
//
//	{"ok": true, "columns": [...], "rows": [...], "row_count": N, "elapsed_ms": N, "table_comment": "..."}
//
// Failure envelope:
//
//	{"ok": false, "error": {"code": "...", "message": "...", "details": {...}}}
type Envelope struct {
	// Ok indicates whether the operation succeeded
	Ok bool `json:"ok"`

	// Success fields (only populated when Ok is true)
	Columns      []Column `json:"columns,omitempty"`
	Rows         []any    `json:"rows,omitempty"`
	RowCount     int      `json:"row_count,omitempty"`
	Elapsed      int64    `json:"elapsed_ms,omitempty"`
	TableComment string   `json:"table_comment,omitempty"` // For describe command: table comment

	// Failure field (only populated when Ok is false)
	Error *ErrorDetail `json:"error,omitempty"`
}

// NewSuccessEnvelope creates a success envelope with query results.
func NewSuccessEnvelope(columns []Column, rows []any, elapsedMs int64) *Envelope {
	return &Envelope{
		Ok:       true,
		Columns:  columns,
		Rows:     rows,
		RowCount: len(rows),
		Elapsed:  elapsedMs,
	}
}

// WithTableComment adds table comment to the envelope and returns it for chaining.
func (e *Envelope) WithTableComment(comment string) *Envelope {
	e.TableComment = comment
	return e
}

// NewErrorEnvelope creates a failure envelope with error details.
func NewErrorEnvelope(code ErrorCode, message string, details map[string]any) *Envelope {
	return &Envelope{
		Ok: false,
		Error: &ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}
