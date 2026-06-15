## ADDED Requirements

### Requirement: DSN source precedence
The configuration layer MUST resolve the DSN from the first non-empty source in this order: `--dsn` flag, `SQL_CLI_DSN` environment variable, profile from a merged config file selected via `--profile <name>`. If all three are absent (or the profile lookup fails because no config file exists or the named profile is not defined), the layer MUST return a `CONFIG_ERROR` indicating that no DSN was provided.

#### Scenario: Flag wins over env
- **WHEN** env `SQL_CLI_DSN=mysql://env` and flag `--dsn=mysql://flag` are both set
- **THEN** the resolved DSN is `mysql://flag`

#### Scenario: Env used when flag absent
- **WHEN** env `SQL_CLI_DSN=mysql://env` is set and no flag is passed
- **THEN** the resolved DSN is `mysql://env`

#### Scenario: Profile used when flag and env absent
- **WHEN** `--profile dev` is passed, neither `--dsn` nor `SQL_CLI_DSN` is set, and the merged config map contains `dev: mysql://u:p@h:3306/devdb`
- **THEN** the resolved DSN is `mysql://u:p@h:3306/devdb`

#### Scenario: Profile unknown
- **WHEN** `--profile missing` is passed and no config file (or no such profile) exists
- **THEN** resolution returns `CONFIG_ERROR` whose message names the requested profile and lists the available profiles (or `(none)` if the config is empty)

#### Scenario: --dsn and --profile together
- **WHEN** both `--dsn` and `--profile` are passed
- **THEN** resolution returns `CONFIG_ERROR` indicating the two flags are mutually exclusive

#### Scenario: Neither set
- **WHEN** no flag, no env var, and no profile
- **THEN** resolution returns `CONFIG_ERROR` with message naming all three sources

### Requirement: DSN URL validation
The configuration layer MUST accept only DSNs whose scheme is `mysql`. The DSN MUST be parseable as a URL with a host component. Otherwise the layer MUST return `CONFIG_ERROR` with a message identifying the validation failure. The DSN MUST NOT contain a literal newline (`\n`) or carriage return (`\r`) anywhere; a DSN containing one of these bytes MUST be rejected with `CONFIG_ERROR`. The same validation MUST apply to every DSN value loaded from a config file; a malformed entry MUST be rejected at load time, not at first use.

#### Scenario: Valid mysql:// DSN
- **WHEN** the DSN is `mysql://user:p@host:3306/db?parseTime=true`
- **THEN** resolution succeeds

#### Scenario: Wrong scheme
- **WHEN** the DSN is `postgres://user:p@host:5432/db`
- **THEN** resolution returns `CONFIG_ERROR` indicating unsupported scheme

#### Scenario: Missing host
- **WHEN** the DSN is `mysql://user:p@/db`
- **THEN** resolution returns `CONFIG_ERROR` indicating missing host

#### Scenario: Newline in DSN
- **WHEN** the DSN is `mysql://user:p@host/db?x=y\nDROP TABLE users`
- **THEN** resolution returns `CONFIG_ERROR` indicating the DSN contains illegal control characters. The envelope's `error` object contains only `code` and `message`; `details` is absent and the raw DSN is NOT echoed in the envelope (the smuggled newline would re-appear in the JSON output otherwise).

#### Scenario: Invalid DSN inside a profile
- **WHEN** a config file contains `dsns: { dev: "http://..." }` and the user invokes `sql-cli --profile dev`
- **THEN** the binary exits 2 with `CONFIG_ERROR` whose message names the config file path and the offending profile

### Requirement: Config file layout and lookup
The configuration layer MUST support loading DSN profiles from a YAML file. The file shape is a single top-level `dsns:` map whose values are `mysql://` URLs. The layer MUST look up two files in order, with later files overriding earlier ones by profile name (shallow merge, not whole-file replacement):

1. **Global config** — the first existing file among `$SQL_CLI_CONFIG_DIR/config.yaml`, `$XDG_CONFIG_HOME/sql-cli/config.yaml`, `~/.sql-cli/config.yaml`.
2. **Local config** — `.sql-cli.yaml` found by walking upward from the current working directory to the filesystem root.

A missing file at any of these locations MUST NOT be an error. A present file that fails to parse as YAML, or that contains a value failing `mysql://` validation, MUST emit `CONFIG_ERROR` with the file path attached.

#### Scenario: Global config provides the profile
- **WHEN** `~/.sql-cli/config.yaml` contains `dsns: { dev: "mysql://..." }` and no local file exists
- **THEN** `sql-cli --profile dev` resolves the DSN from the global file

#### Scenario: Local config overrides a global profile
- **WHEN** the global file defines `dev: mysql://global` and the local `.sql-cli.yaml` (in CWD or a parent) defines `dev: mysql://local`
- **THEN** `--profile dev` resolves to `mysql://local`

#### Scenario: No config files at all
- **WHEN** no global file exists and no local file exists
- **THEN** the merged map is empty; `--profile <any>` emits `CONFIG_ERROR` listing `(none)` as available

#### Scenario: Local config in a parent directory
- **WHEN** CWD is `/repo/services/api` and the file `/repo/.sql-cli.yaml` exists but `/repo/services/api/.sql-cli.yaml` does not
- **THEN** the parent file is loaded
