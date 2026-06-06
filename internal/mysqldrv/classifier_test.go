package mysqldrv

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/qiezi999/sql-cli/internal/output"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		sql            string
		wantCode       output.ErrorCode
		wantMsg        string
		wantDetailsNil bool
		wantMysqlCode  uint16
		wantSQL        string
		wantNoMysqlCode bool // For TIMEOUT errors that shouldn't have mysql_error_code
	}{
		{
			name:           "nil error returns internal error",
			err:            nil,
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeInternalError,
			wantMsg:        "unexpected nil error",
			wantDetailsNil: true,
		},
		{
			name:           "context.DeadlineExceeded returns timeout without mysql_error_code",
			err:            context.DeadlineExceeded,
			sql:            "SELECT * FROM users",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "operation timed out",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "SELECT * FROM users",
		},
		{
			name:           "context.DeadlineExceeded without sql",
			err:            context.DeadlineExceeded,
			sql:            "",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "operation timed out",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "",
		},
		{
			name:           "MySQL error 1045 returns auth error",
			err:            &mysql.MySQLError{Number: 1045, Message: "Access denied for user 'test'@'localhost'"},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeAuthError,
			wantMsg:        "Access denied for user 'test'@'localhost'",
			wantDetailsNil: false,
			wantMysqlCode:  1045,
			wantSQL:        "SELECT 1",
		},
		{
			name:           "MySQL error 1044 returns permission denied (not auth error)",
			err:            &mysql.MySQLError{Number: 1044, Message: "Access denied for database 'testdb'"},
			sql:            "USE testdb",
			wantCode:       output.ErrorCodePermissionDenied,
			wantMsg:        "Access denied for database 'testdb'",
			wantDetailsNil: false,
			wantMysqlCode:  1044,
			wantSQL:        "USE testdb",
		},
		{
			name:           "MySQL error 1142 returns permission denied",
			err:            &mysql.MySQLError{Number: 1142, Message: "SELECT command denied to user"},
			sql:            "SELECT * FROM secrets",
			wantCode:       output.ErrorCodePermissionDenied,
			wantMsg:        "SELECT command denied to user",
			wantDetailsNil: false,
			wantMysqlCode:  1142,
			wantSQL:        "SELECT * FROM secrets",
		},
		{
			name:           "MySQL error 1146 returns query error (table doesn't exist)",
			err:            &mysql.MySQLError{Number: 1146, Message: "Table 'db.nonexistent' doesn't exist"},
			sql:            "SELECT * FROM nonexistent",
			wantCode:       output.ErrorCodeQueryError,
			wantMsg:        "Table 'db.nonexistent' doesn't exist",
			wantDetailsNil: false,
			wantMysqlCode:  1146,
			wantSQL:        "SELECT * FROM nonexistent",
		},
		{
			name:           "MySQL error 1064 returns query error (syntax error)",
			err:            &mysql.MySQLError{Number: 1064, Message: "You have an error in your SQL syntax"},
			sql:            "SELEC * FROM users",
			wantCode:       output.ErrorCodeQueryError,
			wantMsg:        "You have an error in your SQL syntax",
			wantDetailsNil: false,
			wantMysqlCode:  1064,
			wantSQL:        "SELEC * FROM users",
		},
		{
			name:           "MySQL error 3024 returns timeout (not query error)",
			err:            &mysql.MySQLError{Number: 3024, Message: "Query execution was interrupted"},
			sql:            "SELECT * FROM large_table",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "Query execution was interrupted",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "SELECT * FROM large_table",
		},
		{
			name:           "MySQL error 3024 without sql",
			err:            &mysql.MySQLError{Number: 3024, Message: "Query execution was interrupted"},
			sql:            "",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "Query execution was interrupted",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "",
		},
		{
			name:           "MySQL error 1146 without sql",
			err:            &mysql.MySQLError{Number: 1146, Message: "Table doesn't exist"},
			sql:            "",
			wantCode:       output.ErrorCodeQueryError,
			wantMsg:        "Table doesn't exist",
			wantDetailsNil: false,
			wantMysqlCode:  1146,
			wantSQL:        "",
		},
		{
			name:           "MySQL client error (2000-2999) returns query error",
			err:            &mysql.MySQLError{Number: 2005, Message: "Unknown MySQL server host"},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeQueryError,
			wantMsg:        "Unknown MySQL server host",
			wantDetailsNil: false,
			wantMysqlCode:  2005,
			wantSQL:        "SELECT 1",
		},
		{
			name:           "MySQL server error (1000-1999, unmapped) returns internal error with nil details",
			err:            &mysql.MySQLError{Number: 1213, Message: "Deadlock found when trying to get lock"},
			sql:            "UPDATE users SET x=1",
			wantCode:       output.ErrorCodeInternalError,
			wantMsg:        "Deadlock found when trying to get lock",
			wantDetailsNil: true,
		},
		{
			name:           "Non-MySQL error returns internal error with nil details",
			err:            errors.New("some random error"),
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeInternalError,
			wantMsg:        "some random error",
			wantDetailsNil: true,
		},
		{
			name:           "Network error returns connection error",
			err:            &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeConnectionError,
			wantMsg:        "dial tcp: connection refused",
			wantDetailsNil: true,
		},
		{
			name:           "Connection refused error message returns connection error",
			err:            errors.New("connection refused"),
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeConnectionError,
			wantMsg:        "connection refused",
			wantDetailsNil: true,
		},
		{
			name:           "Network timeout error message returns connection error",
			err:            errors.New("network timeout"),
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeConnectionError,
			wantMsg:        "network timeout",
			wantDetailsNil: true,
		},
		{
			name:           "Wrapped MySQL error 1045 is detected",
			err:            wrappedError{&mysql.MySQLError{Number: 1045, Message: "Access denied"}},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeAuthError,
			wantMsg:        "Access denied",
			wantDetailsNil: false,
			wantMysqlCode:  1045,
			wantSQL:        "SELECT 1",
		},
		{
			name:           "Wrapped context.DeadlineExceeded is detected",
			err:            wrappedError{context.DeadlineExceeded},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "operation timed out",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "SELECT 1",
		},
		{
			name:           "Wrapped permission denied error is detected",
			err:            wrappedError{&mysql.MySQLError{Number: 1044, Message: "Access denied"}},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodePermissionDenied,
			wantMsg:        "Access denied",
			wantDetailsNil: false,
			wantMysqlCode:  1044,
			wantSQL:        "SELECT 1",
		},
		{
			name:           "Wrapped timeout error 3024 is detected",
			err:            wrappedError{&mysql.MySQLError{Number: 3024, Message: "Query interrupted"}},
			sql:            "SELECT 1",
			wantCode:       output.ErrorCodeTimeout,
			wantMsg:        "Query interrupted",
			wantDetailsNil: false,
			wantNoMysqlCode: true,
			wantSQL:        "SELECT 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCode, gotMsg, gotDetails := ClassifyError(tt.err, tt.sql)

			if gotCode != tt.wantCode {
				t.Errorf("ClassifyError() code = %v, want %v", gotCode, tt.wantCode)
			}

			if gotMsg != tt.wantMsg {
				t.Errorf("ClassifyError() msg = %v, want %v", gotMsg, tt.wantMsg)
			}

			if tt.wantDetailsNil {
				if gotDetails != nil && len(gotDetails) > 0 {
					t.Errorf("ClassifyError() details = %v, want nil/empty", gotDetails)
				}
			} else {
				if gotDetails == nil {
					t.Errorf("ClassifyError() details is nil, want non-nil")
					return
				}

				// Check mysql_error_code presence/absence
				if tt.wantNoMysqlCode {
					if _, ok := gotDetails["mysql_error_code"]; ok {
						t.Errorf("ClassifyError() details should not contain mysql_error_code for TIMEOUT errors")
					}
				} else {
					mysqlCode, ok := gotDetails["mysql_error_code"]
					if !ok {
						t.Errorf("ClassifyError() details missing mysql_error_code")
					} else if mysqlCode.(uint16) != tt.wantMysqlCode {
						t.Errorf("ClassifyError() mysql_error_code = %v, want %v", mysqlCode, tt.wantMysqlCode)
					}
				}

				// Check sql field
				if tt.wantSQL != "" {
					sql, ok := gotDetails["sql"]
					if !ok {
						t.Errorf("ClassifyError() details missing sql")
					} else if sql.(string) != tt.wantSQL {
						t.Errorf("ClassifyError() sql = %v, want %v", sql, tt.wantSQL)
					}
				} else {
					if _, ok := gotDetails["sql"]; ok && tt.wantSQL == "" {
						// sql field should not be present if empty
						t.Errorf("ClassifyError() details should not contain sql when empty")
					}
				}
			}
		})
	}
}

// wrappedError simulates an error wrapped by another type
type wrappedError struct {
	cause error
}

func (w wrappedError) Error() string {
	return w.cause.Error()
}

func (w wrappedError) Unwrap() error {
	return w.cause
}