package ui

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/mods"
	"mcTUI/internal/operationlock"
	"mcTUI/internal/packs"
)

func TestModsUIOfflineLifecycle(t *testing.T) {
	root := t.TempDir()
	in, err := instances.Create(root, "A", "1.20.4", "Fabric", "0.16.0", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{state: menuScreen, dataRoot: root, instance: in, width: 90, height: 24}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = updated.(Model)
	if cmd != nil || m.state != modrinthHubScreen {
		t.Fatal("Modrinth hub did not open")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil || m.state != modsWorkingScreen {
		t.Fatal("local mods did not open from Modrinth")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	source := filepath.Join(t.TempDir(), "local.jar")
	f, _ := os.Create(source)
	z := zip.NewWriter(f)
	entry, _ := z.Create("fabric.mod.json")
	entry.Write([]byte(`{"id":"test","version":"1","name":"Test"}`))
	z.Close()
	f.Close()
	updated, _ = m.updateModsKey("a")
	m = updated.(Model)
	if m.state != modsPickerScreen {
		t.Fatal("local picker did not open")
	}
	m.pickerPath = filepath.Dir(source)
	m.loadPickerDir()
	if len(m.pickerEntries) != 1 {
		t.Fatal("picker did not list JAR")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modErr != nil || len(m.modItems) != 1 || m.state != modsScreen {
		t.Fatalf("%+v", m.modErr)
	}
	if !strings.Contains(m.View(), "Enabled") || !strings.Contains(m.View(), "LOCAL MODS") {
		t.Fatal(m.View())
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("space did not toggle")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modErr != nil || m.modItems[0].Enabled {
		t.Fatal("not disabled", m.modErr)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = updated.(Model)
	if cmd != nil || m.state != modsConfirmScreen || !strings.Contains(m.View(), "local.jar.disabled") {
		t.Fatal("missing confirmation")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if cmd != nil || len(m.modItems) != 1 {
		t.Fatal("cancel removed file")
	}
	updated, _ = m.updateModsKey("d")
	m = updated.(Model)
	updated, cmd = m.updateModsKey("y")
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modErr != nil || len(m.modItems) != 0 {
		t.Fatal("remove failed", m.modErr)
	}
	release, err := operationlock.Acquire(m.modStore.GameDir)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	updated, cmd = m.updateModsKey("r")
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modBusy == nil || !strings.Contains(m.View(), "LOCKED") {
		t.Fatal("lock not displayed")
	}
	updated, cmd = m.updateModsKey("a")
	m = updated.(Model)
	if cmd != nil || m.state != modsScreen {
		t.Fatal("busy picker opened")
	}
}
func TestModsWorkerCapturesDestinationAndIgnoresStaleResults(t *testing.T) {
	root := t.TempDir()
	a, _ := instances.Create(root, "A", "1", "Vanilla", "", 2048)
	b, _ := instances.Create(root, "B", "1", "Vanilla", "", 2048)
	m := Model{state: menuScreen, dataRoot: root, instance: a}
	updated, cmd := m.openMods()
	m = updated.(Model)
	m.instance = b
	msg := cmd().(modsDoneMsg)
	if msg.Err != nil {
		t.Fatal(msg.Err)
	}
	dir, _ := instances.GameDir(root, b)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("worker touched B")
	}
	m.modRequest++
	updated, _ = m.Update(msg)
	if updated.(Model).state != modsWorkingScreen {
		t.Fatal("stale result accepted")
	}
	if strings.Contains(modText("evil\x1b[2J\nname"), "\x1b") {
		t.Fatal("controls not sanitized")
	}
}
func TestLegacyModsRegistryDoesNotBreakInstanceList(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "instances", "legacy")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "mods.json"), []byte(`{"schema_version":1,"mods":[]}`), 0600)
	list, err := instances.List(root)
	if err != nil || len(list) != 1 || list[0].ID != instances.LegacyID {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestModsViewWidthAndFooterConsistency(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })

	root := t.TempDir()
	in, err := instances.Create(root, "Instalación existente", "1.21", "Fabric", "0.16.0", 4096)
	if err != nil {
		t.Fatal(err)
	}

	mMenu := Model{
		state:         menuScreen,
		cursorMenu:    2,
		username:      "aguZzan",
		versionSelect: "1.21",
		modloader:     "Fabric",
		memoryMB:      4096,
		dataRoot:      root,
		instance:      in,
		width:         160,
		height:        40,
	}

	mMods := Model{
		state:         modsScreen,
		cursorMenu:    2,
		username:      "aguZzan",
		versionSelect: "1.21",
		modloader:     "Fabric",
		memoryMB:      4096,
		dataRoot:      root,
		instance:      in,
		width:         160,
		height:        40,
	}

	// 1. Center width must be identical between menu and mods
	if mMenu.centerWidth() != mMods.centerWidth() {
		t.Fatalf("centerWidth changed between menu (%d) and mods (%d)", mMenu.centerWidth(), mMods.centerWidth())
	}

	// 2. Rendered views must have the exact same width
	wMenu, _ := lipgloss.Size(mMenu.View())
	wMods, _ := lipgloss.Size(mMods.View())
	if wMenu != wMods {
		t.Fatalf("view width changed between menu (%d) and mods (%d)", wMenu, wMods)
	}

	// 3. Mods view must contain empty state message when 0 items
	modsView := mMods.View()
	if !strings.Contains(modsView, "No hay mods locales instalados") {
		t.Errorf("mods view missing empty state message:\n%s", modsView)
	}

	// 4. Footer must be formatted on multiple lines, not a single compact run-on line
	if !strings.Contains(modsView, "0 archivos · Canal: Estable") {
		t.Errorf("mods view missing status line:\n%s", modsView)
	}
	if !strings.Contains(modsView, "[Espacio] Activar") {
		t.Errorf("mods view missing local actions:\n%s", modsView)
	}

}

func TestContentViewsKeepFrameAndOneSetOfControls(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	m := Model{
		state: modsScreen, width: 120, height: 32,
		instance: instances.Instance{Name: "Test", MinecraftVersion: "1.21.1", Loader: "Fabric"},
		modStore: mods.Store{GameDir: "/tmp/instance/minecraft"},
	}
	for i := range 24 {
		m.catalogResults.Hits = append(m.catalogResults.Hits, modrinth.SearchResult{Title: fmt.Sprintf("Mod-%02d", i), Author: "Author"})
	}
	m.catalogResults.Total = 24
	m.catalogCursor = 23
	m.packResults = m.catalogResults
	m.packSearchCursor = 23
	m.mp.step = "home"
	var bottomRow int
	views := []struct {
		state screenState
		kind  packs.Kind
	}{
		{modsScreen, packs.Resource},
		{modrinthSearchScreen, packs.Resource},
		{packListScreen, packs.Resource},
		{packSearchScreen, packs.Resource},
		{packListScreen, packs.Shader},
		{packSearchScreen, packs.Shader},
		{modpackScreen, packs.Resource},
	}
	for i, selected := range views {
		state := selected.state
		m.packKind = selected.kind
		m.state = state
		view := m.View()
		if (state == modrinthSearchScreen || state == packSearchScreen) &&
			(!strings.Contains(view, "► Mod-23") || !strings.Contains(view, "[Esc] Back")) {
			t.Fatalf("search clipped the selected result or its controls:\n%s", view)
		}
		if width, height := lipgloss.Size(view); width > m.width || height > m.height {
			t.Fatalf("state %d exceeds terminal: %dx%d", state, width, height)
		}
		row := -1
		for n, line := range strings.Split(view, "\n") {
			if strings.LastIndex(line, "┘") > 60 {
				row = n
			}
		}
		if row < 0 {
			t.Fatalf("state %d lost content-panel border:\n%s", state, view)
		}
		if i == 0 {
			bottomRow = row
		} else if row != bottomRow {
			t.Fatalf("state %d moved content-panel bottom from row %d to %d", state, bottomRow, row)
		}
		wantHintCount := 0
		if state == modrinthSearchScreen || state == packSearchScreen {
			wantHintCount = 1
		}
		if count := strings.Count(view, strings.TrimSpace(m.controlsHint())); count != wantHintCount {
			t.Fatalf("state %d shows %d copies of its navigation hint, want %d:\n%s", state, count, wantHintCount, view)
		}
	}
}
