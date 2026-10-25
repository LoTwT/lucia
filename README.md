# Lucia

Lucia 是一个基于 [MyGo](https://mygo.egoist.dev/) 原生 UI 的桌面应用底座：界面由 Go 直接描述并在 GPU 上绘制，不使用 WebView；本地开发、打包、GitHub 发布与签名自动更新已经接通。

## 要求

- Go 1.27.2（`go.mod` 固定）。
- MyGo v0.5.5，作为 Go tool 固定在 `go.mod`，通过 `go tool mygo` 使用。
- macOS 打包需要 Xcode 命令行工具；Windows 安装包由 MyGo 在 Windows 上构建时生成 NSIS。

## 本地开发

```sh
go tool mygo dev
```

开发应用名为 `Lucia Dev`，数据与正式版隔离。直接运行或测试：

```sh
go test ./...
go run ./cmd/lucia
```

开发运行与 `go run` 不嵌入更新源，关于页会说明“当前构建未启用自动更新”。

## 外部动画素材包

桌宠默认使用内嵌的鲸鱼素材包。透明动画 WebP 素材包可以安装在用户数据目录，**不进入仓库、CI 或安装包**。内置包与外部包使用同一套 manifest、加载器和播放器。

终端用户流程：打开控制台的“素材包”页，选择 zip 并导入；只有同名素材包已存在时才询问是否替换。导入会校验 manifest、每个 clip 的帧结构与 SHA-256（manifest 提供时逐 clip 核对并写入新摘要）、裁剪、热区和交互引用。失败时保留原素材，成功后桌宠立即切换，无需重启。也可以删除未启用的素材包；内置鲸鱼只读，只能启用和预览。导入和预览在 UI 线程同步执行，大包会短暂卡顿。

开发工具流程：

```sh
go run ./cmd/petpack -zip /path/to/pack.zip
go run ./cmd/petpack -check /path/to/lucia/packs/naiwa
```

默认自动选择最近使用的数据目录（`Lucia Dev`、`lucia` 或打包版 `Lucia`），也可以显式传 `-app`。数据目录跟随应用名：`go tool mygo dev` 是 `Lucia Dev`，`go run ./cmd/lucia` 或仓库根目录的 `lucia` 是 `lucia`，打包版是 `Lucia`。**不要在 Lucia 运行时同时执行开发工具导入**：CLI 导入前先退出应用，或改用控制台。`-force` 覆盖同名包，`-activate=false` 只导入不切换。`-check <dir>` 校验当前 manifest 与全部 clip。素材包缺失或损坏时应用会回退内置包，并清除失效的 active 选择。

manifest 只有 `schema: 1`，不再保留版本分支。热区、交互、菜单、状态、事件、气泡和 motion 都由包声明；完整字段与示例见 [桌宠规格](docs/specs/desktop-pet.md#素材包-manifest) 和 [示例 manifest](docs/examples/pet-pack.json)。

授权不再作为导入门槛；`license` 只在列表和预览中展示，不主张再分发权。

## 打包

```sh
go tool mygo build
```

产物在 `build/<os>-<arch>/`。macOS 生成 `Lucia.app` 与 DMG；Windows 生成 `Lucia.exe` 与 NSIS 每用户安装包。跨平台构建：

```sh
go tool mygo build -platform darwin/arm64
go tool mygo build -platform windows/amd64
```

自检构建产物的版本与更新状态：

```sh
LUCIA_SELFTEST=1 build/darwin-arm64/Lucia.app/Contents/MacOS/Lucia
```

## 发布与更新

版本号取自标签 `v<major.minor.patch>`；`test-v<版本>` 是测试渠道。推送标签后，`.github/workflows/release.yml` 会在两个平台构建、签名更新、核对资产并发布 Release。每个版本必须在 [CHANGELOG.md](CHANGELOG.md) 有对应小节。

更新签名私钥只保存在本机密码管理器与仓库 secret `MYGO_UPDATER_PRIVATE_KEY`，绝不进入仓库。公钥已写入 `mygo.json`；首次发布前把私钥设置到仓库：

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY < ~/.config/lucia/update-keys/mygo-update.key
```

完整流程与发布后验证见 [docs/release.md](docs/release.md)，开发约定见 [docs/development.md](docs/development.md)，模块关系见 [docs/architecture.md](docs/architecture.md)。首次升级演练使用 `test-v` 测试渠道，操作清单见 [docs/testing-updates.md](docs/testing-updates.md)。

## 目录

| 路径 | 内容 |
|---|---|
| `cmd/lucia/` | 入口：目录解析、日志、单实例、菜单、窗口与更新接线。 |
| `internal/ui/` | 原生 UI 模型、页面与视图测试。 |
| `internal/updates/` | 更新状态机与 MyGo 更新器适配。 |
| `internal/settings/` | 设置读写。 |
| `internal/apppaths/` | 各平台用户目录解析。 |
| `internal/diagnostics/` | 文件日志与轮转。 |
| `internal/pet/` | 桌宠模型、动画播放状态机、布局、交互、气泡与透明视图。 |
| `internal/petpack/` | 素材包合同：WebP/ANMF 解码、单一 schema、校验、Store、lockfile、裁剪、帧源与 zip 安装。 |
| `internal/builtinpack/` | 内嵌鲸鱼包、4 帧 WebP、源 PNG、生成脚本与授权说明。 |
| `internal/brand/` | 应用品牌资源，包括从鲸鱼素材派生的托盘图标及其授权说明。 |
| `cmd/petpack/` | 开发用素材包导入与 `-check` 校验工具；不随桌面应用分发。 |
| `resources/` | 图标与随包分发的第三方声明。 |
| `design/` | 图标 SVG 源文件。 |
