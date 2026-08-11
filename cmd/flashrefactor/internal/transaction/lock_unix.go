//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package transaction

import (
	"errors"
	"os"
	"syscall"
)

var errLeaseLocked = errors.New("transaction lease is locked")

func lockExclusive(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return errLeaseLocked
		}
		return err
	}
	return nil
}

func unlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
