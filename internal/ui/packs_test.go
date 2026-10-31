package ui

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/LoTwT/lucia/internal/settings"
	"github.com/LoTwT/lucia/internal/updates"
	"github.com/egoist/mygo/ui"
)

func previewPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPack(active bool) PackSummary {
	return PackSummary{
		ID: "tiny", Name: "Tiny", Active: active, Valid: true,
		Clips: 1, Frames: 3, Interactions: 1, License: "local-only",
		Hotzones: []PackHotzone{{
			ID: "face", Label: "摸摸脸", Interaction: "poke",
			X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5,
		}},
		InteractionList: []PackInteraction{{
			ID: "poke", Label: "摸摸脸", Bubble: "face", Motion: "bounce",
			Steps: []PackInteractionStep{{Action: "laugh"}},
		}},
		ActionList: []PackAction{{
			ID: "happy", Label: "开心", EnterHold: 40, ExitHold: 100,
		}},
	}
}

func newPacksTestModel(t *testing.T, packs []PackSummary) (*Model, *int, *int) {
	t.Helper()
	imports := 0
	activations := 0
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		LoadPacks: func() ([]PackSummary, error) {
			return packs, nil
		},
		LoadPackPreview: func(string) ([]byte, error) { return previewPNG(t), nil },
		InstallPack: func(_ string, replace bool) (PackSummary, error) {
			imports++
			if !replace && len(packs) > 0 {
				return PackSummary{}, &PackExistsError{ID: packs[0].ID, Name: packs[0].Name}
			}
			pack := testPack(true)
			packs = []PackSummary{pack}
			return pack, nil
		},
		ActivatePack: func(id string) error {
			activations++
			for i := range packs {
				packs[i].Active = packs[i].ID == id
			}
			return nil
		},
		DeletePack: func(id string) error {
			filtered := packs[:0]
			for _, pack := range packs {
				if pack.ID != id {
					filtered = append(filtered, pack)
				}
			}
			packs = filtered
			return nil
		},
	})
	return model, &imports, &activations
}

func TestChoosePackZipAsyncUpdatesModel(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		ChoosePackZipAsync: func(done func(string, error)) {
			done("/tmp/tiny.zip", nil)
		},
	})
	model.choosePackZip()
	if model.packZip != "/tmp/tiny.zip" || model.packBusy {
		t.Fatalf("zip=%q busy=%v", model.packZip, model.packBusy)
	}
}

func TestConfirmDeletePackAsyncDeletes(t *testing.T) {
	deleteCalls := 0
	model := New(Config{
		Version:   "0.0.1",
		DataDir:   "/tmp/lucia",
		Updates:   updates.New(disabledSource{}, "0.0.1"),
		Settings:  settings.Default(),
		LoadPacks: func() ([]PackSummary, error) { return nil, nil },
		DeletePack: func(string) error {
			deleteCalls++
			return nil
		},
		ConfirmDeletePackAsync: func(id, name string, done func(bool)) {
			done(true)
		},
	})
	model.confirmDeletePack("tiny", "Tiny")
	if deleteCalls != 1 || model.packBusy {
		t.Fatalf("deleteCalls=%d busy=%v", deleteCalls, model.packBusy)
	}
}

func TestPacksPageShowsMetadataPreviewAndHotzones(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(true)})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"素材包", "本地动画素材", "Tiny", "使用中",
		"预览与热区", "摸摸脸", "交互 poke", "授权：local-only",
		"交互", "laugh", "动作时序", "enter 40ms / loop 0ms / exit 100ms",
	} {
		if !tt.HasText(want) {
			t.Errorf("packs page does not show %q; texts: %v", want, tt.Texts())
		}
	}
}

func TestPacksPageShowsEmptyHintWithOnlyBuiltin(t *testing.T) {
	builtin := testPack(false)
	builtin.ID = "lucia-whale"
	builtin.Name = "Lucia 鲸鱼"
	builtin.Builtin = true
	model, _, _ := newPacksTestModel(t, []PackSummary{builtin})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("还没有安装素材包；内置鲸鱼会继续作为默认皮肤。") {
		t.Fatalf("only-builtin page does not show the empty hint; texts: %v", tt.Texts())
	}
}

func TestPacksPageHidesEmptyHintWithUserPack(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(false)})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("还没有安装素材包；内置鲸鱼会继续作为默认皮肤。") {
		t.Fatalf("page with a user pack shows the empty hint; texts: %v", tt.Texts())
	}
}

func TestPacksPageHidesEmptyHintWhenListingFails(t *testing.T) {
	model, _, _ := newPacksTestModel(t, nil)
	model.packs = nil
	model.packsLoaded = true
	model.packsErr = "读取素材包失败：permission denied"
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("还没有安装素材包；内置鲸鱼会继续作为默认皮肤。") {
		t.Fatalf("failed listing also shows the empty hint; texts: %v", tt.Texts())
	}
	if !tt.HasText("读取素材包失败：permission denied") {
		t.Fatalf("failed listing error is missing; texts: %v", tt.Texts())
	}
}

func TestPacksPageInvalidPackExplainsWhy(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{{
		ID: "broken", Name: "Broken", Active: true, Error: "bad manifest",
	}})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("无效") || !tt.HasText("bad manifest") {
		t.Fatalf("packs page does not explain the broken pack; texts: %v", tt.Texts())
	}
}

func TestImportPackNeedsNoLicenseGate(t *testing.T) {
	model, imports, activations := newPacksTestModel(t, nil)
	model.packZip = "tiny.zip"
	model.importPack()
	if *imports != 1 || *activations != 1 {
		t.Fatalf("imports=%d activations=%d, want 1/1", *imports, *activations)
	}
	if model.packSelected != "tiny" || model.packNotice == "" {
		t.Fatalf("selected=%q notice=%q", model.packSelected, model.packNotice)
	}
}

func TestPacksPageShowsImportPanelWithAsyncInstaller(t *testing.T) {
	model := New(Config{
		Version:            "0.0.1",
		DataDir:            "/tmp/lucia",
		Updates:            updates.New(disabledSource{}, "0.0.1"),
		Settings:           settings.Default(),
		ChoosePackZipAsync: func(func(string, error)) {},
		InstallPackAsync:   func(string, bool, func(PackSummary, error)) {},
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("导入并启用") {
		t.Fatalf("async installer did not render the import panel; texts: %v", tt.Texts())
	}
}

func TestImportPackAsyncKeepsBusyUntilCompletion(t *testing.T) {
	model, imports, activations := newPacksTestModel(t, nil)
	var done func(PackSummary, error)
	model.cfg.InstallPackAsync = func(_ string, _ bool, callback func(PackSummary, error)) {
		done = callback
	}
	model.packZip = "tiny.zip"

	model.importPack()
	if !model.packBusy || done == nil {
		t.Fatalf("busy=%v callback=%v", model.packBusy, done != nil)
	}
	if *imports != 0 {
		t.Fatalf("synchronous installer ran %d times", *imports)
	}

	done(testPack(true), nil)
	if model.packBusy || *activations != 1 {
		t.Fatalf("busy=%v activations=%d", model.packBusy, *activations)
	}
}

func TestImportPackAsyncStaysBusyThroughAsyncActivation(t *testing.T) {
	model, _, _ := newPacksTestModel(t, nil)
	var installDone func(PackSummary, error)
	model.cfg.InstallPackAsync = func(_ string, _ bool, callback func(PackSummary, error)) {
		installDone = callback
	}
	var activateDone func(error)
	model.cfg.ActivatePackAsync = func(_ string, callback func(error)) {
		activateDone = callback
	}
	model.packZip = "tiny.zip"
	model.importPack()
	installDone(testPack(true), nil)
	if !model.packBusy || activateDone == nil {
		t.Fatalf("busy=%v activation callback=%v", model.packBusy, activateDone != nil)
	}
	activateDone(nil)
	if model.packBusy || model.packNotice == "" {
		t.Fatalf("busy=%v notice=%q", model.packBusy, model.packNotice)
	}
}

func TestImportPackAsyncConfirmsReplacement(t *testing.T) {
	model, _, activations := newPacksTestModel(t, []PackSummary{testPack(true)})
	var completions []func(PackSummary, error)
	model.cfg.InstallPackAsync = func(_ string, _ bool, callback func(PackSummary, error)) {
		completions = append(completions, callback)
	}
	model.cfg.ConfirmReplacePackAsync = func(_, _ string, done func(bool)) { done(true) }
	model.packZip = "tiny.zip"

	model.importPack()
	if len(completions) != 1 || !model.packBusy {
		t.Fatalf("completions=%d busy=%v", len(completions), model.packBusy)
	}
	completions[0](PackSummary{}, &PackExistsError{ID: "tiny", Name: "Tiny"})
	if len(completions) != 2 || !model.packBusy {
		t.Fatalf("replace completions=%d busy=%v", len(completions), model.packBusy)
	}
	completions[1](testPack(true), nil)
	if model.packBusy || *activations != 1 {
		t.Fatalf("busy=%v activations=%d", model.packBusy, *activations)
	}
}

func TestImportPackAsyncErrorClearsBusy(t *testing.T) {
	model, _, activations := newPacksTestModel(t, nil)
	model.cfg.InstallPackAsync = func(_ string, _ bool, callback func(PackSummary, error)) {
		callback(PackSummary{}, errors.New("boom"))
	}
	model.packZip = "tiny.zip"
	model.importPack()
	if model.packBusy || *activations != 0 || model.packActionErr == "" {
		t.Fatalf("busy=%v activations=%d err=%q", model.packBusy, *activations, model.packActionErr)
	}
}

func TestImportPackConfirmsOnlyOnSameID(t *testing.T) {
	model, imports, activations := newPacksTestModel(t, []PackSummary{testPack(true)})
	confirmed := 0
	model.cfg.ConfirmReplacePackAsync = func(id, name string, done func(bool)) {
		confirmed++
		if id != "tiny" || name != "Tiny" {
			t.Fatalf("confirm id=%q name=%q", id, name)
		}
		done(true)
	}
	model.packZip = "tiny.zip"
	model.importPack()
	if confirmed != 1 || *imports != 2 || *activations != 1 {
		t.Fatalf("confirmed=%d imports=%d activations=%d, want 1/2/1", confirmed, *imports, *activations)
	}
}

func TestImportPackCancelLeavesPackAlone(t *testing.T) {
	model, imports, activations := newPacksTestModel(t, []PackSummary{testPack(true)})
	model.cfg.ConfirmReplacePackAsync = func(id, name string, done func(bool)) {
		done(false)
	}
	model.packZip = "tiny.zip"
	model.importPack()
	if *imports != 1 || *activations != 0 || model.packBusy {
		t.Fatalf("imports=%d activations=%d busy=%v", *imports, *activations, model.packBusy)
	}
	if model.packNotice == "" {
		t.Fatal("cancel did not report a notice")
	}
}

func TestReplaceActivePackFallsBackToBuiltInWhenActivationFails(t *testing.T) {
	packs := []PackSummary{testPack(true)}
	var activated []string
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		LoadPacks: func() ([]PackSummary, error) {
			return packs, nil
		},
		LoadPackPreview: func(string) ([]byte, error) { return previewPNG(t), nil },
		InstallPack: func(_ string, replace bool) (PackSummary, error) {
			if !replace {
				return PackSummary{}, &PackExistsError{ID: "tiny", Name: "Tiny"}
			}
			return testPack(true), nil
		},
		ConfirmReplacePackAsync: func(id, name string, done func(bool)) { done(true) },
		ActivatePack: func(id string) error {
			activated = append(activated, id)
			if id != "" {
				return errors.New("activation failed")
			}
			return nil
		},
	})
	model.refreshPacks()
	model.packZip = "tiny.zip"
	model.importPack()
	if len(activated) != 2 || activated[0] != "tiny" || activated[1] != "" {
		t.Fatalf("activation calls = %v, want [tiny empty]", activated)
	}
	if model.packActionErr == "" {
		t.Fatal("replace failure was not reported")
	}
}

func TestImportPackKeepsImportedWhenActivationFails(t *testing.T) {
	model, _, _ := newPacksTestModel(t, nil)
	model.cfg.ActivatePack = func(string) error { return errors.New("boom") }
	model.packZip = "tiny.zip"
	model.importPack()
	if model.packActionErr == "" || model.packNotice == "" {
		t.Fatalf("actionErr=%q notice=%q", model.packActionErr, model.packNotice)
	}
	if len(model.packs) != 1 || model.packSelected != "tiny" {
		t.Fatalf("imported pack disappeared: %+v selected=%q", model.packs, model.packSelected)
	}
}

func TestPreviewRetriesAfterFailure(t *testing.T) {
	attempts := 0
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		LoadPackPreview: func(string) ([]byte, error) {
			attempts++
			if attempts == 1 {
				return nil, errors.New("temporary failure")
			}
			return previewPNG(t), nil
		},
	})
	model.packSelected = "tiny"
	model.loadPackPreview()
	if model.packPreviewErr == "" {
		t.Fatal("first preview failure was not reported")
	}
	model.loadPackPreview()
	if model.packPreview == nil || model.packPreviewErr != "" || attempts != 2 {
		t.Fatalf("attempts=%d preview=%v err=%q", attempts, model.packPreview != nil, model.packPreviewErr)
	}
}

func TestActivatePackAsyncKeepsBusyUntilCompletion(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(false)})
	model.refreshPacks()
	var done func(error)
	model.cfg.ActivatePackAsync = func(id string, callback func(error)) {
		if id != "tiny" {
			t.Fatalf("activation id = %q, want tiny", id)
		}
		done = callback
	}
	model.activatePack("tiny")
	if !model.packBusy || done == nil {
		t.Fatalf("busy=%v callback=%v", model.packBusy, done != nil)
	}
	done(nil)
	if model.packBusy || model.packNotice == "" || model.packSelected != "tiny" {
		t.Fatalf("busy=%v notice=%q selected=%q", model.packBusy, model.packNotice, model.packSelected)
	}
}

func TestActivatePackAsyncClearsBusyOnFailure(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(false)})
	model.refreshPacks()
	model.cfg.ActivatePackAsync = func(_ string, callback func(error)) {
		callback(errors.New("boom"))
	}
	model.activatePack("tiny")
	if model.packBusy || model.packActionErr == "" {
		t.Fatalf("busy=%v err=%q", model.packBusy, model.packActionErr)
	}
}

func TestPacksPageOffersEnableWithAsyncActivator(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		LoadPacks: func() ([]PackSummary, error) {
			return []PackSummary{testPack(false)}, nil
		},
		ActivatePackAsync: func(string, func(error)) {},
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("启用"); err != nil {
		t.Fatalf("async activator did not render the enable action: %v", err)
	}
}

func TestPreviewAsyncDropsStaleResults(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
	})
	var callbacks []func([]byte, error)
	model.cfg.LoadPackPreviewAsync = func(_ string, done func([]byte, error)) {
		callbacks = append(callbacks, done)
	}
	model.packSelected = "tiny"
	model.loadPackPreview()
	model.loadPackPreview()
	if len(callbacks) != 1 || !model.packPreviewLoading {
		t.Fatalf("callbacks=%d loading=%v, want one in-flight request", len(callbacks), model.packPreviewLoading)
	}

	model.packSelected = "other"
	model.loadPackPreview()
	if len(callbacks) != 2 || !model.packPreviewLoading {
		t.Fatalf("callbacks=%d loading=%v, want a second request", len(callbacks), model.packPreviewLoading)
	}
	callbacks[0](previewPNG(t), nil)
	if model.packPreview != nil {
		t.Fatal("a stale preview replaced the current selection")
	}
	callbacks[1](previewPNG(t), nil)
	if model.packPreview == nil || model.packPreviewErr != "" || model.packPreviewLoading {
		t.Fatalf("preview=%v err=%q loading=%v", model.packPreview != nil, model.packPreviewErr, model.packPreviewLoading)
	}
}

func TestPreviewPanelSupportsAsyncOnlyHost(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
		LoadPacks: func() ([]PackSummary, error) {
			return []PackSummary{testPack(false)}, nil
		},
		LoadPackPreviewAsync: func(string, func([]byte, error)) {},
	})
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("当前宿主不提供素材包预览。") {
		t.Fatalf("async-only host was reported as unsupported; texts: %v", tt.Texts())
	}
	if !tt.HasText("正在生成预览…") {
		t.Fatalf("async-only host did not start a preview; texts: %v", tt.Texts())
	}
}

func TestPreviewAsyncRetriesAfterFailure(t *testing.T) {
	model := New(Config{
		Version:  "0.0.1",
		DataDir:  "/tmp/lucia",
		Updates:  updates.New(disabledSource{}, "0.0.1"),
		Settings: settings.Default(),
	})
	var callbacks []func([]byte, error)
	model.cfg.LoadPackPreviewAsync = func(_ string, done func([]byte, error)) {
		callbacks = append(callbacks, done)
	}
	model.packSelected = "tiny"
	model.loadPackPreview()
	callbacks[0](nil, errors.New("temporary failure"))
	if model.packPreviewErr == "" || model.packPreviewLoading {
		t.Fatalf("err=%q loading=%v", model.packPreviewErr, model.packPreviewLoading)
	}
	model.loadPackPreview()
	if len(callbacks) != 2 {
		t.Fatalf("callbacks=%d, want a retry after a reported failure", len(callbacks))
	}
	callbacks[1](previewPNG(t), nil)
	if model.packPreview == nil || model.packPreviewErr != "" {
		t.Fatalf("preview=%v err=%q", model.packPreview != nil, model.packPreviewErr)
	}
}

func TestDeletePackRefreshesSelection(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(true)})
	model.packSelected = "tiny"
	model.deletePack("tiny")
	if model.packSelected != "" {
		t.Fatalf("selected = %q, want empty after delete", model.packSelected)
	}
}

func TestDeletePackDropsRowWhenRefreshFails(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(false)})
	model.refreshPacks()
	model.packSelected = "tiny"
	model.cfg.LoadPacks = func() ([]PackSummary, error) {
		return nil, errors.New("permission denied")
	}
	model.deletePack("tiny")
	if len(model.packs) != 0 {
		t.Fatalf("deleted pack stayed in the UI list: %+v", model.packs)
	}
	if model.packNotice != "素材包已删除，但列表刷新失败。" {
		t.Fatalf("notice = %q", model.packNotice)
	}
	if model.packsErr == "" {
		t.Fatal("refresh failure was not reported")
	}
}

func TestBuiltInRowIsReadOnlyAndCanActivate(t *testing.T) {
	builtin := testPack(false)
	builtin.ID = "lucia-whale"
	builtin.Name = "鲸鱼"
	builtin.Builtin = true
	model, _, activations := newPacksTestModel(t, []PackSummary{builtin})
	model.refreshPacks()
	tt := ui.NewTester(model.View, 1000, 700)
	if err := tt.Click("素材包"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"内置", "鲸鱼", "启用"} {
		if !tt.HasText(want) {
			t.Errorf("built-in row does not show %q; texts: %v", want, tt.Texts())
		}
	}
	if tt.HasText("删除") {
		t.Fatalf("built-in row shows a delete button; texts: %v", tt.Texts())
	}
	if err := tt.Click("启用"); err != nil {
		t.Fatal(err)
	}
	if *activations != 1 || model.activePackID() != "lucia-whale" {
		t.Fatalf("activations=%d active=%q", *activations, model.activePackID())
	}
}
