//go:build !windows

package i18n

import (
	"os"
	"strings"
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
	return "en"
}
