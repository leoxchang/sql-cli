## ADDED Requirements

### Requirement: `config add` 子命令

`config add` 子命令 MUST 接受恰好两个位置参数 `<name> <dsn>`，把 `<dsn>` 写入到指定作用域的 YAML 配置文件里，作为名为 `<name>` 的 profile。默认作用域是 **local**——CWD 的 `.sql-cli.yaml`；可用 `--global` 改为写到 `~/.sql-cli/config.yaml`（或 v1 D5 优先级最高的 `globalConfigCandidates` 第一个非空候选）。

`<name>` MUST 匹配正则 `^[A-Za-z0-9_][A-Za-z0-9_.-]*$`（首字符字母数字或 `_`，后续字母数字 + `_` + `.` + `-`），否则 emit `CONFIG_ERROR`（exit 2）。`<dsn>` MUST 通过 `ValidateMySQLURL`（v1 D4 契约），否则 emit `CONFIG_ERROR`（exit 2）。

若目标作用域的 YAML 文件不存在，MUST 在对应路径新建一个，权限 **SHOULD** 为 `0600`（POSIX 文件系统的默认值）。若底层文件系统不支持 `chmod`（FAT、SMB 共享等），改用实际可达的最高限制权限并以 `INTERNAL_ERROR` exit 99 报告，但**写入本身**仍可继续。已存在的文件 MUST 保持其权限不变。

同名 profile 已被存在时 MUST 静默覆盖（用新值替换），**且**若旧值与新值不同，MUST 在 stderr 输出一行 `replacing <name>: <old> → <new>`。若旧值等于新值，stderr 不输出。

成功时 stdout MUST 输出 JSON envelope，含 `ok: true`、`profile`、`dsn`、`path`、`action`（`"created"` 或 `"overwritten"`）、`elapsed_ms`。`path` 是绝对路径。

**DSN 掩码：** `dsn` 字段 MUST 是 `mysql://u:****@host:3306/db` 形式——password 段（`user info` 的 `:` 后到 `@` 前）替换为字面量 `****`。文件里存的仍是完整 DSN，掩码仅在 envelope 输出层。**YAML 文件其他顶层键（如 `version: 1`、`default_profile: dev`）在 `add` 后会被擦掉**——`WriteProfileMap` 只重写 `dsns` 字段。

#### Scenario: 新建 local profile
- **WHEN** CWD 没有 `.sql-cli.yaml` 且用户跑 `sql-cli config add dev mysql://u:p@host:3306/db`
- **THEN** exit 0；stdout `action: "created"`；CWD 出现 `.sql-cli.yaml` 含 `dsns: { dev: "mysql://u:p@host:3306/db" }`；文件权限是 `0600`

#### Scenario: 覆盖已有 profile
- **WHEN** CWD `.sql-cli.yaml` 已有 `dsns: { dev: "mysql://old" }` 且用户跑 `sql-cli config add dev "mysql://new"`
- **THEN** exit 0；stdout `action: "overwritten"`；stderr 含 `replacing dev: mysql://old → mysql://new`；文件现在 `dsns: { dev: "mysql://new" }`；权限仍是原值

#### Scenario: 覆盖相同值不打印 stderr
- **WHEN** CWD `.sql-cli.yaml` 已有 `dsns: { dev: "mysql://x" }` 且用户跑 `sql-cli config add dev "mysql://x"`
- **THEN** exit 0；stdout `action: "overwritten"`；stderr **不**含 `replacing`

#### Scenario: 新建 global profile
- **WHEN** `~/.sql-cli/config.yaml` 不存在且用户跑 `sql-cli config add --global prod "mysql://u:p@host:3306/prod"`
- **THEN** exit 0；stdout `action: "created"`；`~/.sql-cli/config.yaml` 出现且权限 `0600`；含 `dsns: { prod: "mysql://..." }`

#### Scenario: 缺 name
- **WHEN** 用户跑 `sql-cli config add "mysql://..."`（缺 name）
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 缺 DSN
- **WHEN** 用户跑 `sql-cli config add dev`（缺 DSN）
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: profile 名含非法字符
- **WHEN** 用户跑 `sql-cli config add "bad name" "mysql://..."`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: DSN 格式错
- **WHEN** 用户跑 `sql-cli config add dev "not-a-url"`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 缺子命令
- **WHEN** 用户跑 `sql-cli config`（无 `add` 或 `list`）
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 未知子命令
- **WHEN** 用户跑 `sql-cli config remove dev`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 写文件失败
- **WHEN** 目标作用域路径所在目录不可写（例：父目录是只读挂载），或磁盘已满
- **THEN** exit 99 `INTERNAL_ERROR`；details 含系统错误（`syscall.EACCES` / `ENOSPC` 等）

### Requirement: `config list` 子命令

`config list` 子命令 MUST 列出当前 agent 可解析的所有 profile。默认走 merged 视图（local + global，local override global）；可用 `--local` 只列 local，可用 `--global` 只列 global。`--local` 和 `--global` 同给 MUST emit `CONFIG_ERROR`（exit 2）。

接收任何位置参数 MUST emit `CONFIG_ERROR`（exit 2）。

成功时 stdout MUST 输出 JSON envelope，含 `ok: true`、`profiles`（数组，每条 `{name, dsn, source}`）、`count`、`elapsed_ms`。`source` 是该 profile 实际所在文件的绝对路径。`dsn` 字段 MUST 是掩码后的形式（`mysql://u:****@host:3306/db`），password 段替换为字面量 `****`，避免 stdout 泄露密码。

无 profile 时 MUST 返回 `profiles: []`, `count: 0`，**不**是错误。

#### Scenario: merged 视图
- **WHEN** CWD `.sql-cli.yaml` 含 `{dev: "mysql://a"}` 且 `~/.sql-cli/config.yaml` 含 `{staging: "mysql://b"}`
- **THEN** `sql-cli config list` 返回 2 条 profile，**按 name 字母序**；`dev` 的 `source` 指向 CWD 文件，`staging` 的 `source` 指向 home 文件

#### Scenario: local override global
- **WHEN** 两边都有同名 profile `dev`，但值不同
- **THEN** `config list` 只返回一条 `dev`，值取 local，`source` 指向 CWD 文件（与 v1 D5 合并语义一致）

#### Scenario: --local 过滤
- **WHEN** 两边都有 profile 且用户跑 `sql-cli config list --local`
- **THEN** 只返回 local 文件里的 profile；忽略 global

#### Scenario: --global 过滤
- **WHEN** 两边都有 profile 且用户跑 `sql-cli config list --global`
- **THEN** 只返回 global 文件里的 profile；忽略 local

#### Scenario: --local 和 --global 同给
- **WHEN** 用户跑 `sql-cli config list --local --global`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 无 profile
- **WHEN** CWD 没有 `.sql-cli.yaml` 且 home 也没有 config
- **THEN** `sql-cli config list` 返回 `{"ok": true, "profiles": [], "count": 0, "elapsed_ms": <n>}`；exit 0

#### Scenario: 只有 local
- **WHEN** CWD `.sql-cli.yaml` 含 `{dev: "mysql://a"}`，但 home 没有 config 文件
- **THEN** `sql-cli config list` 返回 1 条 profile `dev`，`source` 指向 CWD 文件

#### Scenario: 只有 global
- **WHEN** home `~/.sql-cli/config.yaml` 含 `{staging: "mysql://b"}`，但 CWD 没有 `.sql-cli.yaml`
- **THEN** `sql-cli config list` 返回 1 条 profile `staging`，`source` 指向 home 文件

#### Scenario: 位置参数被拒
- **WHEN** 用户跑 `sql-cli config list dev`
- **THEN** exit 2 `CONFIG_ERROR`
