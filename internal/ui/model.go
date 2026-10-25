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
	PagePacks    = "packs"
	PageAbout    = "about"
)

// Config assembles the main-window model.
type Config struct {
	// Version is the running application version.
	Version string
	// DataDir is the application's data root, shown in the window.
	DataDir string
	// Updates is the update manager the window drives. Required.
	Updates *updates.Manager
	// Settings is the preference set loaded at startup.
	Settings settings.Settings
	// SaveSettings persists the console-owned presentation fields. Nil keeps
	// changes in memory only (tests). Runtime fields such as PetPack and
	// PetIntroPlayed belong to the host, so an implementation must merge
	// rather than replace the whole settings file.
	SaveSettings func(settings.Settings) error
	// ApplyPetVisible shows or hides the desktop pet after the preference
	// changes. Nil hides the control in tests or embedded hosts.
	ApplyPetVisible func(bool)
	// ApplyPetScale resizes the desktop pet after the preference changes.
	// Nil keeps the preference only in tests or embedded hosts.
	ApplyPetScale func(float64)
	// ApplyTheme applies the application's light/dark appearance after the
	// preference changes. Nil keeps the preference only in tests or embedded
	// hosts.
	ApplyTheme func(string)
	// ApplyBubbleTheme reapplies the speech bubble appearance after the
	// preference changes. Nil keeps the preference only in tests or embedded
	// hosts.
	ApplyBubbleTheme func(string)
	// OpenAtLogin reports the login item state. Nil hides the control.
	OpenAtLogin func() bool
	// SetOpenAtLogin changes the login item. Nil hides the control.
	SetOpenAtLogin func(bool) error
	// ResetPetPosition moves the pet back to its first-launch corner.
	ResetPetPosition func()
	// OpenDataDir opens the data root in the system file manager. Nil hides
	// the action.
	OpenDataDir func() error
	// LoadPacks lists installed asset packs. Nil hides the pack page
	// controls that depend on it.
	LoadPacks func() ([]PackSummary, error)
	// ChoosePackZip asks the user for a pack zip and returns its path, or ""
	// when canceled. Nil hides the import form unless ChoosePackZipAsync is
	// set. Tests and embedded hosts can use this synchronous form.
	ChoosePackZip func() (string, error)
	// ChoosePackZipAsync is the UI-safe native dialog form. done must be called
	// on the UI thread; the application wraps the native dialog in a goroutine
	// and posts the result back through Window.Update.
	ChoosePackZipAsync func(done func(path string, err error))
	// InstallPack installs a zip synchronously. It returns PackExistsError
	// when the id is already installed and replace is false. It must not
	// activate the pack; the console calls ActivatePack after a successful
	// install. Tests and embedded hosts can use this form.
	InstallPack func(zipPath string, replace bool) (PackSummary, error)
	// InstallPackAsync is the UI-safe form. It should run the install away
	// from the UI thread and call done on the UI thread. When set, it takes
	// precedence over InstallPack.
	InstallPackAsync func(zipPath string, replace bool, done func(PackSummary, error))
	// ConfirmReplacePackAsync asks before replacing an installed id. done must
	// be called on the UI thread.
	ConfirmReplacePackAsync func(id, name string, done func(bool))
	// ActivatePack switches the running pet and persists petPack.
	ActivatePack func(id string) error
	// DeletePack removes an inactive pack.
	DeletePack func(id string) error
	// ConfirmDeletePack asks before deleting. Nil means no confirmation.
	ConfirmDeletePack func(id, name string) bool
	// ConfirmDeletePackAsync is the UI-safe native dialog form. done must be
	// called on the UI thread.
	ConfirmDeletePackAsync func(id, name string, done func(bool))
	// LoadPackPreview returns a PNG poster for a pack.
	LoadPackPreview func(id string) ([]byte, error)
	// OpenPacksDir opens the install directory.
	OpenPacksDir func() error
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
	// savedSettings is the last value successfully written through
	// SaveSettings; it lets a failed write roll the UI back.
	savedSettings settings.Settings
	md            markdown.Options
	// actionErr reports a failed side effect, such as opening the data
	// directory, next to the action that caused it.
	actionErr string
	// settingsErr reports a failed preference write.
	settingsErr string
	// openAtLogin mirrors the system login item while the console is open.
	openAtLogin      bool
	openAtLoginKnown bool
	// bubbleChoice is the label selected in the bubble frequency control.
	bubbleChoice string
	// themeChoice is the label selected in the application theme control.
	themeChoice string
	// bubbleThemeChoice is the label selected in the bubble theme control.
	bubbleThemeChoice string
	// petScaleChoice is the label selected in the pet size control.
	petScaleChoice string
	// Pack management state. Packs are refreshed explicitly so the view
	// never scans the filesystem on every frame.
	packs          []PackSummary
	packsLoaded    bool
	packsErr       string
	packSelected   string
	packZip        string
	packBusy       bool
	packPreview    *ui.Bitmap
	packPreviewID  string
	packPreviewW   int
	packPreviewH   int
	packPreviewErr string
	packActionErr  string
	packNotice     string
}

// New creates the window model on the overview page.
func New(cfg Config) *Model {
	return &Model{
		cfg:               cfg,
		page:              PageOverview,
		settings:          cfg.Settings,
		savedSettings:     cfg.Settings,
		themeChoice:       themeChoiceFor(cfg.Settings.Theme),
		bubbleChoice:      bubbleChoiceFor(cfg.Settings.BubbleCooldownSeconds),
		bubbleThemeChoice: bubbleThemeChoiceFor(cfg.Settings.BubbleTheme),
		petScaleChoice:    petScaleChoiceFor(cfg.Settings.PetScale),
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
					case PagePacks:
						m.packsPage(c)
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

// ShowOverview switches to the page that holds the pet settings. The pet's
// context menu uses it before bringing the console forward.
func (m *Model) ShowOverview() { m.page = PageOverview }

// RefreshOpenAtLogin makes the next view read the system login-item state
// again, for a console opened after the user changed it outside Lucia.
func (m *Model) RefreshOpenAtLogin() {
	if m.cfg.OpenAtLogin != nil {
		m.openAtLoginKnown = false
	}
}

// setOpenAtLogin applies the requested state and falls back to the system's
// actual state if the platform rejects it.
func (m *Model) setOpenAtLogin(open bool) {
	m.openAtLogin = open
	if m.cfg.SetOpenAtLogin == nil {
		return
	}
	if err := m.cfg.SetOpenAtLogin(open); err != nil {
		m.settingsErr = fmt.Sprintf("设置开机自启失败：%v", err)
		if m.cfg.OpenAtLogin != nil {
			m.openAtLogin = m.cfg.OpenAtLogin()
			m.openAtLoginKnown = true
		}
		return
	}
	m.settingsErr = ""
	if m.cfg.OpenAtLogin != nil {
		m.openAtLogin = m.cfg.OpenAtLogin()
		m.openAtLoginKnown = true
	}
}

// ShowUpdates opens the about page, where the update controls live, and
// starts a check when there is no current result to show. The application
// menu calls it.
func (m *Model) ShowUpdates() {
	m.page = PageAbout
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
				ui.SidebarItem(c, PageOverview, petIcon, "桌宠")
				ui.SidebarItem(c, PagePacks, packIcon, "素材包")
				ui.SidebarItem(c, PageAbout, aboutIcon, "关于").Children(func() {
					if phase := m.cfg.Updates.Snapshot().Phase; phase == updates.PhaseAvailable {
						ui.Badge(c, "新")
					}
				})
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
			ui.Text(c, "桌面宠物").FontSize(11).TextColor(t.TextMuted)
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
	case PagePacks:
		return "素材包", "导入、预览与管理本地动画素材"
	case PageAbout:
		return "关于", "版本、更新与数据位置"
	default:
		return "桌宠", "设置桌宠的显示方式、声音与互动行为"
	}
}

func (m *Model) overviewPage(c *ui.Context) {
	m.petCard(c)
}

var petScaleChoices = []string{"小（0.8×）", "标准（1.0×）", "大（1.25×）", "特大（1.5×）"}

func petScaleChoiceFor(scale float64) string {
	switch {
	case scale <= 0.9:
		return petScaleChoices[0]
	case scale <= 1.1:
		return petScaleChoices[1]
	case scale <= 1.35:
		return petScaleChoices[2]
	default:
		return petScaleChoices[3]
	}
}

func petScaleForChoice(choice string) float64 {
	switch choice {
	case "小（0.8×）":
		return 0.8
	case "大（1.25×）":
		return 1.25
	case "特大（1.5×）":
		return 1.5
	default:
		return 1.0
	}
}

var bubbleChoices = []string{"每次互动", "最多每 30 秒", "最多每 2 分钟", "关闭"}

func bubbleChoiceFor(seconds int) string {
	switch {
	case seconds < 0:
		return "关闭"
	case seconds >= 120:
		return "最多每 2 分钟"
	case seconds >= 30:
		return "最多每 30 秒"
	default:
		return "每次互动"
	}
}

func bubbleSeconds(choice string) int {
	switch choice {
	case "关闭":
		return -1
	case "最多每 2 分钟":
		return 120
	case "最多每 30 秒":
		return 30
	default:
		return 0
	}
}

var themeChoices = []string{"跟随系统", "浅色", "深色"}

func themeChoiceFor(theme string) string {
	switch theme {
	case settings.ThemeLight:
		return themeChoices[1]
	case settings.ThemeDark:
		return themeChoices[2]
	default:
		return themeChoices[0]
	}
}

func themeForChoice(choice string) string {
	switch choice {
	case "浅色":
		return settings.ThemeLight
	case "深色":
		return settings.ThemeDark
	default:
		return settings.ThemeSystem
	}
}

var bubbleThemeChoices = []string{"跟随应用", "浅色", "深色"}

func bubbleThemeChoiceFor(theme string) string {
	switch theme {
	case settings.BubbleThemeLight:
		return bubbleThemeChoices[1]
	case settings.BubbleThemeDark:
		return bubbleThemeChoices[2]
	default:
		return bubbleThemeChoices[0]
	}
}

func bubbleThemeForChoice(choice string) string {
	switch choice {
	case "浅色":
		return settings.BubbleThemeLight
	case "深色":
		return settings.BubbleThemeDark
	default:
		return settings.BubbleThemeApp
	}
}

// bubbleThemeHint states the result of the bubble preference instead of
// making the user infer what "follow the app" means in the current theme.
func bubbleThemeHint(theme, bubbleTheme string) string {
	switch bubbleTheme {
	case settings.BubbleThemeLight:
		return "固定为浅色，不受应用主题影响。"
	case settings.BubbleThemeDark:
		return "固定为深色，不受应用主题影响。"
	default:
		switch theme {
		case settings.ThemeLight:
			return "跟随上方的主题模式；当前为浅色。"
		case settings.ThemeDark:
			return "跟随上方的主题模式；当前为深色。"
		default:
			return "跟随上方的主题模式；当前随系统切换。"
		}
	}
}

// SetPetVisible keeps the console's copy in sync when the tray toggles the
// pet. It also persists the new state and reports whether that write
// succeeded; callers that already changed the pet window must restore it on
// failure to keep the visible state, settings, and disk in agreement.
func (m *Model) SetPetVisible(visible bool) bool {
	if m.settings.PetVisible == visible {
		return true
	}
	m.settings.PetVisible = visible
	return m.saveSettings()
}

func (m *Model) petCard(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(16).Children(func() {
		settingsGroup(c, "显示", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "主题模式").FontSize(14)
					ui.Text(c, "控制 Lucia 窗口与控件的明暗外观。").FontSize(12).
						TextColor(t.TextMuted)
				})
				if ui.Select(c, &m.themeChoice, themeChoices).Label("主题模式").Width(170).Changed() {
					m.settings.Theme = themeForChoice(m.themeChoice)
					if m.saveSettings() && m.cfg.ApplyTheme != nil {
						m.cfg.ApplyTheme(m.settings.Theme)
					}
				}
			})

			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "显示桌宠").FontSize(14)
					ui.Text(c, "隐藏后可从托盘重新显示；重启后保持当前选择。").FontSize(12).
						TextColor(t.TextMuted)
				})
				ui.Switch(c, &m.settings.PetVisible).Label("显示桌宠").OnChange(func() {
					if m.saveSettings() && m.cfg.ApplyPetVisible != nil {
						m.cfg.ApplyPetVisible(m.settings.PetVisible)
					}
				})
			})

			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "桌宠大小").FontSize(14)
					ui.Text(c, "同时缩放窗口、立绘和脸部互动区域。").FontSize(12).
						TextColor(t.TextMuted)
				})
				if ui.Select(c, &m.petScaleChoice, petScaleChoices).Label("桌宠大小").Width(170).Changed() {
					m.settings.PetScale = petScaleForChoice(m.petScaleChoice)
					if m.saveSettings() && m.cfg.ApplyPetScale != nil {
						m.cfg.ApplyPetScale(m.settings.PetScale)
					}
				}
			})
		})

		settingsGroup(c, "气泡", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "气泡主题").FontSize(14)
					ui.Text(c, bubbleThemeHint(m.settings.Theme, m.settings.BubbleTheme)).FontSize(12).
						TextColor(t.TextMuted)
				})
				if ui.Select(c, &m.bubbleThemeChoice, bubbleThemeChoices).Label("气泡主题").Width(170).Changed() {
					m.settings.BubbleTheme = bubbleThemeForChoice(m.bubbleThemeChoice)
					if m.saveSettings() && m.cfg.ApplyBubbleTheme != nil {
						m.cfg.ApplyBubbleTheme(m.settings.BubbleTheme)
					}
				}
			})

			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "气泡频率").FontSize(14)
					ui.Text(c, "限制两次台词气泡之间的最短间隔；关闭后只保留动作和音效。").FontSize(12).
						TextColor(t.TextMuted)
				})
				if ui.Select(c, &m.bubbleChoice, bubbleChoices).Label("气泡频率").Width(170).Changed() {
					m.settings.BubbleCooldownSeconds = bubbleSeconds(m.bubbleChoice)
					_ = m.saveSettings()
				}
			})
		})

		settingsGroup(c, "互动", func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Gap(2).Grow(1).Children(func() {
					ui.Text(c, "互动音效").FontSize(14)
					ui.Text(c, "摸脸和摸摸头时播放一小段提示音。").FontSize(12).
						TextColor(t.TextMuted)
				})
				ui.Switch(c, &m.settings.SoundEnabled).Label("互动音效").OnChange(func() { _ = m.saveSettings() })
			})
		})

		if (m.cfg.OpenAtLogin != nil && m.cfg.SetOpenAtLogin != nil) || m.cfg.ResetPetPosition != nil {
			settingsGroup(c, "系统", func() {
				if m.cfg.OpenAtLogin != nil && m.cfg.SetOpenAtLogin != nil {
					if !m.openAtLoginKnown {
						m.openAtLogin = m.cfg.OpenAtLogin()
						m.openAtLoginKnown = true
					}
					ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
						ui.Column(c).Gap(2).Grow(1).Children(func() {
							ui.Text(c, "开机自启").FontSize(14)
							ui.Text(c, "登录系统后自动启动 Lucia 和桌宠。").FontSize(12).TextColor(t.TextMuted)
						})
						ui.Switch(c, &m.openAtLogin).Label("开机自启").OnChange(func() {
							m.setOpenAtLogin(m.openAtLogin)
						})
					})
				}

				if m.cfg.ResetPetPosition != nil {
					ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
						ui.Column(c).Gap(2).Grow(1).Children(func() {
							ui.Text(c, "桌宠位置").FontSize(14)
							ui.Text(c, "移回主屏右下角；隐藏时会重新显示。").FontSize(12).
								TextColor(t.TextMuted)
						})
						if ui.Button(c, "重置").Clicked() {
							m.settings.PetVisible = true
							if m.saveSettings() {
								m.cfg.ResetPetPosition()
							}
						}
					})
				}
			})
		}
	})
}

// settingsGroup puts the section heading outside its card so the page reads
// as a series of independent settings groups rather than one long form.
func settingsGroup(c *ui.Context, title string, body func()) {
	t := c.Theme()
	ui.Column(c).Gap(5).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(t.TextMuted)
		ui.Column(c).Padding(10, 16).Gap(11).Radius(10).
			Background(t.Background).Border(1, t.Border).Children(body)
	})
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
		return "开发运行、未配置更新源或安装位置不可写时不会检查更新。"
	case updates.PhaseIdle:
		return "点击“检查更新”读取 GitHub Releases 中的签名更新。"
	case updates.PhaseChecking:
		return "正在读取更新清单并比较版本。"
	case updates.PhaseUpToDate:
		return ""
	case updates.PhaseAvailable:
		return "安装后会自动重启进入新版本。"
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
