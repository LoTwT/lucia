# 素材包源目录与生成器

更新日期：2026-10-26。本文维护 `pack.source.json` 的作者契约、`petpack build` 的生成规则和第一版边界。运行时的 `pack.json` 契约仍由 [desktop-pet.md](desktop-pet.md#素材包-manifest) 维护；本文不复制其正文。

## 目标与边界

作者准备透明动画 WebP 和少量元数据，生成器负责裁剪、动作映射、热区归一化、校验和打包。

第一版只接受**已经导出的动画 WebP**：

- 不读取 GIF、APNG、MP4 或 PNG 序列；
- 不内置图片编码器；
- 不做可视化热区编辑器；
- builder 只生成 artifact，不安装、不修改用户数据目录。

## 源目录

```text
my-pet/
  pack.source.json
  clips/
    idle_loop.webp
    belly_taunt.webp
    drag_loop.webp
    ...
```

- 必须存在 clip ID `idle_loop`；默认文件名是 `idle_loop.webp`，也可以通过 `clips` 映射到其他文件名。
- 所有 clip 必须使用同一画布尺寸，且至少有一个非透明像素。
- 默认只扫描 `clips/`；不扫描 MP4 预览或源目录中的其他文件。
- 文件名按现有导入器规则归一化为 clip ID：去掉 `NN_` 前缀和非 ASCII 后缀，只保留小写字母、数字和 `_`。
- 可以用 `clips` 显式映射替代扫描，适合自定义文件名或中文/GBK 文件名。

支持的 clip role：

```text
idle_loop
happy_enter happy_loop happy_exit
laugh_enter laugh_loop laugh_exit
listen_enter listen_loop listen_exit
speak_enter speak_loop speak_exit
think_enter think_loop think_exit
sad_enter sad_loop sad_exit
wave_enter wave_loop wave_exit
grab_enter drag_loop drop_exit
belly_taunt pretend_deaf smug_stumble sleep_denial play_innocent edge_peek
```

显式 `clips` 中出现未知 role 会失败；默认扫描发现未知 WebP 会保留文件并输出 warning，但不会自动生成动作。Clip ID 同时会变成文件名，因此 `con`、`prn`、`aux`、`nul`、`com1`–`com9`、`lpt1`–`lpt9` 等 Windows 保留名会被拒绝。

## `pack.source.json`

使用独立严格 schema。未知字段、多个 JSON 值、`source != 1` 都失败。文件上限 4 MiB。

```jsonc
{
  "source": 1,
  "id": "example",
  "name": "示例桌宠",
  "license": { "status": "local-only" },

  "canvas": { "width": 800, "height": 800 },
  "window": { "width": 248, "height": 310 },
  "sprite": { "width": 240, "height": 300 },
  "crop": { "x": 80, "y": 20, "width": 640, "height": 760 },

  "clips": {
    "idle_loop": "clips/idle_loop_待机_循环.webp",
    "laugh_enter": "clips/laugh_enter_大笑_进入.webp",
    "laugh_loop": "clips/laugh_loop_大笑_循环.webp",
    "laugh_exit": "clips/laugh_exit_大笑_退出.webp",
    "belly_taunt": "clips/01_belly_taunt_肚肚挑衅.webp"
  },

  "bubbleAnchor": { "x": 0.5, "y": 0.05 },

  "hotzones": [
    {
      "id": "face",
      "label": "摸摸脸",
      "rect": { "x": 220, "y": 300, "width": 240, "height": 160 },
      "steps": [{ "action": "laugh" }],
      "bubble": ["别戳脸啦……"],
      "sound": true,
      "motion": "bounce",
      "reducedPoster": "auto"
    }
  ],

  "menu": [
    { "interaction": "face" },
    { "action": "belly_taunt" }
  ],

  "idleMotion": "none"
}
```

### 必需字段

| 字段 | 约束 |
|---|---|
| `source` | 必须为 `1` |
| `id` | 小写字母、数字、`_`、`-`；不能是 `lucia-whale` 或 Windows 保留名 |
| `name` | 非空显示名 |

### 可选字段

| 字段 | 默认/规则 |
|---|---|
| `license` | 可选；缺失时 builder 警告并不推断授权 |
| `canvas` | 可选断言；必须与所有 clip 一致 |
| `window` | 默认 248×310 |
| `sprite` | 默认 240×300；不得大于 window |
| `crop` | canvas 像素；提供即 pin；缺省为所有帧 alpha bounds 并集外扩 2 px |
| `clips` | ID → 相对源目录的 `.webp` 路径；缺省扫描 `clips/` |
| `bubbleAnchor` | 归一化到 crop；默认 `0.5,0.05`；允许显式 `0,0` |
| `hotzones` | 最多 64；canvas 像素矩形；顺序即命中优先级 |
| `menu` | 缺省为六个特殊动作；显式 `[]` 表示不要菜单 |
| `idleMotion` | 空、`none` 或 `bob` |

### 热区

```jsonc
{
  "id": "face",
  "label": "摸摸脸",
  "rect": { "x": 220, "y": 300, "width": 240, "height": 160 },
  "steps": [{ "action": "laugh", "delayMs": 0 }],
  "bubble": ["别戳脸啦……"],
  "sound": true,
  "motion": "bounce",
  "reducedPoster": "auto"
}
```

- `rect` 是 **canvas 像素坐标**，不是 crop 归一化坐标。builder 在确定 crop 后统一平移并归一化；超出 crop 直接失败。未 pin crop 且热区落在自动裁剪范围外时，错误会提示先 pin crop 或移动热区。
- `steps` 只能引用有限动作；不能引用 `idle` 或 `drag`。
- `bubble` 为该热区的气泡台词；最多 32 行，每行最多 200 个字符。
- `steps`、`bubble`、`sound`、`motion` 至少有一个效果。
- motion-only 且没有 steps 时必须写 `reducedPoster`，只接受 `auto` 或 `clip#frame`；`auto` 解析为 idle poster。
- 只支持矩形热区；ellipse 等形状留到后续版本。

### 禁止字段

source 中出现以下内容直接失败：

- `schema`、`base`、`margin`
- `actions`、`states`、`events`
- `interactions`、`bubbles`
- `clips.*.file`、`clips.*.frames`、`clips.*.loop`、`clips.*.sha256`
- `poster`、`loopCount`、`timing`
- `hotzones[].interaction`
- `faceClickable`

`reducedPoster` 是唯一允许的 poster 形态字段。

## 生成命令

```sh
go run ./cmd/petpack build ./my-pet -o ./dist/my-pet.zip
go run ./cmd/petpack build ./my-pet -o ./dist/my-pet.zip -force
```

`-force` 只在确认新产物通过校验后才替换旧产物；校验失败会恢复旧文件。

生成器执行：

1. 严格解析 source。
2. 扫描或解析显式 `clips`，解析 WebP、帧数、画布和 alpha bounds。
3. 校验 `idle_loop`、同画布、非全透明 clip、尺寸/帧数/总量限制。
4. 采用 pin crop 或自动计算 crop。
5. 生成 actions、states、events、menu，复用现有导入逻辑。
6. 把 canvas 像素热区转换为 crop 归一化坐标，生成 interactions/bubbles。
7. 写 staging，运行 `LoadDir` 校验。
8. 生成确定性 zip：固定文件顺序、1980-01-01 时间戳、0644 mode、Store 压缩。
9. 在 artifact zip 中内嵌规范化后的 `pack.source.json`；导入器忽略该文件，安装后的运行时目录不保留 source。
10. 重新打开 zip 并执行一次 `ImportZip` 回环，确保安装后的 `pack.json` 与构建产物字节一致。

`build` 不会安装或激活素材包。

## 预览

```sh
go run ./cmd/petpack preview ./my-pet -o ./preview.png
go run ./cmd/petpack preview ./my-pet -o ./preview.png -clip idle_loop -frame 0
```

预览输出**原生 canvas 尺寸**的完整帧，不做 crop 或缩放；命令同时打印 `canvas` 与 `crop`，热区坐标必须按 canvas 像素书写。预览在临时目录构建并解码，不写用户数据目录；越界 frame 会失败。

## 安装

构建产物使用现有导入流程：

```sh
go run ./cmd/petpack -zip ./dist/my-pet.zip
```

安装与启用是分开的；是否把 `settings.PetPack` 指向新包由安装命令的 `-activate` 决定。

## naiwa 这类无 manifest 源包

如果源目录只有 WebP、没有 hotzones，builder 会生成标准动作、states、events 和菜单，但不会生成点击交互：

```text
warning: no hotzones: the pet is not clickable
```

这是合法产物。旧版 `faceClickable` 已不在当前 schema，不自动恢复；需要点击时必须在 source 中写 canvas 像素热区。

## 第一版不包含

- PNG/GIF/APNG/MP4 转动画 WebP；
- GUI 或拖拽热区编辑器；
- 自动识脸、自动猜坐标；
- ellipse 热区；
- 自定义 action label、loopCount、timing、margin；
- 自定义 license 文本文件复制；
- zip 直接 `-check`（builder 内部已校验 zip）。
