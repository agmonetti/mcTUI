//go:build linux || darwin

package operationlock

import (
	"os"

	"golang.org/x/sys/unix"
)

func init() {
	lock = func(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
	unlock = func(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_UN) }
}
