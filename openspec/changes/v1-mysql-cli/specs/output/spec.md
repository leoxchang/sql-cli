## ADDED Requirements

### Requirement: Success envelope
On successful command execution, the output layer MUST emit a single JSON object to stdout with keys `ok`, `columns`, `rows`, `row_count`, and `elapsed_ms`. The value of `ok` MUST be `true`. No `error` key is present.

#### Scenario: Query success
- **WHEN** `SELECT id, name FROM users` returns two rows
- **THEN** stdout contains `{"ok":true,"columns":[{"name":"id",...},{"name":"name",...}],"rows":[[1,"a"],[2,"b"]],"row_count":2,"elapsed_ms":<number>}`

#### Scenario: Zero-row query
- **WHEN** a query returns no rows
- **THEN** stdout contains `{"ok":true,"columns":[...],"rows":[],"row_count":0,"elapsed_ms":<number>}`

### Requirement: Failure envelope
On any failure, the output layer MUST emit a single JSON object to stdout with keys `ok` and `error`. The value of `ok` MUST be `false`. The `error` object MUST contain `code` (one of the nine codes) and `message`. Optional fields `sql`, `mysql_error_code`, and `details` MAY be present when applicable. No data fields are present.

The `message` field is human-facing prose. It is NOT a contract. Agent implementations MUST NOT parse or branch on the contents of `message`; the only machine-readable fields are `code`, `mysql_error_code`, and the keys of `details` (when present). Wording MAY change between releases without a version bump. The `details` object, when present, is a `map[string]any` whose keys are constrained by code:

- For `CONFIG_ERROR`, `details` is omitted entirely. The `message` field is the only machine-distinguishable signal beyond the `code`; rejected input values are not echoed back (defence against log-injection from control characters or hostile arguments).
- For `AUTH_ERROR`, `PERMISSION_DENIED`, `QUERY_ERROR`, `TIMEOUT`, and `CONNECTION_ERROR`, `details` MUST contain `mysql_error_code` (a `uint16`) and MAY contain `sql` (the SQL string the caller passed in, for echo in the envelope). No other keys are allowed.
- For `SAFETY_BLOCKED`, `details` MUST contain `keyword` (the canonical-uppercase detected keyword as a string) and MAY contain `sql`. No other keys are allowed.
- For `INTERNAL_ERROR`, `details` MUST be absent.

Any other key in `details` is a v1.1+ change and MUST NOT be emitted in v1.0.

#### Scenario: Query error envelope
- **WHEN** the database returns MySQL error 1146 for a missing table
- **THEN** stdout contains `{"ok":false,"error":{"code":"QUERY_ERROR","message":"...","mysql_error_code":1146,"sql":"<the sql>"}}`

#### Scenario: Safety block envelope
- **WHEN** a write keyword is detected
- **THEN** stdout contains `{"ok":false,"error":{"code":"SAFETY_BLOCKED","message":"<prose>","details":{"keyword":"<KEYWORD>","sql":"<the sql>"}}}`. The `keyword` value is the canonical-uppercase detected keyword; `sql` is the offending SQL string the caller passed in.

#### Scenario: Permission denied envelope
- **WHEN** the server returns error 1142
- **THEN** stdout contains `{"ok":false,"error":{"code":"PERMISSION_DENIED","message":"...","mysql_error_code":1142}}`

### Requirement: Column type representation
The `columns` array MUST contain one object per result column with keys `name` and `type`. `name` is the SQL identifier; `type` is the database-reported type name emitted exactly as returned by `Rows.ColumnTypes().DatabaseTypeName()`. The output layer MUST NOT transform the case.

#### Scenario: Column metadata present
- **WHEN** a query returns columns
- **THEN** each entry in `columns` has a non-empty `name` and `type` string

#### Scenario: Type case preserved (uppercase source)
- **WHEN** the database reports a column type as `BIGINT`
- **THEN** the envelope contains `"type":"BIGINT"`

#### Scenario: Type case preserved (lowercase source)
- **WHEN** the database reports a column type as `varchar`
- **THEN** the envelope contains `"type":"varchar"`

### Requirement: Elapsed time capture
The output layer MUST set `elapsed_ms` to the wall-clock duration in whole milliseconds from the subcommand handler entry to the last row read from the result set. The definition is identical for `query` and for the metadata subcommands. `elapsed_ms` MUST be a non-negative integer. Connection setup time is included in the value.

#### Scenario: Elapsed is non-negative integer
- **WHEN** any successful subcommand runs
- **THEN** `elapsed_ms` is an integer ≥ 0

#### Scenario: Elapsed covers connection setup
- **WHEN** a `query` subcommand runs against a fresh connection
- **THEN** `elapsed_ms` is greater than or equal to the time spent in `sql.Open` plus `PingContext`

### Requirement: Output goes to stdout
The output layer MUST write the JSON envelope to stdout. Progress, warnings, and deprecation notices MUST go to stderr. Nothing else may be printed to stdout (except by `sql-cli --help` and `sql-cli --version`, which print plain text and exit 0).

#### Scenario: stderr for warnings
- **WHEN** a query is run with a future-warning condition
- **THEN** the warning text appears on stderr and the JSON envelope appears on stdout

### Requirement: `row_count` semantics
`row_count` in the success envelope MUST equal the number of rows in the result set returned by the query (i.e., the count the agent sees in `rows`). It MUST NOT be the MySQL "rows affected" count. In v1.0 the safety scanner blocks all write statements, so `row_count` for `INSERT`/`UPDATE`/`DELETE` is not defined.

#### Scenario: row_count equals len(rows)
- **WHEN** a `SELECT` returns 5 rows
- **THEN** `row_count` is 5 and `rows` is a JSON array of length 5
