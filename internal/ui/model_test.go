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
		Commit:        "0123456789ab",
		Platform:      "darwin/arm64",
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

func TestOverviewShowsBuildIdentity(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	tt := ui.NewTester(model.View, 1000, 700)
	for _, want := range []string{"Lucia", "v0.0.1", "运行信息", "0123456789ab", "darwin/arm64"} {
		if !tt.HasText(want) {
			t.Errorf("overview does not show %q; texts: %v", want, tt.Texts())
		}
	}
}

func TestOverviewShowsPetControls(t *testing.T) {
	model := New(Config{
		Version:          "0.0.1",
		Commit:           "0123456789ab",
		Platform:         "darwin/arm64",
		DataDir:          "/tmp/lucia",
		Updates:          updates.New(disabledSource{}, "0.0.1"),
		Settings:         settings.Default(),
		OpenAtLogin:      func() bool { return false },
		SetOpenAtLogin:   func(bool) error { return nil },
		ResetPetPosition: func() {},
	})
	tt := ui.NewTester(model.View, 1000, 700)
	for _, want := range []string{"桌宠", "显示桌宠", "互动音效", "气泡频率", "开机自启", "重置桌宠位置"} {
		if !tt.HasText(want) {
			t.Errorf("overview does not show %q; texts: %v", want, tt.Texts())
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

func TestSidebarNavigatesToUpdatesAndAbout(t *testing.T) {
	model, _ := newTestModel(t, fakeSource{})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("更新"); err != nil {
		t.Fatal(err)
	}
	if model.page != PageUpdates {
		t.Fatalf("page %q, want updates", model.page)
	}
	if !tt.HasText("检查更新") || !tt.HasText("启动时自动检查更新") {
		t.Errorf("updates page is missing its controls; texts: %v", tt.Texts())
	}
	if err := tt.Click("关于"); err != nil {
		t.Fatal(err)
	}
	if model.page != PageAbout {
		t.Fatalf("page %q, want about", model.page)
	}
	for _, want := range []string{"com.lotwt.lucia", "MyGo 文档", "LoTwT/lucia"} {
		if !tt.HasText(want) {
			t.Errorf("about page does not show %q; texts: %v", want, tt.Texts())
		}
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
	if err := tt.Click("更新"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"发现新版本 0.2.0", "安装并重启", "新功能", "更快的启动"} {
		if !tt.HasText(want) {
			t.Errorf("updates page does not show %q; texts: %v", want, tt.Texts())
		}
	}
}

func TestDisabledBuildExplainsUpdates(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		Commit:   "unknown",
		Platform: "darwin/arm64",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("更新"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("当前构建未启用自动更新") {
		t.Errorf("updates page does not explain the disabled build; texts: %v", tt.Texts())
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
