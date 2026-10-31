package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// PackExistsError is the UI-facing same-id conflict. The application maps
// petpack.PackExistsError into this type so the UI package stays independent
// of the storage layer.
type PackExistsError struct {
	ID   string
	Name string
}

func (e *PackExistsError) Error() string {
	return fmt.Sprintf("素材包 %q 已存在", e.ID)
}

// PackSummary is the console's view of one installed pack. It deliberately
// contains no filesystem handles; the application layer owns petpack.Store.
type PackSummary struct {
	ID     string
	Name   string
	Active bool
	// Builtin marks the embedded fallback summary. It does not count as an
	// installed user pack for the empty-state hint.
	Builtin         bool
	Valid           bool
	Error           string
	Clips           int
	Frames          int
	Hotzones        []PackHotzone
	Interactions    int
	InteractionList []PackInteraction
	ActionList      []PackAction
	License         string
	InstalledAt     string
}

// PackInteraction is one authored response.
type PackInteraction struct {
	ID     string
	Label  string
	Bubble string
	Motion string
	Steps  []PackInteractionStep
}

// PackInteractionStep is one action and its delay before starting.
type PackInteractionStep struct {
	Action  string
	DelayMs int
}

// PackAction exposes authored phase holds in the preview.
type PackAction struct {
	ID        string
	Label     string
	LoopCount int
	EnterHold int
	LoopHold  int
	ExitHold  int
}

// PackHotzone is one normalized preview rectangle.
type PackHotzone struct {
	ID          string
	Label       string
	Interaction string
	X           float64
	Y           float64
	Width       float64
	Height      float64
}

func (m *Model) packsPage(c *ui.Context) {
	if !m.packsLoaded {
		m.refreshPacks()
	}
	t := c.Theme()
	panel(c, "素材包", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Text(c, "本地动画素材").FontSize(14)
				ui.Text(c, "素材包只安装在本机数据目录，不进入仓库、CI 或安装包。").
					FontSize(12).TextColor(t.TextMuted)
			})
			if ui.Button(c, "刷新").Disabled(m.packBusy).Clicked() {
				m.packsLoaded = false
				m.invalidatePackPreview()
				m.packActionErr = ""
				m.refreshPacks()
			}
			if m.cfg.OpenPacksDir != nil && ui.Button(c, "打开目录").Clicked() {
				if err := m.cfg.OpenPacksDir(); err != nil {
					m.packActionErr = fmt.Sprintf("打开素材包目录失败：%v", err)
				}
			}
		})
		if m.packsErr != "" {
			ui.Text(c, m.packsErr).FontSize(12).TextColor(t.Danger).Selectable()
		}
		if m.packsErr == "" && !hasUserPacks(m.packs) {
			ui.Text(c, "还没有安装素材包；内置鲸鱼会继续作为默认皮肤。").
				FontSize(13).TextColor(t.TextMuted)
		}
		for _, pack := range m.packs {
			m.packRow(c, pack)
		}
	})

	m.packImportPanel(c)
	m.packPreviewPanel(c)
}

func (m *Model) packRow(c *ui.Context, pack PackSummary) {
	t := c.Theme()
	selected := m.packSelected == pack.ID
	bg := t.Surface
	if selected {
		bg = t.Background
	}
	ui.Column(c).Padding(12).Gap(6).Radius(9).Background(bg).
		Border(1, t.Border).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
					ui.Text(c, pack.Name).FontSize(14).Bold()
					ui.Text(c, pack.ID).FontSize(11).TextColor(t.TextMuted)
					if pack.Builtin {
						ui.Badge(c, "内置")
					}
					if pack.Active {
						ui.Badge(c, "使用中")
					}
					if !pack.Valid {
						ui.Badge(c, "无效").TextColor(t.Danger)
					}
				})
				if pack.Valid {
					ui.Textf(c, "%d 个动画 · %d 帧 · %d 个热区 · %d 个交互 · 授权 %s",
						pack.Clips, pack.Frames, len(pack.Hotzones), pack.Interactions, pack.License).
						FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Text(c, pack.Error).FontSize(11).TextColor(t.Danger).Selectable()
				}
			})
			if ui.Button(c, "预览").Disabled(m.packBusy).Clicked() {
				m.packSelected = pack.ID
				m.loadPackPreview()
			}
			if pack.Valid && !pack.Active && (m.cfg.ActivatePack != nil || m.cfg.ActivatePackAsync != nil) {
				if ui.PrimaryButton(c, "启用").Disabled(m.packBusy).Clicked() {
					m.activatePack(pack.ID)
				}
			}
			if !pack.Active && !pack.Builtin && m.cfg.DeletePack != nil {
				if ui.Button(c, "删除").Disabled(m.packBusy).Clicked() {
					m.confirmDeletePack(pack.ID, pack.Name)
				}
			}
		})
	})
}

func (m *Model) packImportPanel(c *ui.Context) {
	if (m.cfg.ChoosePackZip == nil && m.cfg.ChoosePackZipAsync == nil) ||
		(m.cfg.InstallPack == nil && m.cfg.InstallPackAsync == nil) {
		return
	}
	t := c.Theme()
	panel(c, "导入素材包", func() {
		ui.Text(c, "选择包含透明动画 WebP 的 zip。导入会先校验 manifest、全部帧和 SHA-256；仅在同名素材包已存在时询问是否替换。").
			FontSize(13).TextColor(t.TextMuted).MaxWidth(700)
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			zipLabel := "未选择文件"
			if m.packZip != "" {
				zipLabel = m.packZip
			}
			ui.Text(c, zipLabel).FontSize(12).Grow(1).SingleLine().Selectable().
				Tooltip(zipLabel).TextColor(t.TextMuted)
			if ui.Button(c, "选择 zip 文件").Disabled(m.packBusy).Clicked() {
				m.choosePackZip()
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "导入并启用").Disabled(m.packBusy || m.packZip == "").Clicked() {
				m.importPack()
			}
			ui.Text(c, "导入在后台进行；大型素材包可能需要十几秒，期间桌宠保持运行。").
				FontSize(11).TextColor(t.TextMuted)
		})
		if m.packBusy {
			ui.Text(c, "正在处理素材包…").FontSize(12).TextColor(t.TextMuted)
		}
		if m.packNotice != "" {
			ui.Text(c, m.packNotice).FontSize(12).TextColor(t.Success).Selectable()
		}
		if m.packActionErr != "" {
			ui.Text(c, m.packActionErr).FontSize(12).TextColor(t.Danger).Selectable()
		}
	})
}

func (m *Model) packPreviewPanel(c *ui.Context) {
	pack, ok := m.selectedPack()
	if !ok {
		return
	}
	t := c.Theme()
	panel(c, "预览与热区", func() {
		if !pack.Valid {
			ui.Text(c, "该素材包 manifest 无效，无法生成预览。").
				FontSize(13).TextColor(t.Danger)
			return
		}
		ui.Row(c).AlignItems(ui.Start).Gap(18).Children(func() {
			m.packPreviewImage(c)
			ui.Column(c).Gap(7).Grow(1).Children(func() {
				ui.Text(c, pack.Name).FontSize(15).Bold()
				ui.Text(c, pack.ID).FontSize(11).TextColor(t.TextMuted)
				ui.Textf(c, "授权：%s", pack.License).FontSize(12).TextColor(t.TextMuted)
				if pack.InstalledAt != "" {
					ui.Textf(c, "安装于：%s", pack.InstalledAt).FontSize(12).TextColor(t.TextMuted)
				}
				if len(pack.Hotzones) == 0 {
					ui.Text(c, "没有定义热区；桌宠仍会播放 idle，但立绘不可点击。").
						FontSize(12).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "热区").FontSize(12).Bold()
					for _, zone := range pack.Hotzones {
						label := zone.Label
						if label == "" {
							label = zone.ID
						}
						ui.Textf(c, "· %s：交互 %s", label, zone.Interaction).
							FontSize(12).TextColor(t.TextMuted)
					}
				}
				if len(pack.InteractionList) > 0 {
					ui.Text(c, "交互").FontSize(12).Bold()
					for _, interaction := range pack.InteractionList {
						label := interaction.Label
						if label == "" {
							label = interaction.ID
						}
						ui.Textf(c, "· %s：%s（%s/%s）", label, interactionStepsText(interaction.Steps),
							interaction.Bubble, interaction.Motion).FontSize(12).TextColor(t.TextMuted)
					}
				}
				if timed := timedActions(pack.ActionList); len(timed) > 0 {
					ui.Text(c, "动作时序").FontSize(12).Bold()
					for _, action := range timed {
						label := action.Label
						if label == "" {
							label = action.ID
						}
						ui.Textf(c, "· %s：enter %dms / loop %dms / exit %dms",
							label, action.EnterHold, action.LoopHold, action.ExitHold).
							FontSize(12).TextColor(t.TextMuted)
					}
				}
			})
		})
	})
}

func (m *Model) packPreviewImage(c *ui.Context) {
	if m.cfg.LoadPackPreview == nil {
		ui.Text(c, "当前宿主不提供素材包预览。").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	if m.packPreviewErr != "" {
		ui.Text(c, m.packPreviewErr).FontSize(12).TextColor(c.Theme().Danger)
		return
	}
	if m.packPreview == nil || m.packPreviewW <= 0 || m.packPreviewH <= 0 {
		ui.Text(c, "正在生成预览…").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	const maxWidth, maxHeight = 240, 360
	scale := min(maxWidth/float32(m.packPreviewW), maxHeight/float32(m.packPreviewH))
	width := float32(m.packPreviewW) * scale
	height := float32(m.packPreviewH) * scale
	ui.Box(c).Width(width).Height(height).Clip().Radius(8).
		Background(c.Theme().Surface).Children(func() {
		ui.Image(c, m.packPreview).Size(width, height).Fit(ui.Contain).Label("素材包预览")
		pack, ok := m.selectedPack()
		if !ok {
			return
		}
		for _, zone := range pack.Hotzones {
			zone := zone
			label := zone.Label
			if label == "" {
				label = zone.ID
			}
			ui.Box(c).Absolute().
				LeftPercent(float32(zone.X*100)).
				TopPercent(float32(zone.Y*100)).
				WidthPercent(float32(zone.Width*100)).
				HeightPercent(float32(zone.Height*100)).
				Border(1, c.Theme().Accent).Radius(4).
				Label("热区 " + label)
		}
	})
}

func (m *Model) refreshPacks() {
	m.packsLoaded = true
	m.packsErr = ""
	if m.cfg.LoadPacks == nil {
		m.packs = nil
		return
	}
	packs, err := m.cfg.LoadPacks()
	if err != nil {
		m.packsErr = fmt.Sprintf("读取素材包失败：%v", err)
		return
	}
	m.packs = packs
	if _, ok := m.selectedPack(); !ok {
		m.packSelected = ""
		for _, pack := range packs {
			if pack.Active {
				m.packSelected = pack.ID
				break
			}
		}
		if m.packSelected == "" && len(packs) > 0 {
			m.packSelected = packs[0].ID
		}
	}
	m.loadPackPreview()
}

func hasUserPacks(packs []PackSummary) bool {
	for _, pack := range packs {
		if !pack.Builtin {
			return true
		}
	}
	return false
}

func (m *Model) selectedPack() (PackSummary, bool) {
	for _, pack := range m.packs {
		if pack.ID == m.packSelected {
			return pack, true
		}
	}
	return PackSummary{}, false
}

func (m *Model) activePackID() string {
	for _, pack := range m.packs {
		if pack.Active {
			return pack.ID
		}
	}
	return ""
}

func (m *Model) loadPackPreview() {
	if m.packSelected == "" || (m.cfg.LoadPackPreview == nil && m.cfg.LoadPackPreviewAsync == nil) {
		m.invalidatePackPreview()
		return
	}
	if m.packPreviewID == m.packSelected {
		// The in-flight request will deliver this selection; a usable bitmap
		// needs no reload. Only a reported failure is retried.
		if m.packPreviewLoading || (m.packPreview != nil && m.packPreviewErr == "") {
			return
		}
	}
	id := m.packSelected
	m.packPreviewID = id
	m.packPreview = nil
	m.packPreviewErr = ""
	m.packPreviewSeq++
	seq := m.packPreviewSeq
	if m.cfg.LoadPackPreviewAsync != nil {
		m.packPreviewLoading = true
		m.cfg.LoadPackPreviewAsync(id, func(data []byte, err error) {
			if seq != m.packPreviewSeq {
				return
			}
			m.packPreviewLoading = false
			m.applyPackPreview(data, err)
		})
		return
	}
	m.packPreviewLoading = false
	m.applyPackPreview(m.cfg.LoadPackPreview(id))
}

// invalidatePackPreview drops the current poster and makes any in-flight
// request stale, so a result for a previous selection cannot land later.
func (m *Model) invalidatePackPreview() {
	m.packPreviewSeq++
	m.packPreview = nil
	m.packPreviewID = ""
	m.packPreviewW, m.packPreviewH = 0, 0
	m.packPreviewErr = ""
	m.packPreviewLoading = false
}

// applyPackPreview decodes a generated poster on the UI thread. Generation is
// off-thread, but ui.DecodeBitmap must stay here so the bitmap is owned by the
// UI thread.
func (m *Model) applyPackPreview(data []byte, err error) {
	if err != nil {
		m.packPreviewErr = fmt.Sprintf("生成预览失败：%v", err)
		return
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		m.packPreviewErr = fmt.Sprintf("解码预览失败：%v", err)
		return
	}
	m.packPreview = bitmap
	m.packPreviewW, m.packPreviewH = bitmap.Size()
}

func (m *Model) choosePackZip() {
	if m.packBusy {
		return
	}
	if m.cfg.ChoosePackZipAsync != nil {
		m.packBusy = true
		m.packActionErr = ""
		m.cfg.ChoosePackZipAsync(func(path string, err error) {
			m.packBusy = false
			if err != nil {
				m.packActionErr = fmt.Sprintf("选择素材包失败：%v", err)
				return
			}
			if path == "" {
				return
			}
			m.packZip = path
			m.packActionErr = ""
			m.packNotice = ""
		})
		return
	}
	if m.cfg.ChoosePackZip == nil {
		return
	}
	path, err := m.cfg.ChoosePackZip()
	if err != nil {
		m.packActionErr = fmt.Sprintf("选择素材包失败：%v", err)
		return
	}
	if path == "" {
		return
	}
	m.packZip = path
	m.packActionErr = ""
	m.packNotice = ""
}

func (m *Model) confirmDeletePack(id, name string) {
	if m.packBusy {
		return
	}
	if m.cfg.ConfirmDeletePackAsync != nil {
		m.packBusy = true
		m.cfg.ConfirmDeletePackAsync(id, name, func(ok bool) {
			m.packBusy = false
			if ok {
				m.deletePack(id)
			}
		})
		return
	}
	if m.cfg.ConfirmDeletePack == nil || m.cfg.ConfirmDeletePack(id, name) {
		m.deletePack(id)
	}
}

func (m *Model) importPack() {
	if (m.cfg.InstallPack == nil && m.cfg.InstallPackAsync == nil) || m.packZip == "" || m.packBusy {
		return
	}
	m.packBusy = true
	m.packActionErr = ""
	m.packNotice = ""
	m.installPack(false)
}

func (m *Model) installPack(replace bool) {
	previousActive := m.activePackID()
	if m.cfg.InstallPackAsync != nil {
		m.cfg.InstallPackAsync(m.packZip, replace, func(pack PackSummary, err error) {
			m.finishInstall(previousActive, pack, err, replace)
		})
		return
	}
	pack, err := m.cfg.InstallPack(m.packZip, replace)
	m.finishInstall(previousActive, pack, err, replace)
}

func (m *Model) finishInstall(previousActive string, pack PackSummary, err error, replace bool) {
	if err != nil {
		var exists *PackExistsError
		if !replace && errors.As(err, &exists) && m.cfg.ConfirmReplacePackAsync != nil {
			m.cfg.ConfirmReplacePackAsync(exists.ID, exists.Name, func(ok bool) {
				if !ok {
					m.packBusy = false
					m.packNotice = "已取消导入，未修改任何素材包。"
					return
				}
				m.installPack(true)
			})
			return
		}
		m.packBusy = false
		m.packActionErr = fmt.Sprintf("导入素材包失败：%v", err)
		return
	}
	m.packActionErr = ""
	m.packNotice = fmt.Sprintf("已导入 %s，正在启用…", pack.Name)
	m.invalidatePackPreview()
	m.packSelected = pack.ID
	m.activateInstalled(previousActive, pack)
}

// activateInstalled enables a freshly installed pack while the import stays
// busy. It keeps the original fallback: a failed activation of a replaced pack
// switches back to the built-in whale so no running model holds the old files.
func (m *Model) activateInstalled(previousActive string, pack PackSummary) {
	m.activatePackBackend(pack.ID, func(activateErr error) {
		if activateErr != nil {
			m.packActionErr = fmt.Sprintf("素材包已导入，但启用失败：%v", activateErr)
			m.packNotice = fmt.Sprintf("已导入 %s，但未启用。", pack.Name)
			if strings.EqualFold(previousActive, pack.ID) {
				m.activatePackBackend("", func(fallbackErr error) {
					if fallbackErr != nil {
						m.packActionErr += fmt.Sprintf("；回退内置鲸鱼也失败：%v", fallbackErr)
					} else {
						m.packActionErr += "；已切回内置鲸鱼，旧素材包不再被运行中的模型占用"
						m.packNotice = fmt.Sprintf("已导入 %s，但启用失败；已切回内置鲸鱼。", pack.Name)
					}
					m.finishPackChange()
				})
				return
			}
		} else {
			m.packNotice = fmt.Sprintf("已导入并启用 %s。", pack.Name)
		}
		m.finishPackChange()
	})
}

// finishPackChange clears the busy state and reloads the list after an import
// and its activation both completed.
func (m *Model) finishPackChange() {
	m.packBusy = false
	m.packZip = ""
	m.packsLoaded = false
	m.invalidatePackPreview()
	m.refreshPacks()
}

func (m *Model) activatePack(id string) {
	if m.packBusy {
		return
	}
	m.packBusy = true
	m.packActionErr = ""
	m.packNotice = ""
	m.activatePackBackend(id, func(err error) {
		m.packBusy = false
		if err != nil {
			m.packActionErr = fmt.Sprintf("启用素材包失败：%v", err)
			return
		}
		m.packActionErr = ""
		m.packNotice = "已切换素材包；桌宠窗口会立即使用新皮肤。"
		m.packSelected = id
		m.packsLoaded = false
		m.refreshPacks()
	})
}

// activatePackBackend requests an activation and calls done exactly once, on
// the UI thread. Hosts that load packs asynchronously report through done
// later; synchronous hosts call it inline.
func (m *Model) activatePackBackend(id string, done func(error)) {
	if m.cfg.ActivatePackAsync != nil {
		m.cfg.ActivatePackAsync(id, done)
		return
	}
	if m.cfg.ActivatePack == nil {
		done(nil)
		return
	}
	done(m.cfg.ActivatePack(id))
}

func (m *Model) deletePack(id string) {
	if m.cfg.DeletePack == nil {
		return
	}
	if err := m.cfg.DeletePack(id); err != nil {
		m.packActionErr = fmt.Sprintf("删除素材包失败：%v", err)
		return
	}
	m.packActionErr = ""
	m.packNotice = "素材包已删除。"
	if m.packSelected == id {
		m.invalidatePackPreview()
		m.packSelected = ""
	}
	remaining := m.packs[:0]
	for _, pack := range m.packs {
		if pack.ID != id {
			remaining = append(remaining, pack)
		}
	}
	m.packs = remaining
	m.packsLoaded = false
	m.refreshPacks()
	if m.packsErr != "" {
		// The delete succeeded even if the follow-up listing could not read
		// the directory. Do not present the stale row or claim a failed
		// deletion.
		m.packNotice = "素材包已删除，但列表刷新失败。"
	}
}

func interactionStepsText(steps []PackInteractionStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.DelayMs > 0 {
			parts = append(parts, fmt.Sprintf("%s（等待 %dms）", step.Action, step.DelayMs))
		} else {
			parts = append(parts, step.Action)
		}
	}
	if len(parts) == 0 {
		return "仅播放宿主反馈"
	}
	return strings.Join(parts, " → ")
}

func timedActions(actions []PackAction) []PackAction {
	out := make([]PackAction, 0, len(actions))
	for _, action := range actions {
		if action.EnterHold != 0 || action.LoopHold != 0 || action.ExitHold != 0 {
			out = append(out, action)
		}
	}
	return out
}
