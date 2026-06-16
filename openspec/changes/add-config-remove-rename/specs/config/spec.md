## ADDED Requirements

### Requirement: `config remove` 子命令

`config remove` 子命令 MUST 接受恰好一个位置参数 `<name>`，从指定作用域的 YAML 配置文件里删除名为 `<name>` 的 profile。默认作用域是 **local**——CWD 的 `.sql-cli.yaml`；可用 `--global` 改为在 `~/.sql-cli/config.yaml`（或 v1 D5 优先级最高的 `globalConfigCandidates` 第一个非空候选）里删除。

`<name>` MUST 匹配正则 `^[A-Za-z0-9_][A-Za-z0-9_.-]*$`（首字符字母数字或 `_`，后续字母数字 + `_` + `.` + `-`），否则 emit `CONFIG_ERROR`（exit 2）。

`<name>` 不存在时 MUST 视为 idempotent 成功，stdout 输出 `action: "not_found"`，exit 0。**不**写盘。

`<name>` 存在时 MUST 从 YAML 文件的 `dsns` map 里删除该 key，调 `WriteProfileMap` 写回，stdout 输出 `action: "removed"`，exit 0。**不**输出被删的 DSN（避免 envelope 携带可能敏感的连接字符串）。

写文件失败（父目录只读、磁盘满等）MUST emit `INTERNAL_ERROR`（exit 99），details 含系统错误（`syscall.EACCES` / `ENOSPC` 等）。

成功时 stdout MUST 输出 JSON envelope，含 `ok: true`、`action`（`"removed"` 或 `"not_found"`）、`profile`、`path`、`elapsed_ms`。`path` 是绝对路径。

`WriteProfileMap` 沿用 `add` 已有的写入方式——仅重写 `dsns:` 顶层键，**不保留**其他顶层键（如 `version`、`default_profile`）。这是 `add` 时代就已存在的限制（详见 `add-config-subcommand` spec 第 15 行的同样说明），`remove` 不改变这一行为。

**不**支持 merged 视图下的删除：要删哪个作用域必须显式指定（默认 local / `--global`）。

**不**引入 `--yes` / 交互式确认（agent 调用环境无 TTY；副作用可由 `git diff .sql-cli.yaml` / 文件备份回滚）。

#### Scenario: 删除 local 已有 profile
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://u:p@host:3306/db" }` 且用户跑 `sql-cli config remove dev`
- **THEN** exit 0；stdout `action: "removed"`；CWD `.sql-cli.yaml` 不再含 `dev`；其他顶层键保留

#### Scenario: 删除不存在的 profile
- **WHEN** CWD `.sql-cli.yaml` 不存在或不含 `dev` 且用户跑 `sql-cli config remove dev`
- **THEN** exit 0；stdout `action: "not_found"`；**不**创建空文件

#### Scenario: 删除 global profile
- **WHEN** `~/.sql-cli/config.yaml` 含 `dsns: { prod: "..." }` 且用户跑 `sql-cli config remove --global prod`
- **THEN** exit 0；stdout `action: "removed"`，`path` 指向 home config

#### Scenario: local remove 不影响 global 同名
- **WHEN** CWD 和 home 都有同名 `dev` 且用户跑 `sql-cli config remove dev`（默认 local）
- **THEN** exit 0 `action: "removed"`；CWD 文件里 `dev` 消失；home 文件里 `dev` 保留

#### Scenario: 缺 name
- **WHEN** 用户跑 `sql-cli config remove`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: name 含非法字符
- **WHEN** 用户跑 `sql-cli config remove "bad name"`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 未知 flag
- **WHEN** 用户跑 `sql-cli config remove dev --foo`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 多余位置参数
- **WHEN** 用户跑 `sql-cli config remove dev extra`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 写文件失败
- **WHEN** 目标作用域路径所在父目录不可写
- **THEN** exit 99 `INTERNAL_ERROR`；details 含 syscall 错误

### Requirement: `config rename` 子命令

`config rename` 子命令 MUST 接受恰好两个位置参数 `<old> <new>`，在指定作用域的 YAML 配置文件里把名为 `<old>` 的 profile 改名为 `<new>`。默认作用域是 **local**；`--global` 切到 home config。

`<old>` 和 `<new>` MUST 都匹配正则 `^[A-Za-z0-9_][A-Za-z0-9_.-]*$`，否则 emit `CONFIG_ERROR`（exit 2）。

`<old>` 不存在时 MUST 视为 idempotent 成功，stdout `action: "not_found"`，exit 0。**不**写盘。

`<new>` 已存在时 MUST emit `CONFIG_ERROR`（exit 2），`<old>` **不**被改动；message MUST 明确指 `<new>` 冲突。这与 `add` 的"同名静默覆盖"刻意不一致——rename 是不可逆销毁原名场景，严格更安全。

`<old> == <new>` 时 MUST 视为 `unchanged`，exit 0，**不**写盘。

成功 rename（`<old>` 存在、`<new>` 不存在、两者不等）MUST 从 `dsns` map 删 `<old>`、写入 `<new>`，调 `WriteProfileMap` 写回，stdout 输出 `action: "renamed"`、exit 0。

写文件失败 MUST emit `INTERNAL_ERROR`（exit 99）。

成功 envelope 含 `ok: true`、`action`（`"renamed"` / `"not_found"` / `"unchanged"`）、`from`、`to`、`path`、`elapsed_ms`。**不**在 envelope 里泄露原 DSN。

**不**支持跨作用域 rename：`<old>` 和 `<new>` 必须在同一个 YAML 文件里。当前 YAGNI；如未来需要，应拆成 `remove + add` 两个原子操作。

#### Scenario: rename 成功
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://u:p@host:3306/db" }` 且用户跑 `sql-cli config rename dev production`
- **THEN** exit 0；stdout `action: "renamed"`、`from: "dev"`、`to: "production"`；文件含 `production` 键、不含 `dev` 键

#### Scenario: rename old 不存在
- **WHEN** CWD `.sql-cli.yaml` 不含 `dev` 且用户跑 `sql-cli config rename dev production`
- **THEN** exit 0；stdout `action: "not_found"`；**不**创建 `production`

#### Scenario: rename new 已存在
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://a", prod: "mysql://b" }` 且用户跑 `sql-cli config rename dev prod`
- **THEN** exit 2 `CONFIG_ERROR`；文件**不**变（`dev` 仍在，`prod` 值仍是 `mysql://b`）

#### Scenario: rename old == new
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://x" }` 且用户跑 `sql-cli config rename dev dev`
- **THEN** exit 0；stdout `action: "unchanged"`；文件**不**变

#### Scenario: rename global
- **WHEN** `~/.sql-cli/config.yaml` 含 `dsns: { prod: "..." }` 且用户跑 `sql-cli config rename --global prod production`
- **THEN** exit 0；stdout `action: "renamed"`；home config 含 `production` 键、不含 `prod` 键；CWD 文件不变

#### Scenario: 默认 local 作用域不查 global
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://a" }`、home 含 `dsns: { prod: "mysql://b" }` 且用户跑 `sql-cli config rename dev prod`（默认 local）
- **THEN** exit 0 `action: "renamed"`；local 文件含 `prod` 键、不含 `dev` 键；home 文件**不**变（与 add 默认 local 同语义，不跨作用域检查冲突）

#### Scenario: --global 作用域不查 local
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://a" }`、home 含 `dsns: { prod: "mysql://b" }` 且用户跑 `sql-cli config rename --global prod production`
- **THEN** exit 0 `action: "renamed"`；home 文件含 `production` 键、不含 `prod` 键；local 文件**不**变

#### Scenario: 同作用域冲突
- **WHEN** CWD `.sql-cli.yaml` 含 `dsns: { dev: "mysql://a", prod: "mysql://b" }` 且用户跑 `sql-cli config rename dev prod`
- **THEN** exit 2 `CONFIG_ERROR`（local 作用域内 `prod` 已存在）；local 文件不变（`dev` 仍在，`prod` 值仍是 `mysql://b`）

#### Scenario: 跨作用域 rename 被禁止（不允许的 flag 组合）
- **WHEN** 用户跑 `sql-cli config rename --global dev prod` 且 `<dev>` 只存在于 local
- **THEN** exit 2 `CONFIG_ERROR`；message 明确指 `<dev>` 在指定作用域不存在；global 文件**不**变

#### Scenario: 缺参数
- **WHEN** 用户跑 `sql-cli config rename dev`（缺 new）
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: new 含非法字符
- **WHEN** 用户跑 `sql-cli config rename dev "bad name"`
- **THEN** exit 2 `CONFIG_ERROR`

#### Scenario: 未知 flag
- **WHEN** 用户跑 `sql-cli config rename dev prod --foo`
- **THEN** exit 2 `CONFIG_ERROR`

### Requirement: 子命令注册与 help 文本

`HandleConfig` 的 dispatcher MUST 接受 `remove` 和 `rename`，并把它们的 arg slice 转发给 `handleConfigRemove` / `handleConfigRename`。未知子命令仍然 emit `CONFIG_ERROR`（exit 2）。

`printHelp` MUST 追加两行说明：
- `config remove <name> [--global]` — Delete a saved DSN profile
- `config rename <old> <new> [--global]` — Rename a saved DSN profile