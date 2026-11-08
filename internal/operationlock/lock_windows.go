package operationlock

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

func init() {
	acquireFileLock = acquireWindowsMutex
}

func acquireWindowsMutex(file *os.File) (func() error, error) {
	// Name the mutex after the persistent lock file's identity, not its path:
	// aliases and moves into retired/ must continue to refer to the same lock.
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`Global\mcTUI-operation-%08x-%08x%08x`,
		info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow))
	if err != nil {
		return nil, err
	}

	ready := make(chan error)
	stop := make(chan struct{})
	// Windows mutex ownership belongs to an OS thread. Keep that thread for
	// the lifetime of the lock, even if the caller releases from another goroutine.
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		handle, err := windows.CreateMutex(nil, false, name)
		if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			ready <- err
			return
		}
		state, err := windows.WaitForSingleObject(handle, 0)
		if err == nil && state != windows.WAIT_OBJECT_0 && state != windows.WAIT_ABANDONED {
			err = windows.ERROR_LOCK_VIOLATION
		}
		if err != nil {
			windows.CloseHandle(handle)
			ready <- err
			return
		}
		// No file handle may remain open inside a directory being retired.
		if err := file.Close(); err != nil {
			windows.ReleaseMutex(handle)
			windows.CloseHandle(handle)
			ready <- err
			return
		}
		ready <- nil
		<-stop
		releaseErr := windows.ReleaseMutex(handle)
		ready <- errors.Join(releaseErr, windows.CloseHandle(handle))
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			close(stop)
			releaseErr = <-ready
		})
		return releaseErr
	}, nil
}
