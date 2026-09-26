package worlds

import (
	"testing"
	"time"

	"mcTUI/internal/i18n"
)

func TestFormatRelativeTimeUsesSelectedLanguage(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLanguage("en") })

	if got := FormatRelativeTime(0); got != "" {
		t.Fatalf("unknown timestamp = %q, want empty", got)
	}
	now := time.Now()
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{
		{30 * time.Second, "ahora mismo"},
		{2 * time.Minute, "hace 2 min"},
		{3 * time.Hour, "hace 3 h"},
		{48 * time.Hour, "hace 2 días"},
	} {
		if got := FormatRelativeTime(now.Add(-tc.age).UnixMilli()); got != tc.want {
			t.Errorf("FormatRelativeTime(%s) = %q, want %q", tc.age, got, tc.want)
		}
	}
}
