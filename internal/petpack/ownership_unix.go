//go:build darwin || linux

package petpack

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockDirectoryFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrStoreBusy
		}
		return err
	}
}

func unlockDirectoryFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
