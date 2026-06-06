## Why

AI agents need a stable, predictable way to query MySQL from the shell. They parse stdout, branch on exit codes, and have no patience for ambiguous error messages. The existing `mysql` CLI is built for humans, not agents, and the AI-integration shape (MCP servers, ORMs) is too heavy for one-shot bash calls.

`sql-cli` is a small, single-binary CLI purpose-built for this: JSON output with a uniform envelope, structured error codes, and a default read-only posture. v1.0 ships MySQL only and reserves space for other databases without paying the abstraction cost upfront.

## What Changes

- **New CLI binary** `sql-cli` with four subcommands: `databases`, `tables <db>`, `describe <db>.<table>`, `query "<sql>"`.
- **Connection sources** in priority order: `--dsn` flag, then `SQL_CLI_DSN` environment variable. DSN uses `mysql://` URL form.
- **Uniform JSON output envelope** on stdout: `{ok, columns, rows, row_count, elapsed_ms}` on success, `{ok: false, error: {code, message, ...}}` on failure. Nine machine-readable error codes mapped to specific exit codes.
- **Read-only protection** via a lightweight SQL parser that detects write keywords (`INSERT | UPDATE | DELETE | DROP | CREATE | ALTER | TRUNCATE | GRANT | REVOKE | RENAME | REPLACE`) while skipping string literals and comments. The detected keyword is returned in canonical uppercase form regardless of input case. README documents the recommendation to connect with a read-only MySQL account.
- **Metadata subcommand argument whitelist**: `tables` and `describe` validate their argument against `[A-Za-z0-9_]+` (with the dot separator accepted only in `describe`); anything else is `CONFIG_ERROR`. Identifiers are backtick-wrapped when building the SQL.
- **`query` reads from stdin** when the only positional is `-` or when `--stdin` is passed; stdin read times out after 30 seconds.
- **Package layout** that isolates MySQL-specific code under `internal/mysqldrv/`. No `Driver` interface in v1.0; the package boundary is the extension point.
- **Test suite** with `testcontainers-go` spinning up real MySQL for integration tests. 80% coverage target on `internal/{config,safety,output,mysqldrv}`; cli/command are not gated.
- **Out of scope for v1.0** (explicit non-goals): `--allow-write`, transactions, `tx` subcommand, YAML config, table/csv/tsv output, `explain`/`analyze`/`status`, MCP server mode, other database drivers, plugin system, parameter binding, `--timeout` flag. These are deferred to v1.1+.

## Capabilities

### New Capabilities

- `cli`: Command-line surface — flag parsing, subcommand dispatch, exit code mapping, `--help`/`--version` behavior, stdin mode for `query`, stdout isolation.
- `config`: Configuration resolution — precedence between `--dsn` flag and `SQL_CLI_DSN`, DSN URL validation, control-character rejection.
- `output`: JSON envelope — success and failure response shapes, column type representation (server-reported case), uniform elapsed-time capture, error code constants.
- `safety`: Read-only SQL guard — keyword detection (case-insensitive input, canonical uppercase output), string-literal and comment skipping, bypass-test coverage.
- `mysqldrv`: MySQL connection and execution — DSN translation, `database/sql` driver registration, query execution, error-to-code-and-details mapping.
- `query-commands`: Four subcommand implementations — `databases`, `tables`, `describe`, `query`, all producing the uniform envelope. Metadata subcommands whitelist and backtick-wrap their arguments.

### Modified Capabilities

None. There are no pre-existing specs.

## Impact

- **New Go module** at the repo root (`go.mod`), Go 1.22+.
- **New dependency**: `github.com/go-sql-driver/mysql` (driver) and `github.com/testcontainers/testcontainers-go/modules/mysql` (integration tests only).
- **No external services required at runtime**; MySQL is only required for the integration test suite and for end users.
- **No breaking changes for users** — the project is empty; this is a clean addition.
- **Internal contract change for agents** from the original draft: `PERMISSION_DENIED` moves from exit 4 to exit 7 so the two error codes are distinguishable by shell wrappers. Agents branching on the JSON envelope `code` field are unaffected.
- **README** must include a "use a read-only MySQL account" recommendation so users layer defense in depth on top of the keyword filter and the whitelist.
