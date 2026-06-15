## 背景

v1 的 `--profile <name>` 机制允许 agent 通过 YAML 配置文件里的命名 DSN 切换数据库连接（见 `openspec/changes/v1-mysql-cli/specs/config/spec.md`）。但**写入**该配置文件没有工具支持——用户必须手写 YAML。

手写有四个问题：

1. **缩进 / 语法错**。YAML 缩进错就解析失败，新用户常见。
2. **密码出现在命令行**。当前唯一途径是 `sql-cli --dsn mysql://u:p@... config ...` 之类的拼接；`u:p@` 在 shell history 和 `ps` 输出里都能被抓。
3. **没有覆盖保护**。同名 profile 静默覆盖——agent 重跑同一命令是幂等的，但人手滑会丢配置。
4. **无法列出**。用户想"现在可用的 profile 是哪些、从哪来"，只能 `cat ~/.sql-cli/config.yaml`。

新增一个 `config` 命名空间子命令解决这四点：`config add` 写入、`config list` 查看。

## 变更内容

- **新增子命令** `config add [--global] <name> <dsn>` 和 `config list [--local|--global]`，由现有 router 暴露。
- **`config add` 默认作用域 local**：写到 CWD 的 `.sql-cli.yaml`。如果该文件不存在，在 CWD **新建**一个（不向上找——跟 `git init` 在 `/tmp` 跑会留 `.git` 是同一类"留下就留下"的行为）。
- **`config add --global` 写到** `~/.sql-cli/config.yaml`（v1 D5 默认全局路径）。如果该文件不存在，**新建**。
- **写文件是原子的**（`temp file + os.Rename`）。**新建** 时 `chmod 600`；现有文件不改权限。
- **同名 profile 静默覆盖**——但**写入前**在 stderr 打印旧值（`replacing <name>: <old> → <new>`），让 agent 和人都能看到。`config add` 的成功信封里有 `action: "created" | "overwritten"`。
- **`config list` 默认 merged**（local + global，local 优先），可用 `--local` / `--global` 过滤。无 profile 时返回空数组，**不**是错误。
- **JSON 信封**。`add` 成功走成功 envelope；`list` 成功走成功 envelope；所有失败模式走错误 envelope。
- **DSN 验证**：复用 v1 的 `ValidateMySQLURL`——格式错误立即拒绝。
- **Profile 名验证**：`^[A-Za-z0-9_.-]+$`（字母数字、`_`、`.`、`-`）。和 `mysql://` 的 host 段规则一致，避免 YAML 键含特殊字符。

## Capabilities

### 新增能力

- `config`：管理 profile 配置文件。追加 spec 段到 `specs/config/spec.md`。

### 修改能力

无。

## 影响

- **新增 Go 文件** `internal/command/config.go`（约 200 行，沿用 `tables.go` / `describe.go` 模板）。
- **新增 Go 文件** `internal/command/config_test.go`（单元测试，不依赖 driver）。
- **config 包变更**：`internal/config/profile.go` 新增 `WriteProfileMap(path string, pm ProfileMap) error`（原子写、YAML 序列化、chmod 600）；`internal/config/profile.go` 现有 `LoadProfileMap` / `LoadMergedConfig` / `FindGlobalConfigPath` / `FindLocalConfigPath` **不变**。
- **router 改动**：`internal/cli/router.go` 的 `dispatch()` 多一个 `case "config":`；`printHelp()` 多 2 行（`config add` + `config list`）。
- **不新增依赖**。`os.WriteFile` / `os.Rename` / `os.Chmod` 都在 stdlib；YAML 用现有 `gopkg.in/yaml.v3`。
- **README** 多一段 `config` 子命令说明。
- **v1 spec 影响**：无变更。`config` 是新能力，不修改 v1 既有 `config` requirement。

## Out of Scope

- **`config remove`**：用户明确不要。"单纯 add，可覆盖"——`add` 已能删除（旧值被新值覆盖）。
- **`config show <name>`** / **`config get`**：用户明确不要。`list` 已能拿到单个 profile。
- **stdin 读 DSN**：用户明确不要。`config add` 只接受位置参数。
- **交互式提示**：agent 场景下没有 stdin 之外的交互面，位置参数已够。
- **保留 YAML 注释 / 键顺序**：用 `yaml.Marshal` 重写文件——用户的注释会丢。第一次 `add` 之后文件被工具"规整化"。这是有意识的取舍（写 Node-level 树要 3-4 倍代码量）。
- **`config add` 的 local 向上查找**：和 `FindLocalConfigPath` 行为不同（后者向上找）。`add` 只在 CWD 创建——避免"我以为我加到了 dev 的项目配置，结果加到了 ~/projects 里的某个 stale `.sql-cli.yaml`"。
- **profile 嵌套 / 多文件 include**：维持 v1 的"local + global 合并"模型。
- **向已有文件追加行（不重写）**：原子写整个 YAML 文件。

## 依赖

- `os.WriteFile`, `os.Rename`, `os.Chmod` (stdlib)
- `gopkg.in/yaml.v3` (已用)
- `github.com/qiezi999/sql-cli/internal/config` (既有)
- `github.com/qiezi999/sql-cli/internal/output` (既有)
- `github.com/qiezi999/sql-cli/internal/mysqldrv` — **不**用（`config add/list` 不连接数据库）
