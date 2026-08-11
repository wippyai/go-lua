//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package transaction

import (
	"errors"
	"os"
)

var errLeaseLocked = errors.New("transaction lease is locked")

func lockExclusive(file *os.File) error {
	return errors.New("transaction recovery requires advisory file locking on this platform")
}

func unlock(file *os.File) error { return nil }
