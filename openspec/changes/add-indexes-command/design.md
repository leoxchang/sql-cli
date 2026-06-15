# 设计 — `add-indexes-command`

## 背景

v1 的 `describe` 子命令设计上是窄的：只返回列级元数据，外加一个 `Key` 字段提示该列是否参与了索引。一个 agent 想要检视真实的 MySQL schema 时需要更多——例如，重建复合主键的列顺序，或判断一个二级索引是 BTREE 还是 FULLTEXT。本次变更以最小新增面积回答这些疑问：新增一个直接暴露 `INFORMATION_SCHEMA.STATISTICS` 行的子命令。

设计上的张力点都来自 v1 的 D1 / D2 / D8：

1. **数据来源**：`SHOW INDEX FROM` 还是 `INFORMATION_SCHEMA.STATISTICS` 查询。
2. **跨版本兼容性**：MySQL 5.7 在 `STATISTICS` 上不暴露 `IS_VISIBLE`（8.0.0 引入）和 `EXPRESSION`（8.0.13 引入）；8.0.0–8.0.12 有 `IS_VISIBLE` 但没有 `EXPRESSION`；8.0.13+ 两列齐全。输出形状必须保持稳定。
3. **信封一致性**：v1 D2 承诺一种成功信封形状 `{"ok": true, "columns": [...], "rows": [...], "row_count": N, "elapsed_ms": N}`。新子命令必须契合这个形状，不能加 `indexes` 扩展字段。

以下决策同时回应了这三点。

## 目标 / 非目标

**目标：**

- 新增一个子命令 `indexes <db>.<table>`，加两个别名（`idx`、`keys`），输出 `INFORMATION_SCHEMA.STATISTICS` 行的扁平列表。
- 跨 MySQL 5.7 / 8.0（任意补丁版本）输出形状完全一致。
- 参数语法与 `describe` 对齐：双段 `<db>.<table>` 形式 + 单段 `<table>` 形式（DSN 有库路径时单段即为 table，DSN 无库路径时回退到 `<table>` 错误信息）。
- 完全复用 v1 D2 错误码、v1 D8 白名单、v1 D11 `elapsed_ms` 语义。

**非目标：**

- 新增 `ErrorCode` 常量（D2 契约保持封闭）。
- 新增 `Envelope` 字段（D2 契约保持封闭）。
- 索引级嵌套表示（会破坏二维 `columns` / `rows` 模型）。
- 跨表列出（`indexes <db>` 不带表名）。
- `EXPLAIN` / `ANALYZE` 集成。
- 任何向数据库写入的操作。

## 决策

### D1（本次变更）：数据源 — `INFORMATION_SCHEMA.STATISTICS`

**决策：** 直接查询 `INFORMATION_SCHEMA.STATISTICS`，使用参数化 `WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`。按 `INDEX_NAME, SEQ_IN_INDEX` 排序，保证同一索引的行连续、复合索引内列顺序与行顺序一致。

**理由：** 与 v1 `tables.go` 模式一致（同为查询 `INFORMATION_SCHEMA`），安全性和代码风格已经过审查。返回的 14 列恰好是 agent 需要的元数据；`SHOW INDEX FROM` 不过是同一底层视图的糖衣。参数绑定 + v1 D8 白名单是纵深防御：白名单已经拒掉所有 `[A-Za-z0-9_-]` 之外的字符，绑定只是多加一道保险。

**备选方案：**

- *`SHOW INDEX FROM `<db>`.`<table>``* —— 写法更短，但文档明确指出其输出随 MySQL 版本变化（例如 `EXPRESSION` 列在 8.0.13 才出现，之前静默缺失）。`SHOW` 不支持参数绑定，意味着要重新引入 `fmt.Sprintf` 加反引号包裹——严格劣于 `STATISTICS` 路线。
- *手工 JOIN `STATISTICS` 与 `TABLES` 以取表注释* —— 超出范围。`describe` 已经带表注释；agent 想要两者时连续调用 `describe` 和 `indexes` 即可。

### D2（本次变更）：两步查询以兼容 `IS_VISIBLE` 和 `EXPRESSION`

**决策：** 运行主查询前，先跑一个无参数探针，一次查出 `INFORMATION_SCHEMA.STATISTICS` 上 `IS_VISIBLE` 和 `EXPRESSION` 两列的存在性：

```sql
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'information_schema'
  AND TABLE_NAME   = 'statistics'
  AND COLUMN_NAME  IN ('is_visible', 'expression')
```

返回行集合决定主查询的列投影：

| 探测返回的列名 | server 版本 | 主查询第 13–14 列 |
|---|---|---|
| `is_visible` + `expression` | 8.0.13+ | `IS_VISIBLE`, `EXPRESSION`（完整 14 列） |
| `is_visible` | 8.0.0–8.0.12 | `IS_VISIBLE`, `'' AS EXPRESSION` |
| （空） | 5.7 | `'' AS IS_VISIBLE`, `'' AS EXPRESSION` |

三种路径下 `columns` 数组的 14 个名字和顺序完全相同。

上述字面量（`'information_schema'`、`'statistics'`、`'is_visible'`、`'expression'`）是 MySQL 内部元数据名，**不**取自用户输入。v1 D8 白名单（及参数绑定）保护的是用户可控的标识符，不是内部 catalog 名；在这里做参数绑定是没有威胁模型的形式化动作。

**理由：** MySQL 5.7 在 `STATISTICS` 上既没有 `IS_VISIBLE`（8.0.0 引入）也没有 `EXPRESSION`（8.0.13 引入）。8.0.0–8.0.12 有 `IS_VISIBLE` 但没有 `EXPRESSION`。只有 8.0.13+ 两列齐全。直接查询缺少的列会返回 1064 错误。预探测让错误路径保持线性：主查询要么成功、要么走正常的 `ClassifyError` 流程失败。

**为什么合并为单条探测而非两次独立探测：** 两条 `SELECT COUNT(*) ... AND COLUMN_NAME = ?` 多一次往返。用 `IN (...)` 合并为一条，返回值集合（0/1/2 行）即可区分三种版本区间，一次往返足够。

**探测开销：** 每次 `indexes` 调用对 `INFORMATION_SCHEMA.COLUMNS`（InnoDB 元数据视图）多一次往返。健康服务器上亚毫秒，远低于 30 秒超时预算。不做连接级缓存——见 D3。

**备选方案：**

- *策略 B（在进程内缓存 server 版本）* —— 需要引入 driver-version 句柄，这是 v1 D1 原则明确拒绝的抽象。
- *策略 C（先跑完整列集，1064 时回退）* —— 非确定性行为：相同输入会因 server 版本走两条不同路径，单元测试的成功路径会变成 server-version 依赖。
- *永远走最保守兼容路径（`'' AS IS_VISIBLE, '' AS EXPRESSION`）* —— 5.7 / 老 8.0 能跑，但 8.0+ 丢失 `IS_VISIBLE` 的真实 `YES`/`NO` 值和 `EXPRESSION` 的表达式文本。信息损失。
- *永远查 14 列完整集* —— 5.7 上 `IS_VISIBLE` 和 `EXPRESSION` 都不存在，直接 1064 失败。8.0.0–8.0.12 上 `EXPRESSION` 不存在，也失败。

### D3（本次变更）：不缓存探测结果

**决策：** 每次 `indexes` 调用都跑探测。不在 `sync.Map` / driver-handle 字段里跨调用缓存。

**理由：** 每次 `sql-cli` 调用都是一次性进程（v1 D5 明确拒绝连接池）。一个 agent 一次运行里通常只调几次 `indexes`，不是几百次。1–2 毫秒的探测成本在 30 秒超时预算下可忽略，不缓存也保证 command handler 无状态。如果未来工作负载真的需要缓存，可以在 `mysqldrv` 边界加上（D1 原则："包即扩展点"——不需要在 command-handler 层抽象）。

### D4（本次变更）：参数语法与 `describe` 对齐

**决策：** 接受恰好一个位置参数。两种合法形式：

- `<db>.<table>` —— 两段都匹配 `^[A-Za-z0-9_-]+$`，恰好一个 `.` 分隔符。与 `describe` 规则一致。
- `<table>` —— 表段匹配 `^[A-Za-z0-9_-]+$`；数据库取自 DSN URL 路径。若 DSN 无库路径则回退 `CONFIG_ERROR`（exit 2），错误信息点出两个回退源（位置参数 `<db>.<table>` 和 DSN URL 路径）。

**拒收形式**（均 `CONFIG_ERROR` exit 2，不打 DB）：

- 缺位置参数（`len(args) == 0`）。
- 空字符串参数（`args[0] == ""`）。
- 任何 `.`-split 后不产生 1 或 2 个非空段的输入：例子包括 `a.b.c`（三段）、`.`（两段皆空）、`a.` 与 `.a`（两段，一段为空）、`a..b`（三段，中间为空）。
- 任何段包含 `[A-Za-z0-9_-]` 之外的字符。

**理由：** 与 `describe` 语法完全一致，意味着熟悉 `describe` 的 agent 在常见情况下不需要学新规则（双段形式和单段+DSN 库路径回退形式都一致）。

### D5（本次变更）：`elapsed_ms` 语义沿用 v1 D11

**决策：** `elapsed_ms` 是从 handler 入口到读完最后一行之间的墙钟毫秒数，与 `tables` / `describe` / `query` 一致。探测查询也在这段窗口内（handler 入口 → 主查询执行 → 读完最后一行）。

**理由：** v1 D11 给出唯一定义；为 `indexes` 引入另一个时钟会制造"两个字段、一个数字"的问题——D11 明确拒绝过这种做法。

### D6（本次变更）：列类型字符串保留 server 报告的大小写（重申 v1 D12）

**决策：** 14 个 `columns` 条目的 `type` 字段直接取 `Rows.ColumnTypes()` 的 `DatabaseTypeName()` 字符串，driver 怎么返回就怎么用，不做大小写归一化。

**理由：** v1 D12 是项目级规则；新命令沿用。v1 D12 适用于每个 column 条目的 `type` 字段，**不**适用于 `name` 字段：14 个 `name` 是 SQL `SELECT` 列表中的字面量、由本 spec 固定，**不**取自 `Rows.Columns()`。MySQL 8.0 上 `go-sql-driver/mysql` driver 恰好把这些名字大写返回（`INDEX_NAME`、`NON_UNIQUE`…），所以字面量和 driver 输出吻合；若未来某个 driver 用其他大小写返回，agent 会看到不一致，而本 spec 的契约是"14 个 name 是下方 Output Schema 段列出的字面量"，**不**是"name 必须匹配 driver 的大小写"。

## 输出 schema

成功信封（对 v1 形状无扩展）：

```json
{
  "ok": true,
  "columns": [
    {"name": "INDEX_NAME",     "type": "<server>"},
    {"name": "NON_UNIQUE",     "type": "<server>"},
    {"name": "SEQ_IN_INDEX",   "type": "<server>"},
    {"name": "COLUMN_NAME",    "type": "<server>"},
    {"name": "COLLATION",      "type": "<server>"},
    {"name": "CARDINALITY",    "type": "<server>"},
    {"name": "SUB_PART",       "type": "<server>"},
    {"name": "PACKED",         "type": "<server>"},
    {"name": "NULLABLE",       "type": "<server>"},
    {"name": "INDEX_TYPE",     "type": "<server>"},
    {"name": "COMMENT",        "type": "<server>"},
    {"name": "INDEX_COMMENT",  "type": "<server>"},
    {"name": "IS_VISIBLE",     "type": "<server>"},
    {"name": "EXPRESSION",     "type": "<server>"}
  ],
  "rows": [
    ["PRIMARY", 0, 1, "id", "A", 12345, null, null, "NO",  "BTREE", "",      "",      "YES", ""],
    ["idx_name", 1, 1, "name", "A", 9876, null, null, "YES", "BTREE", "",      "",      "YES", ""]
  ],
  "row_count": 2,
  "elapsed_ms": 12
}
```

**列顺序由** SQL `SELECT` 列表固定。`IS_VISIBLE` 永远是第 13 列、`EXPRESSION` 永远是第 14 列；在 5.7 上这两列的值恒为空字符串（兼容路径投影 `'' AS IS_VISIBLE, '' AS EXPRESSION`），在 8.0.0–8.0.12 上 `EXPRESSION` 恒为空字符串而 `IS_VISIBLE` 取真实值。任何情况下都**不**会是 `null`。

**每列的 null / 空 / 零语义。** 消费本信封的 agent 需要知道哪些列可能出现 JSON `null`、哪些列始终是非空标量。下表描述 `INFORMATION_SCHEMA.STATISTICS` 的 server 报告行为。`Nullable?` 那一列问的是"`rows[i][j]` 是否可能为 `null`"。

| # | 列 | 类型（server-reported） | 可空？ | null 语义 |
|---|---|---|---|---|
| 1 | `INDEX_NAME` | `VARCHAR` | 否 | 始终是索引名（主键为 `PRIMARY`，其余为具名索引） |
| 2 | `NON_UNIQUE` | `BIGINT` | 否 | `0` = UNIQUE；`1` = 允许重复。永不 null。 |
| 3 | `SEQ_IN_INDEX` | `BIGINT` | 否 | 索引内 1-based 的列位置。永不 null。 |
| 4 | `COLUMN_NAME` | `VARCHAR` | 否 | 列名；表达式索引上此列为空，由 `EXPRESSION` 承载表达式文本（8.0.13+）。 |
| 5 | `COLLATION` | `VARCHAR` | 是 | BTREE 上为 `A` / `D`；HASH、FULLTEXT、SPATIAL 上为 `null`。 |
| 6 | `CARDINALITY` | `BIGINT` | 是 | 估计的唯一值数；若优化器无估计则为 `null`（罕见，schema 变动后可能出现）。 |
| 7 | `SUB_PART` | `BIGINT` | 是 | 索引前缀长度；索引整列时为 `null`。 |
| 8 | `PACKED` | `VARCHAR` | 是 | 除非启用 `PACK_KEYS=1`（8.0+ 罕见），否则为 `null`。 |
| 9 | `NULLABLE` | `VARCHAR` | 否 | 列允许 NULL 时为 `YES`，否则为 `NO`。永不 null。 |
| 10 | `INDEX_TYPE` | `VARCHAR` | 否 | `BTREE` / `FULLTEXT` / `SPATIAL` / `HASH`。永不 null。 |
| 11 | `COMMENT` | `VARCHAR` | 是 | 索引级信息性注释；未设置时为 `null`。 |
| 12 | `INDEX_COMMENT` | `VARCHAR` | 是 | 索引定义里的 `COMMENT='...'`；未设置时为 `null`。 |
| 13 | `IS_VISIBLE` | `VARCHAR` | 否 | `YES` / `NO`（8.0+）。5.7 兼容路径上为空字符串 `""`（投影 `'' AS IS_VISIBLE`）。永不 null。 |
| 14 | `EXPRESSION` | `VARCHAR` | 否 | 表达式索引的表达式文本；非表达式索引以及 MySQL 5.7 / 8.0 < 8.0.13（兼容路径）上为空字符串。永不 null。 |

`null` 由 Go 的 `database/sql` 在遇到 SQL `NULL` 时返回 `nil` 产生；输出层（v1 D12 行为）把它序列化为 JSON `null`。空字符串来源于兼容路径的 `'' AS IS_VISIBLE` / `'' AS EXPRESSION` 投影、或 server 对非表达式索引报告空 `EXPRESSION` 值。两者在 JSON 里**可区分**：`null` 和 `""` 不是同一个值。

## 错误映射

| 失败模式 | 错误码 | Exit | 来源 |
|---|---|---|---|
| 缺位置参数（`len(args) == 0`） | `CONFIG_ERROR` | 2 | command handler |
| 空字符串参数（`args[0] == ""`） | `CONFIG_ERROR` | 2 | command handler |
| 参数段包含 `[A-Za-z0-9_-]` 之外的字符 | `CONFIG_ERROR` | 2 | command handler |
| 参数的 `.`-split 产生 3 段或更多（含 `.` / `a.` / `.a` / `a..b` 这些 corner） | `CONFIG_ERROR` | 2 | command handler |
| `<table>` 形式下 DSN 库路径为空 | `CONFIG_ERROR` | 2 | command handler |
| 表或库不存在 | （无错误） | 0 | `INFORMATION_SCHEMA.STATISTICS` 参数化查询对不存在的 `<db>` / `<table>` 返回空结果集，不触发服务端错误。子命令输出成功信封 `rows: []`, `row_count: 0`。 |
| 语法错误或权限检查（1064） | `QUERY_ERROR` | 1 | mysqldrv classifier |
| 30 秒超时（3024 或 context deadline） | `TIMEOUT` | 6 | mysqldrv classifier |
| 鉴权失败（1045） | `AUTH_ERROR` | 4 | mysqldrv classifier |
| 对 `INFORMATION_SCHEMA.STATISTICS` 权限不足（1142） | `PERMISSION_DENIED` | 7 | mysqldrv classifier |
| 连接失败 | `CONNECTION_ERROR` | 3 | mysqldrv classifier |
| 未预期（如探测意外失败） | `INTERNAL_ERROR` | 99 | command handler |

无新增错误码。错误信封的 `details` 沿用 v1 D10 携带 `mysql_error_code` 和 `sql`。

## 风险 / 权衡

- **[风险] 5.7 / 老 8.0 加入集成测试矩阵** →
  *缓解*：在现有 `mysql:8.0` fixture 之外新增一个 `mysql:5.7` testcontainer，用以跑兼容探测路径。这与 v1 D7 "用真实 MySQL 测试" 的原则一致；不引入 mock 或 stub driver。若机器不便拉 5.7 镜像可跳过该 fixture（`mysql:8.0` 是默认且必需的）。
- **[风险] 探测多一次往返** → *缓解*：健康服务器上约 1 毫秒；远低于 30 秒预算。一次性进程不做缓存（D3）。
- **[风险] 两次 `INFORMATION_SCHEMA` 读取，一次足矣** → *缓解*：没有 server 版本检测就没法压缩成一次，而 server 版本检测会引入 v1 D1 禁止的抽象。
- **[风险] `CARDINALITY` 是估计值，可能过期** → *缓解*：这是 MySQL 自身的属性，不是 CLI 的。在面向用户的 README 里写明——agent 读取该值时不应视为 ground truth，先 `ANALYZE TABLE` 再用。
- **[风险] 14 列在终端里太宽** → *缓解*：本工具面向 agent，不面向人。Agent 走 JSON 解析，不靠肉眼。`tables` / `describe` 同样不受终端宽度约束。

## Spec 修订引用

本次变更随附对进行中的 v1 spec 的一处小型文档修订。该修订**不**是新能力的一部分；它是纯文档修正，让 v1 归档落地时标识符白名单措辞前后一致。v1 spec 已修订为与实际代码对齐：

- `v1-mysql-cli/specs/query-commands/spec.md` —— `tables` requirement 现在使用 `^[A-Za-z0-9_-]+$`；`describe` requirement 现在描述"split on `.` into exactly two non-empty segments"规则。具体当前文案见该 v1 文件。
- `v1-mysql-cli/design.md` D8 —— 同样的正则修订 + D8 决策文本里的段数说明。
- `v1-mysql-cli/tasks.md` 任务 6.4 和 6.5 —— 同样的正则修订。
- `v1-mysql-cli/proposal.md` line 13 —— 同样的正则修订。

该修订不改变任何行为：`internal/command/tables.go:19` 实际代码已经使用 `^[A-Za-z0-9_-]+$`，`internal/command/describe.go:70-97` 的实际点号计数逻辑也要求恰好两段。修订让 spec 与代码对齐。
