## 1. Module bootstrap

- [ ] 1.1 Initialize `go.mod` (Go 1.22+) and create the directory layout under `cmd/sql-cli/` and `internal/`
- [ ] 1.2 Add `github.com/go-sql-driver/mysql` as a direct dependency
- [ ] 1.3 Add `github.com/testcontainers/testcontainers-go/modules/mysql` as a test-only dependency
- [ ] 1.4 Set up a build tag `//go:build integration` for integration tests

## 2. Output envelope

- [ ] 2.1 Define `ErrorCode` constants and the `Envelope`/`Error` Go structs in `internal/output/`
- [ ] 2.2 Implement `WriteSuccess(w, columns, rows, rowCount, elapsedMs)` and `WriteFailure(w, code, message, opts)` where `opts` carries `details`
- [ ] 2.3 Implement row-to-JSON conversion handling `nil` → `null`, `[]byte` → string, `time.Time` → RFC3339, numeric types as numbers; preserve `Rows.ColumnTypes().DatabaseTypeName()` case verbatim
- [ ] 2.4 Unit tests covering all MySQL column types we expect to encounter (including the case-preservation test)

## 3. Configuration

- [ ] 3.1 Implement DSN resolution in `internal/config/`: flag → env → `CONFIG_ERROR`
- [ ] 3.2 Parse `mysql://` URL, validate scheme is `mysql`, host is non-empty, port segment is present, and that the DSN contains no `\n` or `\r`
- [ ] 3.3 Unit tests for precedence, missing source, wrong scheme, missing host, missing port, control-character rejection

## 4. Safety scanner

- [ ] 4.1 Implement the state machine in `internal/safety/` (states: normal, in-string, in-line-comment, in-block-comment)
- [ ] 4.2 Implement word-boundary keyword detection against the 11-keyword set, normalising detected keywords to canonical uppercase
- [ ] 4.3 Unit tests with table-driven cases: each keyword, each skip case (string literal, line comment, block comment), lowercase, mixed case, multiple keywords, false-positive column names
- [ ] 4.4 Add a "bypass attempt" test file at `internal/safety/testdata/bypass.sql` covering `IN/**/SERT`, `DR/**/OP`, escaped quote, etc.

## 5. MySQL driver layer

- [ ] 5.1 Implement `internal/mysqldrv/` `Open(ctx, dsn)` registering the driver once and pinging
- [ ] 5.2 Implement `mysqlURLToDriverDSN(url)` translation preserving query parameters; require the port segment
- [ ] 5.3 Implement `ClassifyError(err, sql) (ErrorCode, string, map[string]any)` mapping MySQL error numbers 1045, 1044, 1142, 1146, 1064, 3024 and `context.DeadlineExceeded`; populate `mysql_error_code` and `sql` in details. For `INTERNAL_ERROR` the function MUST return a nil/empty `details` map — the output layer drops any details on that path per the failure-envelope schema.
- [ ] 5.4 Unit tests for the classifier with table-driven cases using fake errors that expose the relevant `*mysql.MySQLError` fields

## 6. CLI dispatch and subcommands

- [ ] 6.1 Implement `internal/cli/router.go` parsing the global `--dsn` flag and dispatching by subcommand
- [ ] 6.2 Implement `--help` and `--version` handlers (no DB connection, plain text on stdout)
- [ ] 6.3 Implement `internal/command/databases.go` calling `SHOW DATABASES`
- [ ] 6.4 Implement `internal/command/tables.go` validating the argument against `^[A-Za-z0-9_]+$` and calling `` SHOW TABLES FROM `<db>` ``
- [ ] 6.5 Implement `internal/command/describe.go` validating the argument against `^[A-Za-z0-9_]+\.[A-Za-z0-9_]+$` and calling `` DESCRIBE `<db>`.`<table>` ``
- [ ] 6.6 Implement `internal/command/query.go` running the safety scanner first, then executing the SQL; support `-` and `--stdin` to read SQL from stdin. Apply two separate 30-second timeouts: (a) on the stdin read itself (`TIMEOUT` if the pipe delivers no data), (b) on the MySQL execution via `MAX_EXECUTION_TIME` (`TIMEOUT` if the query runs too long). Both produce the same envelope + exit 6; do not distinguish them in `details`.
- [ ] 6.7 Wire the one-to-one error-code-to-exit-code mapping in a single helper: 0/1/2/3/4/5/6/7/99
- [ ] 6.8 Add a stdout-isolation test that asserts the only bytes written to stdout outside `--help`/`--version` paths are the single JSON envelope

## 7. Main entry

- [ ] 7.1 Implement `cmd/sql-cli/main.go` that builds the router, runs it, and exits with the returned code
- [ ] 7.2 Verify that no progress text or warnings reach stdout (stderr only) on all subcommands

## 8. Integration tests

- [ ] 8.1 Add a `testcontainers-helper.go` (build-tagged `integration`) that returns a running MySQL 8 container and a teardown function
- [ ] 8.2 Integration test: `databases` lists the seeded test database
- [ ] 8.3 Integration test: `tables` lists seeded tables; missing arg is `CONFIG_ERROR`; `app; DROP` arg is `CONFIG_ERROR`; non-existent DB is `QUERY_ERROR` with `mysql_error_code`
- [ ] 8.4 Integration test: `describe` returns the right column metadata; missing dot is `CONFIG_ERROR`; multi-dot is `CONFIG_ERROR`; `app.users; DROP` is `CONFIG_ERROR`
- [ ] 8.5 Integration test: `query` runs `SELECT 1`; `query` is blocked for `UPDATE`; `query` surfaces syntax error as `QUERY_ERROR` 1064; empty SQL is `CONFIG_ERROR`; `query -` reads from stdin
- [ ] 8.6 Integration test: bad credentials exit 4 with `AUTH_ERROR`; permission denied on a table exits 7 with `PERMISSION_DENIED`; bad host exits 3 with `CONNECTION_ERROR`
- [ ] 8.7 Integration test: end-to-end CLI invocation through `os/exec` to confirm stdout/stderr separation, exit codes (0–7, 99), and that `elapsed_ms` is a non-negative integer covering connection setup

## 9. README and polish

- [ ] 9.1 Write `README.md` with: install, usage examples for each subcommand (including `query -` and `--stdin`), DSN format, env var, the read-only-account recommendation, the 9-code error table with exit codes, and the explicit "out of scope" list
- [ ] 9.2 Verify `go test ./...` (unit) and `go test -tags=integration ./...` (with Docker) both pass
- [ ] 9.3 Run `go test -cover ./...` and confirm ≥ 80% on `internal/config`, `internal/safety`, `internal/output`, `internal/mysqldrv` only (cli/command are not gated)
