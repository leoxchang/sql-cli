## ADDED Requirements

### Requirement: `databases` subcommand
The `databases` subcommand MUST execute `SHOW DATABASES` and emit a success envelope whose `columns` array is `[{"name":"Database","type":"varchar"}]` and whose `rows` contains one entry per database name as a single-element array. The CLI MUST connect using the resolved DSN before issuing the query.

#### Scenario: Lists databases
- **WHEN** the MySQL server has databases `app`, `mysql`, `sys`
- **THEN** the response `rows` contains `[["app"],["mysql"],["sys"]]` (or in server-returned order) and `row_count` matches the number returned

### Requirement: `tables <db>` subcommand
The `tables` subcommand MUST accept a database name as an optional positional argument. When omitted, the subcommand MUST use the database segment of the resolved DSN URL (`mysql://.../<db>`) as the target database. The target database — whether supplied as a positional argument or extracted from the DSN — MUST match the regex `^[A-Za-z0-9_]+$`; any other input is a `CONFIG_ERROR` (exit 2) and MUST NOT reach the database. If no positional argument is given AND the DSN URL has no database path, the subcommand MUST emit `CONFIG_ERROR` (exit 2) with a message naming both fallback sources. On valid input the subcommand MUST execute a parameterised query against `INFORMATION_SCHEMA.TABLES` selecting `TABLE_NAME` and `TABLE_COMMENT` filtered by `TABLE_SCHEMA = ?`, and emit a success envelope whose `columns` array is `[{"name":"Tables_in_<db>","type":"VARCHAR"}, {"name":"Table_comment","type":"VARCHAR"}]` and whose `rows` contains one `[name, comment]` pair per table. A database that does not exist MUST surface as `QUERY_ERROR` (server-side error) with the underlying `mysql_error_code`. Tables without a `COMMENT` clause MUST appear with an empty string in the `Table_comment` column.

#### Scenario: Lists tables with comments
- **WHEN** the database `app` contains tables `users` (with `COMMENT='用户表'`) and `orders` (no comment)
- **THEN** the response `rows` contains `[["orders", ""], ["users", "用户表"]]` (sorted by table name) and `row_count` is 2, with both `Tables_in_app` and `Table_comment` columns present

#### Scenario: Lists tables
- **WHEN** the database `app` contains tables `users` and `orders`
- **THEN** the response `rows` contains `[["orders"],["users"]]` (or in server-returned order) and `row_count` is 2

#### Scenario: Missing argument falls back to DSN database
- **WHEN** the user invokes `sql-cli --dsn mysql://u:p@h:3306/app tables` with no positional argument
- **THEN** the subcommand uses `app` as the target database, runs `SHOW TABLES FROM \`app\``, and returns a success envelope identical to passing `app` explicitly

#### Scenario: Missing argument and DSN without database path
- **WHEN** the user invokes `sql-cli --dsn mysql://u:p@h:3306/ tables` (no positional, no DSN path)
- **THEN** the CLI exits 2 with `CONFIG_ERROR` whose message names both fallback sources (positional argument and DSN URL path)

#### Scenario: Missing argument
- **WHEN** the user invokes `sql-cli tables` with no argument
- **THEN** the CLI exits 2 with `CONFIG_ERROR` indicating the database name is required

#### Scenario: Argument with disallowed character
- **WHEN** the user invokes `sql-cli tables "app; DROP TABLE x"`
- **THEN** the CLI exits 2 with `CONFIG_ERROR` indicating the database name must match `[A-Za-z0-9_]+` and no query is sent. The envelope's `error` object contains only `code` and `message`; `details` is absent and the rejected input is not echoed in the envelope.

### Requirement: `describe <db>.<table>` subcommand
The `describe` subcommand MUST accept one positional argument in the form `<db>.<table>` matching the regex `^[A-Za-z0-9_]+\.[A-Za-z0-9_]+$` (exactly one dot, no other special characters). It MUST execute `` DESCRIBE `<db>`.`<table>` `` (each identifier backtick-wrapped) and emit a success envelope whose `columns` are `Field`, `Type`, `Null`, `Key`, `Default`, `Extra` and whose `rows` is one entry per column of the table. An argument that does not match the regex MUST emit `CONFIG_ERROR` (exit 2) and MUST NOT reach the database. The subcommand MUST also be invokable as `desc`; `desc` is a strict alias with no behavioural difference.

#### Scenario: Describe a table
- **WHEN** the user invokes `sql-cli describe app.users`
- **THEN** the response describes each column of `app.users` in MySQL's `DESCRIBE` order

#### Scenario: Describe via the `desc` alias
- **WHEN** the user invokes `sql-cli desc app.users`
- **THEN** the response is identical to `sql-cli describe app.users`

#### Scenario: Argument without dot
- **WHEN** the user invokes `sql-cli describe users`
- **THEN** the CLI exits 2 with `CONFIG_ERROR` indicating the `<db>.<table>` form is required

#### Scenario: Argument with disallowed character
- **WHEN** the user invokes `sql-cli describe "app.users; DROP TABLE x"`
- **THEN** the CLI exits 2 with `CONFIG_ERROR` and no query is sent. The envelope's `error` object contains only `code` and `message`; `details` is absent and the rejected input is not echoed in the envelope.

#### Scenario: Multiple dots
- **WHEN** the user invokes `sql-cli describe "a.b.c"`
- **THEN** the CLI exits 2 with `CONFIG_ERROR` indicating exactly one dot is allowed

### Requirement: `query "<sql>"` subcommand
The `query` subcommand MUST accept either exactly one positional argument containing the SQL text, OR `-` (a single dash) as the only positional argument, OR the `--stdin` flag. When the dash or `--stdin` form is used, the subcommand MUST read the SQL body from stdin until EOF (with a 30-second timeout). The subcommand MUST first run the safety scanner; if any write keyword is detected it MUST emit `SAFETY_BLOCKED` and exit 5 without contacting the database. Otherwise it MUST execute the SQL and emit a success envelope describing the result set.

#### Scenario: Read-only query succeeds
- **WHEN** the user invokes `sql-cli query "SELECT 1 AS one"` with a valid DSN
- **THEN** the response is `{"ok":true,"columns":[{"name":"one","type":"BIGINT"}],"rows":[[1]],"row_count":1,"elapsed_ms":<n>}`

#### Scenario: Write query is blocked
- **WHEN** the user invokes `sql-cli query "UPDATE users SET x=1"`
- **THEN** exit code is 5, `code` is `SAFETY_BLOCKED`, and no query reaches the server

#### Scenario: SQL syntax error
- **WHEN** the user invokes `sql-cli query "SELEC * FORM t"`
- **THEN** exit code is 1, `code` is `QUERY_ERROR`, and `mysql_error_code` is 1064

#### Scenario: Empty SQL
- **WHEN** the user invokes `sql-cli query ""`
- **THEN** the CLI exits 2 with `CONFIG_ERROR` indicating the SQL must be non-empty

#### Scenario: Query from stdin heredoc
- **WHEN** the user invokes `sql-cli query -` and pipes a multi-line SELECT on stdin
- **THEN** the subcommand reads stdin, runs the safety scan, and emits the success envelope
