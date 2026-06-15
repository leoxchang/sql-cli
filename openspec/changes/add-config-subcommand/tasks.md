# 任务 — `add-config-subcommand`

## 1. config 包：原子写

- [ ] 1.1 在 `internal/config/profile.go` 新增 `WriteProfileMap(path string, pm ProfileMap) error`：
  - `if pm == nil { pm = ProfileMap{} }`——nil map 规范化（**防 #2 后续 handler panic**）
  - 序列化前按 key 字母序排序（**防 #7 map 迭代随机导致输出不确定**）：复用 `sortedKeys(pm)` 或 `sort.Strings(keys)`，用 `yaml.Node` 或手写字节流保序
  - `os.CreateTemp` 在 `filepath.Dir(path)` 创建 temp 文件（模式 `0600`）
  - `yaml.Marshal(ProfileConfig{DSNs: pm})` 写 temp
  - 检查目标文件是否存在：
    - 不存在：`temp.Chmod(0o600)`（已经是 0600 但显式确认）；`os.Rename(temp, path)`
    - 存在：直接 `os.Rename(temp, path)`，**不**改权限
  - 任何步骤失败：清理 temp（`os.Remove`）并返回错误
- [ ] 1.2 单元测试 `internal/config/profile_test.go`：
  - `TestWriteProfileMap_NewFile`：`WriteProfileMap` 到新路径 → 文件存在、内容是有效 YAML、能 `LoadProfileMap` 读回
  - `TestWriteProfileMap_Overwrite`：先写一次 `dsns: {a: "x"}`，再写 `dsns: {a: "y", b: "z"}` → 读回含两个 key
  - `TestWriteProfileMap_NewFileChmod600`：新文件 → 权限是 `0600`（`os.Stat` 检查）。**Windows 上 skip**：`if runtime.GOOS == "windows" { t.Skip("chmod semantics differ on Windows") }`
  - `TestWriteProfileMap_ExistingFilePreservesPerm`：用 `os.Chmod(path, 0o644)` 预设 → `WriteProfileMap` 后仍是 `0644`。**Windows 上 skip**：同上
  - `TestWriteProfileMap_PreservesMapOrder`：用 5 个 key 写入 → 读回时 yaml.Unmarshal 出来的 key 顺序与写入顺序一致
  - `TestWriteProfileMap_EmptyMap`：`WriteProfileMap` 空 map → 文件含 `dsns: {}` 或 `dsns:` 标签——不严格，只要 `LoadProfileMap` 读回得到空 map
  - `TestWriteProfileMap_DirNotExist`：写到一个不存在的目录 → 返回错误

## 2. command handler：`HandleConfig`

- [ ] 2.1 新建 `internal/command/config.go`：
  - 函数签名 `HandleConfig(dsn string, args []string) int`
  - **dispatch 子命令：**
    - `add` → `handleConfigAdd(args)`
    - `list` → `handleConfigList(args)`
    - 空 / 未知 → `CONFIG_ERROR`（exit 2）
  - **`handleConfigAdd(args)`：**
    - 解析 flag：`--global`（bool）
    - 位置参数：`<name> <dsn>`（恰好 2 个，否则 CONFIG_ERROR）
    - profile 名匹配 `^[A-Za-z0-9_.-]+$`（否则 CONFIG_ERROR）
    - `config.ValidateMySQLURL(dsn)`（否则 CONFIG_ERROR，错误含"DSN 格式"）
    - 决定目标路径：
      - `--global`：`config.FindGlobalConfigPath()` 若空则用 `~/.sql-cli/config.yaml`（v1 fallback 3）
      - 默认：CWD 的 `.sql-cli.yaml`
    - 读现有 `ProfileMap`（目标文件不存在时为空 map）
    - 检测是否覆盖：`_, exists := existing[name]` 且 `existing[name] != dsn` → 在 stderr 写 `replacing <name>: <old> → <new>`
    - 合并：`existing[name] = dsn`
    - `config.WriteProfileMap(path, existing)`
    - 写 stdout JSON：`{ok, profile, dsn, path, action, elapsed_ms}`
  - **`handleConfigList(args)`：**
    - 解析 flag：`--local`（bool）、`--global`（bool）
    - 位置参数：空（否则 CONFIG_ERROR）
    - `--local` 和 `--global` 同时给 → CONFIG_ERROR
    - 决定来源：
      - `--local` → `config.FindLocalConfigPath(".")`（空时返回 `ProfileMap{}`）
      - `--global` → `config.FindGlobalConfigPath()`（空时返回 `ProfileMap{}`）
      - 默认 → `config.LoadMergedConfig()`
    - 构造数组：每个 `name` 转 `{name, dsn, source: <abs path>}`，**source 字段就是该 profile 实际所在文件的绝对路径**
    - 写 stdout JSON：`{ok, profiles, count, elapsed_ms}`
  - **JSON 输出不走 `output.WriteSuccess`**——自由形状；handler 直接 `json.NewEncoder(os.Stdout).Encode(map[string]any{...})`。错误仍走 `output.WriteError`。

## 3. router

- [ ] 3.1 `internal/cli/router.go` 的 `dispatch()` 加 `case "config":` → `handleConfig(dsn, args)`
- [ ] 3.2 加包装函数 `handleConfig(dsn string, args []string) int` → `command.HandleConfig(dsn, args)`
- [ ] 3.3 `printHelp()` 加 2 行（`config add` + `config list`），风格与现有子命令一致

## 4. 单元测试

- [ ] 4.1 `internal/command/config_test.go`：
  - **`TestHandleConfig_NoSubcommand`**：空 args → CONFIG_ERROR
  - **`TestHandleConfig_UnknownSubcommand`**：`config foo` → CONFIG_ERROR
  - **`TestHandleConfigAdd_MissingName`**：`config add` → CONFIG_ERROR
  - **`TestHandleConfigAdd_MissingDSN`**：`config add dev` → CONFIG_ERROR
  - **`TestHandleConfigAdd_InvalidProfileName`**：`config add "bad name" mysql://...` → CONFIG_ERROR
  - **`TestHandleConfigAdd_InvalidDSN`**：`config add dev "not-a-url"` → CONFIG_ERROR
  - **`TestHandleConfigAdd_LocalCreatesFile`**：chdir 到 `t.TempDir()`（用 `t.Chdir(tempDir)`，**测试结束自动 restore**），跑 `config add dev mysql://u:p@h:3306/db` → CWD 有 `.sql-cli.yaml` 含该 profile；权限 `0600`
  - **`TestHandleConfigAdd_GlobalUsesHome`**：跑 `config add --global dev mysql://...` 时用 `t.Setenv("HOME", tempHome)` 和 `t.Setenv("XDG_CONFIG_HOME", "")` 和 `t.Setenv("SQL_CLI_CONFIG_DIR", "")` → 文件在 `tempHome/.sql-cli/config.yaml`
  - **`TestHandleConfigAdd_EmptyFileDoesNotPanic`**: 预创建空 `.sql-cli.yaml`（`os.WriteFile(path, []byte{}, 0o600)`）→ add 不 panic，profile 写入成功（**防 #3 nil map panic**）
  - **`TestMaskDSN`**（在 `internal/command/config_test.go`）：
    - `mysql://u:p@h:3306/db` → `mysql://u:****@h:3306/db`
    - 无 password 段原样返回：`mysql://u@h:3306/db`
    - 空 password 不掩码：`mysql://:@h:3306/db` → `mysql://:@h:3306/db`（`:` 后到 `@` 前是空串）
    - 非 mysql scheme 原样返回：`http://u:p@h/db` → 原样
    - 无 scheme 原样返回：`not-a-url` → 原样
  - **`TestHandleConfigAdd_OverwriteStderrAnnounces`**：先 add，再 add 同名不同 dsn → 第二次 stderr 含 `replacing` 字样；第二次 stdout JSON `action` 是 `"overwritten"`
  - **`TestHandleConfigAdd_SameValueNoStderr`**：先 add，再 add 同名同 dsn → stderr 不含 `replacing`；action 是 `"overwritten"`（不区分）
  - **`TestHandleConfigList_Empty`**：无 profile → `profiles: []`, `count: 0`
  - **`TestHandleConfigList_Merged`**：local 一个 + global 一个 → 列表含两个，`source` 各自指向文件
  - **`TestHandleConfigList_LocalOnly`**：`--local` → 不含 global
  - **`TestHandleConfigList_GlobalOnly`**：`--global` → 不含 local
  - **`TestHandleConfigList_LocalAndGlobalMutuallyExclusive`**：两个 flag 同给 → CONFIG_ERROR
  - **`TestHandleConfigList_PositionalArgsRejected`**：`config list extra` → CONFIG_ERROR
  - **`TestHandleConfigList_LocalOverrideGlobal`**：local 含 `dev: mysql://local`，global 含 `dev: mysql://global` → `config list` 返回 1 条 `dev`，value 是 `mysql://local`，source 指向 local 文件
  - **`TestHandleConfigList_OnlyLocal`**：只有 local 有 profile → `config list` 返回 1 条，source 指向 local 文件
  - **`TestHandleConfigList_OnlyGlobal`**：只有 global 有 profile → `config list` 返回 1 条，source 指向 global 文件
  - **`TestHandleConfigAdd_WriteFailureInternalError`**：mock 一个不可写目录（`t.TempDir()` 后再 `os.Chmod(tempDir, 0o444)` + Windows skip）→ `config add dev mysql://...` 返回 exit 99 `INTERNAL_ERROR`

## 5. 集成测试

- [ ] 5.1 `internal/command/config_integration_test.go`（`//go:build integration`）：
  - 跑完整二进制（仿 `describe_integration_test.go` 模式：`exec.Command(binaryPath, ...)`）
  - **`EndToEnd_AddLocal`**: chdir 到 `t.TempDir()`，跑 `sql-cli config add dev mysql://u:p@h:3306/db` → 文件存在、可 `LoadProfileMap` 读回（文件存完整 DSN）；**stdout JSON 的 `dsn` 字段是掩码后的** `mysql://u:****@h:3306/db`
  - **`EndToEnd_AddGlobal`**: 跑 `sql-cli --dsn X config add --global dev mysql://...` → `~/.sql-cli/config.yaml` 含该 profile
  - **`EndToEnd_AddThenList`**: add local + add global → `sql-cli config list` 输出两个，**按 name 字母序**；每条 `dsn` 是掩码后的形式（`mysql://u:****@...`）；`source` 各自指向文件
  - **`EndToEnd_AddOverwritesViaBinary`**: add 两次同名不同 dsn → 第二次 stdout JSON `action: "overwritten"`
  - **`EndToEnd_AddInvalidDSN`**: 错 DSN → exit 2
  - **`EndToEnd_AddInvalidProfileName`**: 错 name → exit 2
  - **`EndToEnd_ChmodOnNewFile`**: add 到新路径 → 权限 0600
  - **`EndToEnd_NoChmodOnExistingFile`**: 预设 0644 → add 后还是 0644

## 6. 文档

- [ ] 6.1 README 加 `config` 子命令段，含 add 和 list 例子
- [ ] 6.2 在 "Configuration File" 段补充"用 `config add` 写入"的链接

## 7. 验证

- [ ] 7.1 `rtk test go build ./...` 成功
- [ ] 7.2 `rtk test go test ./...` 通过
- [ ] 7.3 `rtk test go test -tags=integration ./...` 通过
- [ ] 7.4 `rtk test go vet ./...` 干净
