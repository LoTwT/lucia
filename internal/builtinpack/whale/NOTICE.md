# DeepSeek 鲸鱼娘素材说明

本目录的 PNG 是**非官方同人素材**，与 DeepSeek 官方无关联，仅用于 Lucia 的非商业桌宠功能。图片**不适用** Lucia 源代码的 MIT 许可。

## 来源链

| 阶段 | 来源 |
|---|---|
| 原角色「溟月」 | 上善无形，2025 年 6 月创作；据公开报道以 CC BY-NC-SA 4.0 开放二创。 |
| DeepSeek 女仆鲸鱼娘 | B 站用户 ZipZipPipe，2026-04-25 在「溟月」基础上二创。 |
| 本目录三帧 | 取自 [shiyi312/DeepSeek-Whale-Desktop-Pet](https://github.com/shiyi312/DeepSeek-Whale-Desktop-Pet) 的 `expressions/v1_*.png`。该仓库注明角色素材来自 [JiafishNB/DeepSeek-WindowsTable-game.io](https://github.com/JiafishNB/DeepSeek-WindowsTable-game.io)，后者又指向 [MeteorNOX/DeepSeek-Balance-Whale-Widget](https://github.com/MeteorNOX/DeepSeek-Balance-Whale-Widget)。 |
| 授权边界 | MeteorNOX 的 [PROVENANCE.md](https://github.com/MeteorNOX/DeepSeek-Balance-Whale-Widget/blob/main/PROVENANCE.md) 明确说明 `assets/` 不在 MIT 覆盖范围内，按 as-is 随插件分发，不授予再许可。 |

公开来源梳理见 [36 氪报道](https://www.36kr.com/p/3947452108789632)。

## 授权链状态与使用条件

直接上游明确表示 `assets/` 不授予再许可；本目录所依据的 CC BY-NC-SA 4.0 来自公开报道中的原角色二创说明，尚未取得 ZipZipPipe 或更上游作者针对这三张图的一手授权链接。**因此授权链存在不确定性，不能表述为已经取得完整再许可。**

项目所有者已知悉该风险，当前仅在非商业桌宠范围内使用；如果权利人提出异议，会移除或替换素材。Lucia 不对这套素材主张额外权利：

- **署名**：上善无形（原角色）、ZipZipPipe（DeepSeek 二创）、shiyi312 及上游素材整理者。
- **非商业**：不得用于收费、广告、付费分发或其他商业用途。Lucia 当前不进行商业行为。
- **相同方式共享**：对本目录图片的裁剪、缩放、拼帧等修改，仍以 CC BY-NC-SA 4.0 的条件提供。
- 许可全文：<https://creativecommons.org/licenses/by-nc-sa/4.0/>

如果后续要商业化或无法继续接受该不确定性，应替换为原创素材或取得明确的一手授权。

## 文件映射

| 本目录文件 | 上游文件 | 上游 blob SHA-1 |
|---|---|---|
| `idle-open.png` | `expressions/v1_DSniang1.png` | `b11bf7357fd26460512c319607b6c5df1af26245` |
| `idle-half-closed.png` | `expressions/v1_half_closed_eyes.png` | `833b1347f4cf1742ca8eb02591d15eebbf3c4cc9` |
| `idle-closed.png` | `expressions/v1_close_eyes.png` | `ee3a90d8519b8fffd406e2b838bca3fa3e212d0e` |
| `clips/idle_loop.webp` | 由上述三张图按 open → half → closed → half 拼成 4 帧无关卡动画 | — |
| `clips/drowsy.webp` | 由 `idle-half-closed.png` 转成的单帧动画 | — |
| `clips/asleep.webp` | 由 `idle-closed.png` 转成的单帧动画 | — |
| `pack.json` | 本项目为这套素材维护的 schema 1 manifest | — |

三张源图均为 800×800 RGBA，透明底，内容为头肩半身像（不含下半身）；文件仅重命名，源图像素未修改。`clips/*.webp` 是使用 `img2webp -lossless -exact` 生成的拼帧派生作品，解码后像素与源图一致；`pack.json` 声明画布、窗口/立绘尺寸、热区、交互与气泡台词。

托盘图标 `tray.png` 已迁到 `internal/brand/`，仍是 `idle-open.png` 缩放到 32×32 的派生图，授权条件见该目录的 `NOTICE.md`。

- 上游仓库 main 提交：`82a783611c917ac578c2b2e4befb73ab5a84b417`
- 下载日期：2026-10-20

## 权利主张与移除

如果权利人认为本目录素材使用不当，请在仓库 issue 中说明文件名与依据；核实后会移除或替换，不附加其它条件。
