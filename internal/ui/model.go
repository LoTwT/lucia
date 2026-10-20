// Package ui builds Lucia's main window on MyGo's native UI. The model
// renders application state; it starts background work only through the
// update manager and never waits for network or disk in the view.
package ui

import (
	"fmt"
	"time"

	"github.com/LoTwT/lucia/internal/settings"
	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
)

// Pages of the main window, also used as sidebar item IDs.
const (
	PageOverview = "overview"
	PageUpdates  = "updates"
	PageAbout    = "about"
)

// Config assembles the main-window model.
type Config struct {
	// Version is the running application version.
	Version string
	// Commit is the short source revision, or "unknown".
	Commit string
	// Platform is GOOS/GOARCH of the running binary.
	Platform string
	// DataDir is the application's data root, shown in the window.
	DataDir string
	// Updates is the update manager the window drives. Required.
	Updates *updates.Manager
	// Settings is the preference set loaded at startup.
	Settings settings.Settings
	// SaveSettings persists a preference change. Nil keeps changes in
	// memory only (tests).
	SaveSettings func(settings.Settings) error
	// OpenDataDir opens the data root in the system file manager. Nil hides
	// the action.
	OpenDataDir func() error
	// RepositoryURL is the public repository opened from the About page.
	RepositoryURL string
	// UpdateSource names the release feed, such as "LoTwT/lucia".
	UpdateSource string
}

// Model is the state of the main window, confined to the UI thread.
type Model struct {
	cfg      Config
	page     string
	settings settings.Settings
	md       markdown.Options
	// actionErr reports a failed side effect, such as opening the data
	// directory, next to the action that caused it.
	actionErr string
	// settingsErr reports a failed preference write.
	settingsErr string
}

// New creates the window model on the overview page.
func New(cfg Config) *Model {
	return &Model{
		cfg:      cfg,
		page:     PageOverview,
		settings: cfg.Settings,
	}
}

// View builds the window's interface from the model. It is the window's
// content and runs on the UI thread.
func (m *Model) View(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		m.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			title, subtitle := m.pageHeading()
			ui.Column(c).Padding(26, 32, 16).Gap(3).
				BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
				ui.Text(c, title).FontSize(24).Bold()
				ui.Text(c, subtitle).FontSize(13).TextColor(t.TextMuted)
			})
			ui.Scroll(c).Grow(1).Padding(24, 32, 32).Gap(18).Children(func() {
				ui.Column(c).MaxWidth(820).Gap(18).Children(func() {
					switch m.page {
					case PageUpdates:
						m.updatesPage(c)
					case PageAbout:
						m.aboutPage(c)
					default:
						m.overviewPage(c)
					}
					if m.actionErr != "" {
						ui.Text(c, m.actionErr).FontSize(12).TextColor(t.Danger).Selectable()
					}
					if m.settingsErr != "" {
						ui.Text(c, m.settingsErr).FontSize(12).TextColor(t.Danger).Selectable()
					}
				})
			})
		})
	})
}

// ShowUpdates switches to the updates page and starts a check when there is
// no current result to show. The application menu calls it.
func (m *Model) ShowUpdates() {
	m.page = PageUpdates
	switch m.cfg.Updates.Snapshot().Phase {
	case updates.PhaseIdle, updates.PhaseUpToDate, updates.PhaseFailed:
		m.cfg.Updates.CheckAsync()
	}
}

func (m *Model) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(212).FillHeight().Shrink(0).Background(t.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		m.brand(c)
		ui.Sidebar(c, &m.page, func() {
			ui.SidebarSection(c, "应用", nil, func() {
				ui.SidebarItem(c, PageOverview, overviewIcon, "概览")
				ui.SidebarItem(c, PageUpdates, updatesIcon, "更新").Children(func() {
					if phase := m.cfg.Updates.Snapshot().Phase; phase == updates.PhaseAvailable {
						ui.Badge(c, "新")
					}
				})
				ui.SidebarItem(c, PageAbout, aboutIcon, "关于")
			})
		}).Grow(1).Label("页面")
		ui.Textf(c, "v%s", m.cfg.Version).FontSize(11).TextColor(t.TextMuted).Padding(10, 16)
	})
}

func (m *Model) brand(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(10).Padding(16, 14, 10).Children(func() {
		m.logoMark(c, 34)
		ui.Column(c).Gap(0).Children(func() {
			ui.Text(c, "Lucia").FontSize(16).Bold()
			ui.Text(c, "MyGo 原生应用").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

// logoMark is the app's sun-over-horizon mark, drawn in the view so it
// follows the accent and needs no image decoding.
func (m *Model) logoMark(c *ui.Context, size float32) {
	ui.Box(c).Size(size, size).Radius(size * 0.28).Shrink(0).Clip().
		LinearGradient(ui.LinearGradient{
			From:  ui.Hex("#f59e0b"),
			To:    ui.Hex("#6d28d9"),
			Angle: 135,
		}).Children(func() {
		ui.Box(c).Size(size*0.34, size*0.34).Radius(size).Background(ui.Hex("#fff7ed")).
			Attach(ui.AnchorCenter, ui.AnchorCenter).Top(-size * 0.10)
		ui.Box(c).Height(size*0.16).WidthPercent(100).Background(ui.Hex("#0f172a")).
			Attach(ui.AnchorBottom, ui.AnchorBottom)
	})
}

func (m *Model) pageHeading() (title, subtitle string) {
	switch m.page {
	case PageUpdates:
		return "更新", "检查签名更新，安装后自动重启进入新版本"
	case PageAbout:
		return "关于", "构建信息、数据位置与项目入口"
	default:
		return "概览", "Lucia 的运行状态与常用操作"
	}
}

func (m *Model) overviewPage(c *ui.Context) {
	m.hero(c)
	ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
		m.statusCard(c)
		m.updateSummaryCard(c)
	})
}

func (m *Model) hero(c *ui.Context) {
	ui.Column(c).Padding(26).Gap(14).Radius(14).
		LinearGradient(ui.LinearGradient{
			From:  ui.Hex("#1e1b4b"),
			To:    ui.Hex("#4c1d95"),
			Angle: 125,
		}).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(16).Children(func() {
			m.logoMark(c, 58)
			ui.Column(c).Gap(3).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
					ui.Text(c, "Lucia").FontSize(28).Bold().TextColor(ui.Hex("#faf5ff"))
					ui.Badge(c, "v"+m.cfg.Version).Background(ui.RGBA(255, 255, 255, 0.16)).
						TextColor(ui.Hex("#faf5ff"))
				})
				ui.Text(c, "轻量、原生的 MyGo 桌面应用").FontSize(14).
					TextColor(ui.RGBA(250, 245, 255, 0.72))
			})
		})
		ui.Text(c, "界面由 MyGo 原生绘制，无需 WebView；更新由内置的签名校验链路从 GitHub Releases 获取。").
			FontSize(13).TextColor(ui.RGBA(250, 245, 255, 0.78)).MaxWidth(620)
	})
}

func (m *Model) statusCard(c *ui.Context) {
	card(c, "运行信息", func() {
		m.infoRow(c, "版本", "v"+m.cfg.Version)
		m.infoRow(c, "构建", m.cfg.Commit)
		m.infoRow(c, "平台", m.cfg.Platform)
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "数据").FontSize(12).TextColor(c.Theme().TextMuted).Width(52)
			ui.Text(c, m.cfg.DataDir).FontSize(12).Grow(1).SingleLine().Selectable().
				Tooltip(m.cfg.DataDir)
			if m.cfg.OpenDataDir != nil {
				if ui.Button(c, "").Label("打开数据目录").Tooltip("打开数据目录").
					Children(func() { ui.Icon(c, folderIcon) }).Clicked() {
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

func (m *Model) updateSummaryCard(c *ui.Context) {
	t := c.Theme()
	snap := m.cfg.Updates.Snapshot()
	card(c, "更新", func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			statusDot(c, phaseColor(t, snap.Phase))
			ui.Text(c, phaseTitle(snap)).FontSize(14).Bold()
		})
		ui.Text(c, phaseDetail(snap)).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "").Children(func() {
				ui.Icon(c, arrowIcon)
				ui.Text(c, "打开更新")
			}).Clicked() {
				m.page = PageUpdates
			}
		})
	})
}

func (m *Model) infoRow(c *ui.Context, label, value string) {
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(c.Theme().TextMuted).Width(52)
		ui.Text(c, value).FontSize(12).Grow(1).Selectable()
	})
}

// card draws a titled surface that shares a row with another card.
func card(c *ui.Context, title string, body func()) ui.Element {
	return panelElement(c, title, body).Grow(1).Basis(0)
}

// panel draws a titled surface across the content column.
func panel(c *ui.Context, title string, body func()) ui.Element {
	return panelElement(c, title, body)
}

func panelElement(c *ui.Context, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(11).Radius(12).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(15).Bold()
		}
		body()
	})
}

func statusDot(c *ui.Context, color ui.Color) {
	ui.Box(c).Size(8, 8).Radius(99).Background(color).Shrink(0)
}

func phaseColor(t *ui.Theme, phase updates.Phase) ui.Color {
	switch phase {
	case updates.PhaseAvailable, updates.PhaseInstalled, updates.PhaseUpToDate:
		return t.Success
	case updates.PhaseChecking, updates.PhaseDownloading:
		return t.Accent
	case updates.PhaseFailed:
		return t.Danger
	default:
		return t.TextMuted
	}
}

func phaseTitle(s updates.Snapshot) string {
	switch s.Phase {
	case updates.PhaseUnavailable:
		return "当前构建未启用自动更新"
	case updates.PhaseIdle:
		return "尚未检查更新"
	case updates.PhaseChecking:
		return "正在检查更新"
	case updates.PhaseUpToDate:
		return "已是最新版本"
	case updates.PhaseAvailable:
		if s.Release != nil {
			return "发现新版本 " + s.Release.Version
		}
		return "发现新版本"
	case updates.PhaseDownloading:
		return "正在下载并安装"
	case updates.PhaseInstalled:
		return "更新已安装"
	case updates.PhaseFailed:
		return "更新未完成"
	default:
		return "未知状态"
	}
}

func phaseDetail(s updates.Snapshot) string {
	switch s.Phase {
	case updates.PhaseUnavailable:
		return "开发运行、未配置更新源或安装位置不可写时不会检查更新；正式安装版会启用。"
	case updates.PhaseIdle:
		return "点击“检查更新”读取 GitHub Releases 中的签名更新。"
	case updates.PhaseChecking:
		return "正在读取更新清单并比较版本。"
	case updates.PhaseUpToDate:
		if s.LastChecked.IsZero() {
			return "当前版本已是最新。"
		}
		return "当前版本已是最新，上次检查于 " + s.LastChecked.Local().Format("15:04") + "。"
	case updates.PhaseAvailable:
		if s.Release != nil {
			return "新版本 " + s.Release.Version + " 已就绪，可安装并重启。"
		}
		return "新版本已就绪，可安装并重启。"
	case updates.PhaseDownloading:
		if s.Total > 0 {
			return fmt.Sprintf("已下载 %s / %s。", formatBytes(s.Downloaded), formatBytes(s.Total))
		}
		return fmt.Sprintf("已下载 %s。", formatBytes(s.Downloaded))
	case updates.PhaseInstalled:
		return "应用即将重启进入新版本。"
	case updates.PhaseFailed:
		if s.Err != "" {
			return s.Err
		}
		return "请稍后重试。"
	default:
		return ""
	}
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// CheckInterval is how often a running app re-checks the feed while
// automatic checks are on.
const CheckInterval = 24 * time.Hour
