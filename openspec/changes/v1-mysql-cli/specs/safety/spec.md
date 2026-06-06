## ADDED Requirements

### Requirement: Write keyword detection
The safety layer MUST scan a SQL string and return a list of detected write keywords from the set: `INSERT`, `UPDATE`, `DELETE`, `DROP`, `CREATE`, `ALTER`, `TRUNCATE`, `GRANT`, `REVOKE`, `RENAME`, `REPLACE`. Detection is case-insensitive and applies only at word boundaries. The returned list contains one entry per detected keyword, and each entry MUST be the canonical uppercase form of that keyword regardless of the input case.

#### Scenario: Plain write statement blocked
- **WHEN** the input is `DROP TABLE users`
- **THEN** the scanner returns `["DROP"]`

#### Scenario: Lowercase write statement blocked
- **WHEN** the input is `delete from users`
- **THEN** the scanner returns `["DELETE"]` (uppercase)

#### Scenario: Mixed-case write statement blocked
- **WHEN** the input is `InSeRt INTO t VALUES (1)`
- **THEN** the scanner returns `["INSERT"]` (uppercase)

#### Scenario: Multiple write keywords
- **WHEN** the input is `INSERT INTO a SELECT * FROM b; UPDATE b SET x=1`
- **THEN** the scanner returns `["INSERT", "UPDATE"]`

#### Scenario: Read-only statement clean
- **WHEN** the input is `SELECT id FROM users WHERE last_update > NOW()`
- **THEN** the scanner returns an empty list

### Requirement: String literal skipping
The scanner MUST skip content between matching single quotes, treating the interior as opaque text. A doubled single quote (`''`) inside a literal MUST be treated as an escaped quote, not as a terminator.

#### Scenario: Keyword inside string literal
- **WHEN** the input is `SELECT 'INSERT INTO x' FROM t`
- **THEN** the scanner returns an empty list

#### Scenario: Escaped quote inside literal
- **WHEN** the input is `SELECT 'it''s INSERT' FROM t`
- **THEN** the scanner returns an empty list

### Requirement: Comment skipping
The scanner MUST skip content from `--` to end-of-line and content between `/*` and `*/`. Neither range contributes keywords.

#### Scenario: Line comment with keyword
- **WHEN** the input is `SELECT 1 -- DROP TABLE users`
- **THEN** the scanner returns an empty list

#### Scenario: Block comment with keyword
- **WHEN** the input is `SELECT /* UPDATE */ 1`
- **THEN** the scanner returns an empty list

### Requirement: Decision rule
The CLI MUST refuse to execute any SQL for which the scanner returns a non-empty list, emit a `SAFETY_BLOCKED` envelope, and exit 5. The first detected keyword (in canonical uppercase) is included in the envelope message.

#### Scenario: Block and surface the keyword
- **WHEN** the user runs `sql-cli query "DROP TABLE users"`
- **THEN** exit code is 5, stdout contains `SAFETY_BLOCKED` with `DROP` in the message, and no query is sent to the database
