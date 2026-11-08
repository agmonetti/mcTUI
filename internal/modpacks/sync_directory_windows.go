package modpacks

import (
	"fmt"
	"os"
)

func syncDirectory(path string) error {
	// Windows does not provide POSIX directory fsync through os.File.Sync.
	// Regular file contents are still synced before publication; directory
	// metadata durability across power loss is not guaranteed on Windows.
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	return nil
}
