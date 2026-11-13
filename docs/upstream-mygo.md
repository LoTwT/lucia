# MyGo 上游需求记录

更新日期：2026-11-14。本文记录 Lucia 在 MyGo 各版本上遇到、适合由上游解决的问题，以及当前依赖中的支持情况，**不向上游提交 issue、PR 或代码**。是否向上游贡献、以什么形式贡献，必须由项目所有者明确确认。

当前版本：MyGo `v0.9.0`。状态说明：

- **已记录**：已有具体场景和期望接口，尚未提交上游。
- **待确认**：需求方向明确，但 API 形态或平台范围还需要验证。
- **上游已提供，待接入**：当前依赖已有对应 API，Lucia 尚未接线或完成实机验收。
- **已接入**：Lucia 已使用上游 API；具体验收平台与限制由关联记录说明。

## 1. 按像素或区域命中

**状态：已接入；macOS 已验收，Windows 本阶段跳过真机验收。**

**原场景**：透明桌宠窗口整个矩形都接收鼠标。立绘透明边距、头发或尾巴之间的空隙会挡住桌面和其他应用；气泡窗口只能用整窗穿透规避，不能让气泡局部可点。

**期望能力**（二选一即可）：

- `Window.SetInputRegion([]Rectangle)`：按矩形近似输入区域，跨平台实现相对直接。
- `Window.SetInputMask(*image.Alpha)`：按 alpha 生成命中区域，效果最好，但平台实现更重。

**相关代码**：`internal/pet/pet.go` 的透明根节点；`docs/specs/desktop-pet.md` 的风险与回退。

**当前支持**：MyGo v0.9.0 已提供 `Window.SetInputRegion([]Rectangle)`，使用窗口内容区的 DIP 矩形；尚无 alpha mask 接口。macOS/Windows 根据指针位置切换整窗穿透，拖动期间保持原命中状态；Linux 使用原生输入区域。

**Lucia 当前处理**：已根据当前动画帧的 alpha 接入区域命中，行为与精度约束由[桌宠规格](specs/desktop-pet.md#窗口形态)维护，实测见[透明区域点击穿透验收](testing-input-region.md)。气泡继续使用独立的整窗鼠标穿透窗口。MyGo v0.9.0 已实现 Linux 的 `SetIgnoreMouseEvents`，但 Lucia 尚未完成 Linux 气泡验收，仍保持隐藏；Linux 不在当前发布矩阵内。

## 2. 原生拖动会话的开始/结束回调

**状态：已记录。**

**场景**：`DragWindow` 由系统原生拖动循环接管，Lucia 无法知道拖动何时开始、何时结束。当前为桌宠准备了：

- 拖动结束后把越界窗口收回工作区；
- 拖动时隐藏气泡；
- 避免在原生拖动过程中调用 `SetPosition`，否则窗口服务器会吸附并产生边缘/跨屏跳动。

**期望能力**：

- `Window.OnDragStart(func())` / `Window.OnDragEnd(func())`；或
- `Window.StartDrag(func())`，在拖动会话结束时回调。

回调应保证在原生拖动循环退出后、主线程可安全调用窗口 API 时触发。

**相关代码**：`cmd/lucia/main.go` 的拖动收尾；`cmd/lucia/mouse_darwin.go` 的 `NSEvent.pressedMouseButtons` 辅助。

**当前规避**：macOS 用 purego 查询主键是否按下，松手后再收回；Windows/Linux 暂用 300 ms 稳定延迟。

## 3. 原生拖动的位置约束

**状态：待确认。**

**场景**：原生拖动会允许桌宠短暂越出屏幕边缘；Lucia 只能在拖动结束后收回。若上游能在拖动循环内约束窗口，边缘行为会更自然，也不需要额外的松手检测。

**可能接口**：

- `Window.SetDragBounds(Rectangle)`：拖动时至少保留多少可见区域；或
- `WindowOptions.KeepOnScreen` 一类的窗口级开关。

该能力与第 2 项可以合并设计。

## 4. Panel 在 Space 切换时的显示行为

**状态：待确认。**

**场景**：macOS 开启“显示器独立 Space”时，`Panel` 在左右滑动切换桌面会先消失再出现。Lucia 已接受这是系统过渡动画，没有在 A/B 中处理。

**期望能力**：

- 提供 Panel 的 Space 过渡选项，避免不必要的淡出；或
- 明确文档说明这是当前 `Panel` 形态的系统行为，不提供规避。

**相关文档**：`docs/specs/desktop-pet.md` 的风险与回退。

## 5. 其他应用全屏与当前 Space 查询

**状态：待确认。**

**场景**：后续如果要实现“全屏游戏/视频时隐藏桌宠或暂停走动”，需要判断其他应用是否处于全屏，以及桌宠所在 Space。MyGo v0.9.0 仍只提供窗口自身的 `IsFullScreen`，没有跨应用全屏或当前 Space 查询。

**期望能力**：

- `Screen` 或 `App` 级查询：当前是否有其他应用全屏、当前活动 Space；或
- 提供可订阅的状态变化事件。

**当前处理**：不做全屏检测；全屏行为推迟到用户实际反馈后再考虑。

## 6. 固定大小 Panel 的窗口状态保存

**状态：已记录，本地规避已实测。**

**场景与根因**：macOS 27.2 上，固定大小的桌宠 Panel 在普通 248×248 DIP 边界下，`isZoomed` 已返回 `true`。MyGo v0.9.0 的 darwin `Window.IsMaximized` 直接使用该值；`window_state.go` 的 `captureWindowState` 因此跳过正常边界，首次启动没有旧边界时，`pet` 连尺寸都未进入保存结果。Lucia 原有恢复路径又会调用 `Unmaximize`（原生 `zoom:`），在真机上使正常窗口上移约 20 DIP。这个结果尚未证明由 v0.9.0 升级引入。

**期望能力**：固定大小且禁止最大化的 Panel 能正确采集普通边界；正常边界下不会因最大化标志触发反向 `zoom:`。应覆盖窗口首次创建、移动、正常退出与重启。

**当前规避**：macOS 通过公开的 `App.SetWindowStateStore` 接口保存实际桌宠边界，保留默认文件格式与其他窗口的状态；退出前显式写入，关闭后继续保留最后的有效边界，并遵守 `App.ClearWindowStates` 的本次运行清空语义。恢复路径直接校正固定尺寸与屏幕位置，不再按该标志调用 `Unmaximize`。代码为 `cmd/lucia/window_state.go`、`cmd/lucia/pet_window.go`，数据归属见[架构说明](architecture.md#3-用户目录)。

**验证与移除条件**：原始基线失败、修复后的拖动/退出/重启、尺寸切换及隐藏恢复结果见[实机验收的回归复测](testing-mygo-0.9.md#位置恢复修复后的回归复测)。上游提供修复后，先验证固定 Panel 的采集与恢复，再移除本地存储适配；本次未向上游发送内容。

## 维护规则

- 新需求只追加到本文，写清场景、期望接口、当前规避和代码位置。
- 不直接创建上游 issue、PR 或评论；需要贡献时先由项目所有者确认范围与措辞。
- 上游发布替代 API 后，更新本文状态，并在实现中移除对应的本地规避。
