# 架构

更新日期：2026-10-20。本文维护 Lucia 的模块边界、启动顺序、共享目录与更新状态机；开发命令见 [development.md](development.md)，发布规则见 [release.md](release.md)。

## 1. 总体结构

Lucia 是单个 Go 模块的 MyGo 原生 UI 应用，不使用 Web 前端、WebView 或 TypeScript 绑定。界面由 `internal/ui` 的视图函数在 UI 线程构建，MyGo 用 Metal / Direct3D 11 / OpenGL 绘制。

| 模块 | 职责 |
|---|---|
| `cmd/lucia` | 进程入口：自检、目录解析、单实例、诊断日志、菜单、深链接、窗口与更新接线。 |
| `internal/apppaths` | 按平台解析数据、缓存与日志目录，并在启动前创建。 |
| `internal/buildinfo` | 版本、VCS 提交、平台与日志首行的格式化。 |
| `internal/diagnostics` | 文件优先、stderr 镜像的诊断日志与大小轮转。 |
| `internal/settings` | `settings.json` 的默认值、容错读取与原子写入。 |
| `internal/updates` | 更新状态机；`MyGo` 适配器把 `mygo.Updater` 接到 `Source` 接口。 |
| `internal/pet` | 桌宠立绘、idle 动画、点击反应、气泡模型与位置规则；素材用 `go:embed` 进入二进制。 |
| `internal/ui` | 主窗口模型、侧栏与三个页面（概览、更新、关于），以及 `ui.NewTester` 视图测试。 |
| `resources/` | 应用图标与需要随包分发的资源；MyGo 在打包时合并到应用资源根。 |

依赖方向：`cmd` → `internal/*`；`internal/ui` 只依赖 `internal/settings` 与 `internal/updates`；`internal/pet` 只依赖 MyGo 的 `ui` 包与标准库；`internal/updates` 是唯一直接调用 `mygo.Updater` 的包。这样更新逻辑和桌宠动画都可以脱离窗口与真实网络做单元测试。

## 2. 启动顺序

`cmd/lucia/main.go` 的顺序是有意固定的：

1. `LUCIA_SELFTEST` 非空时只输出版本、提交、平台与更新状态并退出；不创建目录、不打开日志、不进入事件循环，供打包工作在制品上断言 `updates=enabled/disabled`。
2. 读取 `mygo.App.Version()` 与 `mygo.App.Name()`，解析并创建用户目录。
3. 调用 `mygo.App.SetPath` 覆盖 `PathUserData`、`PathCache`、`PathLogs`，让框架自己的状态（窗口位置、更新状态）与应用文件落在同一根目录。覆盖必须在 `Run` 前完成。
4. `RequestSingleInstanceLock`；第二个实例把命令行交给第一个实例后直接退出，不接触日志与数据文件。
5. 打开诊断日志，写入首行 `version=… commit=… platform=… data=… logs=…`。打不开日志不阻止启动。
6. 读取设置。文件缺失用默认值；文件损坏记录原因并继续用默认值。
7. 构造更新管理器，把 `CheckUpdatesOnLaunch` 同步到管理器的自动检查开关；构造 `internal/pet` 模型并解码内嵌立绘，素材损坏则退出。
8. 注册单实例、激活、`lucia://` 深链接、退出前与最后窗口关闭回调；设置应用菜单。
9. `WhenReady` 中创建桌宠窗口、默认隐藏的气泡窗口与默认隐藏的控制台窗口，创建托盘，接上管理器的重绘与安装完成回调，再启动自动检查循环。
10. `App.Run`。退出后记录 `exited`。

窗口回调只做界面工作；更新检查、下载与安装在独立 goroutine 中运行，结果通过 `Window.Update` 回到 UI 线程。

## 3. 用户目录

| 平台 | 数据 | 缓存 | 日志 |
|---|---|---|---|
| macOS | `~/Library/Application Support/<应用名>` | `~/Library/Caches/<应用名>` | `~/Library/Logs/<应用名>` |
| Windows | `%LOCALAPPDATA%\<应用名>` | `%LOCALAPPDATA%\<应用名>\cache` | `%LOCALAPPDATA%\<应用名>\logs` |
| 其他 | `$XDG_CONFIG_HOME/<应用名>` | `$XDG_CACHE_HOME/<应用名>` | 数据目录下的 `logs/` |

开发运行由 MyGo 命名为 `Lucia Dev`，因此数据、日志、窗口状态与正式版天然隔离。Windows 使用 `%LOCALAPPDATA%` 而不是漫游 `%APPDATA%`：更新状态与本地数据不应被漫游配置文件或组织策略跨机器同步。

设置文件为数据目录下的 `settings.json`，采用临时文件加改名写入，崩溃不会留下半个 JSON。加载时先取 `Default()` 再反序列化，旧文件缺少新字段时保留默认值。当前持久化 `checkUpdatesOnLaunch`、`petVisible`、`soundEnabled` 与 `bubbleCooldownSeconds`；开机自启由 MyGo 的登录项接口管理，不重复落盘。窗口位置由 MyGo 按 `StateKey` 存在同一数据根：控制台为 `"main"`，桌宠为 `"pet"`，气泡不设 `StateKey`。

## 4. 更新状态机

`internal/updates.Manager` 持有唯一状态，UI 每帧读取 `Snapshot`：

| 阶段 | 含义 | 可执行操作 |
|---|---|---|
| `unavailable` | 开发运行、未嵌入更新源，或安装位置不可写。 | 无，界面解释原因。 |
| `idle` | 尚未检查。 | 检查。 |
| `checking` | 正在读取更新清单。 | 无，按钮禁用。 |
| `up-to-date` | 已是最新。 | 再次检查。 |
| `available` | 找到新版本。 | 安装并重启、再次检查。 |
| `downloading` | 下载与安装中。 | 无；进度条显示字节与百分比。 |
| `installed` | 已替换应用文件，等待重启。 | 入口自动发起 `App.Relaunch`。 |
| `failed` | 检查或安装失败。 | 重新检查；安装失败时保留版本供重试。 |

约束：

- 同一时间只允许一个检查或安装；并发调用返回 `ErrBusy`，不会重复下载。
- `Check` 返回 `nil, nil` 表示已是最新；`mygo.ErrUpdatesDisabled` 会把状态复位为 `unavailable`。
- 安装成功后的重启必须在 UI 线程发起：管理器回调在主线程之外，入口用 `Window.Update` 包住 `App.Relaunch`。
- 自动检查由 `StartAutoChecks` 驱动：启用时启动即检查一次，之后每 24 小时检查一次；关闭后定时器不再发起检查，手动检查仍可用。用户在设置中重新开启时，视图立即发起一次检查。
- 签名、下载、差量回退与替换由 MyGo 完成；本应用不解析清单、不保存私钥。公钥在 `mygo.json`，私钥只存在于发布环境。

## 5. 诊断日志

- 当前文件 `lucia.log` 跨运行追加；达到 1 MiB 后轮转为 `.1`，最多保留 `.1`–`.3`。
- 每行先写文件、再尽力镜像到 stderr。Windows GUI 进程没有控制台，stderr 写失败不能影响文件落盘。
- 轮转失败只放弃本次轮转，继续写当前文件；日志目录不可写时应用仍启动，只退化为 stderr。
- 首行包含版本、提交（有本地修改时带 `-modified`）与平台，用户反馈日志即可定位构建。

## 6. 构建与分发

- `mygo.json` 维护名称、标识 `com.lotwt.lucia`、开发版本 `0.0.1`、入口 `./cmd/lucia`、深链接 scheme `lucia`、输出目录 `build` 与更新源 `LoTwT/lucia`。
- macOS 目标为 `darwin/arm64`，最低系统 13.0（Go 1.27 的 macOS 下限）；ad-hoc 签名，不依赖付费证书。由于没有 Developer ID/公证，测试版必须安装在用户可写的 `~/Applications`；`/Applications` 受 App Management 限制，原地自更新后可能无法启动。正式分发到 `/Applications` 前需要签名与公证，详见 [release.md](release.md#发布后验证)。
- Windows 目标为 `windows/amd64`，MyGo 生成 NSIS 每用户安装包。
- 本应用不需要 cgo，因此 MyGo 默认的 `CGO_ENABLED=0` 可以直接使用；这一点与 youkai 的 OCR 构建不同。
- 发布版本来自标签而不是提交中的版本号：workflow 在检出副本里写入标签版本与渠道前缀，并用 `git update-index --skip-worktree` 让构建提交号保持干净。
- 桌宠立绘通过 `go:embed` 进入二进制。美术素材不适用代码许可，来源与 CC BY-NC-SA 4.0 条件见 `internal/pet/assets/deepseek-whale/NOTICE.md`。

## 7. 明确不包含

- 不包含 Web 前端、浏览器内 UI 或 TypeScript 绑定。
- 当前包含托盘常驻，以及桌宠、气泡与控制台三个窗口；关窗只隐藏，退出由托盘或 Cmd+Q 发起。
- 桌宠当前包含 idle 呼吸/眨眼、拖动、脸部点击压扁回弹、右键“摸摸头”和独立气泡台词；不包含桌面走动、AI 对话、皮肤系统或多只宠物，这些在后续增量中设计。
- 不包含业务数据、账号体系或服务端。Lucia 当前是可运行、可发布的应用底座。
