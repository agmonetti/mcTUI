package i18n

import (
	"testing"
)

func TestDetectSystemLanguage(t *testing.T) {
	for _, tc := range []struct {
		envName  string
		envVal   string
		expected string
	}{
		{"LANG", "es_AR.UTF-8", "es"},
		{"LANG", "es_ES", "es"},
		{"LANG", "en_US.UTF-8", "en"},
		{"LANG", "fr_FR", "en"},
	} {
		t.Run(tc.envVal, func(t *testing.T) {
			t.Setenv("LC_ALL", "")
			t.Setenv("LC_MESSAGES", "")
			t.Setenv("LANG", "")
			t.Setenv(tc.envName, tc.envVal)

			got := DetectSystemLanguage()
			if got != tc.expected {
				t.Fatalf("for %s=%q, expected %q, got %q", tc.envName, tc.envVal, tc.expected, got)
			}
		})
	}
}
