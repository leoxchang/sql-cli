# `indexes` 子命令实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增 `indexes` / `idx` / `keys` 子命令，查询 `INFORMATION_SCHEMA.STATISTICS` 返回 14 列扁平索引元数据，跨 MySQL 5.7 / 8.0 输出形状一致。

**Architecture:** 新建 `internal/command/indexes.go`，沿用 `describe.go` 模板（参数解析 → DSN 转换 → 连接 → 探测 → 主查询 → 输出信封）。探测用单条 `INFORMATION_SCHEMA.COLUMNS` 查询检测 `IS_VISIBLE` / `EXPRESSION` 两列的存在性，根据返回的列名集合选择 3 种主查询投影之一。纯函数 `buildIndexSQL` 负责 SQL 字符串构造，便于单元测试。

**Tech Stack:** Go 1.26, `database/sql`, `github.com/go-sql-driver/mysql`（仅通过 `internal/mysqldrv` 间接使用）, testcontainers-go（集成测试）

**Spec 来源:** `openspec/changes/add-indexes-command/{design.md, proposal.md, tasks.md, specs/query-commands/spec.md}`

---

## 文件结构

| 文件 | 动作 | 职责 |
|---|---|---|
| `internal/command/indexes.go` | 新建 | `HandleIndexes` handler + `parseIndexesArgs` + `buildIndexSQL` |
| `internal/command/indexes_test.go` | 新建 | 参数解析和 SQL 构造的单元测试（无 Docker） |
| `internal/command/indexes_integration_test.go` | 新建 | 端到端集成测试（`//go:build integration`） |
| `internal/cli/router.go` | 修改 | `dispatch()` 加 `indexes` / `idx` / `keys` case；`printHelp()` 加一行 |
| `README.md` | 修改 | 加 `indexes` 子命令说明段 |

---

### Task 1: SQL 构造纯函数 + 单元测试

**Files:**
- Create: `internal/command/indexes.go`（仅放 `buildIndexSQL` 函数和常量）
- Create: `internal/command/indexes_test.go`

- [ ] **Step 1: 写失败的测试 — `buildIndexSQL` 三种路径**

```go
// internal/command/indexes_test.go
package command

import (
	"strings"
	"testing"
)

func TestBuildIndexSQL_FullPath(t *testing.T) {
	probe, main, err := buildIndexSQL("mydb", "users", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Probe SQL checks both IS_VISIBLE and EXPRESSION existence
	if !strings.Contains(probe, "INFORMATION_SCHEMA.COLUMNS") {
		t.Errorf("probe SQL missing INFORMATION_SCHEMA.COLUMNS: %s", probe)
	}
	if !strings.Contains(probe, "'is_visible'") || !strings.Contains(probe, "'expression'") {
		t.Errorf("probe SQL missing column name literals: %s", probe)
	}

	// Full path: 14 columns including IS_VISIBLE and EXPRESSION
	if !strings.Contains(main, "IS_VISIBLE") {
		t.Errorf("full path SQL missing IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "EXPRESSION") {
		t.Errorf("full path SQL missing EXPRESSION: %s", main)
	}
	if strings.Contains(main, "'' AS") {
		t.Errorf("full path SQL should not have '' AS projection: %s", main)
	}
	assertMainQueryStructure(t, main)
}

func TestBuildIndexSQL_OnlyIsVisible(t *testing.T) {
	probe, main, err := buildIndexSQL("mydb", "users", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = probe

	// 8.0.0-8.0.12: IS_VISIBLE real, EXPRESSION faked
	if !strings.Contains(main, "IS_VISIBLE") {
		t.Errorf("should contain real IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "'' AS EXPRESSION") {
		t.Errorf("should contain '' AS EXPRESSION: %s", main)
	}
	assertMainQueryStructure(t, main)
}

func TestBuildIndexSQL_NeitherColumn(t *testing.T) {
	probe, main, err := buildIndexSQL("mydb", "users", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = probe

	// 5.7: both faked
	if !strings.Contains(main, "'' AS IS_VISIBLE") {
		t.Errorf("should contain '' AS IS_VISIBLE: %s", main)
	}
	if !strings.Contains(main, "'' AS EXPRESSION") {
		t.Errorf("should contain '' AS EXPRESSION: %s", main)
	}
	assertMainQueryStructure(t, main)
}

// assertMainQueryStructure checks common structure shared by all 3 SQL variants.
func assertMainQueryStructure(t *testing.T, sql string) {
	t.Helper()
	if !strings.Contains(sql, "INFORMATION_SCHEMA.STATISTICS") {
		t.Errorf("missing INFORMATION_SCHEMA.STATISTICS: %s", sql)
	}
	if !strings.Contains(sql, "TABLE_SCHEMA = ?") {
		t.Errorf("missing TABLE_SCHEMA = ?: %s", sql)
	}
	if !strings.Contains(sql, "TABLE_NAME = ?") {
		t.Errorf("missing TABLE_NAME = ?: %s", sql)
	}
	if !strings.Contains(sql, "ORDER BY INDEX_NAME, SEQ_IN_INDEX") {
		t.Errorf("missing ORDER BY: %s", sql)
	}

	// Verify all 14 column names are present in the SELECT
	expectedColumns := []string{
		"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
		"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
		"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
		"IS_VISIBLE", "EXPRESSION",
	}
	for _, col := range expectedColumns {
		if !strings.Contains(sql, col) {
			t.Errorf("missing column %s in SQL: %s", col, sql)
		}
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `rtk test go test ./internal/command/ -run TestBuildIndexSQL -v`
Expected: FAIL — `buildIndexSQL` 未定义

- [ ] **Step 3: 实现 `buildIndexSQL`**

在 `internal/command/indexes.go` 中写入：

```go
package command

import "fmt"

// probeSQL queries INFORMATION_SCHEMA.COLUMNS to detect which optional columns
// exist on INFORMATION_SCHEMA.STATISTICS. IS_VISIBLE was added in MySQL 8.0.0,
// EXPRESSION in MySQL 8.0.13. MySQL 5.7 has neither.
const probeSQL = "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = 'information_schema' AND TABLE_NAME = 'statistics' AND COLUMN_NAME IN ('is_visible', 'expression')"

// baseColumns are the 12 columns present on all supported MySQL versions.
var baseColumns = []string{
	"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
	"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
	"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
}

// buildIndexSQL returns the probe SQL (always the same) and the main query SQL
// selected based on which optional columns the server exposes.
//
//   - hasIsVisible=true, hasExpression=true  → 8.0.13+: full 14 columns
//   - hasIsVisible=true, hasExpression=false → 8.0.0-8.0.12: 12 + IS_VISIBLE + '' AS EXPRESSION
//   - hasIsVisible=false, hasExpression=false → 5.7: 12 + '' AS IS_VISIBLE + '' AS EXPRESSION
func buildIndexSQL(database, table string, hasIsVisible, hasExpression bool) (probe string, main string, err error) {
	cols := make([]string, len(baseColumns))
	copy(cols, baseColumns)

	if hasIsVisible {
		cols = append(cols, "IS_VISIBLE")
	} else {
		cols = append(cols, "'' AS IS_VISIBLE")
	}

	if hasExpression {
		cols = append(cols, "EXPRESSION")
	} else {
		cols = append(cols, "'' AS EXPRESSION")
	}

	main = fmt.Sprintf(
		"SELECT %s FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX",
		joinColumns(cols),
	)

	return probeSQL, main, nil
}

// joinColumns joins column expressions with ", ".
func joinColumns(cols []string) string {
	result := ""
	for i, c := range cols {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `rtk test go test ./internal/command/ -run TestBuildIndexSQL -v`
Expected: PASS（3 个子测试全部通过）

- [ ] **Step 5: 提交**

```bash
git add internal/command/indexes.go internal/command/indexes_test.go
git commit -m "feat(indexes): add buildIndexSQL with 3-path compatibility probe"
```

---

### Task 2: 参数解析函数 + 单元测试

**Files:**
- Modify: `internal/command/indexes.go`（追加 `parseIndexesArgs`）
- Modify: `internal/command/indexes_test.go`（追加参数解析测试）

- [ ] **Step 1: 写失败的测试 — 参数解析**

在 `internal/command/indexes_test.go` 追加：

```go
func TestParseIndexesArgs_DotSeparated(t *testing.T) {
	db, table, err := parseIndexesArgs("mydb.users", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db != "mydb" {
		t.Errorf("expected database 'mydb', got %q", db)
	}
	if table != "users" {
		t.Errorf("expected table 'users', got %q", table)
	}
}

func TestParseIndexesArgs_TableOnlyWithDSN(t *testing.T) {
	db, table, err := parseIndexesArgs("users", "mysql://u:p@h:3306/mydb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if db != "mydb" {
		t.Errorf("expected database 'mydb', got %q", db)
	}
	if table != "users" {
		t.Errorf("expected table 'users', got %q", table)
	}
}

func TestParseIndexesArgs_TableOnlyNoDSN(t *testing.T) {
	_, _, err := parseIndexesArgs("users", "mysql://u:p@h:3306/")
	if err == nil {
		t.Fatal("expected error for single-segment with no DSN database, got nil")
	}
}

func TestParseIndexesArgs_RejectedInputs(t *testing.T) {
	cases := []struct {
		name string
		arg  string
	}{
		{"empty string", ""},
		{"three segments", "a.b.c"},
		{"dot only", "."},
		{"trailing dot", "a."},
		{"leading dot", ".a"},
		{"double dot", "a..b"},
		{"injection attempt", "mydb.users; DROP TABLE x"},
		{"backtick injection", "`mydb`.`users`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseIndexesArgs(tc.arg, "mysql://u:p@h:3306/mydb")
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc.arg)
			}
		})
	}
}

func TestParseIndexesArgs_EmptyArg(t *testing.T) {
	// Empty string with DSN that has a database — should still reject
	_, _, err := parseIndexesArgs("", "mysql://u:p@h:3306/mydb")
	if err == nil {
		t.Fatal("expected error for empty string argument, got nil")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `rtk test go test ./internal/command/ -run TestParseIndexesArgs -v`
Expected: FAIL — `parseIndexesArgs` 未定义

- [ ] **Step 3: 实现 `parseIndexesArgs`**

在 `internal/command/indexes.go` 追加：

```go
import (
	"fmt"
	"strings"

	"github.com/qiezi999/sql-cli/internal/config"
)

// parseIndexesArgs parses the single positional argument for the indexes command.
// Returns (database, table, error).
//
// Two valid forms:
//   - "<db>.<table>" — exactly one dot, both segments match validIdentifier
//   - "<table>" — database taken from DSN URL path; error if DSN has no database
//
// Rejected: empty string, 0 or 3+ dot-segments, any segment with empty or
// non-whitelist characters.
func parseIndexesArgs(arg string, dsn string) (database string, table string, err error) {
	if arg == "" {
		return "", "", fmt.Errorf("table identifier required")
	}

	parts := strings.Split(arg, ".")

	// Count non-empty segments. Any empty segment (from ".", "a.", ".a", "a..b")
	// is rejected — the split must produce exactly 1 or 2 non-empty parts with
	// no empty parts mixed in.
	nonEmpty := 0
	for _, p := range parts {
		if p != "" {
			nonEmpty++
		}
	}

	switch {
	case nonEmpty == 0 || len(parts) > 2:
		// "." → 2 parts both empty (nonEmpty=0)
		// "a.b.c" → 3 parts
		// "a..b" → 3 parts
		return "", "", fmt.Errorf("invalid identifier format: expected table or database.table")
	case len(parts) == 2 && (parts[0] == "" || parts[1] == ""):
		// "a." → ["a", ""] or ".a" → ["", "a"]
		return "", "", fmt.Errorf("invalid identifier format: expected table or database.table")
	case len(parts) == 2:
		// <db>.<table>
		database = parts[0]
		table = parts[1]
	case len(parts) == 1:
		// <table> — get database from DSN
		table = parts[0]
		dsnDB, dsErr := config.ExtractDatabaseFromURL(dsn)
		if dsErr != nil || dsnDB == "" {
			return "", "", fmt.Errorf("database required: specify as database.table or include database in DSN URL (--dsn mysql://user:pass@host:port/<db>)")
		}
		database = dsnDB
	}

	if !validIdentifier.MatchString(database) {
		return "", "", fmt.Errorf("invalid database identifier: %s", database)
	}
	if !validIdentifier.MatchString(table) {
		return "", "", fmt.Errorf("invalid table identifier: %s", table)
	}

	return database, table, nil
}
```

注意：需要在 `indexes.go` 顶部的 import 块中加入 `"fmt"`、`"strings"` 和 `"github.com/qiezi999/sql-cli/internal/config"`。

- [ ] **Step 4: 运行测试验证通过**

Run: `rtk test go test ./internal/command/ -run TestParseIndexesArgs -v`
Expected: PASS（所有子测试通过）

- [ ] **Step 5: 运行全部单元测试确认无回归**

Run: `rtk test go test ./internal/command/ -v`
Expected: 所有已有测试 + 新测试全部 PASS

- [ ] **Step 6: 提交**

```bash
git add internal/command/indexes.go internal/command/indexes_test.go
git commit -m "feat(indexes): add parseIndexesArgs with describe-compatible grammar"
```

---

### Task 3: HandleIndexes handler 实现

**Files:**
- Modify: `internal/command/indexes.go`（追加 `HandleIndexes`）

- [ ] **Step 1: 实现 `HandleIndexes`**

在 `internal/command/indexes.go` 追加：

```go
import (
	"context"
	"os"
	"time"

	"github.com/qiezi999/sql-cli/internal/mysqldrv"
	"github.com/qiezi999/sql-cli/internal/output"
)

// HandleIndexes executes the indexes subcommand: queries INFORMATION_SCHEMA.STATISTICS
// for the given <db>.<table> and writes a 14-column JSON envelope.
//
// Returns exit code 0 on success (including empty results for non-existent tables),
// or the appropriate error code on failure.
func HandleIndexes(dsn string, args []string) int {
	startTime := time.Now()

	// Validate argument count
	if len(args) < 1 {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			"table identifier required: sql-cli --dsn <dsn> indexes <table> or indexes <database.table>",
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Parse and validate identifier
	database, table, err := parseIndexesArgs(args[0], dsn)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Convert mysql:// URL to driver DSN
	driverDSN, errEnvelope := config.MySQLURLToDriverDSNOrError(dsn)
	if errEnvelope != nil {
		_ = output.WriteError(errEnvelope)
		return errEnvelope.Error.Code.ExitCode()
	}

	// Connect
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := mysqldrv.Open(ctx, driverDSN)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, "")
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}
	defer db.Close()

	// Probe: detect IS_VISIBLE and EXPRESSION column existence
	probeRows, err := db.QueryContext(ctx, probeSQL)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, probeSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}

	hasIsVisible := false
	hasExpression := false
	for probeRows.Next() {
		var colName string
		if err := probeRows.Scan(&colName); err != nil {
			probeRows.Close()
			envelope := output.NewErrorEnvelope(
				output.ErrorCodeInternalError,
				fmt.Sprintf("failed to scan probe result: %v", err),
				nil,
			)
			_ = output.WriteError(envelope)
			return output.ErrorCodeInternalError.ExitCode()
		}
		switch colName {
		case "is_visible":
			hasIsVisible = true
		case "expression":
			hasExpression = true
		}
	}
	probeRows.Close()
	if err := probeRows.Err(); err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, probeSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}

	// Build main query based on probe results
	_, mainSQL, err := buildIndexSQL(database, table, hasIsVisible, hasExpression)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeInternalError,
			fmt.Sprintf("failed to build index query: %v", err),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeInternalError.ExitCode()
	}

	// Execute main query
	rows, err := db.QueryContext(ctx, mainSQL, database, table)
	if err != nil {
		errCode, message, details := mysqldrv.ClassifyError(err, mainSQL)
		envelope := output.NewErrorEnvelope(errCode, message, details)
		_ = output.WriteError(envelope)
		return errCode.ExitCode()
	}
	defer rows.Close()

	// Convert rows
	columns, rowData, err := output.ConvertRows(rows)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeQueryError,
			fmt.Sprintf("failed to convert result rows: %v", err),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeQueryError.ExitCode()
	}

	rowsAsAny := make([]any, len(rowData))
	for i, row := range rowData {
		rowsAsAny[i] = row
	}

	elapsedMs := time.Since(startTime).Milliseconds()

	err = output.WriteSuccess(os.Stdout, columns, rowsAsAny, len(rowData), elapsedMs)
	if err != nil {
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeInternalError,
			fmt.Sprintf("failed to write output: %v", err),
			nil,
		)
		_ = output.WriteError(envelope)
		return output.ErrorCodeInternalError.ExitCode()
	}

	return 0
}
```

注意：`indexes.go` 的 import 块最终应包含：`"context"`、`"fmt"`、`"os"`、`"strings"`、`"time"`、以及项目的 `config`、`mysqldrv`、`output` 包。

- [ ] **Step 2: 验证编译通过**

Run: `rtk test go build ./...`
Expected: 编译成功，无错误

- [ ] **Step 3: 写 handler 级别的参数校验测试**

在 `internal/command/indexes_test.go` 追加：

```go
func TestHandleIndexes_MissingArgument(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleIndexes("mysql://user:pass@localhost:3306/mydb", []string{})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if envelope.Ok {
		t.Error("expected error envelope")
	}
	if envelope.Error.Code != output.ErrorCodeConfigError {
		t.Errorf("expected CONFIG_ERROR, got %s", envelope.Error.Code)
	}
}

func TestHandleIndexes_EmptyStringArgument(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	exitCode := HandleIndexes("mysql://user:pass@localhost:3306/mydb", []string{""})

	w.Close()
	os.Stdout = oldStdout

	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var envelope output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if envelope.Ok {
		t.Error("expected error envelope")
	}
}

func TestHandleIndexes_InvalidIdentifier(t *testing.T) {
	cases := []string{"a.b.c", ".", "a.", ".a", "a..b", "db.table; DROP"}
	for _, arg := range cases {
		t.Run(arg, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			exitCode := HandleIndexes("mysql://user:pass@localhost:3306/mydb", []string{arg})

			w.Close()
			os.Stdout = oldStdout

			if exitCode != 2 {
				t.Errorf("expected exit code 2 for %q, got %d", arg, exitCode)
			}
		})
	}
}
```

需要在 `indexes_test.go` 的 import 中追加 `"bytes"`、`"encoding/json"`、`"os"`。

- [ ] **Step 4: 运行测试验证通过**

Run: `rtk test go test ./internal/command/ -run "TestBuildIndexSQL|TestParseIndexesArgs|TestHandleIndexes" -v`
Expected: 全部 PASS

- [ ] **Step 5: 提交**

```bash
git add internal/command/indexes.go internal/command/indexes_test.go
git commit -m "feat(indexes): implement HandleIndexes handler with probe + 3-path query"
```

---

### Task 4: Router 接入 + help 文本

**Files:**
- Modify: `internal/cli/router.go`

- [ ] **Step 1: 在 `dispatch()` 中加 `indexes` case**

在 `internal/cli/router.go` 的 `dispatch()` 函数中，`case "describe", "desc":` 之后加：

```go
	case "indexes", "idx", "keys":
		return handleIndexes(dsn, args)
```

- [ ] **Step 2: 加 handler 包装函数**

在 `router.go` 底部的包装函数区域（`handleQuery` 之后）加：

```go
func handleIndexes(dsn string, args []string) int {
	return command.HandleIndexes(dsn, args)
}
```

- [ ] **Step 3: 在 `printHelp()` 中加一行**

在 `printHelp()` 的 Subcommands 段中，`describe` 行之后加：

```
  indexes <database.table>
                        Show index metadata (aliases: idx, keys)
```

- [ ] **Step 4: 验证编译**

Run: `rtk test go build ./...`
Expected: 编译成功

- [ ] **Step 5: 快速冒烟**

Run: `rtk test go run ./cmd/sql-cli help`
Expected: 输出包含 `indexes <database.table>` 行

- [ ] **Step 6: 提交**

```bash
git add internal/cli/router.go
git commit -m "feat(indexes): wire indexes/idx/keys into router dispatch and help"
```

---

### Task 5: 集成测试

**Files:**
- Create: `internal/command/indexes_integration_test.go`

- [ ] **Step 1: 写 8.0 成功路径测试**

```go
//go:build integration

package command_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/qiezi999/sql-cli/internal/output"
	"github.com/qiezi999/sql-cli/internal/testhelpers"
)

func TestIndexesSubcommand_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL container: %v", err)
	}
	defer teardown()

	// Seed: create testdb with a table having composite PK + secondary index
	seedIndexesTestTable(t, dsn)

	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	t.Run("Success_ReturnsIndexMetadata", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "indexes", "testdb.idx_test")
		stdout, stderr, exitCode := runCommand(cmd)

		if exitCode != 0 {
			t.Fatalf("Expected exit 0, got %d\nStderr: %s\nStdout: %s", exitCode, stderr, stdout)
		}
		if len(stderr) > 0 {
			t.Errorf("Expected clean stderr, got: %s", stderr)
		}

		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse envelope: %v\nOutput: %s", err, stdout)
		}
		if !envelope.Ok {
			t.Fatalf("Expected ok=true, got error: %s", envelope.Error.Message)
		}

		// 4 rows: PK(tenant_id) + PK(id) + idx_name(tenant_id) + idx_name(name)
		if envelope.RowCount != 4 {
			t.Errorf("Expected 4 rows, got %d", envelope.RowCount)
		}
		if len(envelope.Columns) != 14 {
			t.Errorf("Expected 14 columns, got %d", len(envelope.Columns))
		}

		// Verify column names
		expectedNames := []string{
			"INDEX_NAME", "NON_UNIQUE", "SEQ_IN_INDEX", "COLUMN_NAME",
			"COLLATION", "CARDINALITY", "SUB_PART", "PACKED",
			"NULLABLE", "INDEX_TYPE", "COMMENT", "INDEX_COMMENT",
			"IS_VISIBLE", "EXPRESSION",
		}
		for i, col := range envelope.Columns {
			if col.Name != expectedNames[i] {
				t.Errorf("Column %d: expected %q, got %q", i, expectedNames[i], col.Name)
			}
		}

		// IS_VISIBLE should be "YES" on 8.0+
		assertColumnValue(t, envelope, "IS_VISIBLE", 0, "YES")
		// EXPRESSION should be "" for non-expression indexes
		assertColumnValue(t, envelope, "EXPRESSION", 0, "")
	})

	t.Run("NonExistentTable_EmptyResult", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "--dsn", dsn, "indexes", "testdb.nonexistent")
		stdout, _, exitCode := runCommand(cmd)

		if exitCode != 0 {
			t.Fatalf("Expected exit 0 for non-existent table, got %d", exitCode)
		}

		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse envelope: %v", err)
		}
		if !envelope.Ok {
			t.Fatalf("Expected ok=true for non-existent table")
		}
		if envelope.RowCount != 0 {
			t.Errorf("Expected row_count=0, got %d", envelope.RowCount)
		}
	})

	t.Run("Aliases_IdenticalOutput", func(t *testing.T) {
		commands := []string{"indexes", "idx", "keys"}
		outputs := make([]string, len(commands))
		for i, sub := range commands {
			cmd := exec.Command(binaryPath, "--dsn", dsn, sub, "testdb.idx_test")
			stdout, _, exitCode := runCommand(cmd)
			if exitCode != 0 {
				t.Fatalf("%s: expected exit 0, got %d", sub, exitCode)
			}
			outputs[i] = string(stdout)
		}
		for i := 1; i < len(outputs); i++ {
			if outputs[i] != outputs[0] {
				t.Errorf("Alias %q output differs from indexes", commands[i])
			}
		}
	})

	t.Run("TableOnlyForm_DSNFallback", func(t *testing.T) {
		// DSN includes testdb; single-segment arg should resolve to testdb.idx_test
		cmd := exec.Command(binaryPath, "--dsn", dsn, "indexes", "idx_test")
		stdout, _, exitCode := runCommand(cmd)
		if exitCode != 0 {
			t.Fatalf("Expected exit 0, got %d", exitCode)
		}

		var envelope output.Envelope
		if err := json.Unmarshal(stdout, &envelope); err != nil {
			t.Fatalf("Failed to parse envelope: %v", err)
		}
		if !envelope.Ok || envelope.RowCount != 4 {
			t.Errorf("Expected 4 rows via DSN fallback, got ok=%v rows=%d", envelope.Ok, envelope.RowCount)
		}
	})

	t.Run("ParameterValidation", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
		}{
			{"three segments", []string{"a.b.c"}},
			{"dot only", []string{"."}},
			{"trailing dot", []string{"a."}},
			{"leading dot", []string{".a"}},
			{"double dot", []string{"a..b"}},
			{"injection", []string{"testdb.idx_test; DROP TABLE x"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				fullArgs := append([]string{"--dsn", dsn, "indexes"}, tc.args...)
				cmd := exec.Command(binaryPath, fullArgs...)
				_, _, exitCode := runCommand(cmd)
				if exitCode != 2 {
					t.Errorf("Expected exit 2 for %q, got %d", tc.name, exitCode)
				}
			})
		}
	})
}

func seedIndexesTestTable(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("mysql", mustDriverDSN(t, dsn))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS testdb",
		`CREATE TABLE IF NOT EXISTS testdb.idx_test (
			tenant_id INT NOT NULL,
			id INT NOT NULL,
			name VARCHAR(100),
			PRIMARY KEY (tenant_id, id),
			INDEX idx_name (tenant_id, name)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("Seed failed: %v\nSQL: %s", err, s)
		}
	}
}

// runCommand executes a command and returns (stdout, stderr, exitCode).
func runCommand(cmd *exec.Cmd) ([]byte, []byte, int) {
	var stdout, stderr []byte
	// Capture output
	out, err := cmd.Output()
	stdout = out
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr = exitErr.Stderr
		return stdout, stderr, exitErr.ExitCode()
	}
	if err != nil {
		return stdout, []byte(fmt.Sprintf("exec error: %v", err)), -1
	}
	return stdout, stderr, 0
}

// assertColumnValue checks that a specific column in the first row has the expected value.
func assertColumnValue(t *testing.T, envelope output.Envelope, colName string, rowIdx int, expected string) {
	t.Helper()
	colIdx := -1
	for i, c := range envelope.Columns {
		if c.Name == colName {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		t.Fatalf("Column %q not found", colName)
	}
	if rowIdx >= len(envelope.Rows) {
		t.Fatalf("Row index %d out of range (have %d rows)", rowIdx, len(envelope.Rows))
	}
	row := envelope.Rows[rowIdx]
	rowSlice, ok := row.([]any)
	if !ok {
		t.Fatalf("Row is not []any: %T", row)
	}
	actual := fmt.Sprintf("%v", rowSlice[colIdx])
	if actual != expected {
		t.Errorf("Column %s row %d: expected %q, got %q", colName, rowIdx, expected, actual)
	}
}
```

注意：`mustDriverDSN` 辅助函数需要查看已有的集成测试中是否已有定义。如果 `describe_integration_test.go` 或 `tables_integration_test.go` 中已有，直接复用；如果没有，需要在此文件中加一个：

```go
func mustDriverDSN(t *testing.T, mysqlURL string) string {
	t.Helper()
	dsn, err := config.MySQLURLToDriverDSN(mysqlURL)
	if err != nil {
		t.Fatalf("Failed to convert DSN: %v", err)
	}
	return dsn
}
```

需要在 import 中加 `"github.com/qiezi999/sql-cli/internal/config"`。

- [ ] **Step 2: 写 5.7 兼容路径测试**

在同一个文件中追加：

```go
func TestIndexesSubcommand_Integration_MySQL57(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if skipReason := checkMySQL57Available(); skipReason != "" {
		t.Skip(skipReason)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQL57ContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL 5.7 container: %v", err)
	}
	defer teardown()

	seedIndexesTestTable(t, dsn)

	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	cmd := exec.Command(binaryPath, "--dsn", dsn, "indexes", "testdb.idx_test")
	stdout, stderr, exitCode := runCommand(cmd)

	if exitCode != 0 {
		t.Fatalf("Expected exit 0 on 5.7, got %d\nStderr: %s\nStdout: %s", exitCode, stderr, stdout)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}
	if !envelope.Ok {
		t.Fatalf("Expected ok=true on 5.7")
	}
	if envelope.RowCount != 4 {
		t.Errorf("Expected 4 rows on 5.7, got %d", envelope.RowCount)
	}
	if len(envelope.Columns) != 14 {
		t.Errorf("Expected 14 columns on 5.7, got %d", len(envelope.Columns))
	}

	// On 5.7, IS_VISIBLE and EXPRESSION should be "" (compat projection)
	assertColumnValue(t, envelope, "IS_VISIBLE", 0, "")
	assertColumnValue(t, envelope, "EXPRESSION", 0, "")
}

func checkMySQL57Available() string {
	// Check SKIP_DOCKER env var — same gate as other integration tests
	// Also check if 5.7 image is locally available
	cmd := exec.Command("docker", "image", "inspect", "mysql:5.7")
	if err := cmd.Run(); err != nil {
		return "mysql:5.7 image not available locally; pull with: docker pull mysql:5.7"
	}
	return ""
}
```

**注意：** `testhelpers.SetupMySQL57ContainerWithDSN` 可能不存在。需要先检查 `internal/testhelpers/` 是否已有此函数。如果没有，需要在 Task 5a 中新增。

- [ ] **Step 2a（条件性）: 如果 `SetupMySQL57ContainerWithDSN` 不存在，新建它**

在 `internal/testhelpers/mysql_container.go` 末尾追加（与 `SetupMySQLContainerWithDSN` 结构一致，仅镜像不同）：

```go
// SetupMySQL57ContainerWithDSN creates a MySQL 5.7 test container and returns both
// the container and a mysql:// DSN. Used to test the compatibility probe path where
// IS_VISIBLE and EXPRESSION columns are absent from INFORMATION_SCHEMA.STATISTICS.
func SetupMySQL57ContainerWithDSN(ctx context.Context) (container *MySQLTestContainer, dsn string, teardown func(), err error) {
	mysqlC, err := mysql.Run(ctx,
		"mysql:5.7",
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
		mysql.WithDatabase("testdb"),
	)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create MySQL 5.7 container: %w", err)
	}

	connStr, err := mysqlC.ConnectionString(ctx)
	if err != nil {
		_ = mysqlC.Terminate(ctx)
		return nil, "", nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	dsn = "mysql://" + connStr

	wrapped := &MySQLTestContainer{
		MySQLContainer: mysqlC,
		ctx:            ctx,
	}

	teardown = func() {
		if err := mysqlC.Terminate(ctx); err != nil {
			fmt.Printf("Warning: failed to terminate MySQL 5.7 container: %v\n", err)
		}
	}

	return wrapped, dsn, teardown, nil
}
```

- [ ] **Step 3: 写 8.0 表达式索引测试**

在同一文件中追加：

```go
func TestIndexesSubcommand_Integration_ExpressionIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, dsn, teardown, err := testhelpers.SetupMySQLContainerWithDSN(ctx)
	if err != nil {
		t.Fatalf("Failed to setup MySQL container: %v", err)
	}
	defer teardown()

	// Seed: table with expression index (8.0.13+ feature)
	db, err := sql.Open("mysql", mustDriverDSN(t, dsn))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer db.Close()

	stmts := []string{
		"CREATE DATABASE IF NOT EXISTS testdb",
		`CREATE TABLE IF NOT EXISTS testdb.t_expr (
			a INT, b INT,
			INDEX idx_expr ((a + b))
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("Seed failed: %v\nSQL: %s", err, s)
		}
	}

	binaryPath := "/tmp/sql-cli-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/sql-cli")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	cmd := exec.Command(binaryPath, "--dsn", dsn, "indexes", "testdb.t_expr")
	stdout, _, exitCode := runCommand(cmd)
	if exitCode != 0 {
		t.Fatalf("Expected exit 0, got %d", exitCode)
	}

	var envelope output.Envelope
	if err := json.Unmarshal(stdout, &envelope); err != nil {
		t.Fatalf("Failed to parse envelope: %v", err)
	}
	if !envelope.Ok || envelope.RowCount != 1 {
		t.Fatalf("Expected 1 row, got ok=%v rows=%d", envelope.Ok, envelope.RowCount)
	}

	// EXPRESSION should contain "(a + b)" for the expression index
	colIdx := -1
	for i, c := range envelope.Columns {
		if c.Name == "EXPRESSION" {
			colIdx = i
			break
		}
	}
	row := envelope.Rows[0].([]any)
	exprVal := fmt.Sprintf("%v", row[colIdx])
	if exprVal == "" {
		t.Errorf("Expected non-empty EXPRESSION for expression index, got empty string")
	}

	// COLUMN_NAME should be empty for expression indexes
	colNameIdx := -1
	for i, c := range envelope.Columns {
		if c.Name == "COLUMN_NAME" {
			colNameIdx = i
			break
		}
	}
	colNameVal := fmt.Sprintf("%v", row[colNameIdx])
	if colNameVal != "" {
		t.Errorf("Expected empty COLUMN_NAME for expression index, got %q", colNameVal)
	}
}
```

- [ ] **Step 4: 运行集成测试**

Run: `rtk test go test -tags=integration -run TestIndexesSubcommand -v ./internal/command/`
Expected: PASS（8.0 路径全部通过；5.7 路径视镜像可用性跳过或通过）

- [ ] **Step 5: 提交**

```bash
git add internal/command/indexes_integration_test.go
# If 5.7 helper was added:
git add internal/testhelpers/mysql_container.go
git commit -m "test(indexes): add integration tests for 8.0, 5.7, expression indexes"
```

---

### Task 6: README 文档

**Files:**
- Modify: `README.md`

- [ ] **Step 1: 在 README 中加 `indexes` 子命令说明**

在 `describe` 子命令说明段之后，追加与 `describe` 同结构的段：

```markdown
### `indexes <database.table>`

Show index metadata from `INFORMATION_SCHEMA.STATISTICS`. Returns a 14-column flat list
covering index name, uniqueness, column order, index type, cardinality, visibility,
and expression text.

Aliases: `idx`, `keys`

Argument format matches `describe`:
- `indexes <db>.<table>` — explicit database and table
- `indexes <table>` — uses database from DSN URL path

Output columns: `INDEX_NAME`, `NON_UNIQUE`, `SEQ_IN_INDEX`, `COLUMN_NAME`, `COLLATION`,
`CARDINALITY`, `SUB_PART`, `PACKED`, `NULLABLE`, `INDEX_TYPE`, `COMMENT`, `INDEX_COMMENT`,
`IS_VISIBLE`, `EXPRESSION`

Example:
\`\`\`bash
sql-cli --dsn mysql://root:pass@localhost:3306/mydb indexes users
sql-cli --dsn mysql://root:pass@localhost:3306/mydb idx users
\`\`\`
```

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "docs: add indexes subcommand section to README"
```

---

### Task 7: 最终验证

- [ ] **Step 1: 编译验证**

Run: `rtk test go build ./...`
Expected: 成功

- [ ] **Step 2: 静态检查**

Run: `rtk test go vet ./...`
Expected: 无警告

- [ ] **Step 3: 单元测试**

Run: `rtk test go test ./...`
Expected: 全部 PASS

- [ ] **Step 4: 集成测试**

Run: `rtk test go test -tags=integration ./...`
Expected: 全部 PASS（5.7 测试视镜像可用性跳过）

- [ ] **Step 5: 覆盖率检查**

Run: `rtk test go test -cover ./internal/command/`
Expected: `indexes.go` 覆盖率 ≥ 80%（`internal/command/` 不强制 80% 门控，但期望高覆盖）

- [ ] **Step 6: 最终提交（如有遗漏修改）**

```bash
git add -A
git commit -m "chore(indexes): final verification and cleanup"
```

---

## 构建顺序总结

```
Task 1 (buildIndexSQL + tests)
  ↓
Task 2 (parseIndexesArgs + tests)
  ↓
Task 3 (HandleIndexes + handler tests)
  ↓
Task 4 (router wiring)
  ↓
Task 5 (integration tests)
  ↓
Task 6 (README)
  ↓
Task 7 (final verification)
```

每个 Task 结束时 `go build` 和 `go test` 都应通过。不存在需要"先全部写完才能编译"的情况。
