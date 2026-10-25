package petpack

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	summary, err := store.Install(zipPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != "tiny" || summary.Name != "Tiny" || !summary.Valid {
		t.Fatalf("summary = %+v", summary)
	}
	if len(summary.Hotzones) == 0 || summary.Interactions == 0 {
		t.Fatalf("summary lacks interaction data: %+v", summary)
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

func TestStoreDeleteKeepsTrashWhenCleanupFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions")
	}
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	store := NewStore(data, "")
	if _, err := store.Install(zipPath, false); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(data, "packs", "tiny", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "file"), []byte("locked"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		entries, _ := os.ReadDir(filepath.Join(data, "packs"))
		for _, entry := range entries {
			_ = os.Chmod(filepath.Join(data, "packs", entry.Name(), "locked"), 0o755)
		}
	})
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete("tiny"); err != nil {
		t.Fatalf("Delete reported a failure after the pack was renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "packs", "tiny")); !os.IsNotExist(err) {
		t.Fatalf("deleted pack still exists: %v", err)
	}
	trashes, err := filepath.Glob(filepath.Join(data, "packs", ".trash-tiny-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(trashes) != 1 {
		t.Fatalf("trash directories = %v, want one retained for Recover", trashes)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}
	if err := os.Chmod(filepath.Join(trashes[0], "locked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(trashes[0]); !os.IsNotExist(err) {
		t.Fatalf("Recover did not remove the trash directory: %v", err)
	}
}

func TestStoreDeleteSucceedsWhenLockWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions")
	}
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	store := NewStore(data, "")
	if _, err := store.Install(zipPath, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(data, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(data, 0o755) })

	if err := store.Delete("tiny"); err != nil {
		t.Fatalf("Delete reported a failure after the pack was renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "packs", "tiny")); !os.IsNotExist(err) {
		t.Fatalf("deleted pack still exists: %v", err)
	}
	if err := os.Chmod(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("stale lock entry survived Recover: %+v", list)
	}
}

func TestStoreInstallConflictsOnlyWhenIdExists(t *testing.T) {
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	store := NewStore(data, "")
	if _, err := store.Install(zipPath, false); err != nil {
		t.Fatal(err)
	}
	_, err := store.Install(zipPath, false)
	var exists *PackExistsError
	if !errors.As(err, &exists) || exists.ID != "tiny" || exists.Name != "Tiny" {
		t.Fatalf("existing pack error = %v, want PackExistsError", err)
	}
	if _, err := os.Stat(filepath.Join(data, PendingFile)); !os.IsNotExist(err) {
		t.Fatalf("conflict wrote a pending file: %v", err)
	}
	if _, err := store.Install(zipPath, true); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, "packs", "tiny.backup")); !os.IsNotExist(err) {
		t.Fatalf("replace left a backup: %v", err)
	}
}

func TestImportMetaOnlyKeepsInteractionsAndBuildsActions(t *testing.T) {
	data := t.TempDir()
	zipPath := writeMetaOnlyZip(t, data)
	pack, err := ImportZip(zipPath, filepath.Join(data, "out"), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pack.Manifest.Actions["happy"]; !ok {
		t.Fatal("import did not build the happy action")
	}
	if len(pack.Manifest.Interactions) == 0 || len(pack.Manifest.Hotzones) == 0 {
		t.Fatalf("authored interactions were lost: %+v", pack.Manifest)
	}
	if pack.Manifest.Schema != 1 {
		t.Fatalf("schema = %d, want 1", pack.Manifest.Schema)
	}
	if pack.Manifest.Events.Intro != "wave" || pack.Manifest.Events.Activate != "wave" {
		t.Fatalf("generated events = %+v", pack.Manifest.Events)
	}
	foundSpecial := false
	for _, entry := range pack.Manifest.Menu {
		if entry.Action == "belly_taunt" {
			foundSpecial = true
		}
	}
	if !foundSpecial {
		t.Fatalf("generated menu = %+v, want belly_taunt", pack.Manifest.Menu)
	}
}

func TestStoreReplaceFailureRestoresHealthyPack(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	if _, err := store.Install(writeStoreZip(t, data, true), false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActive("tiny"); err != nil {
		t.Fatal(err)
	}
	badZip := writeZipWithManifest(t, data, "bad-tiny.zip", func(m *Manifest) {
		m.Actions["idle"] = Action{Loop: "missing", Poster: "idle_loop#0", LoopCount: -1}
	})
	if _, err := store.Install(badZip, true); err == nil {
		t.Fatal("Replace accepted a pack with a missing idle clip")
	}
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "tiny" || !list[0].Active || !list[0].Valid {
		t.Fatalf("healthy pack was not restored: %+v", list)
	}
	if _, err := Load(data, "tiny", 1); err != nil {
		t.Fatalf("restored pack does not load: %v", err)
	}
}

func TestStoreRecoverCorruptMetadata(t *testing.T) {
	data := t.TempDir()
	if err := os.WriteFile(filepath.Join(data, LockFile), []byte("{bad lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, PendingFile), []byte("{bad pending"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewStore(data, "")
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{LockFile + ".bad", PendingFile + ".bad"} {
		if _, err := os.Stat(filepath.Join(data, name)); err != nil {
			t.Fatalf("%s was not preserved: %v", name, err)
		}
	}
	if _, err := store.List(); err != nil {
		t.Fatalf("List after corrupt metadata: %v", err)
	}
}

func TestStoreDeleteRejectsRecoveryArtifacts(t *testing.T) {
	store := NewStore(t.TempDir(), "")
	for _, id := range []string{".backup", "tiny.backup", ".import-tiny"} {
		if err := store.Delete(id); err == nil {
			t.Errorf("Delete accepted recovery artifact %q", id)
		}
	}
}

func TestStoreDeleteRejectsCaseVariantOfActive(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "tiny")
	if err := os.MkdirAll(filepath.Join(data, "packs", "Tiny"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("Tiny"); err == nil {
		t.Fatal("Delete removed the active pack through a case variant")
	}
}

func TestStoreInstallWithoutManifestUsesFilenameAndDefaults(t *testing.T) {
	data := t.TempDir()
	store := NewStore(data, "")
	summary, err := store.Install(writeStoreZip(t, data, false), false)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ID != "tiny" || !summary.Valid || len(summary.Hotzones) != 0 ||
		summary.Interactions != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	pack, err := Load(data, "tiny", 1)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.States.Idle != "idle" || len(pack.Manifest.Actions) == 0 {
		t.Fatalf("generated manifest = %+v", pack.Manifest)
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
	if _, err := store.Install(writeStoreZip(t, data, true), false); err != nil {
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
	if _, err := store.Install(writeStoreZip(t, data, true), false); err != nil {
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
	if _, err := store.Install(writeStoreZip(t, data, true), false); err != nil {
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

func TestImportPreservesAuthoredPartialStates(t *testing.T) {
	data := t.TempDir()
	zipPath := writeZipWithManifest(t, data, "partial-states.zip", func(m *Manifest) {
		m.States = States{Idle: "", Drowsy: "happy"}
	})
	pack, err := ImportZip(zipPath, filepath.Join(data, "out"), ImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.States.Idle != "idle" {
		t.Fatalf("idle state = %q, want generated idle", pack.Manifest.States.Idle)
	}
	if pack.Manifest.States.Drowsy != "happy" {
		t.Fatalf("drowsy state = %q, want the authored action", pack.Manifest.States.Drowsy)
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
	if pack.Manifest.ID != "tiny" || pack.Manifest.Name != "Tiny" || pack.Manifest.Schema != 1 {
		t.Fatalf("pack manifest = %+v", pack.Manifest)
	}
	if len(pack.Manifest.Hotzones) == 0 || len(pack.Manifest.Interactions) == 0 {
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

func writeMetaOnlyZip(t *testing.T, dir string) string {
	t.Helper()
	clip, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, id := range []string{
		"idle_loop", "happy_enter", "happy_exit", "laugh_enter", "laugh_exit",
		"wave_enter", "belly_taunt",
	} {
		entry, err := writer.Create(id + ".webp")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(clip); err != nil {
			t.Fatal(err)
		}
	}
	meta := validTestManifest()
	meta.Clips = nil
	meta.Actions = nil
	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := writer.Create("pack.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "meta-tiny.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeZipWithManifest(t *testing.T, dir, name string, mutate func(*Manifest)) string {
	t.Helper()
	clip, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	manifest := validTestManifest()
	if mutate != nil {
		mutate(&manifest)
	}
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create("idle_loop.webp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(clip); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entry, err = writer.Create("pack.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
