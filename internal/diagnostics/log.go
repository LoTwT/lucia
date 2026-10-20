// Package diagnostics keeps a retrievable log next to the app's data.
// Installed GUI apps have no console on Windows, and a terminal is often
// gone by the time a user reports a problem, so the file is the durable
// copy; stderr stays useful in development and when output is redirected.
//
// Each write goes to the file first and to stderr afterwards. The order is
// deliberate: a GUI process's stderr handle can be invalid, and a combined
// writer that failed there first would drop every line before the file saw
// it.
package diagnostics

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	// FileName is the current log file in the log directory.
	FileName = "lucia.log"
	// maxBytes bounds the current file before it rotates.
	maxBytes = 1 << 20 // 1 MiB
	// backups is how many rotated files `.1`..`.3` are kept.
	backups = 3
)

// Writer appends log lines to the file and mirrors them to stderr,
// ignoring failures of the mirror.
type Writer struct {
	mu     sync.Mutex
	file   *os.File
	mirror io.Writer
}

// Open creates dir when needed, rotates a full log, and opens the current
// file for appending. The caller owns the returned writer and should close
// it before exiting. A rotation failure does not prevent logging: the
// current file keeps growing rather than losing lines.
func Open(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create the log directory: %w", err)
	}
	path := filepath.Join(dir, FileName)
	rotate(path)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	return newWriter(file, os.Stderr), nil
}

func newWriter(file *os.File, mirror io.Writer) *Writer {
	return &Writer{file: file, mirror: mirror}
}

// Write appends p to the file and mirrors it. It returns the file's result,
// so a failed mirror never affects the log's own success.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.file.Write(p)
	if w.mirror != nil {
		_, _ = w.mirror.Write(p)
	}
	return n, err
}

// Close closes the underlying file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// rotate moves a full current file through the backups, dropping the
// oldest. Errors are ignored: the priority is to keep logging, even when a
// backup is temporarily locked by another program.
func rotate(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, backups))
	for i := backups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	_ = os.Rename(path, path+".1")
}
