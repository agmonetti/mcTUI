package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/resetconfigs"
)

func resetKey(t *testing.T, m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(key)
	return updated.(Model), cmd
}

func TestResetFlow(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", home)
	} else {
		t.Setenv("HOME", home)
	}
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mods", "saves"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "data"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{state: menuScreen, cursorMenu: 6, height: 24, width: 80}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetModeScreen || m.resetDir != dir || m.resetMode != resetconfigs.Backup || len(m.resetFound) != 1 || !strings.Contains(m.View(), "[Enter] Continue") {
		t.Fatalf("inspection or mode prompt failed: %+v", m)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.resetMode != resetconfigs.Purge {
		t.Fatal("purge mode not selected")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetConfirmScreen || !strings.Contains(m.View(), "PERMANENT DELETION") || !strings.Contains(m.View(), "[y] Delete permanently") {
		t.Fatalf("purge confirmation not visible: %q", m.View())
	}
	m, cmd := resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetConfirmScreen || cmd != nil {
		t.Fatal("Enter must not confirm deletion")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != menuScreen {
		t.Fatal("cancel failed")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "data")); err != nil {
		t.Fatal("cancel modified mods:", err)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.state != resetRunningScreen || cmd == nil {
		t.Fatal("backup did not start asynchronously")
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "data")); err != nil {
		t.Fatal("mutation occurred before command ran:", err)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != resetRunningScreen {
		t.Fatal("running reset must not be dismissed")
	}
	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if m.state != resetResultScreen || m.play || m.resetErr != nil || len(m.resetResult.Errors) != 0 {
		t.Fatalf("incorrect result: %+v", m)
	}
	if !strings.Contains(m.View(), "mods/: moved") {
		t.Fatal("backup result is missing folder status")
	}
	for m.resetScroll < m.resetMaxScroll() {
		m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if !strings.Contains(m.View(), "Backup: ") {
		t.Fatal("backup destination not reachable by scrolling")
	}
	if _, err := os.Stat(filepath.Join(m.resetResult.BackupPath, "mods", "data")); err != nil {
		t.Fatal("backup missing mods:", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "saves", "data")); err != nil {
		t.Fatal("world affected:", err)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetModeScreen || len(m.resetFound) != 0 || !strings.Contains(m.View(), "Nothing to reset") {
		t.Fatal("empty inspection not reported")
	}
	m, cmd = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetModeScreen || cmd != nil {
		t.Fatal("empty reset must not be confirmable")
	}
	if err := os.Mkdir(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "old.jar"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, cmd = resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.state != resetRunningScreen || cmd == nil {
		t.Fatal("explicit purge confirmation failed")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.state != resetResultScreen || m.play || m.resetResult.Folders[0].Status != resetconfigs.Deleted || len(m.resetResult.Errors) != 0 {
		t.Fatalf("purge result incorrect: %+v", m)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods")); !os.IsNotExist(err) {
		t.Fatalf("mods not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "saves", "data")); err != nil {
		t.Fatal("world affected by purge:", err)
	}
}

func TestResetPartialResult(t *testing.T) {
	m := Model{state: resetRunningScreen, resetMode: resetconfigs.Purge, resetDir: "/minecraft", width: 110, height: 40}
	updated, _ := m.Update(ResetDoneMsg{Result: resetconfigs.Result{
		Folders: []resetconfigs.FolderResult{
			{Name: "mods", Path: "/minecraft/mods", Status: resetconfigs.PartiallyDeleted, Removed: []string{"old.jar"}, Err: os.ErrPermission},
			{Name: "config", Path: "/minecraft/config", Status: resetconfigs.Untouched},
		}, Errors: []error{os.ErrPermission},
	}})
	m = updated.(Model)
	for _, want := range []string{"Reset incomplete", "mods/: partially_deleted", "Removed: old.jar", "config/: untouched", "permission denied"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("result missing %q", want)
		}
	}
}
