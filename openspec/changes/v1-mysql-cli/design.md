## Context

`sql-cli` is a greenfield project. The repo contains an `openspec/` directory with only a `config.yaml`. The motivating problem is that AI agents calling MySQL through the existing `mysql` CLI must parse human-oriented tabular output, infer error meaning from free-text strings, and tolerate no exit-code granularity. They need a tool whose stdout is structured, whose errors are categorised, and whose default posture cannot damage data.

The change is a from-scratch Go module. The foundational design tensions to resolve are: (1) where the extension point for "other databases" lives without paying for an abstraction that has only one implementer; (2) how strictly to enforce read-only given that keyword filters are bypassable; (3) how to make every error distinguishable by exit code without bloating the code table. All three are addressed by leaning on Go's package system, layering the keyword filter as advisory UI on top of the README's "use a read-only account" recommendation, and assigning one exit code per error code.

A pre-apply review refined the original draft. The corrections in this design — stricter metadata argument validation, stdin support, canonical keyword casing, uniform `elapsed_ms` semantics, three-way error/exit-code separation — are folded into the relevant decision sections.

## Goals / Non-Goals

**Goals:**

- Ship a single Go binary that runs on Linux/macOS, with four subcommands and JSON-only output.
- Produce a uniform envelope on stdout so agents parse one shape, not four.
- Detect write keywords at the token level with string-literal and comment skipping, so column names containing `update` or string literals containing `INSERT` are not false positives; normalise detected keywords to canonical uppercase so agents can log against a constant.
- Map every failure mode to one of nine error codes, with a one-to-one mapping to fixed exit codes, so agent branching logic is stable at both the `$?` level and the JSON level.
- Validate `tables` and `describe` arguments against a strict identifier whitelist so attacker-controlled bytes cannot reach the database.
- Read query SQL from stdin (heredocs, multi-line statements) without changing the default positional-argument behaviour.
- Isolate all MySQL-specific code under `internal/mysqldrv/` so adding a Postgres driver later does not touch the read path.

**Non-Goals:**

- Driver/Dialect/Metadata interface hierarchy. The package boundary is the extension point; an interface is premature.
- YAML config files, profile aliases, `.env` loading, multi-connection management. Agents use environment variables and flags.
- `--allow-write` flag and write subcommands. v1.0 is read-only by design.
- `--timeout` flag and `tx` begin/commit subcommands. Hardcoded 30-second `MAX_EXECUTION_TIME` is sufficient for v1.0.
- Parameter binding via `?` or `:name` placeholders. The MySQL driver supports it natively but adds UX surface; deferred.
- MCP server, plugin system, table/csv/tsv output, LLM/NL features, other database drivers. All deferred.
- ORM, query builder, schema introspection beyond `describe`.

## Decisions

### D1: Package boundary as the extension point, not an interface

**Decision:** Put all MySQL code in `internal/mysqldrv/`. Expose a single package-level `Open(ctx, dsn) (*sql.DB, error)` plus an unexported driver registration. Future Postgres work would land in `internal/postgresdrv/` with the same shape. No `Driver` interface is defined in v1.0.

**Why:** CLAUDE.md rule "no abstractions for one-time code" applies. The boundary cost is zero today, the future cost of extracting an interface once two implementations exist is low, and writing the interface now would be guessing at the right method set. Go's package system already enforces import boundaries — `internal/` prevents external use, and the package name is the convention future readers will see.

**Alternatives considered:**

- *Define `Driver` interface now* — would force us to predict the method set, and any future driver that does not fit would have to wrap. Rejected.
- *Single `db` package with conditional compilation via build tags* — works but hides intent. Rejected; explicit `mysqldrv` package is self-documenting.

### D2: Uniform JSON envelope, nine error codes, one-to-one exit codes

**Decision:** Every subcommand emits one of two envelope shapes:

```json
{"ok": true, "columns": [...], "rows": [...], "row_count": N, "elapsed_ms": N}
{"ok": false, "error": {"code": "<CODE>", "message": "...", ...}}
```

Nine codes: `CONFIG_ERROR`, `CONNECTION_ERROR`, `AUTH_ERROR`, `PERMISSION_DENIED`, `SAFETY_BLOCKED`, `QUERY_ERROR`, `TIMEOUT`, `INTERNAL_ERROR`. (`PERMISSION_DENIED` was added in the pre-apply review; the original draft merged it with `AUTH_ERROR` under one exit code.) Exit codes: 0=success, 1=QUERY_ERROR, 2=CONFIG_ERROR, 3=CONNECTION_ERROR, 4=AUTH_ERROR, 5=SAFETY_BLOCKED, 6=TIMEOUT, 7=PERMISSION_DENIED, 99=INTERNAL_ERROR. The mapping is one-to-one.

**Why:** Agents parse stdout; envelope gives them a single branching point. Fixed codes are a contract; adding a tenth code is a v1.1 decision. Exit codes let simple scripts (`if [ $? -eq 5 ]`) work without JSON parsing at all. Splitting auth (4) from permission (7) means a wrapper that wants to "retry on bad credentials but not on bad grants" can branch at the shell level instead of parsing JSON.

**Alternatives considered:**

- *Free-form stderr error, JSON only on success* — forces agents to merge streams. Rejected.
- *Eight codes with AUTH and PERMISSION_DENIED sharing exit 4* — what the original draft said; defeated the shell-level branching. Rejected.
- *PERMISSION_DENIED=4, AUTH=7* — also a one-to-one split, but auth failures are the more common case and giving them the smaller number is the conventional choice.

### D3: Lightweight SQL scanner, not a full parser

**Decision:** Implement a single-pass scanner in `internal/safety/` that walks the SQL string, tracks a single state (`in_string` toggled by an unescaped single quote, with `''` recognised as an escaped quote and not a terminator; `in_line_comment` toggled by `--` to newline; `in_block_comment` toggled by `/*...*/`), tokenises on word boundaries, and matches case-insensitively against the keyword list. The detected keyword is normalised to its canonical uppercase form before being returned. No AST, no statement analysis.

**Why:** A real SQL parser (vitess/sqlparser, pingcap/parser) is a megabyte of code and a maintenance commitment. Token-level matching catches the common case — agent accidentally typing `UPDATE` — without false positives on column names like `last_update` or string literals like `'INSERT INTO'`. The recommendation to use a read-only MySQL account handles anything the scanner misses. Returning the keyword in canonical uppercase lets agents log it against a constant without a `strings.ToUpper` step.

**Alternatives considered:**

- *No keyword filter, README-only* — simpler, but loses the agent-visible "I am running in safe mode" affordance. Rejected.
- *Full parser* — overkill. Rejected.
- *Return the keyword in the original case* — preserves the user's input but pushes a transformation onto every consumer. Rejected.

### D4: DSN handling — `mysql://` URL only

**Decision:** Accept only `mysql://user:pass@host:port/db?param=value` form. Parse with `net/url`, then translate to the `go-sql-driver/mysql` DSN form internally. Reject any DSN that does not parse as a `mysql://` URL with `CONFIG_ERROR`. Reject any DSN containing `\n` or `\r` (DSN smuggling in shell-quoted values).

**Why:** Agents generate DSNs cross-tool; URL form is universal. The driver's native DSN is awkward (`user:pass@tcp(host:port)/db`). Restricting to URL form is a small validation win and reduces parser branching. The control-character check is one extra line and blocks a class of injection where a `\n` is smuggled into a quoted DSN flag.

**Alternatives considered:**

- *Accept driver's native DSN directly* — saves a translation step but creates two parsers. Rejected.
- *Accept both* — doubles the test surface for marginal benefit. Rejected.
- *No port segment: silently fall through to the driver, which would fail at `sql.Open` time with a non-actionable error* — original draft behaviour; rejected because the failure is invisible to the agent and the message does not point at "you forgot the port".

### D5: Configuration precedence and absence behaviour

**Decision:** Resolution order is `--dsn` flag → `SQL_CLI_DSN` env var → profile from config file (selected by `--profile <name>`) → absent (emit `CONFIG_ERROR` and exit 2). No fallback to `~/.my.cnf`.

Config file lookup runs only when both `--dsn` and `SQL_CLI_DSN` are absent and `--profile` is non-empty. The merged profile map is built from two YAML files in this order (later wins by name):

1. **Global config** — first existing path among `$SQL_CLI_CONFIG_DIR/config.yaml`, `$XDG_CONFIG_HOME/sql-cli/config.yaml`, `~/.config/sql-cli/config.yaml`.
2. **Local config** — `.sql-cli.yaml` found by walking upward from the current working directory to the filesystem root.

The on-disk schema is a single top-level `dsns:` map of `name: mysql://url` pairs. Each value is validated as a `mysql://` URL at load time; a malformed DSN in the config emits `CONFIG_ERROR` with the file path and the offending profile name attached. `--dsn` and `--profile` are mutually exclusive; supplying both emits `CONFIG_ERROR`.

**Why:** A read-only MySQL CLI for agents needs a way to keep DSNs out of shell history and `ps` output. The flag and env paths are already that — the config file lets the same agent binary switch between dev/staging/prod without re-quoting credentials in each command. We rejected the more ambitious "multi-connection management" feature (connection pools, named sessions) because the tool is one-shot and read-only: there is no benefit to keeping a connection warm across invocations. We do not read `~/.my.cnf` because that file's `[client]` group has a different shape (key-value pairs, not `mysql://` URLs) and the user's agent would have to learn yet another format. YAML is the format humans edit; we accept the small dependency for that ergonomics.

**Alternatives considered:**

- *Keep flag/env only* — pushes users toward shell-aliased DSNs in `~/.bashrc` or env files, both of which leak into `ps` and process listings. Rejected.
- *JSON config* — fine for machines, less friendly for humans editing connection lists. Rejected.
- *Look in `~/.my.cnf` as a fourth tier* — different format, surprising cross-tool behaviour. Rejected.
- *Profile in env (`$SQL_CLI_PROFILE`)* — duplicates `--profile` without buying anything; env var surface is already used for the DSN itself. Rejected.
- *Mutually exclusive `--dsn` and `--profile`* (chosen) — silent precedence is the kind of "surprising the agent" behaviour the spec rejects at every other layer.

### D6: `database/sql` directly, no ORM, no `sqlx`

**Decision:** Use the standard `database/sql` package plus `github.com/go-sql-driver/mysql`. No `sqlx`, no `ent`, no `gorm`. Hand-written `Scan` into `[]any` slices.

**Why:** The four subcommands are trivial SQL: `SHOW DATABASES`, `SHOW TABLES FROM`, `DESCRIBE`, and arbitrary `query`. An ORM adds dependency weight and indirected types for nothing. `sqlx` adds `StructScan` convenience we do not need. Direct `database/sql` keeps types honest: the column list comes straight from `Rows.ColumnTypes()` and the row data is a `[][]any` that we marshal ourselves, which gives us full control over the JSON shape.

**Alternatives considered:**

- *Use `sqlx`* — small convenience, large habit cost. Rejected.
- *ORM* — over-engineered. Rejected.

### D7: Test strategy — unit + testcontainers integration

**Decision:** Unit tests for `config`, `safety`, `output`, and the `mysqldrv` classifier. Integration tests for subcommand end-to-end behaviour using `testcontainers-go/modules/mysql` to spin a real MySQL 8 container. Coverage target 80% on `internal/{config,safety,output,mysqldrv}`; `internal/cli` and `internal/command` are not gated.

**Why:** The keyword scanner is pure-function unit territory. Connection lifecycle, error mapping, and subcommand behaviour need a real driver to exercise. Skipping integration tests would let the safety parser and the error mapping drift apart from the driver's actual behaviour. Gating `cli` and `command` at 80% would force tests that exist only to satisfy the gate — these packages are dispatch and glue and are better covered by the end-to-end integration tests.

**Alternatives considered:**

- *Mock the MySQL driver* — possible with `go-sqlmock` but reduces fidelity on the most error-prone paths (auth, network, charset). Rejected.
- *Skip integration, rely on unit tests* — too thin for a tool whose value is "it works against your real DB". Rejected.
- *Gate all six packages at 80%* — what the original draft said; would have produced coverage-theatre tests for thin glue packages. Rejected.

### D8: Strict identifier whitelist for `tables` and `describe` arguments

**Decision:** `tables` validates its argument against `^[A-Za-z0-9_]+$`. `describe` validates against `^[A-Za-z0-9_]+\.[A-Za-z0-9_]+$` (exactly one dot). Both reject anything else with `CONFIG_ERROR`, exit 2, before any SQL is built. Accepted identifiers are wrapped in backticks at SQL build time.

**Why:** Without the whitelist, the SQL string is built by Go string concatenation (`fmt.Sprintf("SHOW TABLES FROM %s", db)`). Backtick injection (`app`; DROP TABLE x; -- `) is technically blocked by Go's `%s` not interpreting SQL, but the resulting statement is still a MySQL syntax error or worse. The whitelist is the simplest defence: the regex matches every legal MySQL identifier and nothing an attacker can use. Combined with the read-only MySQL account recommended in the README, the metadata subcommands are safe even if the agent is compromised.

**Alternatives considered:**

- *Wrap every identifier in backticks at build time only* — works, but adds a code path that is only ever exercised by mistake; the whitelist prevents the mistake entirely.
- *Apply the safety scanner to `tables` and `describe` arguments* — the scanner looks for write keywords, but `app; DROP TABLE x` does not contain a write keyword on its own; the injection vector is the statement terminator, not the DDL verb. Wrong tool for the job.

### D9: `query` reads from stdin when `-` or `--stdin` is used

**Decision:** `sql-cli query -` and `sql-cli query --stdin` both read the SQL body from stdin until EOF. The current positional-argument behaviour is unchanged otherwise. Two separate 30-second timeouts apply, both producing the same `TIMEOUT` (exit 6) envelope: (a) the stdin read itself, in case the pipe delivers no data; (b) the subsequent MySQL execution, via the driver's `MAX_EXECUTION_TIME`. The two timeouts are not distinguished in the envelope — the agent sees a single `TIMEOUT` code and exit 6 either way.

**Why:** Heredocs and multi-line SQL are common in shell scripts. The original spec only allowed "one positional argument containing the SQL text" which silently broke at the first newline. A 12-line SQL query wrapped in a heredoc is the canonical agent-shaped input. Splitting the two timeouts in the spec (but not in the envelope) lets us harden each independently without expanding the public contract.

**Alternatives considered:**

- *Always read from stdin when no positional is given* — surprising; agents with no heredoc would suddenly hang waiting on stdin.
- *Read from a file path* — `query @path.sql` would work but adds a new prefix convention; `-` is the universal Unix convention for "stdin".
- *Single combined timeout covering both read and execution* — simpler, but a long-running read followed by a slow query would consume the budget on the read and exit before the query got a chance. Rejected.

### D10: `ClassifyError` returns `(code, message, details)`

**Decision:** The `mysqldrv` classifier signature is `(code, message, details)` where `details` is a `map[string]any` carrying `mysql_error_code` (a `uint16`) and optionally `sql` (the SQL string the caller passed in, for echo in the envelope). The output layer copies `details` into the failure envelope verbatim. The only keys emitted in v1.0 are `mysql_error_code` and `sql`.

**Why:** Centralising the echo in the classifier removes the per-caller plumbing of `mysql_error_code` and makes it impossible for a future error path to forget to include it. The free-form shape lets v1.1 add `query_was_select`, `affected_rows`, etc. without another breaking change.

**Alternatives considered:**

- *Keep `(code, message)` and let the caller pass the original SQL* — preserves the original signature but pushes the responsibility up, where it will be forgotten at least once.

### D11: `elapsed_ms` is uniform — handler entry to last row read

**Decision:** `elapsed_ms` is the wall-clock milliseconds from when the subcommand handler function was called to when the last row was read from `*sql.Rows`. The definition is identical for `query` and for the metadata subcommands. Connection setup time is included.

**Why:** Two definitions, one field, zero agent ability to interpret it. Connection time is a small fixed cost and is not what an agent is going to time out on — they are going to time out on `SELECT` from a billion-row table, and that is exactly the path covered by the single rule.

**Alternatives considered:**

- *Add a separate `connect_ms` field* — better, but doubles the surface for a number nobody is going to use in v1.0.
- *Keep the split (handler entry for metadata, connection-established for query) and document* — what the original draft said; the proposal rejected it.

### D12: Column type strings preserve server-reported case

**Decision:** The `type` string in each `columns` entry is emitted exactly as reported by `Rows.ColumnTypes().DatabaseTypeName()`. The output layer does not lowercase, uppercase, or otherwise transform the value.

**Why:** `DESCRIBE` against `information_schema` returns lowercase (`varchar`); a direct `SELECT` returns the column type in its declared case (`VARCHAR` or `BIGINT`). The case difference is real and reflects the SQL path the query took; normalising would lose information. Documented as a non-breaking clarification.

**Alternatives considered:**

- *Lowercase everything* — uniform-looking, but the type case is information an agent can use.
- *Uppercase everything* — same problem, opposite direction.

## Risks / Trade-offs

- **[Risk] Keyword scanner is bypassable** by an attacker who controls the SQL → *Mitigation*: README mandates read-only MySQL accounts. The scanner is defence-in-depth, not the security boundary. Honest framing in docs.
- **[Risk] `database/sql` exposes `[]any` rows that need manual type handling** (NULL → null, []byte → base64 or string, time.Time → RFC3339) → *Mitigation*: centralise the row-to-JSON conversion in `internal/output/`; covered by unit tests with table-driven cases for each MySQL type we expect.
- **[Risk] `testcontainers-go` adds a non-trivial dev dependency and requires Docker for `go test`** → *Mitigation*: gate integration tests behind a build tag (`//go:build integration`) so default `go test ./...` stays fast and Docker-free. Document in README.
- **[Risk] Nine error codes may not be enough; new failure modes appear** → *Mitigation*: codes are documented as a stable contract; adding codes is a breaking change for agents and should be deliberate (v1.1 proposal). `INTERNAL_ERROR` is the catch-all so we never crash silently.
- **[Risk] DSN password in `ps` output** when the DSN is passed as a flag → *Mitigation*: this is inherent to any CLI; the env var form is the recommended path and is invisible to `ps`. Documented in README.
- **[Risk] No retry / reconnect on transient network failures** → *Mitigation*: out of scope; agent can rerun the command. Documented as expected behaviour.
- **[Risk] Stdin read can hang the process on a half-closed pipe** → *Mitigation*: the read is wrapped in a 30-second context timeout. A stalled pipe exits with `TIMEOUT` (6). Documented in the spec.
- **[Risk] `details` is a free-form object, making strict consumers wary** → *Mitigation*: documented as "the only keys emitted in v1.0 are `mysql_error_code` and `sql`". Any other key is a v1.1+ change.
- **[Risk] Whitelist is too strict for legitimate database names** → *Mitigation*: MySQL identifiers cannot contain `-`, `/`, or any non-ASCII character; if a user has such a database, the read-only account they are told to use cannot be created in the first place. The whitelist is aligned with MySQL's own rules, not ours.
