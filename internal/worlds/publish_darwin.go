package worlds

import "golang.org/x/sys/unix"

func publishWorld(stage, destination string) error {
	return unix.RenamexNp(stage, destination, unix.RENAME_EXCL)
}
