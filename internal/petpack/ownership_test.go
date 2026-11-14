package petpack

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPackOwnershipHelper(t *testing.T) {
	data := os.Getenv("LUCIA_PACK_OWNERSHIP_TEST_DIR")
	if data == "" {
		return
	}
	store := NewStore(data, "")
	if err := store.Acquire(); err != nil {
		t.Fatal(err)
	}
	fmt.Println("ready")
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func startOwnershipProcess(t *testing.T, data string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	command := exec.CommandContext(ctx, executable, "-test.run=^TestPackOwnershipHelper$")
	command.Env = append(os.Environ(), "LUCIA_PACK_OWNERSHIP_TEST_DIR="+data)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	input, err := command.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = input.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
		cancel()
	})
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "ready\n" {
		_ = command.Wait()
		t.Fatalf("lock helper did not become ready: %q, %v; %s", line, err, stderr.String())
	}
	return command, input
}

func TestStoreRejectsOtherProcessBeforeChangingData(t *testing.T) {
	data := t.TempDir()
	zipPath := writeStoreZip(t, data, true)
	setup := newTestStore(t, data, "")
	if _, err := setup.Install(zipPath, false); err != nil {
		t.Fatal(err)
	}
	if err := setup.Close(); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(filepath.Join(data, LockFile))
	if err != nil {
		t.Fatal(err)
	}
	pending := []byte("an in-progress import must not be recovered by a contender")
	if err := os.WriteFile(filepath.Join(data, PendingFile), pending, 0o644); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(data, "packs", ".import-owner")
	if err := os.Mkdir(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	startOwnershipProcess(t, data)
	contender := newTestStore(t, data, "")
	operations := map[string]func() error{
		"acquire":  contender.Acquire,
		"recover":  contender.Recover,
		"list":     func() error { _, err := contender.List(); return err },
		"activate": func() error { return contender.SetActive("tiny") },
		"install":  func() error { _, err := contender.Install(zipPath, true); return err },
		"import":   func() error { _, err := contender.Import(zipPath, ImportOptions{Force: true}); return err },
		"raw import": func() error {
			_, err := ImportZip(zipPath, contender.PacksDir(), ImportOptions{Force: true})
			return err
		},
		"delete": func() error { return contender.Delete("tiny") },
	}
	for name, operation := range operations {
		if err := operation(); !errors.Is(err, ErrStoreBusy) {
			t.Errorf("%s error = %v, want ErrStoreBusy", name, err)
		}
	}
	for name, want := range map[string][]byte{LockFile: metadata, PendingFile: pending} {
		got, err := os.ReadFile(filepath.Join(data, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("contender changed %s: %v", name, err)
		}
	}
	for _, name := range []string{stage, filepath.Join(data, "packs", "tiny")} {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("contender removed %s: %v", name, err)
		}
	}
	independent := newTestStore(t, t.TempDir(), "")
	if err := independent.Acquire(); err != nil {
		t.Fatalf("different data directory was blocked: %v", err)
	}
}

func TestStoreOwnershipReleasedAfterExitOrCrash(t *testing.T) {
	for _, crash := range []bool{false, true} {
		t.Run(fmt.Sprintf("crash=%v", crash), func(t *testing.T) {
			data := t.TempDir()
			command, input := startOwnershipProcess(t, data)
			store := newTestStore(t, data, "")
			if err := store.Acquire(); !errors.Is(err, ErrStoreBusy) {
				t.Fatalf("contender error = %v, want ErrStoreBusy", err)
			}
			if crash {
				if err := command.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			if err := command.Wait(); err != nil && !crash {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for {
				err := store.Acquire()
				if err == nil {
					break
				}
				if !errors.Is(err, ErrStoreBusy) || time.Now().After(deadline) {
					t.Fatalf("could not acquire after process exit: %v", err)
				}
				time.Sleep(5 * time.Millisecond)
			}
			if _, err := os.Stat(filepath.Join(store.PacksDir(), ownershipFile)); err != nil {
				t.Fatalf("stable lock file was removed: %v", err)
			}
		})
	}
}

func TestStoreClosePreventsReuseAndReleasesOwnership(t *testing.T) {
	data := t.TempDir()
	store := newTestStore(t, data, "")
	if err := store.Acquire(); err != nil {
		t.Fatal(err)
	}
	other := newTestStore(t, data, "")
	if err := other.Acquire(); !errors.Is(err, ErrStoreBusy) {
		t.Fatalf("independent Store did not contend: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := store.Recover(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed store recovered data: %v", err)
	}
	if err := other.Acquire(); err != nil {
		t.Fatalf("Close did not release ownership: %v", err)
	}
}

func TestStoreImportPreservesCLIOverridesAndPendingTransaction(t *testing.T) {
	data := t.TempDir()
	store := newTestStore(t, data, "")
	var stages []string
	pack, err := store.Import(writeStoreZip(t, data, true), ImportOptions{
		ID: "renamed", Name: "Renamed", OnStage: func(stage string) { stages = append(stages, stage) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.ID != "renamed" || pack.Manifest.Name != "Renamed" || len(stages) == 0 {
		t.Fatalf("CLI overrides/stages lost: %+v, %v", pack.Manifest, stages)
	}
	if _, err := os.Stat(filepath.Join(data, PendingFile)); !os.IsNotExist(err) {
		t.Fatalf("successful import retained pending record: %v", err)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 || list[0].ID != "renamed" {
		t.Fatalf("import metadata = %+v, %v", list, err)
	}
}
