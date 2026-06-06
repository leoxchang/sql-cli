## ADDED Requirements

### Requirement: MySQL driver registration
The `mysqldrv` package MUST register the `github.com/go-sql-driver/mysql` driver exactly once at package init time. No other package in the binary MAY import that driver directly.

#### Scenario: Single import of driver
- **WHEN** the binary is compiled
- **THEN** only `internal/mysqldrv` imports `github.com/go-sql-driver/mysql`

### Requirement: DSN translation
The package MUST accept a `mysql://` URL and translate it to the driver-native DSN form expected by `go-sql-driver/mysql`. Query parameters from the URL MUST be preserved in the translated DSN. The port segment of the URL is required; an input without a port MUST be rejected with `CONFIG_ERROR`.

#### Scenario: URL to driver DSN
- **WHEN** the input is `mysql://u:p@host:3306/db?parseTime=true&loc=Local`
- **THEN** the translated DSN is `u:p@tcp(host:3306)/db?parseTime=true&loc=Local`

#### Scenario: URL with no port segment rejected
- **WHEN** the input is `mysql://u:p@host/db` (the port segment is absent)
- **THEN** translation returns a `CONFIG_ERROR` indicating the port is required

### Requirement: Connection establishment
The package MUST expose `Open(ctx, dsn) (*sql.DB, error)`. The returned `*sql.DB` MUST be ready for `QueryContext` and `Close` without further configuration. The package MUST call `PingContext` to validate the connection before returning success.

#### Scenario: Open succeeds
- **WHEN** the DSN is valid and the server is reachable
- **THEN** `Open` returns a non-nil `*sql.DB` and no error

#### Scenario: Open fails on bad host
- **WHEN** the host is unreachable
- **THEN** `Open` returns a `CONNECTION_ERROR` mapped error

### Requirement: MySQL error classification
The package MUST classify MySQL driver errors into the nine error codes and return a `(code, message, details)` triple. `details` is a `map[string]any` whose only keys emitted in v1.0 are `mysql_error_code` (a `uint16` mirroring the MySQL error number) and `sql` (the SQL string the caller passed in, for echo in the envelope). Required mappings: error numbers 1045 (access denied) → `AUTH_ERROR`; 1044/1142 (no permission) → `PERMISSION_DENIED`; 1146 (no such table), 1064 (syntax error), and other client errors → `QUERY_ERROR`; 3024 (query execution interrupted) and context deadline exceeded → `TIMEOUT`. Network errors map to `CONNECTION_ERROR`. Anything unmapped becomes `INTERNAL_ERROR`.

#### Scenario: Access denied
- **WHEN** the server returns error 1045
- **THEN** the classified code is `AUTH_ERROR` and `details["mysql_error_code"]` is `uint16(1045)`

#### Scenario: Permission denied on table
- **WHEN** the server returns error 1142
- **THEN** the classified code is `PERMISSION_DENIED` and `details["mysql_error_code"]` is `uint16(1142)`

#### Scenario: Context deadline
- **WHEN** `ctx` deadline expires during query execution
- **THEN** the classified code is `TIMEOUT`

#### Scenario: Unknown error
- **WHEN** the error matches none of the above
- **THEN** the classified code is `INTERNAL_ERROR` and `details["mysql_error_code"]` is absent

#### Scenario: SQL echoed in details
- **WHEN** the classifier is called with the SQL string `SELECT 1` and the server returns any known MySQL error
- **THEN** `details["sql"]` equals `SELECT 1`
