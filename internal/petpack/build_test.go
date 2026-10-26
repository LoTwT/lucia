package petpack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSourceFixture(t *testing.T, source Source, files []string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, "clips", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	body, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sourceFileName), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildSourceWithHotzoneAndPinnedCrop(t *testing.T) {
	source := Source{
		Source:       1,
		ID:           "tiny",
		Name:         "Tiny",
		License:      json.RawMessage(`{"status":"local-only"}`),
		Canvas:       &Size{Width: 16, Height: 16},
		Crop:         &Rect{Width: 16, Height: 16},
		BubbleAnchor: &Anchor{X: 0, Y: 0},
		Hotzones: []SourceHotzone{{
			ID: "face", Label: "摸摸脸",
			Rect:   Rect{X: 4, Y: 4, Width: 8, Height: 8},
			Steps:  []SourceStep{{Action: "belly_taunt"}},
			Bubble: []string{"你好"},
			Sound:  true,
		}},
		Menu: &[]SourceMenuEntry{{Interaction: "face"}},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp", "belly_taunt.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	result, err := Build(dir, out, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Clips != 2 || result.Actions == 0 || result.Hotzones != 1 || result.Interactions != 1 {
		t.Fatalf("result = %+v", result)
	}
	manifest, err := readZipManifestFromPath(out)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Crop != (Rect{Width: 16, Height: 16}) {
		t.Fatalf("crop = %+v", manifest.Crop)
	}
	if manifest.BubbleAnchor != (Anchor{X: 0, Y: 0}) || !manifest.BubbleAnchorSet {
		t.Fatalf("bubble anchor = %+v set=%v", manifest.BubbleAnchor, manifest.BubbleAnchorSet)
	}
	if len(manifest.Hotzones) != 1 || manifest.Hotzones[0].Rect != (NormalizedRect{X: 0.25, Y: 0.25, Width: 0.5, Height: 0.5}) {
		t.Fatalf("hotzones = %+v", manifest.Hotzones)
	}
	if _, ok := manifest.Interactions["face"]; !ok {
		t.Fatal("missing face interaction")
	}
	if got := manifest.Bubbles["face"]; len(got) != 1 || got[0] != "你好" {
		t.Fatalf("bubbles = %+v", manifest.Bubbles)
	}
	if !manifest.Clips["idle_loop"].Loop {
		t.Fatal("idle_loop is not marked as a loop clip")
	}
	if manifest.Clips["belly_taunt"].Loop {
		t.Fatal("belly_taunt is marked as a loop clip")
	}
	if !zipContains(t, out, sourceFileName) {
		t.Fatal("source file is not embedded in the generated zip")
	}
}

func TestBuildPinsCrop(t *testing.T) {
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Crop: &Rect{X: 2, Y: 2, Width: 12, Height: 12},
		Hotzones: []SourceHotzone{{
			ID: "face", Rect: Rect{X: 4, Y: 4, Width: 4, Height: 4}, Sound: true,
		}},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	manifest, err := readZipManifestFromPath(out)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Crop != (Rect{X: 2, Y: 2, Width: 12, Height: 12}) {
		t.Fatalf("crop = %+v", manifest.Crop)
	}
	rect := manifest.Hotzones[0].Rect
	if math.Abs(rect.X-1.0/6) > 1e-9 || math.Abs(rect.Width-1.0/3) > 1e-9 {
		t.Fatalf("hotzone = %+v", rect)
	}
}

func TestBuiltSourceRebuildPreservesInteractions(t *testing.T) {
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Hotzones: []SourceHotzone{{
			ID: "face", Label: "摸摸脸",
			Rect:   Rect{X: 4, Y: 4, Width: 8, Height: 8},
			Steps:  []SourceStep{{Action: "belly_taunt"}},
			Bubble: []string{"你好"},
		}},
		Menu: &[]SourceMenuEntry{{Interaction: "face"}},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp", "belly_taunt.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	rebuildDir := t.TempDir()
	extractZip(t, out, rebuildDir)
	out2 := filepath.Join(t.TempDir(), "tiny-rebuilt.zip")
	if _, err := Build(rebuildDir, out2, false); err != nil {
		t.Fatal(err)
	}
	first, err := readZipFile(out, "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	second, err := readZipFile(out2, "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("rebuilding from the embedded source changed pack.json")
	}
	manifest, err := readZipManifestFromPath(out2)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Hotzones) != 1 || len(manifest.Interactions) != 1 || len(manifest.Bubbles) != 1 {
		t.Fatalf("rebuild lost interaction metadata: %+v", manifest)
	}
}

func TestBuildRejectsPartiallyClippedHotzone(t *testing.T) {
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Crop: &Rect{X: 4, Y: 4, Width: 8, Height: 8},
		Hotzones: []SourceHotzone{{
			ID: "face", Rect: Rect{X: 2, Y: 4, Width: 4, Height: 4}, Sound: true,
		}},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp"})
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "outside crop") {
		t.Fatalf("Build error = %v", err)
	}
}

func TestBuildIsDeterministicAndRoundTrips(t *testing.T) {
	source := Source{Source: 1, ID: "tiny", Name: "Tiny"}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp"})
	out1 := filepath.Join(t.TempDir(), "one.zip")
	out2 := filepath.Join(t.TempDir(), "two.zip")
	if _, err := Build(dir, out1, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(dir, out2, false); err != nil {
		t.Fatal(err)
	}
	one, err := os.ReadFile(out1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := os.ReadFile(out2)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(one) != sha256.Sum256(two) {
		t.Fatal("two builds produced different bytes")
	}
	packsRoot := filepath.Join(t.TempDir(), "packs")
	if _, err := ImportZip(out1, packsRoot, ImportOptions{ID: "tiny"}); err != nil {
		t.Fatal(err)
	}
	built, err := readZipFile(out1, "pack.json")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(packsRoot, "tiny", "pack.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(built, installed) {
		t.Fatal("installed manifest differs from the built manifest")
	}
}

func TestBuildAcceptsSoundOnlyHotzone(t *testing.T) {
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Hotzones: []SourceHotzone{{
			ID: "face", Rect: Rect{X: 4, Y: 4, Width: 8, Height: 8}, Sound: true,
		}},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	manifest, err := readZipManifestFromPath(out)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Interactions["face"].Sound {
		t.Fatal("sound-only interaction was not preserved")
	}
}

func TestBuildRequiresForceToReplaceOutput(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(dir, out, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second Build error = %v", err)
	}
	if _, err := Build(dir, out, true); err != nil {
		t.Fatalf("force Build: %v", err)
	}
}

func TestBuildForceRejectsDirectoryOutput(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	parent := t.TempDir()
	out := filepath.Join(parent, "existing")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(out, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Build(dir, out, true)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Build error = %v, want non-regular output error", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output directory disappeared: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("output is no longer a directory: %v", info.Mode())
	}
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("directory contents changed: %v", err)
	}
	if string(data) != "keep me" {
		t.Fatalf("sentinel = %q", data)
	}
	backups, err := filepath.Glob(filepath.Join(parent, ".petpack-bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("leftover backups: %v", backups)
	}
}

func TestBuildHonorsEmptyMenu(t *testing.T) {
	empty := []SourceMenuEntry{}
	source := Source{Source: 1, ID: "tiny", Name: "Tiny", Menu: &empty}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp", "belly_taunt.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	manifest, err := readZipManifestFromPath(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest.Actions["belly_taunt"]; !ok {
		t.Fatal("fixture did not generate a special action; empty-menu assertion is not meaningful")
	}
	if len(manifest.Menu) != 0 {
		t.Fatalf("menu = %+v, want empty", manifest.Menu)
	}
}

func TestBuildRejectsFullyTransparentClip(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	if err := os.WriteFile(filepath.Join(dir, "clips", "happy_loop.webp"), transparentClip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "fully transparent") {
		t.Fatalf("Build error = %v, want fully transparent clip error", err)
	}
}

func TestBuildSkipsNonWebPSymlinkDuringScan(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	target := filepath.Join(dir, "preview.mp4")
	if err := os.WriteFile(target, []byte("not a clip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "clips", "preview.mp4")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false); err != nil {
		t.Fatalf("Build rejected a non-WebP symlink: %v", err)
	}
}

func TestBuildRejectsWebPSymlinkDuringScan(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	link := filepath.Join(dir, "clips", "happy_loop.webp")
	if err := os.Symlink(filepath.Join(dir, "clips", "idle_loop.webp"), link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Build error = %v, want symlink rejection", err)
	}
}

func TestBuildForceKeepsUnrelatedBakFile(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "tiny.zip")
	if _, err := Build(dir, out, false); err != nil {
		t.Fatal(err)
	}
	sentinel := out + ".bak"
	if err := os.WriteFile(sentinel, []byte("user backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(dir, out, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "user backup" {
		t.Fatalf("unrelated .bak was replaced: %q", data)
	}
}

func TestBuildWarnsWhenPackHasNoHotzones(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	result, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(result.Warnings, "no hotzones: the pet is not clickable") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestImportZipMarksBubbleAnchorSet(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "tiny.zip")
	writeTinyZip(t, zipPath)
	pack, err := ImportZip(zipPath, filepath.Join(dir, "packs"), ImportOptions{
		ID: "tiny",
		Base: Base{
			Window: Size{Width: 20, Height: 20},
			Sprite: Size{Width: 16, Height: 16},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pack.Manifest.BubbleAnchorSet {
		t.Fatal("imported pack lost its explicit bubble anchor marker")
	}
}

func TestBuildPreviewWritesFullCanvasPNG(t *testing.T) {
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Crop: &Rect{X: 2, Y: 2, Width: 12, Height: 12},
	}
	dir := writeSourceFixture(t, source, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "preview.png")
	if _, err := PreviewSource(dir, out, "", 0); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 16 || image.Bounds().Dy() != 16 {
		t.Fatalf("preview bounds = %v, want full 16x16 canvas", image.Bounds())
	}
}

func TestDocumentedSourceExampleIsValid(t *testing.T) {
	body, err := os.ReadFile("../../docs/examples/pet-pack.source.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := DecodeSource(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSource(source); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRejectsOutOfRangeFrame(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp"})
	out := filepath.Join(t.TempDir(), "preview.png")
	if _, err := PreviewSource(dir, out, "idle_loop", 99); err == nil || !strings.Contains(err.Error(), "outside clip") {
		t.Fatalf("PreviewSource error = %v", err)
	}
}

func TestSourceStrictlyRejectsManifestFields(t *testing.T) {
	body := []byte(`{"source":1,"id":"tiny","name":"Tiny","actions":{}}`)
	if _, err := DecodeSource(body); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("DecodeSource error = %v", err)
	}
}

func TestBuildRejectsSymlinkedExplicitClip(t *testing.T) {
	outside := t.TempDir()
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	outsideClip := filepath.Join(outside, "idle_loop.webp")
	if err := os.WriteFile(outsideClip, data, 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "clips", "idle_loop.webp")
	if err := os.Symlink(outsideClip, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	source := Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Clips: map[string]string{"idle_loop": "clips/idle_loop.webp"},
	}
	body, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sourceFileName), body, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Build error = %v", err)
	}
}

func TestBuildRejectsUnknownExplicitClipRole(t *testing.T) {
	dir := writeSourceFixture(t, Source{
		Source: 1, ID: "tiny", Name: "Tiny",
		Clips: map[string]string{"custom": "clips/idle_loop.webp"},
	}, []string{"idle_loop.webp"})
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "not a supported role") {
		t.Fatalf("Build error = %v", err)
	}
}

func TestBuildRejectsWindowsReservedClipID(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"idle_loop.webp", "con.webp"})
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "reserved Windows") {
		t.Fatalf("Build error = %v", err)
	}
}

func TestBuildMissingIdleLoop(t *testing.T) {
	dir := writeSourceFixture(t, Source{Source: 1, ID: "tiny", Name: "Tiny"}, []string{"wave.webp"})
	_, err := Build(dir, filepath.Join(t.TempDir(), "tiny.zip"), false)
	if err == nil || !strings.Contains(err.Error(), "missing idle_loop") {
		t.Fatalf("Build error = %v", err)
	}
}

func readZipManifestFromPath(path string) (Manifest, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, err
	}
	defer reader.Close()
	manifest, _, err := readZipManifest(reader.File)
	return manifest, err
}

func readZipFile(path, name string) ([]byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		entry, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer entry.Close()
		return io.ReadAll(entry)
	}
	return nil, os.ErrNotExist
}

func extractZip(t *testing.T, path, dir string) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		target := filepath.Join(dir, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		entry, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(target)
		if err != nil {
			entry.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(out, entry)
		closeEntryErr := entry.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
		if closeEntryErr != nil {
			t.Fatal(closeEntryErr)
		}
		if closeOutErr != nil {
			t.Fatal(closeOutErr)
		}
	}
}

func zipContains(t *testing.T, path, name string) bool {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name == name {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
