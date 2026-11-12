package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestUpdateScrollReleaseNotes(t *testing.T) {
	cfg := config.Data{
		Username: "UpdaterTester",
		Language: "en",
		Theme:    "mctui",
	}
	body := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10"
	rel := updater.Release{
		TagName: "v2.1.0",
		Name:    "v2.1.0 Release",
		Body:    body,
	}

	m := New([]string{"1.20.4"}, cfg, rel, "v2.0.0")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updated.(Model)

	// Initial view: scroll is 0, should show down arrow '↓' but not up arrow '↑' in release notes
	v0 := m.View()
	if !strings.Contains(v0, "    ↓") {
		t.Errorf("expected down arrow in release notes when more lines are below:\n%s", v0)
	}
	if strings.Contains(v0, "    ↑") {
		t.Errorf("did not expect up arrow in release notes at top:\n%s", v0)
	}
	if !strings.Contains(v0, "Line 1") {
		t.Errorf("expected Line 1 to be visible at top:\n%s", v0)
	}

	// Press down
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.updateScroll != 1 {
		t.Fatalf("expected updateScroll = 1, got %d", m.updateScroll)
	}
	v1 := m.View()
	if !strings.Contains(v1, "    ↑") {
		t.Errorf("expected up arrow in release notes after scrolling down:\n%s", v1)
	}
	if !strings.Contains(v1, "    ↓") {
		t.Errorf("expected down arrow in release notes when more lines still exist below:\n%s", v1)
	}

	// Press up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.updateScroll != 0 {
		t.Fatalf("expected updateScroll = 0 after pressing up, got %d", m.updateScroll)
	}
	v2 := m.View()
	if strings.Contains(v2, "    ↑") {
		t.Errorf("did not expect up arrow in release notes after scrolling back to top:\n%s", v2)
	}
}

func TestUpdateSuccessSchedulesRestartAndQuitsOnTimer(t *testing.T) {
	origDelay := updateRestartDelay
	updateRestartDelay = 5 * time.Millisecond
	t.Cleanup(func() { updateRestartDelay = origDelay })

	m := Model{
		state:         updateScreen,
		updateStage:   "installing",
		latestRelease: updater.Release{TagName: "v2.1.0"},
		appVersion:    "v2.0.0",
	}

	// 1. Finished message transitions to done, marks pendingRestart, and schedules restart
	updated, cmd := m.Update(UpdateFinishedMsg{Err: nil})
	m = updated.(Model)
	if m.updateStage != "done" {
		t.Fatalf("expected updateStage = done, got %s", m.updateStage)
	}
	if !m.pendingRestart {
		t.Fatal("expected pendingRestart = true")
	}
	if cmd == nil {
		t.Fatal("expected schedule restart cmd, got nil")
	}

	// 2. Escape while on done must not dismiss the screen
	updated, escCmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != updateScreen || m.updateStage != "done" {
		t.Fatalf("expected to remain on updateScreen done stage, got state=%v stage=%v", m.state, m.updateStage)
	}
	if escCmd != nil {
		t.Fatalf("expected nil cmd for esc on done stage, got %v", escCmd)
	}

	// 3. Restart message triggers tea.Quit, cleans up, and sets ShouldRestart
	msg := cmd()
	if _, ok := msg.(updateRestartMsg); !ok {
		t.Fatalf("cmd produced %T, want updateRestartMsg", msg)
	}
	quitModel, quitCmd := m.Update(msg)
	final := quitModel.(Model)
	if !final.ShouldRestart() {
		t.Fatal("expected ShouldRestart() = true after updateRestartMsg")
	}
	if quitCmd == nil {
		t.Fatal("expected quit cmd, got nil")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit cmd returned %T, want tea.QuitMsg", quitCmd())
	}
}

func TestUpdateFailureDoesNotScheduleRestart(t *testing.T) {
	m := Model{
		state:         updateScreen,
		updateStage:   "downloading",
		latestRelease: updater.Release{TagName: "v2.1.0"},
	}

	updated, cmd := m.Update(UpdateFinishedMsg{Err: errors.New("network failure")})
	m = updated.(Model)
	if m.updateStage != "error" {
		t.Fatalf("expected stage error, got %s", m.updateStage)
	}
	if m.pendingRestart || m.ShouldRestart() {
		t.Fatal("expected pendingRestart and ShouldRestart to be false on update error")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd on error, got %v", cmd)
	}

	// Esc on error must return to configScreen without restarting
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != configScreen {
		t.Fatalf("expected Esc to return to configScreen, got %v", m.state)
	}
	if m.ShouldRestart() {
		t.Fatal("ShouldRestart must remain false")
	}
}

func TestUpdateEnterOnDoneTriggersImmediateRestart(t *testing.T) {
	m := Model{
		state:          updateScreen,
		updateStage:    "done",
		pendingRestart: true,
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if !m.ShouldRestart() {
		t.Fatal("expected ShouldRestart = true on Enter during done stage")
	}
	if cmd == nil {
		t.Fatal("expected quit cmd, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd returned %T, want tea.QuitMsg", cmd())
	}
}
