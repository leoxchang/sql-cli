## ADDED Requirements

### Requirement: DSN source precedence
The configuration layer MUST resolve the DSN from the first non-empty source in this order: `--dsn` flag, `SQL_CLI_DSN` environment variable. If neither is set, the layer MUST return a `CONFIG_ERROR` indicating that no DSN was provided.

#### Scenario: Flag wins over env
- **WHEN** env `SQL_CLI_DSN=mysql://env` and flag `--dsn=mysql://flag` are both set
- **THEN** the resolved DSN is `mysql://flag`

#### Scenario: Env used when flag absent
- **WHEN** env `SQL_CLI_DSN=mysql://env` is set and no flag is passed
- **THEN** the resolved DSN is `mysql://env`

#### Scenario: Neither set
- **WHEN** no flag and no env var are set
- **THEN** resolution returns `CONFIG_ERROR` with message naming `SQL_CLI_DSN`

### Requirement: DSN URL validation
The configuration layer MUST accept only DSNs whose scheme is `mysql`. The DSN MUST be parseable as a URL with a host component. Otherwise the layer MUST return `CONFIG_ERROR` with a message identifying the validation failure. The DSN MUST NOT contain a literal newline (`\n`) or carriage return (`\r`) anywhere; a DSN containing one of these bytes MUST be rejected with `CONFIG_ERROR`.

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
