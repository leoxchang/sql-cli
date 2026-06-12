# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

`sql-cli` 是面向 AI agent 的只读 MySQL 命令行工具。核心特征：

- 单 Go 二进制文件，JSON-only 输出（stdout 始终是单一 JSON 信封）
- 9 个标准化错误码，与 9 个退出码一一对应，便于 agent 编程处理
- 内置 SQL 关键字扫描器，拦截 INSERT/UPDATE/DELETE/DROP 等写操作
- 支持 stdin 读取 SQL（heredoc 多语句）
- 固定 30 秒超时；通过 `mysql://` URL DSN 连接

**重要的设计原则（来自 `openspec/changes/v1-mysql-cli/design.md`）：**

- 包边界即为扩展点（`internal/mysqldrv/`），不为单一实现定义 `Driver` 接口
- 关键字扫描器是 defense-in-depth，主防线是 MySQL 账户的 SELECT 权限
- 错误的 `message` 字段是给人看的，会变化；agent 只能解析 `code`、`mysql_error_code`、`details` 结构化字段

## 常用命令

> **重要**：本项目中的 `go` 命令必须通过 `rtk` 包装器执行（`rtk` 是 token 优化的 CLI 代理），不允许直接调用 `go`。
> 已发现的可用形式：`rtk test go <subcommand>`、`rtk ls`、`rtk read` 等。

```bash
# 构建二进制
rtk test go build -o sql-cli ./cmd/sql-cli

# 单元测试（不需要 Docker）
rtk test go test ./...

# 集成测试（需要 Docker，会拉取 mysql:8.0 镜像）
rtk test go test -tags=integration ./...

# 跳过集成测试中的 Docker 依赖
SKIP_DOCKER=1 rtk test go test -tags=integration ./...

# 带覆盖率
rtk test go test -cover ./...
rtk test go test -tags=integration -cover ./...

# 静态检查 + 编译验证
rtk test go vet ./...
rtk test go build ./...

# 运行单个包测试
rtk test go test ./internal/safety/...
rtk test go test -tags=integration -run TestSetupMySQLContainer ./internal/testhelpers/...
```

**集成测试要求**：`testcontainers-go` 需要 Docker。`SKIP_DOCKER=1` 可在没有 Docker 的环境下跳过。

**覆盖率门槛（CI 强制）**：`internal/{config,safety,output,mysqldrv}` 四个包 ≥ 80%；`cli/` 和 `command/` 不强制。

## 架构与代码结构

```
cmd/sql-cli/main.go         # 入口，仅调用 cli.Run 并 os.Exit
internal/cli/router.go      # 全局 flag 解析 + 子命令分发 + 帮助/版本处理
internal/command/           # 四个子命令的实现
  databases.go                # SHOW DATABASES
  tables.go                   # SHOW TABLES FROM `<db>`（白名单 + 反引号转义）
  describe.go                 # SHOW FULL COLUMNS FROM `<db>`.`<table>` + 表注释
  query.go                    # 跑安全扫描 → 加 LIMIT → 执行；支持 stdin
internal/config/            # DSN 解析与校验
  resolver.go                 # 优先级: --dsn flag > SQL_CLI_DSN env > 错误
  validator.go                # mysql:// URL 校验 + 转 driver DSN + 提取 db 名
internal/safety/            # SQL 写操作扫描器 + LIMIT 注入器
  scanner.go                  # 状态机: 正常/字符串/行注释/块注释；11 个写关键字
  limit.go                    # SELECT 无 LIMIT 时追加，默认 1000（SQL_CLI_MAX_ROWS 可覆盖）
internal/mysqldrv/          # MySQL 驱动封装（包边界即为未来扩展点）
  driver.go                   # Open(ctx, dsn) (*sql.DB, error)
  classifier.go               # 错误分类: MySQL error number → ErrorCode
internal/output/            # JSON 信封
  types.go                    # ErrorCode 常量 + Envelope/Column/ErrorDetail 结构
  writer.go                   # WriteSuccess / WriteError / WriteEnvelope / WriteFailure
  converter.go                # sql.Rows → []Column + [][]any（[]byte→string，time.Time→RFC3339，列类型大小写保持原样）
internal/testhelpers/       # 仅 integration 标签：testcontainers MySQL 8 容器
```

### 关键数据流（以 `query` 为例）

```
cli.Run
  → config.ResolveDSN (--dsn → SQL_CLI_DSN → 错)
  → dispatch("query", dsn, args)
    → command.HandleQuery
      1. parseQueryArgs  （位置参数 / `-` / `--stdin`）
      2. config.MySQLURLToDriverDSN  （mysql:// URL → driver DSN）
      3. safety.ScanKeywords  （命中 → SAFETY_BLOCKED，exit 5）
      4. safety.AddLimitIfNeeded  （SELECT 无 LIMIT → 追加 1000）
      5. mysqldrv.Open（30s context）
      6. db.QueryContext
      7. output.ConvertRows  （列类型保留 DatabaseTypeName 大小写）
      8. output.WriteSuccess
  → 返回 exit code
```

### 错误码到退出码的映射（来自 `internal/output/types.go:43`）

| ErrorCode          | Exit | 触发场景                                            |
| ------------------ | ---- | --------------------------------------------------- |
| QUERY_ERROR        | 1    | SQL 语法/执行错误（MySQL 1146、1064、2000-2999）     |
| CONFIG_ERROR       | 2    | DSN 缺失/格式错误、空 SQL、非法标识符                |
| CONNECTION_ERROR   | 3    | 网络层连接失败                                      |
| AUTH_ERROR         | 4    | MySQL 1045                                          |
| SAFETY_BLOCKED     | 5    | 写关键字命中                                        |
| TIMEOUT            | 6    | MySQL 3024 或 `context.DeadlineExceeded`            |
| PERMISSION_DENIED  | 7    | MySQL 1044、1142                                    |
| INTERNAL_ERROR     | 99   | 未分类错误（**details 必须为 nil/空**）             |

`INTERNAL_ERROR` 的 details 必须为 nil — 见 `output.NewErrorEnvelope` 与 `mysqldrv.classifyMySQLError` 的 `default` 分支。

### 安全相关约束

- **`tables` 和 `describe` 的标识符白名单**：`^[A-Za-z0-9_-]+$`（只允许字母数字、下划线、连字符；不允许反引号/`.`/`\0`），用反引号包裹构造 SQL
- **DSN 注入防护**：`ValidateMySQLURL` 先检查 `\r`、`\n` 再解析 URL，防止 DSN smuggling
- **安全扫描器状态机**（`safety/scanner.go`）：必须正确处理 `''` 转义、`--` 行注释、`/* */` 块注释；扫描结果统一转为大写

### 写代码时的注意事项

- 不要在 `internal/mysqldrv/` 之外引用 `github.com/go-sql-driver/mysql`（D1：包边界即为扩展点）
- 不要在错误处理中解析 `message` 字段 — 用 `code` / `mysql_error_code`
- 新增子命令需在 `internal/cli/router.go:dispatch` 中加一个 case，并在 `printHelp` 中追加说明
- 输出必须经 `output.WriteSuccess`/`WriteError`/`WriteEnvelope`，不要直接 `fmt.Println` — stdout 隔离是协议级保证（`internal/cli/stdout_cleanliness_test.go` 验证）
- 不要为写操作开新口子；v1.0 设计上拒绝 `--allow-write`
- `elapsed_ms` 是从 handler 入口到最后一行的 wall-clock 时长（含连接建立），调用方传 `time.Since(startTime).Milliseconds()`

## Spec / 设计文档

- `openspec/changes/v1-mysql-cli/design.md` — 核心设计决策 D1-D12（必读）
- `openspec/changes/v1-mysql-cli/proposal.md` — 变更范围、能力清单
- `openspec/changes/v1-mysql-cli/tasks.md` — 实施任务清单
- `openspec/changes/v1-mysql-cli/specs/` — 增量能力规格

## 环境与工具链

- Go 1.26.4（`go.mod`）
- `go` 命令必须用 `rtk` 前缀调用，否则会失败（沙箱未安装 Go 在 PATH 中）
- 集成测试依赖 Docker（mysql:8.0 镜像）
- 项目根有 `.gopath/pkg/mod`（本地 module 缓存）
- 已构建的本地二进制 `sql-cli-test`（8.2M）— 可能是某次实验构建的产物，不要误用
