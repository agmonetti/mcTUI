package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/updater"
)

func TestUpdateUIFlow(t *testing.T) {
	cfg := config.Data{
		Username: "UpdaterTester",
		Language: "en",
		Theme:    "mctui",
	}
	rel := updater.Release{
		TagName: "v2.1.0",
		Name:    "v2.1.0 Release",
		Body:    "- Added new feature\n- Security patch",
		Assets: []updater.Asset{
			{Name: "checksums.txt", Size: 100},
			{Name: "mctui-linux-amd64", Size: 1000000},
		},
	}

	m := New([]string{"1.20.4"}, cfg, rel, "v2.0.0")

	// 1. Verify footer contains update shortcut
	view := m.View()
	if !strings.Contains(view, "[u] Update") && !strings.Contains(view, "v2.1.0") {
		t.Fatalf("expected view to contain update notification in footer:\n%s", view)
	}

	// 2. Press 'u' to open update screen
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(Model)
	if m.state != updateScreen {
		t.Fatalf("expected state updateScreen after pressing 'u', got %v", m.state)
	}

	// 3. Verify updater panel content
	updateView := m.View()
	if !strings.Contains(updateView, "MCTUI UPDATER") {
		t.Errorf("expected MCTUI UPDATER header in update view:\n%s", updateView)
	}
	if !strings.Contains(updateView, strings.Repeat("─", 46)) {
		t.Errorf("expected 46-character fixed divider in update view:\n%s", updateView)
	}
	if !strings.Contains(updateView, "v2.0.0") || !strings.Contains(updateView, "v2.1.0") {
		t.Errorf("expected current and latest version in update view:\n%s", updateView)
	}

	// 4. Test progress message
	updated, _ = m.Update(UpdateProgressMsg{Stage: "downloading", Percent: 0.5})
	m = updated.(Model)
	if m.updateStage != "downloading" || m.updateProgress != 0.5 {
		t.Errorf("expected downloading stage and 0.5 progress, got %s / %f", m.updateStage, m.updateProgress)
	}

	// 5. Test finished message
	updated, _ = m.Update(UpdateFinishedMsg{Err: nil})
	m = updated.(Model)
	if m.updateStage != "done" {
		t.Errorf("expected done stage after finish, got %s", m.updateStage)
	}
	doneView := m.View()
	if !strings.Contains(doneView, "SHA-256") || !strings.Contains(doneView, "v2.1.0") || !strings.Contains(doneView, "mcTUI") {
		t.Errorf("expected success message in done view:\n%s", doneView)
	}

	// 6. Test Enter on done quits the app
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Errorf("expected quit cmd after Enter on done stage")
	}

	// 7. Test Esc returns to configScreen
	m.state = updateScreen
	m.updateStage = "idle"
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != configScreen {
		t.Errorf("expected Esc on idle updateScreen to return to configScreen, got %v", m.state)
	}

	// 8. Test entering updateScreen from configScreen cursorConfig 8
	m.state = configScreen
	m.cursorConfig = 8
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != updateScreen {
		t.Errorf("expected Enter on config cursor 8 to enter updateScreen, got %v", m.state)
	}
}
