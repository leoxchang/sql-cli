## ADDED Requirements

### Requirement: `indexes <db>.<table>` 子命令
`indexes` 子命令 MUST 接受恰好一个位置参数，形式是以下两种之一：

1. `<db>.<table>` —— 两段非空、恰好一个 `.` 分隔符，每段匹配正则 `^[A-Za-z0-9_-]+$`。前导段是数据库；末尾段是表。
2. `<table>` —— 一个非空段，匹配正则 `^[A-Za-z0-9_-]+$`。数据库取自已解析 DSN URL 的库段（`mysql://.../<db>`）。若 DSN URL 无库路径，子命令 MUST emit `CONFIG_ERROR`（exit 2），错误信息点出两个回退源（位置参数 `<db>.<table>` 和 DSN URL 路径）。

**单段形式**下，数据库取自 DSN URL 路径。任何不满足按 `.` split 后得到 1 或 2 个非空段的输入（被拒情况包括：0 段、3 段或更多段、存在空段的输入——例如 `.`、`a.`、`.a`、`a..b`）MUST emit `CONFIG_ERROR`（exit 2）且 MUST NOT 触达数据库。错误信封的 `error` 对象 MUST 只含 `code` 和 `message`；`details` MUST 缺失，被拒输入 MUST NOT 回显到信封里。

子命令 MUST 也可被调用为 `idx` 或 `keys`；两者是严格别名，行为无差。

#### Scenario: 列出含复合主键表的索引
- **WHEN** 用户对一张主键为 `PRIMARY KEY (tenant_id, id)`、且有二级 `BTREE` 索引 `(tenant_id, name)` 的表调用 `sql-cli indexes mydb.users`
- **THEN** 响应的 `rows` 每条 `(INDEX_NAME, SEQ_IN_INDEX)` 对应一行，按 `INDEX_NAME` 升序、同索引内 `SEQ_IN_INDEX` 升序排列。前两行 `INDEX_NAME="PRIMARY"`、`NON_UNIQUE=0`、`SEQ_IN_INDEX` 属于 `{1, 2}`、`COLUMN_NAME` 属于 `{"tenant_id", "id"}` 且顺序正确；接下来两行 `INDEX_NAME` 是二级索引名、`NON_UNIQUE=1`、`SEQ_IN_INDEX` 属于 `{1, 2}`、`COLUMN_NAME` 属于 `{"tenant_id", "name"}`。每行 `INDEX_TYPE` 都是 `BTREE`。

#### Scenario: MySQL 5.7 / 8.0 < 8.0.13 兼容
- **WHEN** MySQL server 不暴露 `INFORMATION_SCHEMA.STATISTICS` 上的全部 14 列（`IS_VISIBLE` 为 8.0.0 引入，`EXPRESSION` 为 8.0.13 引入）
- **THEN** 子命令通过对 `INFORMATION_SCHEMA.COLUMNS` 的单条预探测（`WHERE COLUMN_NAME IN ('is_visible', 'expression')`）检测两列的存在性，并根据返回的列名集合选择主查询的投影：
  - 两列均存在（8.0.13+）：完整 14 列 `SELECT`。
  - 仅 `is_visible` 存在（8.0.0–8.0.12）：12 列 + `IS_VISIBLE` + `'' AS EXPRESSION`。
  - 两列均不存在（5.7）：12 列 + `'' AS IS_VISIBLE` + `'' AS EXPRESSION`。
  三种路径下 `columns` 数组的 14 个名字和顺序完全相同。

#### Scenario: 5.7 与 8.0 列集一致且兼容列行为相同
- **WHEN** 任何 agent 对同一个 `<db>.<table>` 在 MySQL 5.7 server 或 MySQL 8.0.13+ server 上查询（即同一契约跨两种受支持部署成立）
- **THEN** 三种路径下 `columns` 数组完全一致（同样的 14 个名字、同样的顺序）。每行 `EXPRESSION` 值都是空字符串（5.7 兼容路径投影 `''`）。每行 `IS_VISIBLE` 值在 5.7 上为空字符串（兼容路径投影 `''`），在 8.0.13+ 上为 `"YES"` 或 `"NO"`。`columns` 数组和 14 个列名跨版本稳定，agent 的下游解析器不需要按 server 版本分支。

#### Scenario: 服务端 `EXPRESSION` 有值（8.0.13+）
- **WHEN** MySQL server 是 8.0.13 或更新版本，且该表至少有一个基于表达式的索引
- **THEN** 响应中该索引对应行携带非空 `EXPRESSION` 值（即 server 报告的索引表达式文本）。

#### Scenario: `<db>.<table>` 形式
- **WHEN** 用户在 DSN 无库路径的情况下调用 `sql-cli --dsn mysql://u:p@h:3306/ indexes mydb.users`
- **THEN** 子命令把 `mydb` 当作数据库、`users` 当作表，跑探测和主查询，返回成功信封。不需要回退到 DSN URL 路径。

#### Scenario: `<table>` 形式回退到 DSN 库
- **WHEN** 用户在位置参数中没有数据库的情况下调用 `sql-cli --dsn mysql://u:p@h:3306/mydb indexes users`
- **THEN** 子命令把 `mydb`（来自 DSN URL 路径）当作数据库、`users` 当作表，返回与 `indexes mydb.users` 相同字节的成功信封。

#### Scenario: `<table>` 形式 + DSN 无库路径
- **WHEN** 用户在 DSN 无库路径的情况下调用 `sql-cli --dsn mysql://u:p@h:3306/ indexes users`
- **THEN** CLI exit 2 `CONFIG_ERROR`，错误信息点出两个回退源（位置参数 `<db>.<table>` 形式和 DSN URL 路径）。

#### Scenario: 三段或更多 `.`-分隔段
- **WHEN** 用户调用 `sql-cli indexes a.b.c`
- **THEN** CLI exit 2 `CONFIG_ERROR`，指出参数必须按 `.` split 后恰好得到两个非空段。

#### Scenario: 参数含不允许字符
- **WHEN** 用户调用 `sql-cli indexes "mydb.users; DROP TABLE x"`
- **THEN** CLI exit 2 `CONFIG_ERROR`，不发查询。错误信封的 `error` 对象只含 `code` 和 `message`；`details` 缺失，被拒输入不回显。

#### Scenario: 空输入
- **WHEN** 用户在没有位置参数的情况下调用 `sql-cli --dsn ... indexes`
- **THEN** CLI exit 2 `CONFIG_ERROR`，指出需要表标识符。

#### Scenario: 空字符串参数
- **WHEN** 用户调用 `sql-cli indexes ""`（位置参数为空字符串）
- **THEN** CLI exit 2 `CONFIG_ERROR`。错误信封的 `error` 对象只含 `code` 和 `message`；`details` 缺失，被拒输入不回显。

#### Scenario: `idx` 与 `keys` 别名
- **WHEN** 用户调用 `sql-cli idx mydb.users` 或 `sql-cli keys mydb.users`
- **THEN** 响应信封与 `sql-cli indexes mydb.users` 字节相同。

#### Scenario: 表或库不存在
- **WHEN** 用户调用 `sql-cli indexes mydb.unknown_table`（表不存在）或 `sql-cli indexes unknown_db.users`（库不存在）
- **THEN** 子命令返回成功信封，`rows` 为空数组，`row_count` 为 `0`。`INFORMATION_SCHEMA.STATISTICS` 的参数化查询对不存在的 `<db>` / `<table>` 不触发服务端错误（与 `SHOW INDEX FROM` 行为不同）。

#### Scenario: 对 `INFORMATION_SCHEMA.STATISTICS` 权限不足
- **WHEN** 已连接 MySQL 用户对 `INFORMATION_SCHEMA.STATISTICS` 缺 `SELECT` 权限（MySQL 错误 1142）
- **THEN** CLI exit 7 `PERMISSION_DENIED`。

#### Scenario: 输出列集
- **WHEN** 子命令返回成功信封
- **THEN** `columns` 是一个 14 元素的 `{"name": <string>, "type": <string>}` 数组，名字按序为：`INDEX_NAME`、`NON_UNIQUE`、`SEQ_IN_INDEX`、`COLUMN_NAME`、`COLLATION`、`CARDINALITY`、`SUB_PART`、`PACKED`、`NULLABLE`、`INDEX_TYPE`、`COMMENT`、`INDEX_COMMENT`、`IS_VISIBLE`、`EXPRESSION`。每个 `type` 取 `Rows.ColumnTypes().DatabaseTypeName()` 的原始值（v1 D12 重申）。
- **AND** 以下列的 null 语义成立（agent 解析 JSON 值时需区分 `null` 与 `""`）：
  - **永不 null**：`INDEX_NAME`、`NON_UNIQUE`（`0`/`1` 整数）、`SEQ_IN_INDEX`（正整数）、`COLUMN_NAME`、`NULLABLE`（`"YES"`/`"NO"`）、`INDEX_TYPE`、`IS_VISIBLE`（8.0+ 上 `"YES"`/`"NO"`；5.7 兼容路径上为空字符串 `""`）、`EXPRESSION`（非表达式索引及 5.7 / 8.0 < 8.0.13 兼容路径上为空字符串 `""`，不为 `null`）。
  - **可能为 `null`**：`COLLATION`（HASH/FULLTEXT/SPATIAL 索引上为 `null`）、`CARDINALITY`（优化器无估计时为 `null`）、`SUB_PART`（索引整列时为 `null`）、`PACKED`（未启用 `PACK_KEYS` 时为 `null`）、`COMMENT`、`INDEX_COMMENT`。
