# 架构

更新日期：2026-10-25。本文维护 Lucia 的模块边界、启动顺序、共享目录与更新状态机；开发命令见 [development.md](development.md)，发布规则见 [release.md](release.md)。

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
| `internal/pet` | 桌宠模型、动画播放状态机、布局、交互、气泡模型与位置规则。 |
| `internal/petpack` | 素材包合同：WebP/ANMF 解码、单一 schema、校验、Store、lockfile、pending 恢复、zip 安装与裁剪校验。 |
| `internal/builtinpack` | 内嵌鲸鱼包、4 帧 WebP、源 PNG、生成脚本与授权说明。 |
| `internal/brand` | 应用品牌资源，包括托盘图标及其授权说明。 |
| `internal/ui` | 主窗口模型、侧栏与三个页面（桌宠、素材包、关于），以及 `ui.NewTester` 视图测试。 |
| `resources/` | 应用图标与需要随包分发的资源；MyGo 在打包时合并到应用资源根。 |
| `cmd/petpack/` | 开发用导入与 `-check` 校验工具；不随桌面应用分发。 |

依赖方向：`cmd` → `internal/*`；`internal/ui` 只依赖 `internal/settings` 与 `internal/updates`，素材包页面使用自己的 `PackSummary` 视图模型，不导入 `internal/petpack`；`internal/pet` 依赖 MyGo 的 `ui`、标准库与 `internal/petpack`；`internal/petpack` 只依赖标准库、`golang.org/x/image` 与 `golang.org/x/text`，不反向依赖 `internal/pet` 或 `internal/ui`。这样 WebP/ANMF 解码、导入事务和桌宠状态机可以独立测试，窗口接线保持在上层。`internal/updates` 是唯一直接调用 `mygo.Updater` 的包。

## 2. 启动顺序

`cmd/lucia/main.go` 的顺序是有意固定的：

1. `LUCIA_SELFTEST` 非空时只输出版本、提交、平台与更新状态并退出；不创建目录、不打开日志、不进入事件循环，供打包工作在制品上断言 `updates=enabled/disabled`。
2. 读取 `mygo.App.Version()` 与 `mygo.App.Name()`，解析并创建用户目录。
3. 调用 `mygo.App.SetPath` 覆盖 `PathUserData`、`PathCache`、`PathLogs`，让框架自己的状态（窗口位置、更新状态）与应用文件落在同一根目录。覆盖必须在 `Run` 前完成。
4. `RequestSingleInstanceLock`；第二个实例把命令行交给第一个实例后直接退出，不接触日志与数据文件。
5. 打开诊断日志，写入首行 `version=… commit=… platform=… data=… logs=…`。打不开日志不阻止启动。
6. 读取设置，应用保存的应用主题（`mygo.Theme.SetSource`），并构造进程内唯一的 `settings.Store`。文件缺失用默认值；文件损坏记录原因并继续用默认值。
7. 构造 `petpack.Store`，先执行 `Recover` 清理中断的导入、恢复 `.backup`、重建 lockfile，再把 `petPack` 写入 active；构造更新管理器，把 `CheckUpdatesOnLaunch` 同步到管理器的自动检查开关；加载内嵌鲸鱼包，再构造桌宠模型：`petPack` 非空时从 `<数据目录>/packs/<id>` 加载外部包，失败则记录日志、退回内嵌包并清除 active 选择，避免控制台状态与桌面不一致。动画包在后台解码 idle 缓存，首帧海报立即可见。
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

设置文件为数据目录下的 `settings.json`，采用临时文件加改名写入，崩溃不会留下半个 JSON。加载时先取 `Default()` 再反序列化，旧文件缺少新字段时保留默认值。当前持久化 `checkUpdatesOnLaunch`、`theme`、`petVisible`、`petScale`、`soundEnabled`、`bubbleCooldownSeconds`、`bubbleTheme`、`petPack` 与 `petIntroPlayed`；开机自启由 MyGo 的登录项接口管理，不重复落盘。进程内所有写入都经过 `settings.Store.Update`；控制台只合并展示类字段，桌宠运行时拥有的 `petPack`/`petIntroPlayed` 不会被旧副本覆盖。窗口位置由 MyGo 按 `StateKey` 存在同一数据根：控制台为 `"main"`，桌宠为 `"pet"`，气泡不设 `StateKey`。外部素材包位于 `<数据目录>/packs/<id>/`，由控制台或 `cmd/petpack` 导入；它不属于窗口状态，也不进入仓库或安装包。

## 3.1 素材包

- 目录：`<数据目录>/packs/<id>/pack.json` 与 `clips/*.webp`。`petPack` 选择 id；空值使用内嵌包。`lucia-whale` 是保留 id，不写入 settings/lockfile。
- manifest：`internal/petpack.Manifest` 维护画布、裁剪、窗口/立绘 DIP 尺寸、气泡锚点、clip 列表、动作链、状态、事件、热区、交互、气泡与菜单。只有一个 `schema: 1`，严格解码，无版本分支或旧格式升级。
- 交互：`hotzones` 是有序数组，`rect` 归一化到裁剪后的立绘，后声明者优先命中。每个热区引用一个 `interaction`；交互包含可选 steps、气泡键、声音、motion 与 `reducedPoster`。`menu` 每项引用 interaction 或有限 action。
- 安装事务：`petpack.Store` 串行化安装/删除/激活，`packs.lock.json` 缓存已安装元数据，`packs.pending.json` 记录安装阶段。启动时 `Recover` 清理 `.import-*`/`.trash-*`、恢复 `.backup`、删除已完成的 pending，并从磁盘重建 lockfile。正在使用的包不能删除；内置包只读。
- 解码：`internal/petpack` 解析 WebP RIFF/ANMF，使用 `golang.org/x/image/webp` decode 单帧，再按 blend/dispose 顺序合成；不引入 cgo。每帧保留自身时长，支持特殊动作的长停顿帧。
- 播放：`internal/pet` 的动画状态机按 UI 时钟推进 `enter → loop → exit`；idle 预解码缓存，其他 clip 在 UI 线程顺序解码。缓存为后台工作，不驱动 UI。Reduce Motion 使用 `reducedPoster`。
- 安装：控制台和 `cmd/petpack` 共用导入器。读取 zip 中任意层级最浅的 `pack.json`/`pack.meta.json`（4 MiB 上限），优先使用带 CRC32 校验的 0x7075 Unicode Path，回退 GBK；只接受 `.webp`，跳过 `__MACOSX/` 与 `._*` 资源叉，限制单文件 64 MiB、总量 512 MiB，写临时目录后备份旧包、安装并校验，失败回滚。`Store.Install(zip, replace)` 仅在同名且未请求替换时返回 `PackExistsError`，此时没有写入；控制台据此弹出确认。授权不再作为导入门槛。
- 控制台：素材包页列出内嵌包与有效/无效的用户包、clip/帧/热区/交互数量与授权状态，生成海报预览、叠加热区，并列出交互步骤与动作停留时序；内嵌包只读。导入后立即加载新模型并切换，失败则保留旧模型和旧设置；替换正在使用的包后若启用失败，会切回内置包。导入/预览同步跑在 UI 线程，大包会短暂阻塞控制台。运行中的应用与 `cmd/petpack` 不能并发导入（只有进程内互斥）。
- 回退：manifest/解码/海报任一失败都记录日志并使用内嵌包，并清除失效的 active 选择；可选素材不能阻止应用启动。
- 分发：naiwa 等外部素材不进入 Git、CI、DMG/NSIS 或发布资产；授权由用户本地确认。

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
- 本应用不需要 cgo，因此 MyGo 默认的 `CGO_ENABLED=0` 可以直接使用；这一点与 youkai 的 OCR 构建不同。外部动画包使用纯 Go 的 `x/image/webp` 解码，也不引入 cgo。
- 发布版本来自标签而不是提交中的版本号：workflow 在检出副本里写入标签版本与渠道前缀，并用 `git update-index --skip-worktree` 让构建提交号保持干净。
- 内置鲸鱼包通过 `go:embed` 进入二进制。外部包按用户本地选择导入数据目录，不进入二进制；两者都不适用代码许可。内置素材来源与 CC BY-NC-SA 4.0 条件见 `internal/builtinpack/whale/NOTICE.md`，托盘图标的派生说明见 `internal/brand/NOTICE.md`。

## 7. 明确不包含

- 不包含 Web 前端、浏览器内 UI 或 TypeScript 绑定。
- 当前包含托盘常驻，以及桌宠、气泡与控制台三个窗口；关窗只隐藏，退出由托盘或 Cmd+Q 发起。
- 桌宠当前包含内嵌鲸鱼包的 idle 呼吸/眨眼、四档尺寸、非方形布局、拖动、声明式热区与交互、右键菜单、独立气泡，以及外部包的 idle/情绪/特殊动作/拖动动画、交互步骤和动作级停留时序；更新检查/下载、更新失败、首次启动、换包和重置位置通过 manifest 事件映射。控制台提供素材包导入、同名确认、校验预览、启用/切换/删除；内置包只读。暂不包含可视化拖拽编辑热区/交互、跨进程导入锁、`listen` 产品触发源、`speak` 与气泡时长联动、随机闲时动作、桌面走动、AI 对话和多只宠物。外部素材包是用户本地导入的可选能力，不是随包分发的默认皮肤。
- 不包含业务数据、账号体系或服务端。Lucia 当前是可运行、可发布的应用底座。
