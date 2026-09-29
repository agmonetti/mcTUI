package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/worlds"
)

func TestCopyWorldAcrossDifferentVersionsThroughUI(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { i18n.SetLanguage("en") })
	root := t.TempDir()
	source, err := instances.Create(root, "Old", "1.20.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := instances.Create(root, "New", "1.21.1", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	sourceDir, err := instances.GameDir(root, source)
	if err != nil {
		t.Fatal(err)
	}
	destDir, err := instances.GameDir(root, destination)
	if err != nil {
		t.Fatal(err)
	}
	worldDir := filepath.Join(sourceDir, "saves", "Home")
	if err := os.MkdirAll(filepath.Join(worldDir, "region"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "level.dat"), []byte("sample save"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "region", "r.0.0.mca"), []byte("chunks"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(destDir, "saves", "Home"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "saves", "Home", "keep"), []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}

	m := Model{state: worldsScreen, dataRoot: root, instance: source, worlds: []worlds.Info{{FolderName: "Home", LevelName: "My Home", Version: "1.20.1"}}}
	press := func(key tea.KeyMsg) tea.Cmd {
		t.Helper()
		next, cmd := m.Update(key)
		m = next.(Model)
		return cmd
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.state != worldCopyDestinationScreen || len(m.worldCopyTargets) != 2 {
		t.Fatalf("destination screen = %v, targets = %d", m.state, len(m.worldCopyTargets))
	}
	for i, in := range m.worldCopyTargets {
		if in.ID == destination.ID {
			m.worldCopyCursor = i
		}
	}
	press(tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != worldCopyConfirmScreen || !strings.Contains(m.View(), "1.20.1") || !strings.Contains(m.View(), "1.21.1") || !strings.Contains(m.View(), "can damage it") {
		t.Fatalf("missing version warning: %s", m.View())
	}
	cmd := press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.state != worldCopyRunningScreen || cmd == nil {
		t.Fatalf("copy did not start: %v", m.state)
	}
	if done := cmd(); done != nil {
		next, _ := m.Update(done)
		m = next.(Model)
	}
	if m.state != worldCopyResultScreen || m.worldCopyErr != nil {
		t.Fatalf("copy failed: %v: %v", m.state, m.worldCopyErr)
	}
	copied, err := os.ReadFile(filepath.Join(destDir, "saves", m.worldCopyFolder, "region", "r.0.0.mca"))
	if err != nil || string(copied) != "chunks" || m.worldCopyFolder == "Home" {
		t.Fatalf("collision copy: folder=%q, bytes=%q, err=%v", m.worldCopyFolder, copied, err)
	}
	original, err := os.ReadFile(filepath.Join(destDir, "saves", "Home", "keep"))
	if err != nil || string(original) != "untouched" {
		t.Fatalf("destination overwritten: %q, %v", original, err)
	}
	press(tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != worldsScreen {
		t.Fatalf("return state: %v", m.state)
	}
}
