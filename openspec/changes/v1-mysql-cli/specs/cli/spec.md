## ADDED Requirements

### Requirement: Subcommand dispatch
The CLI MUST dispatch on the first positional argument to one of `databases`, `tables`, `describe`, or `query`. An unknown subcommand MUST exit with code 2 and emit a `CONFIG_ERROR` envelope explaining the unknown subcommand name.

#### Scenario: Known subcommand runs
- **WHEN** the user invokes `sql-cli databases`
- **THEN** the `databases` handler runs and emits a success envelope

#### Scenario: Unknown subcommand
- **WHEN** the user invokes `sql-cli frobnicate`
- **THEN** the CLI exits 2 and stdout contains `{"ok":false,"error":{"code":"CONFIG_ERROR","message":"..."}}`. The envelope's `error` object contains only `code` and `message`; `details` is absent and the unknown subcommand name is not echoed (the caller already knows what they typed).

#### Scenario: Missing subcommand
- **WHEN** the user invokes `sql-cli` with no positional argument
- **THEN** the CLI exits 2 and emits a `CONFIG_ERROR` envelope stating that a subcommand is required

### Requirement: Flag parsing
The CLI MUST accept `--dsn <value>` as a global flag usable before the subcommand. The flag MUST override the env var. The CLI MUST accept `--help` and `--version` and exit 0 without contacting a database. The `query` subcommand MUST accept `--stdin` to read the SQL body from stdin.

#### Scenario: --dsn overrides env var
- **WHEN** `SQL_CLI_DSN` is set to `A` and the user passes `--dsn=B`
- **THEN** connection is attempted with `B`

#### Scenario: --help
- **WHEN** the user invokes `sql-cli --help`
- **THEN** the CLI prints usage text to stdout and exits 0

#### Scenario: --version
- **WHEN** the user invokes `sql-cli --version`
- **THEN** the CLI prints the version string to stdout and exits 0

#### Scenario: --stdin for query
- **WHEN** the user invokes `sql-cli --dsn=... query --stdin` and pipes SQL on stdin
- **THEN** the `query` subcommand reads SQL from stdin and runs the safety scan + execute path

### Requirement: Exit code mapping
The CLI MUST map error codes to the fixed exit codes: 0 success, 1 `QUERY_ERROR`, 2 `CONFIG_ERROR`, 3 `CONNECTION_ERROR`, 4 `AUTH_ERROR`, 5 `SAFETY_BLOCKED`, 6 `TIMEOUT`, 7 `PERMISSION_DENIED`, 99 `INTERNAL_ERROR`. The mapping is one-to-one.

#### Scenario: Success exits 0
- **WHEN** a query completes successfully
- **THEN** the process exit code is 0

#### Scenario: Safety block exits 5
- **WHEN** a write keyword is detected
- **THEN** the process exit code is 5

#### Scenario: Connection failure exits 3
- **WHEN** the MySQL server is unreachable
- **THEN** the process exit code is 3

#### Scenario: Auth failure exits 4
- **WHEN** MySQL returns error 1045 (access denied)
- **THEN** the process exit code is 4

#### Scenario: Permission denied exits 7
- **WHEN** MySQL returns error 1142 (no permission on a table) or 1044 (no permission on a database)
- **THEN** the process exit code is 7

### Requirement: Stdin mode for query
The `query` subcommand MUST accept a single `-` as its positional argument, OR the flag `--stdin`, and in either case MUST read the SQL body from stdin until EOF. The behaviour is otherwise identical to passing the SQL as a positional argument. The CLI applies two separate timeouts, both 30 seconds:

1. **Stdin read timeout**: if the stdin pipe delivers no data within 30 seconds, the CLI surfaces a `TIMEOUT` envelope and exits 6.
2. **Query execution timeout**: after the SQL has been received from stdin (or from a positional argument), the subsequent MySQL execution is bounded by the driver's `MAX_EXECUTION_TIME` (also 30 seconds). If exceeded, the CLI surfaces a `TIMEOUT` envelope and exits 6.

Either timeout produces the same envelope and exit code; the failure-envelope `details` MUST NOT distinguish them.

#### Scenario: Read SQL from stdin via dash
- **WHEN** the user invokes `sql-cli query -` and pipes SQL on stdin
- **THEN** the subcommand reads SQL from stdin, runs the safety scan, and emits the same envelope as a positional SQL argument

#### Scenario: Stdin read times out
- **WHEN** the user invokes `sql-cli query -` and the stdin pipe stays open with no data for 30 seconds
- **THEN** the CLI exits 6 with `TIMEOUT`

### Requirement: No non-JSON output on stdout
The CLI MUST NOT print anything other than the JSON envelope to stdout. Progress, warnings, deprecation notices, and debug output MUST go to stderr. A test MUST assert that nothing is written to stdout outside the envelope writer. The exceptions are `sql-cli --help` and `sql-cli --version`, which print plain text and exit 0.

#### Scenario: Stdout isolation
- **WHEN** any subcommand runs against a normal input
- **THEN** the only bytes written to stdout are the single JSON envelope object

#### Scenario: --help bypasses JSON envelope
- **WHEN** the user invokes `sql-cli --help`
- **THEN** stdout contains plain-text usage and exit code is 0; no JSON envelope is emitted
