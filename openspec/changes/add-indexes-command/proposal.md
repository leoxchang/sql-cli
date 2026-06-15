## 背景

v1 的 `describe` 子命令返回列级元数据，包括 `Key` 字段，取值为 `PRI` / `UNI` / `MUL` / 空之一。该字段告诉 agent **哪些列参与了某种索引**，但不告诉：

- **索引名**（`PRIMARY` 与具名 UNIQUE / 二级索引之分）；
- 复合索引内**列顺序**（`PRIMARY KEY (a, b, c)` 的每一列都显示 `Key=PRI`，顺序信息丢失）；
- **唯一性**作为一等公民（当前只能从 `UNI` / `MUL` 启发式推断）；
- **索引类型**（`BTREE` / `FULLTEXT` / `SPATIAL` / `HASH`）；
- **基数值**（估计的唯一值数，对选择性推理有用）。

新增一个 `indexes` 子命令即可弥合这一缺口：把 `INFORMATION_SCHEMA.STATISTICS` 暴露为扁平的只读列表。这是最窄的增量——一个新子命令，不扩展共享信封，不新增错误码。

## 变更内容

- **新增子命令** `indexes <db>.<table>`（别名：`idx`、`keys`），由现有 router 暴露。参数语法匹配 `describe`：`<db>.<table>` 或 `<table>`（后者回退到 DSN URL 的库段）。
- **两步查询** 以兼容 MySQL 5.7 / 8.0：先用 `INFORMATION_SCHEMA.COLUMNS` 的一条探测查（`WHERE COLUMN_NAME IN ('is_visible', 'expression')`）检测 `IS_VISIBLE`（8.0.0 引入）和 `EXPRESSION`（8.0.13 引入）两列的存在性，再根据返回的列名集合跑对应的 14 列查询（完整 14 列 / 12 列 + `IS_VISIBLE` + `'' AS EXPRESSION` / 12 列 + `'' AS IS_VISIBLE` + `'' AS EXPRESSION`）。三种路径输出列集一致，agent 只解析一种形状。
- **不新增错误码。** 所有失败模式复用 v1 D2 九错误码契约：参数错误为 `CONFIG_ERROR`（exit 2）、服务端错误为 `QUERY_ERROR`（exit 1）、`TIMEOUT`（exit 6）、`AUTH_ERROR`（exit 4）、`PERMISSION_DENIED`（exit 7）、`CONNECTION_ERROR`（exit 3）、`INTERNAL_ERROR`（exit 99）。
- **信封形状不变。** 成功路径仍然输出 `{"ok": true, "columns": [...], "rows": [...], "row_count": N, "elapsed_ms": N}`。`columns` 是一个 14 元素的 `{"name": <column>, "type": <server-reported-type>}` 列表。
- **标识符白名单** 沿用 `describe`：`^[A-Za-z0-9_-]+$` 每段，双段形式下恰好一个 `.` 分隔符。这与本次同步落地的 v1 spec 修订一致（白名单此前文档化为 `^[A-Za-z0-9_]+$`，实际代码早已接受 `-`）。

## Capabilities

### 新增能力

无。`indexes` 是对现有 `query-commands` 能力的追加；其 spec 与 v1 追加位于同一目录 `specs/query-commands/spec.md`。

### 修改能力

- `query-commands`：追加 `Requirement: indexes <db>.<table> subcommand` 块。现有 requirement 不动。

## 影响

- **新增 Go 文件** `internal/command/indexes.go`（约 150 行，沿用 `tables.go` / `describe.go` 模板）。
- **新增 Go 文件** `internal/command/indexes_test.go` 与 `internal/command/indexes_integration_test.go`。单元测试覆盖参数解析、校验、标识符分段以及从已解析参数构造探测 SQL 和三条主 SQL 字符串——这些都能在没有 driver 的情况下测。集成测试在 testcontainers fixture（现有 `mysql:8.0` 加新 `mysql:5.7` 镜像）上同时跑 8.0 成功路径和 5.7 兼容路径。不引入 `sqlmock`、不手写 `*sql.DB` stub——v1 D7 "用真实 MySQL 测试" 的原则在此延伸。
- **router 改动**：`internal/cli/router.go` 的 `dispatch` 里多一个 `case "indexes", "idx", "keys":`，`printHelp()` 多一行。
- **不新增依赖。** 探测和主查询都用 `database/sql` 直接打 `INFORMATION_SCHEMA`。
- **README** 多一段与 `describe` 同型的子命令说明。九错误码表不变。
- **5.7 兼容覆盖**：`mysql:5.7` 镜像加入 testcontainers fixture 集合（与 `mysql:8.0` 并列），让兼容路径跑在真实 5.7 server 上而非 stub。5.7 fixture 是可选的（在不便拉 5.7 镜像的机器上可跳过）；8.0 fixture 是默认且必需的。

## Out of Scope

- **索引级聚合**（一行一索引、列嵌套其中）—— 会破坏 `columns` / `rows` 二维模型。未来工作，不在本次变更范围。
- **存储引擎细节**（InnoDB buffer-pool 页数、索引物理大小）—— `STATISTICS` 不暴露。
- **优化器提示**（`USE INDEX`、`FORCE INDEX`）及 `EXPLAIN` / `EXPLAIN ANALYZE`。v1 设计明确推迟这些。
- **写回数据库**（CREATE INDEX、DROP INDEX、ANALYZE TABLE）。v1 是只读；本次变更不动这个边界。
- **列出库内所有索引**（`<db>` 形式）。`SHOW INDEX` 不带 `WHERE` 会返回库内所有索引——结果集无界。设计要求显式表名。
