package diagnostics

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAppendsAndMirrors(t *testing.T) {
	dir := t.TempDir()
	w, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var mirror bytes.Buffer
	w.mirror = &mirror
	if _, err := w.Write([]byte("first line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "first line\n" {
		t.Errorf("file %q, want first line", data)
	}
	if got := mirror.String(); got != "first line\n" {
		t.Errorf("mirror %q, want first line", got)
	}

	w2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := w2.Write([]byte("second line\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w2.Close()
	data, err = os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := "first line\nsecond line\n"; string(data) != want {
		t.Errorf("file %q, want %q (append, not truncate)", data, want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no console") }

func TestFileWriteSurvivesFailedMirror(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := newWriter(file, failingWriter{})
	if _, err := w.Write([]byte("kept\n")); err != nil {
		t.Fatalf("Write reported the mirror's failure: %v", err)
	}
	w.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "kept\n" {
		t.Errorf("file %q, want the line despite the failed mirror", data)
	}
}

func TestRotateKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rotate(path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("current file still exists after rotation: %v", err)
	}
	data, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != maxBytes || strings.Trim(string(data), "x") != "" {
		t.Errorf("backup .1 has %d bytes, want the %d-byte current file", len(data), maxBytes)
	}
	data, err = os.ReadFile(path + ".2")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "older\n" {
		t.Errorf("backup .2 %q, want the previous .1", data)
	}
}
