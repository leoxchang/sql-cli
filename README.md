# sql-cli

A read-only MySQL command-line interface for AI agents. Emits structured JSON output with one-to-one error-code-to-exit-code mapping, includes a built-in safety scanner, and supports reading SQL from stdin.

## Installation

Build from source:

```bash
go build -o sql-cli ./cmd/sql-cli
```

Or install directly:

```bash
go install github.com/qiezi999/sql-cli/cmd/sql-cli@latest
```

## Usage Examples

### databases - List all databases

```bash
sql-cli --dsn mysql://user:pass@host:3306/ databases
```

Output:
```json
{
  "ok": true,
  "columns": [{"name": "Database", "type": "VARCHAR"}],
  "rows": [["information_schema"], ["mydb"], ["test"]],
  "row_count": 3,
  "elapsed_ms": 42
}
```

### tables - List tables in a database

```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb tables mydb
# database argument is optional when DSN URL already names the database:
sql-cli --dsn mysql://user:pass@host:3306/mydb tables
```

Output:
```json
{
  "ok": true,
  "columns": [
    {"name": "Tables_in_mydb", "type": "VARCHAR"},
    {"name": "Table_comment", "type": "VARCHAR"}
  ],
  "rows": [
    ["orders", ""],
    ["products", "商品表"],
    ["users", "用户表"]
  ],
  "row_count": 3,
  "elapsed_ms": 15
}
```

The `Table_comment` column mirrors MySQL's `TABLE_COMMENT` (set via `COMMENT='…'` in `CREATE TABLE`). Tables without a `COMMENT` clause have an empty string in this column.

### describe - Show table structure

```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb describe mydb.users
# or use the short alias:
sql-cli --dsn mysql://user:pass@host:3306/mydb desc mydb.users
```

Output:
```json
{
  "ok": true,
  "columns": [
    {"name": "Field", "type": "VARCHAR"},
    {"name": "Type", "type": "VARCHAR"},
    {"name": "Null", "type": "VARCHAR"},
    {"name": "Key", "type": "VARCHAR"},
    {"name": "Default", "type": "VARCHAR"},
    {"name": "Extra", "type": "VARCHAR"}
  ],
  "rows": [
    ["id", "bigint", "NO", "PRI", "NULL", "auto_increment"],
    ["name", "varchar(255)", "YES", "", "NULL", ""]
  ],
  "row_count": 2,
  "elapsed_ms": 8
}
```

### indexes - Show index metadata

```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb indexes mydb.users
# or use aliases:
sql-cli --dsn mysql://user:pass@host:3306/mydb idx mydb.users
sql-cli --dsn mysql://user:pass@host:3306/mydb keys mydb.users
```

Argument format matches `describe`: `<db>.<table>` or just `<table>` (database taken from DSN URL path).

Output columns: `INDEX_NAME`, `NON_UNIQUE`, `SEQ_IN_INDEX`, `COLUMN_NAME`, `COLLATION`, `CARDINALITY`, `SUB_PART`, `PACKED`, `NULLABLE`, `INDEX_TYPE`, `COMMENT`, `INDEX_COMMENT`, `IS_VISIBLE`, `EXPRESSION`

Compatible with MySQL 5.7, 8.0, and 8.0.13+ — the `IS_VISIBLE` and `EXPRESSION` columns are probed at runtime and faked as empty strings on versions that don't expose them.

### query - Execute SQL queries

#### Positional argument

```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb query "SELECT id, name FROM users LIMIT 10"
```

#### Read from stdin with `-`

```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb query - <<'EOF'
SELECT id, name, created_at
FROM users
WHERE created_at > NOW() - INTERVAL 7 DAY
ORDER BY created_at DESC
LIMIT 100
EOF
```

#### Read from stdin with `--stdin`

```bash
echo "SELECT COUNT(*) AS total_users FROM users" | \
  sql-cli --dsn mysql://user:pass@host:3306/mydb query --stdin
```

Output:
```json
{
  "ok": true,
  "columns": [{"name": "total_users", "type": "BIGINT"}],
  "rows": [[15042]],
  "row_count": 1,
  "elapsed_ms": 23
}
```

#### Database resolution

The `query` subcommand requires that the target database can be resolved. Exactly one of the following must hold:

1. The DSN contains a default database (e.g. `mysql://user:pass@host:3306/mydb`), OR
2. The SQL statement uses an explicit `db.table` qualifier (e.g. `SELECT * FROM mydb.users`)

If neither holds, MySQL returns `1046 No database selected` and sql-cli surfaces it as `QUERY_ERROR` (exit 1). The database name is **not** carried as a separate flag — it must be encoded in the DSN path or in the SQL itself.

Failure example (DSN with no path, unqualified SQL):

```bash
$ sql-cli --dsn mysql://root:pass@localhost:3306/ query "SELECT * FROM users"
```

```json
{
  "ok": false,
  "error": {
    "code": "QUERY_ERROR",
    "message": "Error 1046 (3D000): No database selected",
    "details": { "mysql_error_code": 1046 }
  }
}
```

## DSN Format

The tool accepts MySQL DSNs in URL format only:

```
mysql://user:password@host:port/database?param=value
```

**Requirements:**
- Scheme must be exactly `mysql` (not `mysqls`, `http`, etc.)
- Port segment is required (e.g., `:3306`)
- Host must be non-empty
- Database path is optional (useful for `databases` subcommand)
- No newline (`\n`) or carriage return (`\r`) characters allowed

**Examples:**
```bash
# With database
mysql://root:password@localhost:3306/mydb?parseTime=true

# Without database (for SHOW DATABASES)
mysql://root:password@localhost:3306/?parseTime=true

# With query parameters
mysql://user:pass@host:3306/db?parseTime=true&loc=Local&charset=utf8mb4
```

**Invalid DSNs:**
```bash
# Missing port
mysql://user:pass@host/db                    # ERROR

# Wrong scheme
postgres://user:pass@host:5432/db            # ERROR

# Contains newline (DSN smuggling attack)
mysql://user:pass@host:3306/db?x=y\nDROP     # ERROR
```

## Environment Variables

Set `SQL_CLI_DSN` to avoid passing `--dsn` every time:

```bash
export SQL_CLI_DSN="mysql://user:pass@host:3306/mydb?parseTime=true"

# Now run commands without --dsn
sql-cli databases
sql-cli tables mydb
sql-cli query "SELECT * FROM users LIMIT 10"
```

**Precedence:** `--dsn` flag > `SQL_CLI_DSN` environment variable > `--profile` from config file > error

## Configuration File

For agents that switch between dev / staging / prod connections, put the DSNs in a YAML file and select one with `--profile <name>`. The file shape is a single `dsns:` map:

```yaml
# ~/.sql-cli/config.yaml (global)
dsns:
  dev:     mysql://root:pass@127.0.0.1:3306/devdb
  staging: mysql://readonly:pw@db.staging.example.com:3306/?parseTime=true
  prod-ro: mysql://readonly:pw@db.prod.example.com:3306/proddb
```

```bash
sql-cli --profile staging databases
sql-cli --profile dev tables
```

**Lookup order (later overrides earlier by name, not whole-file):**

1. `$SQL_CLI_CONFIG_DIR/config.yaml` (if set)
2. `$XDG_CONFIG_HOME/sql-cli/config.yaml` (if set)
3. `~/.sql-cli/config.yaml` (default)
4. `.sql-cli.yaml` in the current working directory, walking upward to the filesystem root

A project-local `.sql-cli.yaml` typically contains only the profile names specific to that repo (e.g. `local`, `ci`), so a developer's personal global profiles are not duplicated per project.

**`--dsn` and `--profile` are mutually exclusive.** If both are passed, the binary emits `CONFIG_ERROR` (exit 2). This is intentional: silent precedence between an explicit DSN and a named profile is the kind of "surprising the agent" behaviour this tool rejects.

**Risks:** the file is read in plaintext; anyone with read access to the file can see the password. The same risk exists for `--dsn` and `SQL_CLI_DSN` (visible in `ps` and process listings), so the config file is not strictly worse — but it does centralise the secrets in a single location worth filesystem-permissioning (e.g. `chmod 600 ~/.sql-cli/config.yaml`).

### config add — Save a DSN profile

```bash
# Add to local config (./.sql-cli.yaml)
sql-cli config add dev mysql://user:pass@localhost:3306/devdb

# Add to global config (~/.sql-cli/config.yaml)
sql-cli config add --global prod mysql://user:pass@prod:3306/proddb
```

Profile names must match `^[A-Za-z0-9_][A-Za-z0-9_.-]*$` — first char must be a letter, digit, or underscore; subsequent chars may also include `.` and `-`. Names starting with `-` or `.` are rejected.

If a profile with the same name already exists, it is overwritten silently. If the value differs, a warning is printed to stderr:

```
replacing dev: mysql://user:old@localhost:3306/devdb → mysql://user:pass@localhost:3306/devdb
```

Successful output:
```json
{
  "ok": true,
  "profile": "dev",
  "dsn": "mysql://user:****@localhost:3306/devdb",
  "path": "/home/user/project/.sql-cli.yaml",
  "action": "created",
  "elapsed_ms": 3
}
```

The `path` field shows the absolute file path. New config files are created with `0600` permissions (owner read/write only). Existing files preserve their permissions.

### config list — List saved profiles

```bash
# List all profiles (local + global, local overrides global)
sql-cli config list

# List only local profiles
sql-cli config list --local

# List only global profiles
sql-cli config list --global
```

Output:
```json
{
  "ok": true,
  "profiles": [
    {"name": "dev", "dsn": "mysql://user:****@localhost:3306/devdb", "source": "/home/user/project/.sql-cli.yaml"},
    {"name": "prod", "dsn": "mysql://user:****@prod:3306/proddb", "source": "/home/user/.sql-cli/config.yaml"}
  ],
  "count": 2,
  "elapsed_ms": 2
}
```

Profiles are listed in alphabetical order by name. DSNs are masked in output (password replaced with `****`). The `source` field shows which file each profile came from. Local profiles override global ones with the same name.

### config remove — Delete a saved profile

```bash
# Remove from the local config (default).
sql-cli config remove dev

# Remove from the global config.
sql-cli config remove --global prod
```

Exit code 0 either way. If the profile does not exist, the command still succeeds and returns `action: "not_found"` (idempotent — safe to retry).

Output:
```json
{
  "ok": true,
  "action": "removed",
  "profile": "dev",
  "path": "/home/user/project/.sql-cli.yaml",
  "elapsed_ms": 1
}
```

### config rename — Rename a saved profile

```bash
# Rename in the local config (default).
sql-cli config rename dev production

# Rename in the global config.
sql-cli config rename --global prod production
```

If the new name already exists in the same scope, the command exits 2 with `CONFIG_ERROR` and the old profile is preserved unchanged. Renaming a profile to its current name (`config rename dev dev`) is a no-op that returns `action: "unchanged"` without rewriting the file.

Output:
```json
{
  "ok": true,
  "action": "renamed",
  "from": "dev",
  "to": "production",
  "path": "/home/user/project/.sql-cli.yaml",
  "elapsed_ms": 2
}
```

### Config file locations

- **Local**: `.sql-cli.yaml` in the current working directory
- **Global**: `~/.sql-cli/config.yaml` (or `$SQL_CLI_CONFIG_DIR/config.yaml`, `$XDG_CONFIG_HOME/sql-cli/config.yaml` if set)

### Implementation details

- Config files are written atomically (temp file + rename) to prevent corruption
- Parent directory (`~/.sql-cli/`) is created with `0700` permissions
- Profile keys are sorted alphabetically for deterministic output
- Comments in existing YAML files are not preserved (file is rewritten)


## Security Recommendations

### Use a Read-Only MySQL Account

**The safety scanner is defense-in-depth, not a complete solution.** Always connect with a MySQL account that has only `SELECT` privileges:

```sql
-- Create a restricted user for agent queries
CREATE USER 'agent_readonly'@'%' IDENTIFIED BY 'secure_password';

-- Grant SELECT only on specific databases
GRANT SELECT ON mydb.* TO 'agent_readonly'@'%';

-- Verify permissions
SHOW GRANTS FOR 'agent_readonly'@'%';
```

### Safety Scanner

The tool includes a SQL keyword scanner that blocks queries containing:
- `INSERT`, `UPDATE`, `DELETE`, `DROP`
- `CREATE`, `ALTER`, `TRUNCATE`, `REPLACE`
- `GRANT`, `REVOKE`, `LOCK`

The scanner handles:
- Case-insensitive matching (normalizes to uppercase)
- String literals (with `''` escape sequences)
- Line comments (`-- comment`)
- Block comments (`/* comment */`)

**Limitations:** The scanner can be bypassed with advanced SQL tricks. It's a safety net, not a security boundary. **Database permissions are the primary defense.**

Example blocked query:
```bash
sql-cli --dsn mysql://user:pass@host:3306/mydb query "DROP TABLE users"
```

Output:
```json
{
  "ok": false,
  "error": {
    "code": "SAFETY_BLOCKED",
    "message": "query blocked by safety scanner: detected write keyword(s): DROP",
    "details": {
      "keywords": ["DROP"],
      "sql": "DROP TABLE users"
    }
  }
}
```

Exit code: `5`

## Error Codes and Exit Codes

The tool uses 9 standardized error codes with one-to-one exit code mapping:

| Error Code          | Exit Code | Description                           |
|---------------------|-----------|---------------------------------------|
| Success             | 0         | Operation completed successfully      |
| QUERY_ERROR         | 1         | SQL syntax error or execution error   |
| CONFIG_ERROR        | 2         | Invalid DSN, missing arguments        |
| CONNECTION_ERROR    | 3         | Failed to connect to MySQL server     |
| AUTH_ERROR          | 4         | Authentication failed (error #1045)   |
| SAFETY_BLOCKED      | 5         | Write keyword detected by scanner     |
| TIMEOUT             | 6         | Operation exceeded 30-second limit    |
| PERMISSION_DENIED   | 7         | Insufficient privileges (error #1044, #1142) |
| INTERNAL_ERROR      | 99        | Unexpected internal failure           |

### Error Response Format

All errors emit a JSON envelope:

```json
{
  "ok": false,
  "error": {
    "code": "QUERY_ERROR",
    "message": "query error: Table 'mydb.users' doesn't exist",
    "details": {
      "mysql_error_code": 1146,
      "sql": "SELECT * FROM users"
    }
  }
}
```

**Important:**
- The `message` field is human-readable prose and may change. **Do not parse it.**
- Branch on `code`, `mysql_error_code`, and structured `details` keys only.
- `INTERNAL_ERROR` and `CONFIG_ERROR` may have `details: null` or omitted

### MySQL Error Code Mapping

| MySQL Error # | Error Code          | Example Cause                     |
|---------------|---------------------|-----------------------------------|
| 1045          | AUTH_ERROR          | Access denied for user           |
| 1044          | PERMISSION_DENIED   | Access denied for database       |
| 1142          | PERMISSION_DENIED   | SELECT command denied            |
| 1146          | QUERY_ERROR         | Table doesn't exist              |
| 1064          | QUERY_ERROR         | SQL syntax error                 |
| 3024          | TIMEOUT             | Query execution was interrupted  |

## Out of Scope (v1.0)

These features are **not** implemented and will not be added in v1.0:

- **Write operations**: No `--allow-write` flag, no `INSERT/UPDATE/DELETE` execution
- **Transactions**: No `BEGIN/COMMIT/ROLLBACK`, no `tx` subcommand
- **Timeout configuration**: Fixed 30-second timeout (no `--timeout` flag)
- **Parameter binding**: No prepared statements, no `?` placeholder support
- **Output formats**: JSON only (no CSV, TSV, table, YAML)
- **Additional commands**: No `explain`, `analyze`, `show status`, `show processlist`
- **Other databases**: MySQL only (no PostgreSQL, SQLite, MariaDB support)
- **MCP server mode**: No Model Context Protocol integration
- **Plugin system**: No driver plugins or extension mechanism

If you need these features, consider using the official `mysql` CLI or a full MySQL client library.

## Integration Tests

The project includes integration tests using testcontainers (requires Docker):

```bash
# Run unit tests only
go test ./...

# Run integration tests (requires Docker)
go test -tags=integration ./...

# Run integration tests without Docker
SKIP_DOCKER=1 go test -tags=integration ./...

# Run with coverage
go test -cover ./...
go test -tags=integration -cover ./...
```

**Coverage targets:**
- `internal/config`: ≥ 80%
- `internal/safety`: ≥ 80%
- `internal/output`: ≥ 80%
- `internal/mysqldrv`: ≥ 80%

## Development

```bash
# Build binary
go build -o sql-cli ./cmd/sql-cli

# Run tests
go test ./...

# Run with integration tests
go test -tags=integration ./...

# Verify build and lint
go vet ./...
go build ./...

# Show help
./sql-cli --help

# Show version
./sql-cli --version
```

## Architecture

```
cmd/sql-cli/main.go          # Entry point, flag parsing, DB connection
internal/cli/router.go       # Subcommand dispatch, exit code mapping
internal/command/            # Subcommand handlers (databases, tables, describe, query)
internal/config/             # DSN resolution and validation
internal/safety/             # SQL keyword scanner
internal/mysqldrv/           # MySQL driver wrapper, error classifier
internal/output/             # JSON envelope types, row conversion
```

**Design principles:**
- Single binary, no external dependencies
- JSON output to stdout only (errors go to stdout as JSON, debug info to stderr)
- Fixed timeout (30 seconds) for all operations
- Read-only by default (safety scanner + recommended read-only account)
- One-to-one error code to exit code mapping for programmatic error handling

## License

MIT

## Contributing

This is a focused tool for AI agents. Feature requests for out-of-scope items will be declined. Bug reports and documentation improvements welcome.