# 设计 — `add-config-subcommand`

## 背景张力点

来自 v1 既有 `config` spec（`openspec/changes/v1-mysql-cli/specs/config/spec.md`）：

1. **只读契约**——v1 明确把 `config` 包定义为"加载 + 合并"，无写入。本次破例，**新加** `WriteProfileMap` 是 v1 spec 的扩展。
2. **local vs global 优先级**——v1 D5 是"global 优先、local override"。`config add` 必须明确改哪一个；`config list` 默认 merged 视图。
3. **YAML 文件格式**——v1 文档化的是 `dsns: { name: mysql://url }` 形状。`WriteProfileMap` 必须能重写这个形状而**不破坏**已有 key。

本次决策同时回应这三点。

## 目标 / 非目标

**目标：**

- 新增 `config add [--global] <name> <dsn>` 和 `config list [--local|--global]`，不引入新错误码、不引入新 envelope 字段。
- 写文件原子、可恢复（崩溃不留半截 YAML）；新文件 `chmod 600`。
- 同名覆盖前 stderr 提示旧值。
- 输出走 v1 既有 JSON envelope（成功 / 错误）。

**非目标：**

- `config remove` / `config show` / `config get`。
- stdin / 交互式读 DSN。
- 保留 YAML 注释 / 未知字段。
- local 向上查找 / 多文件链。
- 数据库连接（`config add` / `config list` 不打 DB）。

## 决策

### D1（本次变更）：写文件 = 原子重写整个文件

**决策：** 用 `yaml.Marshal(ProfileConfig{DSNs: merged})` 把整个 map 重写回文件。流程：

1. 读现有文件 → 解析为 `ProfileMap`（若不存在则空 map）。
2. 在内存中 `merged[name] = newDSN`。
3. 写 `os.CreateTemp(dir, ".sql-cli.yaml.*")` 到目标目录。
4. `yaml.Marshal` 到 temp。
5. `temp.Chmod(0o600)`（**仅**当目标文件原本不存在）。
6. `os.Rename(temp, target)`——POSIX 原子。
7. 删除 temp（如 rename 失败）。

**理由：** `os.Rename` 在同一文件系统内是原子的；崩溃只会留 temp 文件，下次写入覆盖。`yaml.Marshal` 重写文件比 Node-tree 保留注释简单 5-10 倍代码；丢注释是有意识取舍。

**备选方案：**
- *Node-tree 插入新键并 Marshal 树*——保留注释和键顺序。**严格更优但代码量翻倍**，超出本次范围。
- *就地 `os.WriteFile` 覆盖*——非原子；崩溃留半截 YAML。
- *追加到文件末尾*——YAML 不支持；需要重写整个 map。

### D2（本次变更）：local 在 CWD 创建，**不**向上找

**决策：** `config add`（无 `--global`）的目标路径是 CWD 的 `.sql-cli.yaml`。若不存在则创建。**不**走 `FindLocalConfigPath` 的向上遍历。

**理由：** `add` 是显式动作——"我在这个项目加个 profile"。向上找会让"我在 `/tmp/yyy/` 加一个，结果写到了 `~/projects/foo/.sql-cli.yaml`"这种事成为可能。`git init` 也不向上找。

**与 `FindLocalConfigPath` 的分歧：** 加载用向上找（"我所在的项目有哪些 profile"），写入只用 CWD（"我显式要写哪里"）。两者**不**是同一函数。

### D3（本次变更）：profile 名格式 `^[A-Za-z0-9_.-]+$`

**决策：** profile 名必须匹配 `^[A-Za-z0-9_.-]+$`（字母数字 + `_` + `.` + `-`）。否则 `CONFIG_ERROR` exit 2。

**理由：** YAML 键可以含几乎任何字符，但当 profile 名出现在命令行、错误信息、shell tab-completion 时，受限字符集能避免转义噩梦。这个集合跟 MySQL host 段、URL 用户名段的允许子集一致。

**为什么拒绝空格和 `/`：** YAML 里 `dev: mysql://...` 的 `dev` 是单 token；含空格需要引号。YAML 里可以用引号，但**键**的引号在错误消息里更难看。

**为什么不复用 v1 `validIdentifier`（`^[A-Za-z0-9_-]+$`）：** profile 名是 YAML 键的语义层级标签，允许 `.`（如 `db.dev` 表示"dev profile for db"）；`validIdentifier` 是 MySQL 标识符白名单，不含 `.`。两者用途不同——profile 字符串**不**会进入 SQL，宽松一点不会引入 SQL 注入面。

### D4（本次变更）：覆盖前 stderr 提示

**决策：** 如果目标 profile 已存在且值不同，**先**写 stderr 一行 `replacing <name>: <old> → <new>`，**再**写入。stdout 的 JSON envelope 仍有 `action: "overwritten"`。

**理由：** agent 重跑同一命令是幂等的——但人手滑覆盖 `dev` 是个灾难。stderr 不污染 stdout 的 JSON（v1 已有"stdout 是协议"约束，stderr 是诊断通道）。

**边界：** 旧值等于新值时（同一命令重跑）不打印——避免噪音。

### D5（本次变更）：`config list` 默认 merged，可 `--local` / `--global` 过滤

**决策：**
- 无 flag → 走 `LoadMergedConfig()`，local 覆盖 global。
- `--local` → 只列 `FindLocalConfigPath(".")` 的文件（不存在时输出空数组，不是错误）。
- `--global` → 只列 `FindGlobalConfigPath()` 的文件（不存在时输出空数组）。

**理由：** 默认 merged 视图回答"我能用什么"（最常见问题）。`--local` / `--global` 回答"我配的来自哪里"（调试用）。

**未来 `--show-origin` 标记**不在本次范围——`source` 字段已经在每条 profile 里。

### D6（本次变更）：错误信封沿用 v1 D10

| 失败模式 | 错误码 | Exit | 来源 |
|---|---|---|---|
| 缺子命令（`sql-cli config`，无 `add`/`list`） | `CONFIG_ERROR` | 2 | command handler |
| `config add` 缺 profile 名 | `CONFIG_ERROR` | 2 | command handler |
| `config add` 缺 DSN | `CONFIG_ERROR` | 2 | command handler |
| `config add` profile 名不匹配 `^[A-Za-z0-9_.-]+$` | `CONFIG_ERROR` | 2 | command handler |
| `config add` DSN 验 `ValidateMySQLURL` 失败 | `CONFIG_ERROR` | 2 | command handler |
| `config list` 收到位置参数 | `CONFIG_ERROR` | 2 | command handler |
| `--local` 和 `--global` 同时给 | `CONFIG_ERROR` | 2 | command handler |
| 写文件失败（磁盘满 / 权限拒绝） | `INTERNAL_ERROR` | 99 | command handler |

**不新增错误码。**

### D7（本次变更）：成功信封形状

**`config add` 成功：**
```json
{
  "ok": true,
  "profile": "dev",
  "dsn": "mysql://u:p@host:3306/db",
  "path": "/abs/path/to/.sql-cli.yaml",
  "action": "created" | "overwritten",
  "elapsed_ms": 3
}
```

**`config list` 成功：**
```json
{
  "ok": true,
  "profiles": [
    {"name": "dev", "dsn": "mysql://...", "source": "/abs/path/to/.sql-cli.yaml"},
    {"name": "staging", "dsn": "mysql://...", "source": "/home/x/.sql-cli/config.yaml"}
  ],
  "count": 2,
  "elapsed_ms": 1
}
```

**实现：** handler 直接 `json.NewEncoder(w).Encode(map[string]any{...})`——不走 `output.WriteSuccess`，因为 v1 D2 的 envelope 形状（`columns`/`rows`/`row_count`）不适用。`config add` / `config list` 产生**自由形状**对象，根必有 `"ok": true`；agent 判别式只看 `ok` 字段。错误仍走 `output.WriteError`。这是 v1 D2 的有意破例——`help` / `version` 已走纯文本，`config` 类同样不契合"产生数据子命令的列状信封"模型。

## 输出 schema

**`config add`：**
```json
{
  "ok": true,
  "profile": "<string>",
  "dsn": "<string>",
  "path": "<string>",
  "action": "created" | "overwritten",
  "elapsed_ms": <int>
}
```

**`config list`：**
```json
{
  "ok": true,
  "profiles": [
    {
      "name": "<string>",
      "dsn": "<string>",
      "source": "<string>"
    }
  ],
  "count": <int>,
  "elapsed_ms": <int>
}
```

`source` 是绝对路径。`count` 等于 `len(profiles)`。

## 风险 / 权衡

- **[风险] 重写文件丢注释** → *缓解*：在 help 和 README 明确说明。文档化。
- **[风险] 误覆盖 profile** → *缓解*：stderr 打印旧值；JSON 里 `action` 字段显式标记。
- **[风险] agent 写脚本时不希望 stderr 噪音** → *缓解*：旧值等于新值时不打印。
- **[风险] `chmod 600` 在某些文件系统上失败** → *缓解*：`INTERNAL_ERROR` exit 99 上送，但写入仍完成（与 spec 同步：SHOULD 0600，文件系统不支持时 INTERNAL_ERROR 报告）。
- **[风险] CWD 不可写** → *缓解*：`os.WriteFile` 报错 → `INTERNAL_ERROR`，details 含系统错误。

## 错误映射

见 D6。

## 验证

- `rtk test go build ./...` 成功
- `rtk test go test ./...` 通过
- `rtk test go vet ./...` 干净
- 集成测试（需要 `os.Chmod` 行为）跑在临时目录下，不污染 home
