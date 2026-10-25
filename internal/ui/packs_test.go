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
			ID: "poke", Label: "摸摸脸", Bubble: "face", Motion: "squash",
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

func TestDeletePackRefreshesSelection(t *testing.T) {
	model, _, _ := newPacksTestModel(t, []PackSummary{testPack(true)})
	model.packSelected = "tiny"
	model.deletePack("tiny")
	if model.packSelected != "" {
		t.Fatalf("selected = %q, want empty after delete", model.packSelected)
	}
}

func TestUseBuiltInWhaleWhenActive(t *testing.T) {
	model, _, activations := newPacksTestModel(t, []PackSummary{testPack(true)})
	model.refreshPacks()
	if model.activePackID() != "tiny" {
		t.Fatalf("active = %q", model.activePackID())
	}
	model.activatePack("")
	if *activations != 1 {
		t.Fatalf("activations = %d, want 1", *activations)
	}
}
