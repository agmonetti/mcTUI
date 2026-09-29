package worlds

import (
	"golang.org/x/sys/unix"
)

// RENAME_NOREPLACE publishes a completed save atomically without ever
// replacing a world created by another writer while this copy was running.
func publishWorld(stage, destination string) error {
	return unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
}
