package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

func (m *Model) updatesPage(c *ui.Context) {
	t := c.Theme()
	snap := m.cfg.Updates.Snapshot()

	panel(c, "状态", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(9).Children(func() {
			statusDot(c, phaseColor(t, snap.Phase))
			ui.Text(c, phaseTitle(snap)).FontSize(16).Bold()
			ui.Spacer(c)
			ui.Badge(c, "当前 v"+m.cfg.Version).TextColor(t.TextMuted)
		})
		ui.Text(c, phaseDetail(snap)).FontSize(13).TextColor(t.TextMuted).MaxWidth(640)
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
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			checkLabel := "检查更新"
			if snap.Phase == updates.PhaseFailed && snap.Release == nil {
				checkLabel = "重新检查"
			}
			if ui.Button(c, checkLabel).
				Disabled(snap.Busy || snap.Phase == updates.PhaseUnavailable).Clicked() {
				m.cfg.Updates.CheckAsync()
			}
			if canInstall(snap) {
				if ui.PrimaryButton(c, "安装并重启").Disabled(snap.Busy).Clicked() {
					m.cfg.Updates.InstallAsync()
				}
			}
			if snap.Phase == updates.PhaseAvailable && snap.Release != nil {
				ui.Text(c, "安装完成后会自动重启。").FontSize(12).TextColor(t.TextMuted)
			}
		})
	})

	if snap.Release != nil {
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

	panel(c, "自动检查", func() {
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
	})

	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Icon(c, checkIcon).FontSize(13).TextColor(t.TextMuted)
		if m.cfg.UpdateSource != "" {
			ui.Textf(c, "更新由 MyGo 校验签名并从 GitHub Releases（%s）获取；开发构建不启用。",
				m.cfg.UpdateSource).FontSize(12).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "更新由 MyGo 校验签名并从 GitHub Releases 获取；开发构建不启用。").
				FontSize(12).TextColor(t.TextMuted)
		}
	})
}

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
				ui.Text(c, "基于 MyGo 原生 UI 的桌面应用").FontSize(13).TextColor(t.TextMuted)
			})
		})
		ui.Text(c, "一个可运行、可打包、可发版的 MyGo 应用底座：原生界面、设置持久化、诊断日志、签名更新与双平台发布流程已经接好。").
			FontSize(13).TextColor(t.TextMuted).MaxWidth(620)
	})

	panel(c, "构建", func() {
		m.infoRow(c, "版本", "v"+m.cfg.Version)
		m.infoRow(c, "构建", m.cfg.Commit)
		m.infoRow(c, "平台", m.cfg.Platform)
		m.infoRow(c, "标识", "com.lotwt.lucia")
		if m.cfg.UpdateSource != "" {
			m.infoRow(c, "更新源", m.cfg.UpdateSource)
		}
	})

	panel(c, "数据", func() {
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

	panel(c, "链接", func() {
		ui.Row(c).Gap(18).Wrap().Children(func() {
			ui.Link(c, "MyGo 文档", "https://mygo.egoist.dev/docs/")
			if m.cfg.RepositoryURL != "" {
				ui.Link(c, "源码与发布记录", m.cfg.RepositoryURL)
			}
		})
		ui.Text(c, "MyGo v0.5.5 · Go 1.27.2 · 更新私钥只保存在发布环境，不进入仓库。").
			FontSize(12).TextColor(t.TextMuted)
	})
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
