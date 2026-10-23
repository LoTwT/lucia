package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LoTwT/lucia/internal/settings"
	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo/ui"
)

type fakeSource struct {
	release *updates.Release
}

func (f fakeSource) Enabled() bool { return true }

func (f fakeSource) Check(context.Context) (*updates.Release, error) {
	return f.release, nil
}

func (f fakeSource) Install(context.Context, *updates.Release, func(int64, int64)) error {
	return nil
}

func newTestModel(t *testing.T, src updates.Source) (*Model, *updates.Manager) {
	t.Helper()
	manager := updates.New(src, "0.0.1")
	saved := 0
	model := New(Config{
		Version:       "0.0.1",
		DataDir:       "/Users/ada/Library/Application Support/Lucia",
		Updates:       manager,
		Settings:      settings.Default(),
		SaveSettings:  func(settings.Settings) error { saved++; return nil },
		OpenDataDir:   func() error { return nil },
		RepositoryURL: "https://github.com/LoTwT/lucia",
		UpdateSource:  "LoTwT/lucia",
	})
	return model, manager
}

func TestAboutShowsVersionWithoutBuildDetails(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Lucia", "v0.0.1", "版本、更新与数据位置", "检查更新"} {
		if !tt.HasText(want) {
			t.Errorf("about page does not show %q; texts: %v", want, tt.Texts())
		}
	}
	for _, unwanted := range []string{"0123456789ab", "darwin/arm64", "com.lotwt.lucia"} {
		if tt.HasText(unwanted) {
			t.Errorf("about page still shows build detail %q; texts: %v", unwanted, tt.Texts())
		}
	}
}

func TestOverviewShowsPetControls(t *testing.T) {
	model := New(Config{
		Version:          "0.0.1",
		DataDir:          "/tmp/lucia",
		Updates:          updates.New(disabledSource{}, "0.0.1"),
		Settings:         settings.Default(),
		OpenAtLogin:      func() bool { return false },
		SetOpenAtLogin:   func(bool) error { return nil },
		ResetPetPosition: func() {},
	})
	tt := ui.NewTester(model.View, 1000, 700)
	for _, want := range []string{
		"桌宠",
		"显示桌宠",
		"隐藏后可从托盘重新显示；重启后保持当前选择。",
		"桌宠大小",
		"互动音效",
		"摸脸和摸摸头时播放一小段提示音。",
		"气泡频率",
		"开机自启",
		"桌宠位置",
		"移回主屏右下角；隐藏时会重新显示。",
		"重置",
	} {
		if !tt.HasText(want) {
			t.Errorf("pet page does not show %q; texts: %v", want, tt.Texts())
		}
	}
	if tt.HasText("运行信息") {
		t.Errorf("pet page still shows the duplicate runtime card; texts: %v", tt.Texts())
	}
}

func TestResetPetPositionRow(t *testing.T) {
	reset := false
	prefs := settings.Default()
	prefs.PetVisible = false
	model := New(Config{
		Version:          "0.0.1",
		DataDir:          "/tmp/lucia",
		Updates:          updates.New(disabledSource{}, "0.0.1"),
		Settings:         prefs,
		ResetPetPosition: func() { reset = true },
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("重置"); err != nil {
		t.Fatal(err)
	}
	if !reset || !model.settings.PetVisible {
		t.Fatalf("reset=%v visible=%v, want true/true", reset, model.settings.PetVisible)
	}
}

func TestPhaseDetailDeduplicatesUpdateStatus(t *testing.T) {
	if got := phaseDetail(updates.Snapshot{Phase: updates.PhaseUpToDate}); got != "" {
		t.Fatalf("up-to-date detail = %q, want empty", got)
	}
	got := phaseDetail(updates.Snapshot{
		Phase:   updates.PhaseAvailable,
		Release: &updates.Release{Version: "0.2.0"},
	})
	if got != "安装后会自动重启进入新版本。" {
		t.Fatalf("available detail = %q", got)
	}
}

func TestPetScaleChoiceMapping(t *testing.T) {
	tests := []struct {
		scale  float64
		choice string
	}{
		{scale: 0.8, choice: "小（0.8×）"},
		{scale: 1.0, choice: "标准（1.0×）"},
		{scale: 1.25, choice: "大（1.25×）"},
		{scale: 1.5, choice: "特大（1.5×）"},
	}
	for _, tt := range tests {
		if got := petScaleChoiceFor(tt.scale); got != tt.choice {
			t.Errorf("petScaleChoiceFor(%v) = %q, want %q", tt.scale, got, tt.choice)
		}
		if got := petScaleForChoice(tt.choice); got != tt.scale {
			t.Errorf("petScaleForChoice(%q) = %v, want %v", tt.choice, got, tt.scale)
		}
	}
}

func TestBubbleChoiceMapping(t *testing.T) {
	tests := []struct {
		seconds int
		choice  string
	}{
		{seconds: -1, choice: "关闭"},
		{seconds: 0, choice: "每次互动"},
		{seconds: 30, choice: "最多每 30 秒"},
		{seconds: 120, choice: "最多每 2 分钟"},
	}
	for _, tt := range tests {
		if got := bubbleChoiceFor(tt.seconds); got != tt.choice {
			t.Errorf("bubbleChoiceFor(%d) = %q, want %q", tt.seconds, got, tt.choice)
		}
		if got := bubbleSeconds(tt.choice); got != tt.seconds {
			t.Errorf("bubbleSeconds(%q) = %d, want %d", tt.choice, got, tt.seconds)
		}
	}
}

func TestSetPetVisiblePersists(t *testing.T) {
	var saved settings.Settings
	model := New(Config{
		Version:      "0.0.1",
		Updates:      updates.New(disabledSource{}, "0.0.1"),
		Settings:     settings.Default(),
		SaveSettings: func(s settings.Settings) error { saved = s; return nil },
	})
	model.SetPetVisible(false)
	if model.settings.PetVisible {
		t.Fatal("model still shows the pet")
	}
	if saved.PetVisible {
		t.Fatal("saved settings still show the pet")
	}
}

func TestOpenAtLoginFailureRollsBack(t *testing.T) {
	model := New(Config{
		Version:        "0.0.1",
		Updates:        updates.New(disabledSource{}, "0.0.1"),
		Settings:       settings.Default(),
		OpenAtLogin:    func() bool { return false },
		SetOpenAtLogin: func(bool) error { return errors.New("denied") },
	})
	model.openAtLoginKnown = true
	model.openAtLogin = true
	model.setOpenAtLogin(true)
	if model.openAtLogin {
		t.Fatal("failed login-item write left the switch on")
	}
	if model.settingsErr == "" {
		t.Fatal("failed login-item write did not report an error")
	}
}

func TestRefreshOpenAtLogin(t *testing.T) {
	model := New(Config{
		Version:     "0.0.1",
		Updates:     updates.New(disabledSource{}, "0.0.1"),
		Settings:    settings.Default(),
		OpenAtLogin: func() bool { return true },
	})
	model.openAtLoginKnown = true
	model.RefreshOpenAtLogin()
	if model.openAtLoginKnown {
		t.Fatal("login-item state was not refreshed")
	}
}

func TestSidebarNavigatesBetweenPetAndAbout(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if model.page != PageAbout {
		t.Fatalf("page %q, want about", model.page)
	}
	for _, want := range []string{"检查更新", "启动时自动检查更新", "LoTwT/lucia", "源码与发布记录"} {
		if !tt.HasText(want) {
			t.Errorf("about page does not show %q; texts: %v", want, tt.Texts())
		}
	}
	if err := tt.Click("桌宠"); err != nil {
		t.Fatal(err)
	}
	if model.page != PageOverview {
		t.Fatalf("page %q, want overview", model.page)
	}
	if !tt.HasText("显示桌宠") {
		t.Fatalf("pet page does not show pet settings; texts: %v", tt.Texts())
	}
}

func TestShowUpdatesOpensAbout(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	model.ShowUpdates()
	if model.page != PageAbout {
		t.Fatalf("page %q, want about", model.page)
	}
}

func TestShowOverviewReturnsToPetSettings(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	model.page = PageAbout
	model.ShowOverview()
	if model.page != PageOverview {
		t.Fatalf("page %q, want overview", model.page)
	}
	tt := ui.NewTester(model.View, 1000, 700)
	if !tt.HasText("显示桌宠") {
		t.Fatalf("overview does not show pet settings; texts: %v", tt.Texts())
	}
}

func TestAvailableUpdateShowsNotesAndInstall(t *testing.T) {
	date := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	src := fakeSource{release: &updates.Release{
		Version: "0.2.0",
		Notes:   "## 新功能\n\n- 更快的启动",
		Date:    date,
	}}
	model, manager := newTestModel(t, src)
	if err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"发现新版本 0.2.0", "安装并重启", "新功能", "更快的启动"} {
		if !tt.HasText(want) {
			t.Errorf("about page does not show %q; texts: %v", want, tt.Texts())
		}
	}
}

func TestDisabledBuildExplainsUpdates(t *testing.T) {
	model := New(Config{
		Version:      "0.0.1",
		DataDir:      "/tmp/lucia",
		Updates:      updates.New(disabledSource{}, "0.0.1"),
		Settings:     settings.Default(),
		UpdateSource: "LoTwT/lucia",
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("当前构建未启用自动更新") {
		t.Errorf("about page does not explain the disabled build; texts: %v", tt.Texts())
	}
	if tt.HasText("LoTwT/lucia") {
		t.Errorf("disabled build still shows the update source; texts: %v", tt.Texts())
	}
}

type disabledSource struct{}

func (disabledSource) Enabled() bool { return false }

func (disabledSource) Check(context.Context) (*updates.Release, error) {
	return nil, nil
}

func (disabledSource) Install(context.Context, *updates.Release, func(int64, int64)) error {
	return nil
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
	}
	for _, tt := range tests {
		if got := formatBytes(tt.in); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCanInstall(t *testing.T) {
	release := &updates.Release{Version: "0.2.0"}
	if canInstall(updates.Snapshot{Phase: updates.PhaseAvailable, Release: release}) != true {
		t.Error("an available release must be installable")
	}
	if canInstall(updates.Snapshot{Phase: updates.PhaseFailed, Release: release}) != true {
		t.Error("a release retained after a failed install must be retryable")
	}
	if canInstall(updates.Snapshot{Phase: updates.PhaseAvailable}) {
		t.Error("install was offered without a release")
	}
	if canInstall(updates.Snapshot{Phase: updates.PhaseDownloading, Release: release}) {
		t.Error("install was offered while the manager is busy")
	}
}
