//go:build windows

package i18n

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

func detectPlatformLanguage() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val := strings.ToLower(strings.TrimSpace(os.Getenv(env)))
		if strings.HasPrefix(val, "es") {
			return "es"
		}
		if strings.HasPrefix(val, "en") {
			return "en"
		}
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultLocaleName")
	if proc.Find() != nil {
		return "en"
	}

	b := make([]uint16, 85)
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if r > 0 {
		name := syscall.UTF16ToString(b)
		if strings.HasPrefix(strings.ToLower(name), "es") {
			return "es"
		}
	}
	return "en"
}
