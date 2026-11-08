// Package operationlock serializes filesystem mutations for one game directory.
package operationlock

import (
	"fmt"
	"os"
	"path/filepath"
)

// Platform implementations install the OS-specific non-blocking operations.
var lock = func(*os.File) error { return fmt.Errorf("OS file locking unavailable") }
var unlock = func(*os.File) error { return fmt.Errorf("OS file locking unavailable") }
var openLockFile = func(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
}

// Acquire takes a non-blocking OS lock. The lock file is intentionally never
// removed: removing it could let another process lock a different inode.
func Acquire(gameDir string) (func() error, error) {
	if !filepath.IsAbs(gameDir) || filepath.Clean(gameDir) != gameDir {
		return nil, fmt.Errorf("unsafe game directory: %q", gameDir)
	}
	path := filepath.Join(gameDir, ".mctui-operation.lock")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe operation lock: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	opened, statErr := file.Stat()
	if err != nil || statErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()
		return nil, fmt.Errorf("operation lock changed or is not a regular file: %s", path)
	}
	if err := lock(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("instance busy (%s): %w", gameDir, err)
	}
	return func() error {
		err := unlock(file)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		return err
	}, nil
}
