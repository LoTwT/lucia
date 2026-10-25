# 桌宠规格（声明式素材包）

更新日期：2026-10-25。本文维护桌宠窗口、常驻生命周期、idle 行为、交互、气泡与素材包合同的单一事实来源；模块关系见 [architecture.md](../architecture.md)，内置素材授权见 [NOTICE.md](../../internal/builtinpack/whale/NOTICE.md)。

## 目标

启动后直接出现一只透明、无边框、置顶的桌宠。它可以拖动，有 idle 动画和眨眼，位置在重启后恢复；点击热区与右键菜单会触发素材包声明的交互。托盘可以显示或隐藏它、打开 Lucia 控制台并退出应用。

内置鲸鱼和外部皮肤共用同一个 manifest、加载器与播放器。内置包通过 `embed.FS` 编入二进制，不释放到数据目录；以后更换默认皮肤只需替换 `internal/builtinpack/whale/`，不改渲染代码。

## 窗口形态

| 选项 | 值 |
|---|---|
| 内置素材尺寸 | 窗口 248×248 DIP，立绘 240×240 居中 |
| 外部素材尺寸 | 由 manifest 的 `base.window` 与 `base.sprite` 定义 |
| 缩放 | 窗口、立绘、热区和气泡锚点按用户尺寸档位同比缩放 |
| `Frameless` / `Transparent` | `true`，窗口本身没有标题栏和底色 |
| `AlwaysOnTop` / `Panel` | `true`，浮在其他应用之上，不抢当前应用的焦点 |
| `SkipTaskbar` / `DisableShadow` | `true`，不占任务栏，不出现矩形窗口阴影 |
| 固定形态 | `DisableResize`、`DisableMinimize`、`DisableMaximize`、`DisableFullScreen` 均为 `true` |
| 位置 | `StateKey: "pet"`；首次出现在主屏工作区右下角，距边缘 24 DIP |

视图根节点使用透明背景，立绘以 `ui.Image` 绘制。窗口本身是 `DragWindow` 手柄，拖动主体、衣领或窗口其他区域移动窗口；热区是独立的透明点击层。透明边距不额外扩大窗口。

当素材宽高比与 `base.sprite` 不同时，立绘按 `Contain` 居中留边；热区和气泡锚点跟随实际绘制区域，而不是外框。`Model` 的 `WindowSize()`、`SpriteSize()`、`ImageRect()`、`Hotzones()` 和 `BubbleRect()` 是窗口接线的唯一几何来源。窗口从 1.0× 切换尺寸档位或换素材包时保持底边中点不动。

## 窗口位置约束

拖动和恢复使用两套时机，避免把“跨显示器”误判成“拖出屏幕”：

- 拖动由系统原生 `DragWindow` 完成；跨显示器时由系统接管，不会出现自绘 `SetPosition` 触发的吸附和跳动。
- 原生拖动允许宠物短暂越过屏幕边缘。`Window.OnMove` 发现窗口落在有效区域外时开始观察，等主键松开后再收回。macOS 用 `NSEvent.pressedMouseButtons`，Windows 用 `GetAsyncKeyState`，Linux 暂用 300 ms 稳定延迟。
- 恢复时，窗口完整落在一个显示器工作区内就不动；横跨两块显示器且完整落在物理范围并集内时保留；其余位置按最小位移收回到最近的显示器工作区。工作区排除菜单栏、Dock 和任务栏。
- 启动和显示器变化也走恢复路径：先撤销意外的最大化、恢复固定尺寸，再按同一规则恢复位置。

`pet.Display` 保留 `ScaleFactor`，供后续按显示器重建缓存；当前动画缓存按同一比例缩放到 `base.sprite × 2` 与 `crop` 原生尺寸的上限内，保持 `crop` 宽高比。

## 交互与热区

- 热区由 manifest 的 `hotzones` 有序数组声明。每项 `{id, label, rect, interaction}`，`rect` 归一化到 `crop`；数组顺序就是重叠优先级，后声明者优先命中。
- interact 由 `interactions` 声明：`{label, steps, bubble, sound, motion, reducedPoster}`。`steps` 可为空；此时通过气泡、声音或 `motion` 提供反馈。`motion` 取 `none`、`squash`、`lift`。
- `squash` 是 450 ms 压扁回弹；`lift` 是 620 ms、幅度更小并带轻微上抬。变换原点为底部中心，窗口位置不动。
- `steps` 每项 `{action, delayMs}`。第一步的 `delayMs` 从触发时开始等待，后续步骤的 `delayMs` 在上一步结束后等待；上限 60000 ms。等待期间继续播放 idle。
- 动作播放期间是否接受热区点击由动作的 `interruptible` 决定。声明了热区时，`states.idle` 指向的动作必须为 `interruptible: true`。
- 右键菜单由 manifest 的 `menu` 声明：每项引用一个 `interaction` 或一个有限 `action`。宿主固定的“桌宠设置 / 隐藏宠物 / 重置位置”永远在分隔线下方。
- 内置鲸鱼的“摸摸脸”是 `poke` 交互，“摸摸头”是 `pat` 交互；两者都只有气泡、声音和 `motion`，不播放额外 clip。外部包可以用 `steps` 播放任意有限动作。
- 拖动、特殊动作和 Reduce Motion 会清空未开始的步骤。拖动期间热区不可点击。
- Reduce Motion 下不播放连续动画：idle 显示海报，交互显示 `reducedPoster` 1.2 秒，状态切换时才预约下一帧。

## 气泡

- 气泡是独立的透明 Panel 窗口，固定 240×100 DIP；卡片为 228×80，外侧留出尾巴空间；不设 `StateKey`，不写入窗口位置。
- 整窗调用 `SetIgnoreMouseEvents(true)`，所有点击穿透；Linux 后端不支持时隐藏气泡，当前发布矩阵也不包含 Linux。
- 台词全部来自 manifest 的 `bubbles` 命名表，交互通过 `bubble` 键引用；同一键内按顺序循环。`none` 表示不显示。
- 气泡锚点是 `BubbleRect()`：横坐标取角色头部中心，纵坐标取头部上方；气泡翻到下方时使用窗口底边。内置鲸鱼的锚点是立绘顶边居中。
- 气泡优先显示在宠物上方，间距 8 DIP；上方放不下时翻到下方；水平位置限制在当前显示器工作区内。
- 气泡尾巴指向宠物中心；被屏幕边缘夹取时，尾巴横向位置也夹在卡片圆角内。
- 显示使用 `ShowInactive`，不抢焦点；2.6 秒后自动隐藏。
- 设置里的气泡冷却会跳过冷却期内的后续气泡；选择“关闭”时只保留动作和音效。
- 宠物开始移动、被托盘隐藏时立即隐藏气泡；拖动中不跟随宠物。

## 生命周期

- 主窗口（Lucia 控制台）启动时隐藏，由托盘“打开 Lucia”、第二实例或 `lucia://` 深链接打开。
- 关闭桌宠窗口、气泡窗口或控制台窗口只隐藏窗口，不退出进程；`OnBeforeQuit` 放行退出时的关闭。
- `OnActivate` 只在没有任何可见窗口时打开控制台。
- 托盘提供“显示/隐藏宠物”“重置宠物位置”“打开 Lucia”“退出”。
- 事件由 manifest 的 `events` 映射：`intro`、`activate`、`reset`、`update.checking`、`update.failed`，每项引用一个有限动作。没有声明的事件就是 no-op。
- 首次启动只在当前包声明了 `events.intro` 时播放，并在首帧后写入 `petIntroPlayed`；内置鲸鱼不声明 intro，因此不会消耗这次机会。切换素材包时播放 `events.activate`，重置位置播放 `events.reset`。

## 设置

- `settings.json` 包含 `petVisible`、`petScale`、`soundEnabled`、`bubbleCooldownSeconds`、`petPack` 和 `petIntroPlayed`。`petPack` 为空表示应用默认包。
- `petPack` 指向 `<数据目录>/packs/<id>`；`lucia-whale` 保留给内置包，不作为用户包加载。
- 素材包缺失、manifest 损坏或动画解码失败时记录日志并退回内置包，同时清除 `petPack`/active，避免控制台继续显示“使用中”。
- 加载时先取默认值再合并文件，旧配置缺少字段时保留默认值。
- “桌宠大小”提供小（0.8×）、标准（1.0×）、大（1.25×）、特大（1.5×）四档；窗口、立绘、热区和气泡锚点一起缩放。
- “互动音效”默认关闭；开启后交互的 `sound: true` 会播放内存生成的双音提示音，不引入第三方音频素材。
- “气泡频率”提供“每次互动 / 最多每 30 秒 / 最多每 2 分钟 / 关闭”。冷却只限制气泡，不影响动作和音效。
- 开机自启由 MyGo 登录项接口读写；控制台每次打开时刷新系统状态。

## 内置鲸鱼包

- `internal/builtinpack/whale/` 是一个标准 `schema: 1` 包：manifest、4 帧无损 idle WebP、源 PNG、LICENSE/NOTICE 和生成脚本。
- idle WebP 帧时长为 3800 / 60 / 90 / 50 ms，循环一次正好 4 秒；困倦和睡着复用 `idle_loop#1` / `idle_loop#2` 海报，不额外生成单帧 WebP。
- `crop` 固定为整张 800×800 画布，避免导入器自动裁剪改变缩放和热区位置。
- `idleMotion: "bob"` 提供 3.2 秒周期、2.5 DIP 的呼吸浮动；awake、drowsy、asleep 都继续呼吸。
- 源 PNG 保留在仓库作为生成输入，但不嵌入；`generate.sh` 使用 `img2webp -lossless -exact`，解码后像素与源图一致。
- 托盘图标迁到 `internal/brand/`，属于应用品牌，不属于可替换皮肤；授权仍按原素材的 CC BY-NC-SA 条件。
- `PetPack=""` 表示“应用默认包”，settings/lockfile 不写入 `lucia-whale`。读取到手工写入的保留 id 时归一化为空。
- 内置包只读，只能启用和预览；`Delete`、`Install` 拒绝保留 id。内置包加载失败属于构建缺陷，`selfTest` 会直接失败。

## 素材包 manifest

素材包放在 `<数据目录>/packs/<id>/`，manifest 固定为 `pack.json`，clip 放在 `clips/`，统一使用 ASCII 文件名。

```jsonc
{
  "schema": 1,
  "id": "example",
  "name": "示例桌宠",
  "license": { "status": "..." },
  "canvas": { "width": 800, "height": 800 },
  "crop": { "x": 0, "y": 0, "width": 800, "height": 800 },
  "base": { "window": {...}, "sprite": {...}, "margin": 4 },
  "bubbleAnchor": { "x": 0.5, "y": 0.05 },
  "clips": { "idle_loop": { "file": "clips/idle_loop.webp", "frames": 4, "loop": true } },
  "actions": {
    "idle": { "loop": "idle_loop", "poster": "idle_loop#0", "loopCount": -1, "interruptible": true }
  },
  "states": { "idle": "idle" },
  "events": { "intro": "wave" },
  "hotzones": [{ "id": "face", "label": "摸摸脸", "rect": {...}, "interaction": "poke" }],
  "interactions": {
    "poke": { "label": "摸摸脸", "bubble": "face", "sound": true, "motion": "squash" }
  },
  "bubbles": { "face": ["..."] },
  "menu": [{ "interaction": "poke" }],
  "idleMotion": "bob"
}
```

- 只有 `schema: 1`。缺失、其他值或未知字段都在严格解码阶段失败；没有版本分支、旧格式识别或迁移路径。
- `canvas`、`crop`、`base.window`、`base.sprite`、`bubbleAnchor` 定义几何；`crop` 是画布内的裁剪矩形，导入时计算不透明区域并外扩 2 px。
- `clips` 保存文件名、帧数、循环标志和 SHA-256；`actions` 把状态和事件映射到 `enter/loop/exit/once` clip。`loopCount = -1` 表示持续循环，`0` 跳过 loop，`> 0` 播放指定遍数。
- `states` 必填 `idle`，可声明 `drag`、`drowsy`、`asleep`。`asleep` 必须循环或提供 poster；未声明时显示 idle 海报。
- `events`、`hotzones`、`interactions`、`bubbles`、`menu` 都可为空。没有热区时桌宠仍播放 idle，但立绘不可点击。
- `idleMotion` 取 `none` 或 `bob`；`interaction.motion` 取 `none`、`squash`、`lift`。
- 校验拒绝：空 rect 或超出 0–1、重复热区 id、未知 interaction/action/bubble、循环动作作为 steps 或菜单动作、负数或超过 60 秒的 delay/hold、motion-only 交互缺少 `reducedPoster`、空气泡行、超过 200 字的气泡、Windows 保留目录名，以及保留 id `lucia-whale`。
- 示例 manifest 见 [docs/examples/pet-pack.json](../examples/pet-pack.json)；示例中的 `frames` 是结构示意，实际值必须与 WebP 文件一致。
- 画布与窗口边长上限为 4096 px，单个 clip 帧数上限为 1000；解码器在分配画布前再次校验。

## 导入与控制台

- 控制台与 `cmd/petpack` 共用导入器。zip 中任意层级最浅的 `pack.json` 或 `pack.meta.json` 提供 id、名称、几何、动作、状态、事件、热区、交互和菜单；缺失时从 zip 文件名推导 id 并生成显式 actions/states/events/menu，不生成热区或交互。
- 导入只接受 `.webp`，丢弃 MP4 预览；限制 manifest 4 MiB、单文件 64 MiB、解压总量 512 MiB，防止 zip-slip 与 zip 炸弹；先写临时目录，旧包备份后安装新包并校验，失败时回滚。
- `Store.Install(zip, replace)` 在 id 已存在且 `replace=false` 时返回 `*PackExistsError`，此时磁盘没有任何写入。控制台收到后弹出原生替换确认，确认后再以 `replace=true` 重试；取消无副作用。
- 导入不再要求授权勾选，`license` 仅展示。同名冲突才出现替换确认。
- 导入成功后桌宠立即切换；失败时保留原模型。列表可启用、删除未使用的包并预览海报、热区、交互步骤和动作时序。正在使用的包不能删除。
- 导入、替换和预览仍在 UI 线程同步执行，大包会短暂阻塞控制台；运行中的应用与开发工具不能同时导入。
- 开发命令：
  ```sh
  go run ./cmd/petpack -zip naiwa.zip
  go run ./cmd/petpack -zip naiwa.zip -app Lucia
  go run ./cmd/petpack -check <pack-dir>
  ```
  `-force` 覆盖同名包，`-activate=false` 只导入不切换。数据目录默认自动选择最近使用的 `Lucia Dev`、`lucia` 或 `Lucia`。

## 解码与播放

- 动画 WebP 的 `ANMF` 帧只覆盖画布的一部分，并依赖前一帧的 blend/dispose 状态；解码器必须按顺序合成，不能随机跳帧。循环回到第 0 帧时清空画布重新开始。
- 使用 `golang.org/x/image/webp` 解码单帧，Lucia 自己解析 RIFF/ANMF；不引入 cgo。
- 每帧使用自身时长（41/42 ms，特殊动作存在 500 ms、1083 ms 的停顿帧），不能按固定 24 fps 推导。
- idle 在启动时先显示海报帧，再在后台解码并缓存全部帧；缓存按同一比例缩放到 `base.sprite × 2` 以内，不超过 `crop` 原生尺寸，保持 `crop` 宽高比。
- 其他 clip 不预加载，播放时按顺序解码；解码发生在 UI 线程。
- 状态机按“拖动 > 用户交互 > 事件/菜单动作 > 困倦 > idle”排序。`enter` 播完进入 `loop`；`loopCount = 0` 直接进入 `exit`，`-1` 持续循环，`> 0` 播完指定遍数后进入 `exit`。步骤在当前动作自然结束后按 delay 链式启动。
- `idleMotion: bob` 时，`nextDelay` 取当前帧剩余时长与状态帧间隔的较小值：清醒 33 ms、困倦 66 ms、睡着 100 ms；交互 motion 期间 16 ms。没有 `bob` 时回到按帧时长调度。
- 困倦状态：动作可循环或播放一次；有限动作结束后显示其 poster。睡着状态：循环动作持续播放，或显示 poster；未声明时显示 idle 海报。
- 系统事件接法：更新检查/下载阶段映射 `update.checking`，更新失败映射 `update.failed`；首次启动映射 `intro`，换包映射 `activate`，重置位置映射 `reset`。
- Reduce Motion 不播连续动画：idle 显示静态海报，交互显示 `reducedPoster` 1.2 秒，拖动显示 drag 状态海报，动作菜单入口隐藏；状态切换时才预约下一帧。
- 动画时钟仍由 `ui.Context.Now()` 推导；idle 缓存的后台解码是唯一新增的后台工作，它不驱动 UI，完成后由下一次调度帧读取。

## 非目标

当前增量不包含：可视化拖拽编辑热区/交互、按 clip 单独配置热点、`listen` 的产品触发源、`speak` 与气泡时长联动、随机闲时动作开关、跨进程导入锁、Windows 真机验收。`edge_peek` 目前只作为手动特殊动作，不实现窗口越界探头。

## 后续产品项

- 可视化素材编辑器：在控制台里拖拽热区、编辑交互 steps 与 timing，并直接写回 `pack.json`。
- 预览与导入异步化，避免大包阻塞控制台 UI。
- 为 CLI 与控制台增加跨进程文件锁。
- 自定义气泡样式、音效文件和更多 motion 枚举，需要先定义可验证的封闭集合。

## 验收

自动检查：

```sh
gofmt -l .
go vet ./...
go test ./...
go test -race ./internal/pet ./internal/petpack ./internal/ui ./internal/settings ./cmd/lucia
go tool mygo vet .
```

`internal/petpack` 覆盖：WebP ANMF 解析、部分帧、blend/dispose、VP8L 与 lossy ALPH、裁剪缩放、单一 schema 校验、严格解码、热区顺序与重复 id、交互/菜单/状态/事件/气泡校验、循环动作拒绝、Windows 保留名、内置 id 保留、manifest 海报、zip 导入与 manifest 合并、meta-only 显式生成、Store 安装/同名错误/替换/激活/删除、替换失败回滚、lockfile/pending/backup 恢复、损坏元数据隔离、预览 PNG、0x7075/GBK 文件名、clip ID 归一化。

`internal/pet` 覆盖：素材包布局、缩放、热区、气泡锚点、透明视图、idle 循环、交互 steps、首步 delay、气泡与声音事件、动作停留时序、解码失败自愈、Reduce Motion 海报、idleMotion 帧时钟、菜单声明顺序。

`internal/builtinpack` 覆盖：内置包加载、4 帧 idle 与源 PNG 逐字节一致、帧时长、poster 帧。`internal/brand` 覆盖托盘图标。`internal/settings`/`cmd/lucia` 覆盖 Store 并发/失败/字段保留与 fallback active 清理；`internal/ui` 覆盖素材包列表、无效包说明、同名确认、导入后启用、替换 active 失败回退与删除。

macOS 真机手工检查：

- 内置鲸鱼仍为 248×248，拖动、眨眼、呼吸、压扁回弹和气泡不回归。
- 导入 naiwa 后窗口为 248×310，idle 动起来，点脸/摸头/拖动/特殊动作按 manifest 播放。
- 切换四档尺寸时窗口、立绘、热区和气泡锚点同步缩放，底边中点不跳。
- Reduce Motion 下只显示静态海报，交互海报 1.2 秒，动作菜单入口消失。
- 素材包删除或损坏后启动退回鲸鱼，控制台不再显示“使用中”。
- 气泡靠近屏幕顶部翻到下方时，尾巴仍指向头部。
- 隐藏宠物后不持续解码；重新显示后恢复 idle。
- CPU 预缩放后的鲸鱼清晰度与旧 GPU 缩放并排对比，确认没有可见退化。

## 性能观测

2026-10-20 在 Apple M1 Pro、内置 1512×982 与外接 2560×1440 双屏、开发构建下测得内置静态素材：桌宠可见且空闲时，`top` 采样 `%CPU` 约 16–19%，常驻内存约 78–142 MB。当时 `View` 每帧调用 `AnimationFrame()`。

2026-10-21 改为按状态调度后复核：旧构建 41 分钟空闲 `ps` 累计约 22.3%；新构建前 5 秒（清醒）`ps` 约 6.1、`top` 约 7.6–8.7%，60 秒后困倦 `ps` 约 3.4、`top` 约 5.4%，3 分钟后睡着 `ps` 约 1.8、`top` 约 2–4%，常驻内存约 80–86 MB。隐藏状态 `top` 约 0.0–0.1%。

动画素材包的目标（Apple M1 Pro）：清醒 idle `%CPU` ≤ 10%，流式 clip ≤ 25%，睡着 ≤ 3%，隐藏 ≈ 0；驻留内存 ≤ 200 MB；点击到首帧 ≤ 50 ms；启动先出海报 < 150 ms。以上是待验证目标。

## 风险与回退

`Panel + Transparent` 的原生 UI 如果出现不透明底色、拖拽异常或焦点被抢，回退为普通 `Frameless + Transparent + AlwaysOnTop + SkipTaskbar` 窗口。

素材回退路径：`petPack` 为空、素材包缺失、manifest 校验失败、WebP 解码失败或海报生成失败时，记录日志并使用内置包；启动时清除失效 active。内置包自身加载失败属于构建缺陷，直接失败而不是保留第二套渲染器。

Windows 的透明窗口、纹理上传、拖动检测和高 DPI 缩放仍需真机验证；Linux 不在发布矩阵。`edge_peek` 如果未来要求窗口一部分藏到屏幕外，需要先解决 `KeepOnScreen` 与越界探头的冲突。
