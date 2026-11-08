package operationlock

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	// Keep the lock held while its containing instance is moved to retired/.
	// Go's default file sharing mode prevents that rename on Windows.
	openLockFile = func(path string) (*os.File, error) {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}
		handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(handle), path), nil
	}
	lock = func(file *os.File) error {
		var overlapped windows.Overlapped
		return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	}
	unlock = func(file *os.File) error {
		var overlapped windows.Overlapped
		return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
	}
}
