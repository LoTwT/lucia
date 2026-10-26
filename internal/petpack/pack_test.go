package petpack

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTinyClip(t *testing.T) {
	clip, err := OpenClip("tiny", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	if clip.Width != 16 || clip.Height != 16 {
		t.Fatalf("canvas = %dx%d, want 16x16", clip.Width, clip.Height)
	}
	if clip.FrameCount() != 3 {
		t.Fatalf("frames = %d, want 3", clip.FrameCount())
	}
	if got := clip.TotalDuration(); got != 150*time.Millisecond {
		t.Fatalf("duration = %v, want 150ms", got)
	}
	if got := clip.Frames[0].Rect; got != image.Rect(2, 2, 13, 13) {
		t.Fatalf("frame 0 rect = %v", got)
	}
	if !clip.Frames[0].Dispose {
		t.Fatal("frame 0 should dispose to background")
	}
	if !clip.Frames[1].Blend {
		t.Fatal("frame 1 should blend")
	}
	if clip.Frames[2].Blend {
		t.Fatal("frame 2 should not blend")
	}
}

func TestDecodeTinyClip(t *testing.T) {
	clip, err := OpenClip("tiny", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	frames, err := clip.DecodeAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("decoded frames = %d, want 3", len(frames))
	}
	for i, frame := range frames {
		if frame.Bounds() != image.Rect(0, 0, 16, 16) {
			t.Fatalf("frame %d bounds = %v", i, frame.Bounds())
		}
		if _, _, _, a := frame.At(0, 0).RGBA(); a != 0 {
			t.Fatalf("frame %d corner alpha = %d, want transparent", i, a)
		}
	}
	if _, _, _, a := frames[0].At(8, 8).RGBA(); a == 0 {
		t.Fatal("frame 0 does not contain the first drawing")
	}
	if _, _, _, a := frames[1].At(4, 2).RGBA(); a == 0 {
		t.Fatal("frame 1 does not contain the second drawing")
	}
	if _, _, _, a := frames[2].At(8, 8).RGBA(); a == 0 {
		t.Fatal("frame 2 does not contain the third drawing")
	}
}

func TestDecodeLossyAlphaClip(t *testing.T) {
	clip, err := OpenClip("tiny-lossy", "testdata/tiny-lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	frames, err := clip.DecodeAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("decoded frames = %d, want 3", len(frames))
	}
	if _, _, _, a := frames[0].At(8, 8).RGBA(); a == 0 {
		t.Fatal("lossy ALPH frame is transparent at the center")
	}
}

func TestCropAndScale(t *testing.T) {
	clip, err := OpenClip("tiny", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	clip.Crop = image.Rect(2, 2, 14, 14)
	frames, err := clip.DecodeAllScaled(6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 {
		t.Fatalf("frames = %d", len(frames))
	}
	for i, frame := range frames {
		if frame.Bounds() != image.Rect(0, 0, 6, 6) {
			t.Fatalf("frame %d bounds = %v", i, frame.Bounds())
		}
	}
}

func TestLoadPackAndPoster(t *testing.T) {
	dir := writeTinyPack(t)
	pack, err := Load(dir, "tiny", 2)
	if err != nil {
		t.Fatal(err)
	}
	if pack.PixelWidth != 16 || pack.PixelHeight != 16 {
		t.Fatalf("cache size = %dx%d, want 16x16", pack.PixelWidth, pack.PixelHeight)
	}
	poster, err := pack.Poster(pack.Manifest.Actions["idle"])
	if err != nil {
		t.Fatal(err)
	}
	if poster.Bounds() != image.Rect(0, 0, 16, 16) {
		t.Fatalf("poster bounds = %v", poster.Bounds())
	}
}

func TestImportZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "tiny.zip")
	writeTinyZip(t, zipPath)
	packsRoot := filepath.Join(dir, "custom-packs")
	pack, err := ImportZip(zipPath, packsRoot, ImportOptions{
		ID:   "tiny",
		Name: "Tiny",
		Base: Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.ID != "tiny" || len(pack.Clips) != 1 {
		t.Fatalf("imported pack = %+v", pack.Manifest)
	}
	if _, ok := pack.Manifest.Actions["idle"]; !ok {
		t.Fatal("import did not create the idle action")
	}
	if _, err := os.Stat(filepath.Join(packsRoot, "tiny", "clips", "idle_loop.webp")); err != nil {
		t.Fatalf("imported clip: %v", err)
	}
}

func TestImportZipSkipsFullyTransparentClip(t *testing.T) {
	dir := t.TempDir()
	tiny, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(dir, "transparent.zip")
	writeClipZip(t, zipPath, map[string][]byte{
		"idle_loop.webp":        tiny,
		"transparent_loop.webp": transparentClip(t),
	})
	pack, err := ImportZip(zipPath, filepath.Join(dir, "packs"), ImportOptions{
		ID:   "transparent",
		Base: Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pack.Clips["transparent_loop"]; !ok {
		t.Fatal("transparent clip was dropped from the manifest")
	}
	tinyClip, err := OpenClip("idle_loop", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := tinyClip.AlphaBounds()
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.Rect(0, 0, 16, 16)
	crop := bounds.Inset(-2).Intersect(canvas)
	want := Rect{X: crop.Min.X, Y: crop.Min.Y, Width: crop.Dx(), Height: crop.Dy()}
	if pack.Manifest.Crop != want {
		t.Fatalf("crop = %+v, want %+v (transparent clip affected alpha bounds)", pack.Manifest.Crop, want)
	}
}

func TestImportZipAllTransparentClipsFallBackToCanvas(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "transparent.zip")
	writeClipZip(t, zipPath, map[string][]byte{
		"idle_loop.webp": transparentClip(t),
	})
	pack, err := ImportZip(zipPath, filepath.Join(dir, "packs"), ImportOptions{
		ID:   "transparent",
		Base: Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Rect{Width: 16, Height: 16}); pack.Manifest.Crop != want {
		t.Fatalf("crop = %+v, want the full canvas %+v", pack.Manifest.Crop, want)
	}
}

func TestImportZipInfersIDFromFilename(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "tiny.zip")
	writeTinyZip(t, zipPath)
	pack, err := ImportZip(zipPath, filepath.Join(dir, "packs"), ImportOptions{
		Base: Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.ID != "tiny" {
		t.Fatalf("inferred id = %q, want tiny", pack.Manifest.ID)
	}
}

func TestInvalidPackID(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "a b"} {
		if _, err := Load(t.TempDir(), id, 1); err == nil {
			t.Errorf("Load accepted invalid id %q", id)
		}
	}
}

func TestPackDirectoryIDsAreLowercase(t *testing.T) {
	for _, id := range []string{"tiny-pack", "pack_2"} {
		if !validPackDirID(id) {
			t.Errorf("validPackDirID(%q) = false", id)
		}
	}
	for _, id := range []string{"Tiny", "TINY", "Pack_2"} {
		if validPackDirID(id) {
			t.Errorf("validPackDirID accepted mixed-case id %q", id)
		}
	}
}

func TestClipIDFromName(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"idle_loop_待机_循环.webp", "idle_loop"},
		{"happy_enter_开心_进入.webp", "happy_enter"},
		{"01_belly_taunt_肚肚挑衅.webp", "belly_taunt"},
		{"03_smug_stumble_得意过头.webp", "smug_stumble"},
		{"drag_loop_轻悬循环.webp", "drag_loop"},
	}
	for _, tt := range tests {
		got, err := clipIDFromName(tt.name)
		if err != nil {
			t.Errorf("clipIDFromName(%q): %v", tt.name, err)
			continue
		}
		if got != tt.want {
			t.Errorf("clipIDFromName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestUnicodePathExtraField(t *testing.T) {
	name := "透明动画_WebP/idle_loop_待机_循环.webp"
	original := "idle_loop.webp"
	data := make([]byte, 5+len(name))
	data[0] = 1
	binary.LittleEndian.PutUint32(data[1:5], crc32.ChecksumIEEE([]byte(original)))
	copy(data[5:], name)
	extra := make([]byte, 4+len(data))
	binary.LittleEndian.PutUint16(extra[0:2], 0x7075)
	binary.LittleEndian.PutUint16(extra[2:4], uint16(len(data)))
	copy(extra[4:], data)
	got, ok := unicodePath(extra, original)
	if !ok || got != name {
		t.Fatalf("unicodePath = %q, %v; want %q, true", got, ok, name)
	}

	bad := append([]byte(nil), extra...)
	bad[4+1] ^= 0xff // corrupt the CRC
	if got, ok := unicodePath(bad, original); ok {
		t.Fatalf("unicodePath accepted a bad CRC: %q", got)
	}
}

func TestParseClipRejectsHugeCanvas(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	index := bytes.Index(data, []byte("VP8X"))
	if index < 0 || len(data) < index+18 {
		t.Fatal("test clip has no VP8X chunk")
	}
	// VP8X stores width and height as 24-bit values one less than the edge.
	writeU24Value(data[index+12:index+15], maxCanvasEdge)
	writeU24Value(data[index+15:index+18], 16)
	if _, err := ParseClip("huge", data); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ParseClip error = %v, want canvas limit error", err)
	}
}

func TestNewDecoderRejectsHugeCanvas(t *testing.T) {
	clip := &Clip{ID: "huge", Width: maxCanvasEdge + 1, Height: 1, data: []byte("webp")}
	if _, err := clip.NewDecoder(); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("NewDecoder error = %v, want canvas limit error", err)
	}
}

func TestCacheSizePreservesCropAspect(t *testing.T) {
	tests := []struct {
		name             string
		spriteW, spriteH int
		cropW, cropH     int
		scale            float64
		wantW, wantH     int
	}{
		{"naiwa", 240, 300, 379, 577, 2, 379, 577},
		{"wide crop limited by width", 240, 300, 500, 700, 2, 429, 600},
		{"small crop is not upscaled", 240, 300, 100, 100, 2, 100, 100},
		{"crop already fits", 240, 300, 200, 250, 2, 200, 250},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := cacheSize(tt.spriteW, tt.spriteH, tt.cropW, tt.cropH, tt.scale)
			if w != tt.wantW || h != tt.wantH {
				t.Fatalf("cacheSize = %dx%d, want %dx%d", w, h, tt.wantW, tt.wantH)
			}
			wantAspect := float64(tt.cropW) / float64(tt.cropH)
			if got := float64(w) / float64(h); math.Abs(got-wantAspect) > 0.01 {
				t.Fatalf("aspect = %.4f, want %.4f", got, wantAspect)
			}
			if w > int(math.Round(float64(tt.spriteW)*tt.scale)) ||
				h > int(math.Round(float64(tt.spriteH)*tt.scale)) {
				t.Fatalf("cacheSize %dx%d exceeds the sprite box", w, h)
			}
		})
	}
}

func TestLoadFSUsesTheGivenFilesystem(t *testing.T) {
	dir := writeTinyPack(t)
	packDir := filepath.Join(dir, "packs", "tiny")
	pack, err := LoadFS(os.DirFS(packDir), "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pack.Clip("idle_loop"); !ok {
		t.Fatal("LoadFS did not load idle_loop")
	}
	if _, err := ReadManifestFS(os.DirFS(packDir), "tiny"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifestFS(os.DirFS(dir), "tiny"); err == nil {
		t.Fatal("ReadManifestFS found a manifest outside the pack filesystem")
	}
}

func TestClipDetectsFileReplacedAfterLoad(t *testing.T) {
	dir := writeTinyPack(t)
	pack, err := Load(dir, "tiny", 2)
	if err != nil {
		t.Fatal(err)
	}
	clip := pack.Clips["idle_loop"]
	clipPath := filepath.Join(dir, "packs", "tiny", filepath.FromSlash(clip.name))
	if err := os.WriteFile(clipPath, []byte("different bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := clip.NewDecoder(); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("NewDecoder error = %v, want changed-on-disk error", err)
	}
}

func TestWrapFrameRejectsOutOfRangeChunk(t *testing.T) {
	clip, err := OpenClip("tiny", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := clip.NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	clip.Frames[0].chunks[0].offset = len(decoder.data) + 1
	if _, _, err := decoder.Next(); err == nil || !strings.Contains(err.Error(), "outside clip data") {
		t.Fatalf("Next error = %v, want out-of-range error", err)
	}
}

func TestNextScaledAlternatesBuffers(t *testing.T) {
	clip, err := OpenClip("tiny", "testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	decoder, err := clip.NewDecoder()
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.Close()
	frames := make([]*image.RGBA, 3)
	for i := range frames {
		img, _, err := decoder.NextScaled(16, 16)
		if err != nil {
			t.Fatal(err)
		}
		frames[i] = img.(*image.RGBA)
	}
	if frames[0] == frames[1] {
		t.Fatal("consecutive scaled frames share one buffer")
	}
	if frames[0] != frames[2] {
		t.Fatal("scaled frames did not alternate buffers")
	}
}

func TestImportRequiresIdleLoop(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("happy_enter_开心.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "no-idle.zip")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ImportZip(zipPath, filepath.Join(dir, "packs"), ImportOptions{
		ID: "no-idle", Name: "No idle",
		Base: Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
	})
	if err == nil || !strings.Contains(err.Error(), "idle_loop") {
		t.Fatalf("ImportZip error = %v, want missing idle_loop error", err)
	}
}

func writeU24Value(dst []byte, value int) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
}

func writeTinyPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	packDir := filepath.Join(dir, "packs", "tiny")
	if err := os.MkdirAll(filepath.Join(packDir, "clips"), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "clips", "idle_loop.webp"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Schema:       1,
		ID:           "tiny",
		Canvas:       Size{Width: 16, Height: 16},
		Crop:         Rect{Width: 16, Height: 16},
		Base:         Base{Window: Size{20, 20}, Sprite: Size{16, 16}},
		BubbleAnchor: Anchor{X: 0.5, Y: 0.1},
		Clips: map[string]ClipEntry{
			"idle_loop": {File: "clips/idle_loop.webp", Frames: 3, Loop: true},
		},
		Actions: map[string]Action{
			"idle": {Loop: "idle_loop", Poster: "idle_loop#0", Interruptible: true, LoopCount: -1},
		},
		States: States{Idle: "idle"},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "pack.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// transparentClip is a 16×16, one-frame animated WebP whose pixels are all
// transparent. It is embedded here instead of adding another artwork fixture;
// the VP8L payload comes from a solid transparent PNG encoded with img2webp.
const transparentClipBase64 = "UklGRlIAAABXRUJQVlA4WAoAAAACAAAADwAADwAAQU5JTQYAAAAAAAAAAABBTk1GJgAAAAAAAAAAAA8AAA8AAGQAAABWUDhMDQAAAC8PwAMQBxAREYiI/gcA"

func transparentClip(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(transparentClipBase64)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeClipZip(t *testing.T, path string, clips map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, data := range clips {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTinyZip(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	metadata, err := writer.Create("__MACOSX/._idle_loop_待机_循环.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metadata.Write([]byte("resource fork")); err != nil {
		t.Fatal(err)
	}
	entry, err := writer.Create("idle_loop_待机_循环.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
