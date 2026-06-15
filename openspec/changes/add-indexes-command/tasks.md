# 任务 — `add-indexes-command`

## 1. Spec 修订（v1-mysql-cli）

> **状态：** v1 spec 已在本次头脑风暴的前一轮中修订。下列四个文件已经是修订后状态。本节仅作可追溯性记录，已标完成；此处无需进一步工作。修订是纯文档——把标识符白名单正则从 `^[A-Za-z0-9_]+$` 改为 `^[A-Za-z0-9_-]+$`（与 `internal/command/tables.go:19` 实际代码一致），并把 `describe` requirement 改写为"split on `.` into exactly two non-empty segments"规则。**无行为变化、无测试变化、无 `internal/` 文件变化。**

- [x] 1.1 在 `openspec/changes/v1-mysql-cli/specs/query-commands/spec.md`，把 `tables` requirement（line 11）和 `Argument with disallowed character` scenario（line 35）改为使用 `^[A-Za-z0-9_-]+$`。
- [x] 1.2 在同一文件，把 `describe` requirement（line 38）改写为"split on `.` into exactly two non-empty segments, each matching `^[A-Za-z0-9_-]+$`"规则，替换原 `^[A-Za-z0-9_]+\.[A-Za-z0-9_]+$` 正则和"exactly one dot, no other special characters"措辞。
- [x] 1.3 在 `openspec/changes/v1-mysql-cli/design.md` D8（line 133），应用同样的正则修订（`^[A-Za-z0-9_]+$` → `^[A-Za-z0-9_-]+$` for `tables`，加上 `describe` 的段数说明）。
- [x] 1.4 在 `openspec/changes/v1-mysql-cli/tasks.md`，把任务 6.4（line 40）改为使用 `^[A-Za-z0-9_-]+$`，把任务 6.5（line 41）改为"validate that the argument splits on `.` into exactly two non-empty segments, each matching `^[A-Za-z0-9_-]+$`"。
- [x] 1.5 在 `openspec/changes/v1-mysql-cli/proposal.md` line 13，把 `[A-Za-z0-9_]+` 改为 `[A-Za-z0-9_-]+`，并说明 `describe` 额外要求恰好一个点、恰好两个非空段。

## 2. `indexes` 子命令 — 实现

- [ ] 2.1 新建 `internal/command/indexes.go`，handler 签名 `HandleIndexes(dsn string, args []string) int`，沿用 `tables.go` 模板。
  - **与 `tables.go` 的差异**（在此点出，避免实现时盲目 copy-paste）：
    - 参数是一个字符串而非数据库名。按 `.` split 成 1 或 2 段，每段用 `^[A-Za-z0-9_-]+$` 校验（与 `tables.go:19` 的 `validIdentifier` 同一正则）。
    - 单段形式下，数据库取自 DSN URL 路径（与 `describe` 行为一致）。若 DSN 无库路径则回退 `CONFIG_ERROR`。
    - 在主查询之前，对 `INFORMATION_SCHEMA` 有一次探测往返：对 `INFORMATION_SCHEMA.COLUMNS` 跑 `SELECT COLUMN_NAME ... WHERE COLUMN_NAME IN ('is_visible', 'expression')`，根据返回的列名集合（0/1/2 行）选择主查询的投影方式。
    - 三种主查询路径：
      - 两列均存在（8.0.13+）：完整 14 列 `SELECT ... FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`。
      - 仅 `is_visible` 存在（8.0.0–8.0.12）：基础 12 列 + `IS_VISIBLE` + `'' AS EXPRESSION`。
      - 两列均不存在（5.7）：基础 12 列 + `'' AS IS_VISIBLE` + `'' AS EXPRESSION`。
      三种路径输出列集一致（同样的 14 个列名、同样的顺序）。
    - 不取表注释（不同于 `describe.go` 额外用 `QueryRowContext` 取 `TABLE_COMMENT`）。
  - **参数校验**（覆盖 design.md Error Mapping 表的五种 `CONFIG_ERROR` 拒收情况）：
    - 按 `.` split 成 1 或 2 个非空段，每段匹配 `^[A-Za-z0-9_-]+$`。其他一律 `CONFIG_ERROR`（exit 2），错误信息不回显被拒输入。Error Mapping 表逐行列出五种拒收情况。
    - 单段形式下，数据库取自 DSN URL 路径。若 DSN 无库路径，emit `CONFIG_ERROR` 且错误信息点出两个回退源。
  - **连接**：以项目标准连接超时打开（`context.WithTimeout`，目前 30 秒——与 `tables.go` / `describe.go` / `query.go` 同一值；见 v1 D11 / design D5）。30 秒硬编码值由整个 command 包共用，不是 `indexes` 专属。
  - 跑 `INFORMATION_SCHEMA.COLUMNS` 上的探测查询。
  - 跑对应的主查询（完整或兼容）并经 `output.WriteSuccess` 写出成功信封。
  - `elapsed_ms` 是从 handler 入口到读完最后一行之间的墙钟毫秒数（v1 D11 / design D5 重申）。
- [ ] 2.2 在 `internal/cli/router.go` 的 `dispatch()` 里新增 `indexes` case，紧挨 `describe` / `desc`。接受三个子命令名：`indexes`、`idx`、`keys`。
- [ ] 2.3 在 `printHelp()` 里为新子命令加一行，风格与 `describe` 一致。

## 3. 测试

> **测试策略**（与 v1 D7 一致）：`internal/command/` 不在四个被门控的包（`config`、`safety`、`output`、`mysqldrv`）里，所以不受 80% 覆盖率门槛约束。SQL 执行对着真实 MySQL 跑是集成测试的事。`indexes.go` 的单元测试因此**只覆盖不依赖 driver 的部分**：参数解析、校验、标识符分段，以及从已解析参数构造探测 SQL 和三条主 SQL 字符串。不引入 `sqlmock`、不手写 `*sql.DB` stub——v1 D7 "用真实 MySQL 测试" 的原则在此延伸。5.7 / 8.0 < 8.0.13 的兼容路径由一个 `mysql:5.7` testcontainer 镜像的集成测试覆盖（`//go:build integration`，受 `SKIP_DOCKER=1` 跳过门控）。

- [ ] 3.1 `internal/command/indexes_test.go`（单元，无 Docker）：
  - **参数解析**：缺位置参数、空字符串参数、三段或更多段，以及 corner 输入（`.`、`a.`、`.a`、`a..b`）均 `CONFIG_ERROR` exit 2，错误信息不回显被拒输入。handler 在以上任何一种情况下都**不**调用 `mysqldrv.Open`。
  - **`<db>.<table>` 形式**：成功解析出 `database = "mydb"`、`table = "users"`。探测 SQL 是 design D2 的字面量字符串，主 SQL 是 14 列 `SELECT ... FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?` 模板，含正确数量的 `?` 占位符（2 个），后缀是 `ORDER BY INDEX_NAME, SEQ_IN_INDEX`。
  - **`<table>` 形式 + DSN 库路径**：成功解析，数据库段取自 DSN URL 路径；主 SQL 仍用 2 个占位符。
  - **`<table>` 形式 + DSN 库路径为空**：`CONFIG_ERROR` exit 2，错误信息点出两个回退源（位置参数 `<db>.<table>` 和 DSN URL 路径）。
  - **标识符白名单** 段级强制：包含 `[A-Za-z0-9_-]` 之外字符的段在 SQL 构造前被拒。（段内或段首尾的空白由同一规则拒；单独的"whitespace-only"用例不必单列。）
  - **SQL 字符串构造**：探测 SQL 含 design D2 的四个字面量字符串（`'information_schema'`、`'statistics'`、`'is_visible'`、`'expression'`），用 `IN (...)` 查 `COLUMN_NAME`；主 SQL 有 3 种变体，每种都含 2 个 `?` 占位符、`INFORMATION_SCHEMA.STATISTICS`，以及 spec 中按序列出的 14 个固定列名——区别在于第 13–14 列是原始列名还是 `'' AS` 投影。测试通过一个包内私有函数（如 `buildIndexSQL(database, table, hasIsVisible, hasExpression) (probe, main string, error)`）断言 SQL 字符串。
  - 这些测试**不**驱动 `db.QueryContext`；构造步骤是 handler 唯一在 driver 成功/失败路径之外有实质分支的部分，而该分支通过 handler 产出的 SQL 字符串可观察。测试通过上述 `buildIndexSQL` 函数断言 SQL 字符串，handler 在 `mysqldrv.Open` 之前调用一次。这与 v1 `tables.go` / `describe.go` 校验参数的写法一致：纯函数 helper，无 driver 介入，不引入 mock 层。
- [ ] 3.2 `internal/command/indexes_integration_test.go`（`//go:build integration`）：
  - **8.0 成功路径**：建一个 schema，里面一张表有复合主键 `(tenant_id, id)` 和二级 `BTREE` 索引 `(tenant_id, name)`。断言成功信封返回 4 行（主键 2 列 × 2 行 + 二级索引 2 列 × 2 行），`INDEX_NAME` 排序为 `PRIMARY` 然后是二级索引名，`SEQ_IN_INDEX` 在每个索引内升序。断言 `EXPRESSION` 列存在且四行均为空字符串（非表达式索引）；断言 `IS_VISIBLE` 列存在且四行均为 `"YES"`（集成目标是 `mysql:8.0` 镜像 ≥ 8.0.13，走完整 14 列路径）。
  - **8.0 表达式索引**：在 8.0 fixture 上建一张含表达式索引的表（`CREATE TABLE t_expr (a INT, b INT, INDEX idx_expr ((a + b)))`）。断言 `idx_expr` 对应行的 `EXPRESSION` 值为 `"(a + b)"`（server 报告的表达式文本），`COLUMN_NAME` 为空字符串（表达式索引无单列承载）。此测试仅在 `mysql:8.0`（≥ 8.0.13）上跑；5.7 fixture 不跑此用例。
  - **5.7 兼容路径**：同一 fixture，target 改为 `mysql:5.7` testcontainer。断言与 8.0 成功路径相同（4 行、`INDEX_NAME` 排序、`SEQ_IN_INDEX` 升序），但 `IS_VISIBLE` 期望值为空字符串 `""`（而非 8.0 上的 `"YES"`），因为 handler 在 5.7 上走的是带 `'' AS IS_VISIBLE, '' AS EXPRESSION` 第 13–14 列的兼容 SQL。`EXPRESSION` 期望值仍为空字符串（与 8.0 一致）。此测试像所有集成测试一样受 `SKIP_DOCKER=1` 跳过门控；此外它是项目里唯一需要 5.7 镜像的测试，可在不便拉 5.7 镜像的机器上跳过。
  - **不存在表/库（空结果）**：
    - `indexes app.unknown_table` 返回成功信封，`rows` 为空数组，`row_count` 为 `0`。
    - `indexes unknown_db.users` 同样返回空成功信封。
    - 不触发服务端错误（`INFORMATION_SCHEMA.STATISTICS` 参数化查询对不存在的对象返回空结果，与 `SHOW INDEX FROM` 不同）。
  - **权限不足**：用对 `INFORMATION_SCHEMA.STATISTICS` 缺 `SELECT` 权限的用户连接，exit 7 `PERMISSION_DENIED`（MySQL 1142）。
  - **连接失败**：把 DSN 指向不可达的 host/port，exit 3 `CONNECTION_ERROR`。信封 `details` 不携带 `mysql_error_code`（网络层失败，没有 MySQL 错误可上送）；`sql` 缺失。
  - **超时**：在一个故意慢的 fixture 上跑 `indexes`（或对 `indexes` 跑一个 1 秒 deadline 的 context），exit 6 `TIMEOUT`。错误信封不区分探测超时和主查询超时（v1 D11 / design D5）。
  - **参数校验**（跑完整二进制）：
    - `indexes "app.users; DROP TABLE x"` exit 2 `CONFIG_ERROR`；被拒输入不写入信封。
    - `indexes a.b.c`、`indexes .`、`indexes a.`、`indexes .a`、`indexes a..b` 均 exit 2 `CONFIG_ERROR`。
    - `indexes`（无位置参数）exit 2 `CONFIG_ERROR`。
  - **别名**：`idx app.users` 和 `keys app.users` 返回与 `indexes app.users` 字节相同的信封。单段回退形式同样成立：DSN 为 `mysql://.../mydb` 时，`idx users` 和 `keys users` 都返回与 `indexes mydb.users` 相同的信封。
  - **`<table>` 形式**：DSN `mysql://.../mydb` 下 `indexes users` 返回与 `indexes mydb.users` 相同的信封。

## 4. 文档

- [ ] 4.1 在 `README.md` 加一段 `indexes` 子命令说明，结构与 `describe` 段一致。配一个示例：`sql-cli --dsn ... indexes app.users`。
- [ ] 4.2 在 "Out of scope" 列表里，确认 `EXPLAIN` / `ANALYZE` 与写回数据库仍属范围外（无需改动；只是 sanity check）。

## 5. 验证

- [ ] 5.1 `rtk test go build ./...` 成功。
- [ ] 5.2 `rtk test go test ./...` 通过（仅单元测试，新增的 `indexes_test.go` 不依赖 Docker）。
- [ ] 5.3 `rtk test go test -tags=integration ./...` 在 testcontainers MySQL fixture 上通过。
- [ ] 5.4 `rtk test go test -cover ./...` 确认 `internal/command/indexes.go` 的覆盖率不低于 v1 D7 四个被门控包的目标。（注：`internal/command/` 不在四个被门控包之列，但对新文件高覆盖率仍是期望。）
- [ ] 5.5 `rtk test go vet ./...` 干净。
