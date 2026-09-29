package worlds

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func publishWorld(stage, destination string) error {
	from, err := windows.UTF16PtrFromString(stage)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	// MOVEFILE_REPLACE_EXISTING is deliberately absent: publication may
	// not replace an existing world, even if it appears during the copy.
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		if _, statErr := os.Lstat(destination); statErr == nil {
			return fs.ErrExist
		}
		return err
	}
	return nil
}
