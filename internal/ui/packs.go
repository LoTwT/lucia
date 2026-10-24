package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// PackSummary is the console's view of one installed pack. It deliberately
// contains no filesystem handles; the application layer owns petpack.Store.
type PackSummary struct {
	ID          string
	Name        string
	Schema      int
	Active      bool
	Valid       bool
	Error       string
	Clips       int
	Frames      int
	Hotzones    []PackHotzone
	Presets     int
	PresetList  []PackPreset
	ActionList  []PackAction
	License     string
	InstalledAt string
}

// PackPreset is one reusable interaction sequence.
type PackPreset struct {
	ID       string
	Reaction string
	Bubble   string
	Steps    []PackPresetStep
}

// PackPresetStep is one action and its delay before starting.
type PackPresetStep struct {
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
	ID     string
	Label  string
	Preset string
	Action string
	X      float64
	Y      float64
	Width  float64
	Height float64
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
			if ui.Button(c, "刷新").Clicked() {
				m.packsLoaded = false
				m.packPreviewID = ""
				m.packActionErr = ""
				m.refreshPacks()
			}
			if m.cfg.ActivatePack != nil && m.activePackID() != "" {
				if ui.Button(c, "使用内置鲸鱼").Clicked() {
					m.activatePack("")
				}
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
		if len(m.packs) == 0 {
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
					if pack.Active {
						ui.Badge(c, "使用中")
					}
					if !pack.Valid {
						ui.Badge(c, "无效").TextColor(t.Danger)
					}
				})
				if pack.Valid {
					ui.Textf(c, "schema %d · %d 个动画 · %d 帧 · %d 个热区 · %d 个预设 · 授权 %s",
						pack.Schema, pack.Clips, pack.Frames, len(pack.Hotzones), pack.Presets, pack.License).
						FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Text(c, pack.Error).FontSize(11).TextColor(t.Danger).Selectable()
				}
			})
			if ui.Button(c, "预览").Clicked() {
				m.packSelected = pack.ID
				m.loadPackPreview()
			}
			if pack.Valid && !pack.Active && m.cfg.ActivatePack != nil {
				if ui.PrimaryButton(c, "启用").Clicked() {
					m.activatePack(pack.ID)
				}
			}
			if !pack.Active && m.cfg.DeletePack != nil {
				if ui.Button(c, "删除").Clicked() {
					if m.cfg.ConfirmDeletePack == nil || m.cfg.ConfirmDeletePack(pack.ID, pack.Name) {
						m.deletePack(pack.ID)
					}
				}
			}
		})
	})
}

func (m *Model) packImportPanel(c *ui.Context) {
	if m.cfg.ChoosePackZip == nil || m.cfg.ImportPack == nil {
		return
	}
	t := c.Theme()
	panel(c, "导入素材包", func() {
		ui.Text(c, "选择包含透明动画 WebP 的 zip。导入会先校验 manifest、全部帧和 SHA-256，失败时不会替换现有素材。").
			FontSize(13).TextColor(t.TextMuted).MaxWidth(700)
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			zipLabel := "未选择文件"
			if m.packZip != "" {
				zipLabel = m.packZip
			}
			ui.Text(c, zipLabel).FontSize(12).Grow(1).SingleLine().Selectable().
				Tooltip(zipLabel).TextColor(t.TextMuted)
			if ui.Button(c, "选择 zip…").Clicked() {
				path, err := m.cfg.ChoosePackZip()
				if err != nil {
					m.packActionErr = fmt.Sprintf("选择素材包失败：%v", err)
				} else if path != "" {
					m.packZip = path
					m.packLicense = false
					m.packActionErr = ""
					m.packNotice = ""
				}
			}
		})
		ui.Checkbox(c, &m.packLicense, "我确认拥有该素材包的本地使用授权，且不会再分发其中的美术资源").
			Label("素材包授权确认")
		if m.cfg.ReplacePack != nil {
			ui.Checkbox(c, &m.packReplace, "替换同名素材包（失败会回滚到旧版本）").
				Label("替换同名素材包")
		}
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "导入并启用").Disabled(m.packZip == "" || !m.packLicense).Clicked() {
				m.importPack()
			}
			ui.Text(c, "导入过程中窗口可能短暂卡顿；完成后素材包立即生效。").
				FontSize(11).TextColor(t.TextMuted)
		})
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
				ui.Textf(c, "%s · schema %d", pack.ID, pack.Schema).FontSize(11).TextColor(t.TextMuted)
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
						action := zone.Action
						if action == "" {
							action = "使用预设动作"
						}
						ui.Textf(c, "· %s：预设 %s，动作 %s", label, zone.Preset, action).
							FontSize(12).TextColor(t.TextMuted)
					}
				}
				if len(pack.PresetList) > 0 {
					ui.Text(c, "交互预设").FontSize(12).Bold()
					for _, preset := range pack.PresetList {
						ui.Textf(c, "· %s（%s/%s）：%s", preset.ID, preset.Reaction, preset.Bubble,
							presetStepsText(preset.Steps)).FontSize(12).TextColor(t.TextMuted)
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
	if m.packSelected == "" || m.cfg.LoadPackPreview == nil {
		m.packPreview = nil
		m.packPreviewID = ""
		m.packPreviewErr = ""
		return
	}
	if m.packPreviewID == m.packSelected {
		return
	}
	m.packPreviewID = m.packSelected
	m.packPreview = nil
	m.packPreviewErr = ""
	data, err := m.cfg.LoadPackPreview(m.packSelected)
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

func (m *Model) importPack() {
	if m.cfg.ImportPack == nil || m.packZip == "" || !m.packLicense {
		return
	}
	importFn := m.cfg.ImportPack
	if m.packReplace && m.cfg.ReplacePack != nil {
		importFn = m.cfg.ReplacePack
	}
	pack, err := importFn(m.packZip, m.packLicense)
	if err != nil {
		m.packActionErr = fmt.Sprintf("导入素材包失败：%v", err)
		m.packNotice = ""
		return
	}
	m.packActionErr = ""
	m.packNotice = fmt.Sprintf("已导入 %s，正在启用…", pack.Name)
	m.packSelected = pack.ID
	if err := m.activatePackBackend(pack.ID); err != nil {
		m.packActionErr = fmt.Sprintf("素材包已导入，但启用失败：%v", err)
	} else {
		m.packNotice = fmt.Sprintf("已导入并启用 %s。", pack.Name)
	}
	m.packZip = ""
	m.packLicense = false
	m.packReplace = false
	m.packsLoaded = false
	m.packPreviewID = ""
	m.refreshPacks()
}

func (m *Model) activatePack(id string) {
	if err := m.activatePackBackend(id); err != nil {
		m.packActionErr = fmt.Sprintf("启用素材包失败：%v", err)
		return
	}
	m.packActionErr = ""
	m.packNotice = "已切换素材包；桌宠窗口会立即使用新皮肤。"
	m.packSelected = id
	m.packsLoaded = false
	m.refreshPacks()
}

func (m *Model) activatePackBackend(id string) error {
	if m.cfg.ActivatePack == nil {
		return nil
	}
	return m.cfg.ActivatePack(id)
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
		m.packSelected = ""
	}
	m.packsLoaded = false
	m.refreshPacks()
}

func presetStepsText(steps []PackPresetStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.DelayMs > 0 {
			parts = append(parts, fmt.Sprintf("%s（等待 %dms）", step.Action, step.DelayMs))
		} else {
			parts = append(parts, step.Action)
		}
	}
	if len(parts) == 0 {
		return "使用热区动作"
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
