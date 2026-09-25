package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	m := Model{state: menuScreen, cursorMenu: 6, height: 40, width: 160}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetModeScreen || m.resetDir != dir || m.resetMode != resetconfigs.Backup || len(m.resetFound) != 1 || !strings.Contains(m.View(), "[Enter] Continue") {
		t.Fatalf("inspection or mode prompt failed: %+v", m)
	}
	view := m.View()
	for _, want := range []string{"Minecraft Terminal UI", "CURRENT SETUP", "[OPERATOR]", "OPTIONS", "Resets Configs", "Minecraft directory:"} {
		if !strings.Contains(view, want) {
			t.Errorf("reset view lost original layout element %q", want)
		}
	}
	if strings.Contains(view, "FUTURE CHANGES") {
		t.Fatal("future changes panel should be omitted during reset")
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
	if m.state != resetPickerScreen {
		t.Fatal("backup should open directory picker")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
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

func TestBackupPickerSelectsExternalDirectory(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(home, "backups")
	for _, path := range []string{base, filepath.Join(home, "visible")} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	m := Model{state: menuScreen, cursorMenu: 6, width: 160, height: 40}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetPickerScreen || !strings.Contains(m.View(), "BACKUP DESTINATION") || !strings.Contains(m.View(), "OPTIONS") {
		t.Fatal("picker replaced the dashboard or did not open")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if len(m.pickerEntries) != 2 || strings.Contains(m.View(), "▸ .minecraft") {
		t.Fatal("hidden directory shown by default")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	if len(m.pickerEntries) != 3 || !strings.Contains(m.View(), "▸ .minecraft") {
		t.Fatal("Ctrl+H did not reveal hidden directory")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlH})
	if len(m.pickerEntries) != 2 {
		t.Fatal("Ctrl+H did not hide hidden directory again")
	}
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m = updated.(Model)
	if m.pickerCursor != 1 {
		t.Fatalf("wheel did not navigate picker: %d", m.pickerCursor)
	}
	for i, entry := range m.pickerEntries {
		if entry.Name() == "backups" {
			m.pickerCursor = i
			break
		}
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetConfirmScreen || m.resetBackupDir != base || !strings.Contains(m.View(), "Backup destination:") {
		t.Fatalf("selected backup destination missing: %q", m.View())
	}
	m, cmd := resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.resetErr != nil || len(m.resetResult.Errors) != 0 || filepath.Dir(m.resetResult.BackupPath) != base {
		t.Fatalf("backup did not use chosen directory: %+v %v", m.resetResult, m.resetErr)
	}
}

func TestRestoreFlowRequiresPerActionConfirmation(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "backup.jar"), []byte("backup"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := resetconfigs.Run(resetconfigs.Backup)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mods", "active.jar"), []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}

	m := Model{state: menuScreen, cursorMenu: 6, width: 160, height: 40}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetBackupListScreen || !strings.Contains(m.View(), filepath.Base(created.BackupPath)) {
		t.Fatalf("backup history not shown: %q", m.View())
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetReplaceConfirmScreen || !strings.Contains(m.View(), "permanently removes the current contents") {
		t.Fatalf("replacement was not confirmed: %q", m.View())
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.state != resetRetentionScreen {
		t.Fatal("replacement confirmation did not advance")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetRetentionScreen {
		t.Fatal("restore proceeded without an explicit backup-handling choice")
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetRestoreConfirmScreen || !strings.Contains(m.View(), "removed from history") {
		t.Fatalf("backup consumption not confirmed: %q", m.View())
	}
	m, cmd := resetKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.state != resetRunningScreen || cmd == nil {
		t.Fatal("restore did not wait for explicit final confirmation")
	}
	updated, _ := m.Update(cmd())
	m = updated.(Model)
	if m.state != resetResultScreen || !m.resetIsRestore || len(m.resetResult.Errors) != 0 {
		t.Fatalf("restore failed: %+v", m.resetResult)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "backup.jar")); err != nil {
		t.Fatal("backup contents were not restored:", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "active.jar")); !os.IsNotExist(err) {
		t.Fatal("confirmed old contents were not replaced:", err)
	}
	if _, err := os.Stat(created.BackupPath); !os.IsNotExist(err) {
		t.Fatal("confirmed consumed backup remains:", err)
	}
}

func TestResetModeScrollsBothDirectionsWithoutLooping(t *testing.T) {
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
	if err := os.MkdirAll(filepath.Join(dir, "mods"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{state: menuScreen, cursorMenu: 6, height: 26, width: 90}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != resetModeScreen {
		t.Fatal("not in resetModeScreen")
	}
	// Scroll down repeatedly
	for i := 0; i < 10; i++ {
		m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.resetChoice != 2 {
		t.Fatalf("choice should clamp at 2, got %d", m.resetChoice)
	}
	// The first action must stay visible when selected, even if its description is higher up.
	for m.resetChoice != 0 {
		m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	}
	if !strings.Contains(m.View(), "> Reset and create backup") {
		t.Fatal("selected first action is outside the viewport")
	}
	for i := 0; i < 100; i++ {
		m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	}
	if m.resetScroll != 0 {
		t.Fatalf("scroll should return to 0, got %d", m.resetScroll)
	}
	if !strings.Contains(m.View(), "INSTALLATION") {
		t.Fatal("top of screen should be visible after scrolling up")
	}
	for i := 0; i < 100; i++ {
		updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
		m = updated.(Model)
	}
	if m.resetScroll != m.resetMaxScroll() || m.resetChoice != 0 || m.resetMode != resetconfigs.Backup {
		t.Fatalf("wheel should scroll without changing action: scroll=%d choice=%d mode=%s", m.resetScroll, m.resetChoice, m.resetMode)
	}
	for i := 0; i < 100; i++ {
		updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
		m = updated.(Model)
	}
	if m.resetScroll != 0 || m.resetChoice != 0 {
		t.Fatalf("wheel up should reach the top without changing action: scroll=%d choice=%d", m.resetScroll, m.resetChoice)
	}
	m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	updated, _ := m.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	m = updated.(Model)
	if m.resetChoice != 1 || m.resetMode != resetconfigs.Purge {
		t.Fatal("wheel changed the selected reset mode")
	}
}

func TestResetResultShowsMoreLines(t *testing.T) {
	m := Model{state: resetResultScreen, width: 130, height: 40, resetDir: "/minecraft", resetMode: resetconfigs.Backup,
		resetResult: resetconfigs.Result{BackupPath: "/minecraft/" + strings.Repeat("nested-backup-folder/", 40) + "reset-1", Folders: []resetconfigs.FolderResult{
			{Name: "mods", Path: "/minecraft/mods", Status: resetconfigs.Moved},
			{Name: "config", Path: "/minecraft/config", Status: resetconfigs.Moved},
		}}}
	if !strings.Contains(m.View(), "more lines") || !strings.Contains(m.View(), "completed successfully") {
		t.Fatalf("result does not indicate success and hidden content: %q", m.View())
	}
	for m.resetScroll < m.resetMaxScroll() {
		m, _ = resetKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if !strings.Contains(m.View(), "Backup:") {
		t.Fatal("backup destination hidden after scrolling")
	}
	if strings.Contains(m.View(), "Worlds are preserved") || strings.Contains(m.View(), "Preserved: saves/") {
		t.Fatal("result repeats warnings already shown before confirmation")
	}
}

func TestResetKeepsDashboardAtNormalSize(t *testing.T) {
	for _, height := range []int{34, 35, 36, 40} {
		m := Model{state: resetModeScreen, cursorMenu: 6, width: 160, height: height,
			resetDir: "/minecraft", resetFound: []string{"mods", "config"}}
		view := m.View()
		for _, want := range []string{"Minecraft Terminal UI", "CURRENT SETUP", "[OPERATOR]", "OPTIONS", "RESETS CONFIGS", "AFFECTED FOLDERS", "[Enter] Continue"} {
			if !strings.Contains(view, want) {
				t.Errorf("reset replaced the dashboard at height %d: missing %q", height, want)
			}
		}
		if strings.Contains(view, "FUTURE CHANGES") {
			t.Fatal("future changes must be hidden during reset")
		}
		if width, usedHeight := lipgloss.Size(view); width > m.width || usedHeight > m.height {
			t.Fatalf("dashboard exceeds %dx%d terminal: %dx%d", m.width, m.height, width, usedHeight)
		}
	}
}

func TestResetScreensFitTerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, size := range [][2]int{{80, 24}, {90, 26}, {122, 24}, {160, 24}, {160, 32}, {160, 34}, {160, 35}, {160, 36}, {122, 40}, {160, 40}} {
		for _, state := range []screenState{resetModeScreen, resetConfirmScreen, resetPickerScreen, resetResultScreen} {
			m := Model{state: state, cursorMenu: 6, width: size[0], height: size[1],
				resetDir: "/minecraft", resetFound: []string{"mods", "config"}, resetMode: resetconfigs.Purge,
				pickerPath: "/minecraft", pickerEntries: nil}
			view := m.View()
			width, height := lipgloss.Size(view)
			if width > m.width || height > m.height {
				t.Errorf("state %d, terminal %dx%d: view measures %dx%d", state, m.width, m.height, width, height)
			}
			if state == resetConfirmScreen && !strings.Contains(view, "PERMANENT DELETION") {
				t.Errorf("deletion warning hidden at %dx%d", m.width, m.height)
			}
			if !strings.Contains(view, "mcTUI") || !strings.Contains(view, "OPTIONS") {
				t.Errorf("dashboard context missing at %dx%d", m.width, m.height)
			}
		}
	}
}

func TestResetConfirmationFooterAndWrapping(t *testing.T) {
	m := Model{state: resetReplaceConfirmScreen, width: 160, height: 40,
		resetConflicts: []string{"mods"}}
	view := m.resetPanelContent()
	if strings.Count(view, "[y] Confirm replacement") != 1 || strings.Contains(view, "[Esc] Back") {
		t.Fatal("confirmation should have one consistent action footer")
	}
	lines := wrapResetLines([]string{"Worlds are preserved, but do not load mod-dependent worlds"}, 25)
	if strings.Join(lines, " ") != "Worlds are preserved, but do not load mod-dependent worlds" {
		t.Fatalf("warning was split inside a word: %q", lines)
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
