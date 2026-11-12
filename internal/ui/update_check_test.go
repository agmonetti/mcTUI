package ui

import (
	"strings"
	"testing"

	"mcTUI/internal/i18n"
	"mcTUI/internal/updater"
)

func TestUpdateCheckErrorShowsQuietNotice(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}

	m := Model{}.WithUpdateCheckError(updater.ErrRateLimited)
	if m.updateCheckErr == nil {
		t.Fatal("WithUpdateCheckError did not store the error")
	}

	var panel strings.Builder
	m.renderUpdatePanel(&panel)
	if !strings.Contains(panel.String(), "Update check unavailable") {
		t.Errorf("update panel = %q, want the quiet rate-limit notice", panel.String())
	}
	if strings.Contains(panel.String(), "Up to date") {
		t.Errorf("update panel = %q, must not claim the app is up to date", panel.String())
	}
}
