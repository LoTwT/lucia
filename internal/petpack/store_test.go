package petpack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreImportListActivateDelete(t *testing.T) {
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	store := NewStore(data, "")
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	summary, err := store.Import(zipPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != "tiny" || summary.Name != "Tiny" || !summary.Valid {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Schema != 2 || len(summary.Hotzones) == 0 || summary.Presets == 0 {
		t.Fatalf("summary lacks schema 2 interaction data: %+v", summary)
	}
	if _, err := os.Stat(filepath.Join(data, LockFile)); err != nil {
		t.Fatalf("lockfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "packs", "tiny", "pack.json")); err != nil {
		t.Fatalf("pack manifest: %v", err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Active {
		t.Fatalf("list = %+v", list)
	}
	if err := store.SetActive("tiny"); err != nil {
		t.Fatal(err)
	}
	list, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Active {
		t.Fatalf("active list = %+v", list)
	}
	if err := store.Delete("tiny"); err == nil || !strings.Contains(err.Error(), "不能删除") {
		t.Fatalf("delete active error = %v", err)
	}
	if err := store.SetActive(""); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("tiny"); err != nil {
		t.Fatal(err)
	}
	list, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}
}

func TestStoreImportRequiresLicenseAndDoesNotOverwrite(t *testing.T) {
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	store := NewStore(data, "")
	if _, err := store.Import(zipPath, false); err == nil || !strings.Contains(err.Error(), "授权") {
		t.Fatalf("license error = %v", err)
	}
	if _, err := store.Import(zipPath, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Import(zipPath, true); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("existing pack error = %v", err)
	}
	if _, err := store.Replace(zipPath, true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "packs", "tiny.backup")); !os.IsNotExist(err) {
		t.Fatalf("replace left a backup: %v", err)
	}
}

func TestStoreImportWithoutManifestUsesFilenameAndDefaults(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	summary, err := store.Import(writeStoreZip(t, data, false), true)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != "tiny" || !summary.Valid || len(summary.Hotzones) == 0 || summary.Presets == 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestStoreListsBrokenAndMissingActive(t *testing.T) {
	data := t.TempDir()
	dir := filepath.Join(data, "packs", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(data, "missing")
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	var broken, missing bool
	for _, summary := range list {
		switch summary.ID {
		case "broken":
			broken = !summary.Valid && summary.Error != ""
		case "missing":
			missing = summary.Active && !summary.Valid
		}
	}
	if !broken || !missing {
		t.Fatalf("list = %+v, want broken and missing active entries", list)
	}
}

func TestStoreDeletesInvalidDirectorySafely(t *testing.T) {
	data := t.TempDir()
	dir := filepath.Join(data, "packs", "bad name")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewStore(data, "")
	if err := store.Delete("bad name"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("invalid directory still exists: %v", err)
	}
	if err := store.Delete("../escape"); err == nil {
		t.Fatal("Delete accepted a path escape")
	}
}

func TestStorePreviewPNG(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	if _, err := store.Import(writeStoreZip(t, data, true), true); err != nil {
		t.Fatal(err)
	}
	preview, err := store.Preview("tiny")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(preview, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("preview does not start with a PNG signature: %x", preview[:min(len(preview), 8)])
	}
}

func TestStoreRecoverPendingRestoresBackup(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	if _, err := store.Import(writeStoreZip(t, data, true), true); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(data, "packs", "tiny")
	backup := final + ".backup"
	if err := os.RemoveAll(backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(final, backup); err != nil {
		t.Fatal(err)
	}
	pending := pendingFile{
		Operation: "import", ID: "tiny", Stage: "installing",
		Backup: backup, UpdatedAt: time.Now().UTC(),
	}
	if err := store.writePendingLocked(pending); err != nil {
		t.Fatal(err)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("final pack was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, PendingFile)); !os.IsNotExist(err) {
		t.Fatalf("pending file still exists: %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
}

func TestStoreRecoverRestoresBackupOverBrokenFinal(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	if _, err := store.Import(writeStoreZip(t, data, true), true); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(data, "packs", "tiny")
	backup := final + ".backup"
	if err := os.Rename(final, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(final, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(final, "pack.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(final, "tiny"); err != nil {
		t.Fatalf("broken final was not replaced by the backup: %v", err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("backup still exists: %v", err)
	}
}

func TestStoreRecoverRemovesInterruptedDirectories(t *testing.T) {
	data := t.TempDir()
	for _, name := range []string{".import-tiny-1", ".trash-tiny-2"} {
		if err := os.MkdirAll(filepath.Join(data, "packs", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	store := NewStore(data, "")
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".import-tiny-1", ".trash-tiny-2"} {
		if _, err := os.Stat(filepath.Join(data, "packs", name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists: %v", name, err)
		}
	}
}

func TestImportZipCarriesPackManifest(t *testing.T) {
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	manifest, has, err := ReadZipManifest(zipPath)
	if err != nil || !has {
		t.Fatalf("ReadZipManifest = %+v, %v, %v", manifest, has, err)
	}
	if manifest.ID != "tiny" || manifest.Name != "Tiny" {
		t.Fatalf("manifest = %+v", manifest)
	}
	pack, err := ImportZip(zipPath, filepath.Join(data, "out"), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.ID != "tiny" || pack.Manifest.Name != "Tiny" || pack.Manifest.Schema != 2 {
		t.Fatalf("pack manifest = %+v", pack.Manifest)
	}
	if len(pack.Manifest.Hotzones) == 0 || len(pack.Manifest.Presets) == 0 {
		t.Fatalf("manifest interaction data was lost: %+v", pack.Manifest)
	}
	if got := pack.Manifest.Actions["happy"].Once; got != "idle_loop" {
		t.Fatalf("happy action = %q", got)
	}
}

func writeStoreZip(t *testing.T, dir string, withManifest bool) string {
	t.Helper()
	clip, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("idle_loop_待机_循环.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(clip); err != nil {
		t.Fatal(err)
	}
	if withManifest {
		manifest := validTestManifest()
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := writer.Create("pack.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tiny.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
