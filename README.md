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

开发运行与 `go run` 不嵌入更新源，更新页会说明“当前构建未启用自动更新”。

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
| `internal/pet/` | 桌宠模型、idle 动画、点击反应、气泡台词与透明视图。 |
| `internal/pet/assets/deepseek-whale/` | 桌宠立绘与授权说明；美术素材按 CC BY-NC-SA 4.0 的条件使用，授权链未完全核实，仅限非商业使用。 |
| `resources/` | 图标与随包分发的第三方声明。 |
| `design/` | 图标 SVG 源文件。 |
