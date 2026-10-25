# 开发指南

更新日期：2026-10-25。本文维护环境要求、本地启动、测试与日志排查；架构见 [architecture.md](architecture.md)，发布见 [release.md](release.md)。

## 环境要求

| 工具 | 要求与用途 |
|---|---|
| Go | 1.27.2（`go.mod` 固定 `go 1.27.2`）。 |
| MyGo | v0.5.5，作为 Go tool 固定在 `go.mod`（`tool github.com/egoist/mygo/cmd/mygo`），经 `go tool mygo` 使用，不依赖全局 `mygo`。 |
| Xcode 命令行工具 | macOS 打包与 DMG 所需。 |
| NSIS | 仅在需要本地生成 Windows 安装包时要求；在 Windows 上构建时 MyGo 会自行下载，macOS 交叉构建缺少 `makensis` 时只警告并跳过安装包。 |

项目不使用 Bun、Node.js、WebView2 或 cgo。

## 本地启动

在仓库根目录：

```sh
go tool mygo dev
```

- 开发应用名为 `Lucia Dev`，标识为 `com.lotwt.lucia.dev`，数据、日志与单实例锁都与正式版隔离。
- 修改 Go 代码、`mygo.json`、图标或资源后自动重建并重启；编译失败时保留上一个可运行版本。
- 退出应用或按 Ctrl+C 结束；终端中按 `r` 可手动重建。

直接运行（不经过 MyGo 开发壳）：

```sh
go run ./cmd/lucia
```

这种方式没有应用包，`mygo.App.Version()` 为空，界面显示 `dev`；更新器也处于 `unavailable`。

构建产物的无窗口自检：

```sh
LUCIA_SELFTEST=1 build/darwin-arm64/Lucia.app/Contents/MacOS/Lucia
```

输出形如：

```text
lucia: self-test: version=0.0.1 commit=0123456789ab platform=darwin/arm64 updates=enabled
```

开发运行与未嵌入更新源的构建为 `updates=disabled`；发布构建必须为 `updates=enabled`。Windows 是 GUI 子系统，没有控制台，需用 PowerShell 重定向 stderr 才能看到输出。

## 常用命令

| 命令 | 用途 |
|---|---|
| `gofmt -l .` | 格式检查，无输出即通过。 |
| `go vet ./...` | Go 静态检查。 |
| `go test ./...` | 全部单元测试与 `ui.NewTester` 视图测试。 |
| `go test -race ./internal/updates ./internal/ui ./internal/pet ./internal/petpack ./internal/settings` | 更新状态机、窗口模型、动画缓存、解码与设置 Store 的竞态检查。 |
| `go tool mygo vet .` | MyGo 的 UI 生命周期检查；参数是项目根，不能写成 `./...`。 |
| `go tool mygo build` | 打包当前平台到 `build/<os>-<arch>/`。 |
| `go tool mygo build -platform windows/amd64` | 交叉构建 Windows EXE 与安装包。 |
| `LUCIA_SELFTEST=1 <应用可执行文件>` | 在构建产物上核对版本、提交与更新状态。 |

## 测试范围

- `internal/apppaths`：三平台目录解析与 Windows 本地数据回退。
- `internal/buildinfo`：提交缩写、`-modified` 标记与版本回退。
- `internal/diagnostics`：追加写入、stderr 失败不影响文件、轮转顺序。
- `internal/settings`：默认值合并、旧文件新增字段迁移、原子写入往返、损坏文件回退。
- `internal/updates`：禁用构建不联网、检查结果、失败可见、并发拒绝、安装进度、安装失败保留重试、自动检查启动。
- `internal/pet`：内置立绘解码、眨眼帧选择、点击反应曲线、脸部点击区、身体拖动区、非方形布局、动画 idle/互动/拖动/特殊动作、Reduce Motion 海报、右键菜单、气泡台词与定位、透明根节点、可见像素、托盘图标尺寸，以及窗口位置约束。
- `internal/petpack`：动画 WebP 的 ANMF 解析、部分帧、blend/dispose、裁剪缩放、单一 schema 严格校验、热区/交互/菜单/状态/事件/气泡、海报帧、zip 安装与 manifest 合并、Store/lockfile/pending 恢复、Unicode Path/GBK 文件名与 clip ID 归一化。
- `internal/ui`：桌宠设置、应用/气泡主题映射与提示、分组设置卡片、侧栏页面切换、关于页版本/更新/数据信息、可用更新显示说明与安装按钮、禁用构建的解释。

视图测试不打开窗口，但会执行真实布局与点击；`ui.NewTester` 对重复 key 与生命周期错误是严格的。位置约束和气泡定位是纯几何测试；桌宠的透明、置顶、拖动、跨屏、气泡穿透与托盘必须另做真机验收，测试器不能代替窗口行为。

## 数据与日志

| 平台 | 数据与日志 |
|---|---|
| macOS | 数据 `~/Library/Application Support/Lucia`（开发版 `Lucia Dev`）；日志 `~/Library/Logs/Lucia/lucia.log`。 |
| Windows | 数据 `%LOCALAPPDATA%\Lucia`；日志 `%LOCALAPPDATA%\Lucia\logs\lucia.log`。 |

- `settings.json` 位于数据目录，保存应用主题、桌宠可见性、大小、音效、气泡冷却、气泡主题、外部素材包选择与首次挥手的提示状态。进程内写入经过 `internal/settings.Store`，控制台只合并展示类字段。
- 外部素材包位于数据目录的 `packs/<id>/`，lockfile 为 `packs.lock.json`，中断安装记录为 `packs.pending.json`。控制台“素材包”页可导入/启用/删除；开发工具仍可用 `go run ./cmd/petpack -zip <file>`，`-check <dir>` 校验当前 schema 与全部 clip。不要在应用运行时并发执行开发工具导入；它不进入仓库、CI 或安装包。
- 日志单文件上限 1 MiB，保留 `.1`–`.3` 三份备份；每行先写文件再镜像到 stderr。
- 窗口位置由 MyGo 按 `StateKey: "main"` 保存到同一数据根。
- 在应用内“关于”页可直接打开数据目录；菜单“文件 → 打开数据目录”同样可用。

## 更新在开发期的行为

- `go run`、`go test` 与 `go tool mygo dev` 的产物不嵌入更新源，关于页显示“当前构建未启用自动更新”，`LUCIA_SELFTEST` 报告 `updates=disabled`。
- 本地 `go tool mygo build` 会嵌入 `mygo.json` 的更新源；若设置了 `MYGO_UPDATER_PRIVATE_KEY`，同时产出签名更新档案，否则只警告并跳过更新档案。产物仍会检查正式渠道。
- 真实“发现新版本 → 下载 → 签名校验 → 替换 → 自动重启”需要至少两个已发布版本，验证步骤见 [release.md](release.md#发布后验证)。不要用正式 `v*` 标签做升级演练；使用 `test-v*` 测试渠道，具体操作见 [testing-updates.md](testing-updates.md)。

## 更新密钥

本项目已生成一对 Ed25519 更新密钥：

| 文件 | 位置 | 处理 |
|---|---|---|
| 私钥 | `~/.config/lucia/update-keys/mygo-update.key` | 只应存入密码管理器与仓库 secret `MYGO_UPDATER_PRIVATE_KEY`；绝不提交。 |
| 公钥 | `~/.config/lucia/update-keys/mygo-update.pub` | 已写入 `mygo.json` 的 `updates.publicKey`。 |

不要重新生成密钥覆盖这一对：已安装应用只接受该私钥签名的更新，丢失会使它们无法再更新。需要轮换时先设计迁移方案。

发布前设置仓库 secret：

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY < ~/.config/lucia/update-keys/mygo-update.key
```

## 常见问题

| 现象 | 原因与处理 |
|---|---|
| `go: no such tool "mygo"` | 在项目目录外执行，或 `go.mod` 缺少 tool 指令；回到仓库根执行 `go tool mygo ...`。 |
| 开发运行显示未启用更新 | 预期行为：开发运行不嵌入更新源，也不会发起网络检查。 |
| 日志没有出现在终端 | 安装版或 GUI 进程没有有效 stderr；查看日志文件。开发运行同时写两处。 |
| 第二实例没有打开新窗口 | 单实例把命令行交给第一个实例；检查第一个实例是否正常运行。 |
| macOS 打包缺少 DMG | 需要完整的 Xcode 命令行工具与 `hdiutil`；查看构建日志中的警告。 |
