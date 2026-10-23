package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (m *Model) aboutPage(c *ui.Context) {
	t := c.Theme()
	panel(c, "", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
			m.logoMark(c, 64)
			ui.Column(c).Gap(3).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "Lucia").FontSize(26).Bold()
					ui.Badge(c, "v"+m.cfg.Version).TextColor(t.TextMuted)
				})
			})
		})
	})

	m.updateStatusPanel(c)
	m.releaseNotesPanel(c)
	m.dataPanel(c)
	m.aboutLinks(c)
}

func (m *Model) updateStatusPanel(c *ui.Context) {
	t := c.Theme()
	snap := m.cfg.Updates.Snapshot()

	panel(c, "更新", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
			statusDot(c, phaseColor(t, snap.Phase))
			ui.Text(c, phaseTitle(snap)).FontSize(16).Bold()
		})
		if detail := phaseDetail(snap); detail != "" {
			ui.Text(c, detail).FontSize(13).TextColor(t.TextMuted).MaxWidth(640)
		}
		if !snap.LastChecked.IsZero() && snap.Phase != updates.PhaseChecking {
			ui.Textf(c, "上次检查：%s", checkTime(snap.LastChecked)).FontSize(12).TextColor(t.TextMuted)
		}
		if snap.Phase == updates.PhaseDownloading {
			value := -1.0
			if snap.Total > 0 {
				value = snap.Progress
			}
			ui.Progress(c, value).Label("更新下载进度")
			if snap.Total > 0 {
				ui.Textf(c, "%d%%", int(value*100)).FontSize(12).TextColor(t.TextMuted)
			}
		}
		if snap.Phase != updates.PhaseUnavailable {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				checkLabel := "检查更新"
				if snap.Phase == updates.PhaseFailed && snap.Release == nil {
					checkLabel = "重新检查"
				}
				if ui.Button(c, checkLabel).Disabled(snap.Busy).Clicked() {
					m.cfg.Updates.CheckAsync()
				}
				if canInstall(snap) {
					if ui.PrimaryButton(c, "安装并重启").Disabled(snap.Busy).Clicked() {
						m.cfg.Updates.InstallAsync()
					}
				}
			})
			ui.Box(c).Height(1).WidthPercent(100).Background(t.Border)
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "启动时自动检查更新").FontSize(14)
					ui.Text(c, "启动时检查一次，运行期间每 24 小时再检查一次；关闭后仍可手动检查。").FontSize(12).
						TextColor(t.TextMuted)
				})
				ui.Switch(c, &m.settings.CheckUpdatesOnLaunch).Label("启动时自动检查更新").
					OnChange(func() {
						m.saveSettings()
						m.cfg.Updates.SetAutoCheck(m.settings.CheckUpdatesOnLaunch)
						if m.settings.CheckUpdatesOnLaunch {
							m.cfg.Updates.CheckAsync()
						}
					})
			})
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				ui.Icon(c, checkIcon).FontSize(13).TextColor(t.TextMuted)
				if m.cfg.UpdateSource != "" {
					ui.Textf(c, "更新经签名校验后从 GitHub Releases（%s）获取。",
						m.cfg.UpdateSource).FontSize(12).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "更新经签名校验后从 GitHub Releases 获取。").
						FontSize(12).TextColor(t.TextMuted)
				}
			})
		}
	})
}

func (m *Model) releaseNotesPanel(c *ui.Context) {
	snap := m.cfg.Updates.Snapshot()
	if snap.Release == nil {
		return
	}
	t := c.Theme()
	release := snap.Release
	panel(c, "版本说明", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, "v"+release.Version).FontSize(15).Bold()
			if !release.Date.IsZero() {
				ui.Text(c, release.Date.Local().Format("2006-01-02")).FontSize(12).
					TextColor(t.TextMuted)
			}
		})
		notes := strings.TrimSpace(release.Notes)
		if notes == "" {
			ui.Text(c, "这个版本没有提供说明。").FontSize(13).TextColor(t.TextMuted)
		} else {
			markdown.View(c, notes, &m.md)
		}
	})
}

func (m *Model) dataPanel(c *ui.Context) {
	t := c.Theme()
	panel(c, "数据目录", func() {
		ui.Text(c, "设置、窗口位置与更新状态保存在本机数据目录，不会写入安装目录。").
			FontSize(13).TextColor(t.TextMuted)
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, m.cfg.DataDir).FontSize(12).Grow(1).SingleLine().Selectable().
				Tooltip(m.cfg.DataDir)
			if m.cfg.OpenDataDir != nil {
				if ui.Button(c, "打开").Clicked() {
					if err := m.cfg.OpenDataDir(); err != nil {
						m.actionErr = fmt.Sprintf("打开数据目录失败：%v", err)
					} else {
						m.actionErr = ""
					}
				}
			}
		})
	})
}

func (m *Model) aboutLinks(c *ui.Context) {
	if m.cfg.RepositoryURL != "" {
		ui.Link(c, "源码与发布记录", m.cfg.RepositoryURL)
	}
}

func (m *Model) saveSettings() {
	if m.cfg.SaveSettings == nil {
		return
	}
	if err := m.cfg.SaveSettings(m.settings); err != nil {
		m.settingsErr = fmt.Sprintf("保存设置失败：%v", err)
		return
	}
	m.settingsErr = ""
}

func canInstall(s updates.Snapshot) bool {
	return s.Release != nil && (s.Phase == updates.PhaseAvailable || s.Phase == updates.PhaseFailed)
}

// checkTime renders when a check last ran, for tests and for pages that
// show a relative time later.
func checkTime(t time.Time) string {
	if t.IsZero() {
		return "从未"
	}
	return t.Local().Format("2006-01-02 15:04")
}
