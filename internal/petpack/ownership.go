package petpack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrStoreBusy means another Store or importer owns this pack directory.
// The caller should retry after that owner exits, never remove the lock file.
var ErrStoreBusy = errors.New("素材目录已被另一个进程占用，请等待导入结束或退出对应应用后重试")

const ownershipFile = ".write.lock"

type directoryLock struct{ file *os.File }

func acquireDirectoryLock(packsRoot string) (*directoryLock, error) {
	if err := os.MkdirAll(packsRoot, 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(packsRoot, ownershipFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockDirectoryFile(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%w (%s)", err, packsRoot)
	}
	return &directoryLock{file: file}, nil
}

func (lock *directoryLock) Close() error {
	// Keep the file in place. Unlinking it would let another process lock a
	// new inode while a contender still has the previous inode open.
	return errors.Join(unlockDirectoryFile(lock.file), lock.file.Close())
}

// Acquire reserves the directory until Close. Call it before reading settings
// or recovering imports: both the player and settings cache outlive one write.
// Store operations also acquire lazily for callers that do not preload state.
func (s *Store) Acquire() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquireLocked()
}

func (s *Store) acquireLocked() error {
	if s.closed {
		return os.ErrClosed
	}
	if s.ownership == nil {
		lock, err := acquireDirectoryLock(s.PacksDir())
		if err != nil {
			return err
		}
		s.ownership = lock
	}
	return nil
}

// Close releases directory ownership. It waits for a running Store operation,
// is idempotent, and makes subsequent writes fail with os.ErrClosed.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.ownership == nil {
		return nil
	}
	err := s.ownership.Close()
	s.ownership = nil
	return err
}
