package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/resetconfigs"
	"mcTUI/internal/theme"
)

func TestVerifyInstallationDetailsUseReadableLines(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })
	root := t.TempDir()
	in, err := instances.Create(root, "TestInstance", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		state:         confirmScreen,
		username:      "Steve",
		versionSelect: "1.20.4",
		modloader:     "Vanilla",
		memoryMB:      2048,
		dataRoot:      root,
		instance:      in,
		width:         120,
		height:        35,
	}
	view := m.View()
	for _, want := range []string{"VERIFY INSTALLATION", "Instance: TestInstance", "Game data:", "Shared runtime:", "Java:", "[y/Enter] YES"} {
		if !strings.Contains(view, want) {
			t.Errorf("verification screen missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, `\n`) {
		t.Fatalf("verification screen rendered escaped newlines literally:\n%s", view)
	}
}

func TestMainMenuNavigationAndTransitions(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })

	root := t.TempDir()
	in, err := instances.Create(root, "MainTest", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}

	m := Model{
		state:         menuScreen,
		cursorMenu:    0,
		username:      "Alex",
		versionSelect: "1.20.4",
		modloader:     "Vanilla",
		memoryMB:      2048,
		dataRoot:      root,
		instance:      in,
		width:         120,
		height:        35,
	}

	// Up when cursorMenu is 0 should remain at 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursorMenu != 0 {
		t.Fatalf("expected cursorMenu 0, got %d", m.cursorMenu)
	}

	// Down arrow moves cursor down to menuOptionsCount-1 (7)
	for expected := 1; expected <= 7; expected++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
		if m.cursorMenu != expected {
			t.Fatalf("step %d: expected cursorMenu %d, got %d", expected, expected, m.cursorMenu)
		}
	}

	// Down when at cursorMenu 7 should stay at 7
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursorMenu != 7 {
		t.Fatalf("expected cursorMenu to stay at 7, got %d", m.cursorMenu)
	}

	// Enter on Quit (cursor 7) quits
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit cmd when pressing Enter on Quit option")
	}

	// Test transitions for options:
	// 0: Play -> confirmScreen
	m.cursorMenu = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != confirmScreen {
		t.Fatalf("expected confirmScreen for Play, got %v", m.state)
	}

	// Esc returns to menuScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected menuScreen after Esc, got %v", m.state)
	}

	// 1: Accounts -> accountsScreen
	m.cursorMenu = 1
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected accountsScreen for Accounts, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected menuScreen after Esc, got %v", m.state)
	}

	// 2: Worlds -> worldsScreen
	m.cursorMenu = 2
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != worldsScreen {
		t.Fatalf("expected worldsScreen for Worlds, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected menuScreen after Esc, got %v", m.state)
	}

	// 3: Modrinth opens a category hub; mods remain reachable from it.
	m.cursorMenu = 3
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != modrinthHubScreen {
		t.Fatalf("expected Modrinth hub, got %v", m.state)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != modsWorkingScreen || cmd == nil {
		t.Fatalf("expected local mods from the hub, got state %v", m.state)
	}
	m.state = menuScreen

	// 4: Change Instances -> instancesScreen
	m.cursorMenu = 4
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != instancesScreen {
		t.Fatalf("expected instancesScreen, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected menuScreen after Esc, got %v", m.state)
	}

	// 5: Configs -> configScreen
	m.cursorMenu = 5
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != configScreen || m.cursorConfig != 0 {
		t.Fatalf("expected configScreen with cursorConfig 0, got state=%v, cursorConfig=%d", m.state, m.cursorConfig)
	}

	// 6: Roadmap -> roadmapScreen
	m.state = menuScreen
	m.cursorMenu = 6
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != roadmapScreen {
		t.Fatalf("expected roadmapScreen, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected menuScreen after Esc from roadmap, got %v", m.state)
	}
}

func TestConfigSubmenuOptionsAndNavigation(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })

	root := t.TempDir()
	in, err := instances.Create(root, "ConfigTest", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}

	m := Model{
		state:         configScreen,
		cursorMenu:    5,
		cursorConfig:  0,
		username:      "Steve",
		versionSelect: "1.20.4",
		modloader:     "Vanilla",
		memoryMB:      2048,
		dataRoot:      root,
		instance:      in,
		width:         120,
		height:        35,
	}

	view := m.View()

	// Header in left panel must show CONFIGS
	if !strings.Contains(view, "CONFIGS") && !strings.Contains(view, "Configs") {
		t.Errorf("configScreen should show CONFIGS header:\n%s", view)
	}

	// Config screen must contain all 9 config options
	expectedConfigs := []string{
		"Accounts",
		"Minecraft: 1.20.4",
		"Modloader: Vanilla",
		"Memory: 2048 MB",
		"Resets Configs",
		"Java: Automatic",
		"Language: English",
		"Theme: mcTUI (Default)",
		"Up to date",
	}
	for _, opt := range expectedConfigs {
		if !strings.Contains(view, opt) {
			t.Errorf("configScreen missing expected option %q:\n%s", opt, view)
		}
	}

	// Test cursorConfig navigation: up at 0 stays at 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.cursorConfig != 0 {
		t.Fatalf("expected cursorConfig 0, got %d", m.cursorConfig)
	}

	// Down navigates through all 9 options (indices 0..8)
	for expected := 1; expected <= 8; expected++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
		if m.cursorConfig != expected {
			t.Fatalf("step %d: expected cursorConfig %d, got %d", expected, expected, m.cursorConfig)
		}
	}

	// Down at 8 stays at 8
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursorConfig != 8 {
		t.Fatalf("expected cursorConfig to stay at 8, got %d", m.cursorConfig)
	}

	// Memory panel rendered when cursorConfig == 3
	m.cursorConfig = 3
	memView := m.View()
	if !strings.Contains(memView, "MEMORY CONFIGURATION") || !strings.Contains(memView, "RECOMMENDED RAM") {
		t.Fatalf("expected memory configuration panel when cursorConfig is 3:\n%s", memView)
	}

	// Version option (cursorConfig == 1) must NOT render worlds hint
	m.cursorConfig = 1
	verView := m.View()
	if strings.Contains(verView, "LOCAL WORLDS") {
		t.Fatalf("version option in configScreen should NOT render worlds hint:\n%s", verView)
	}

	// Esc returns to menuScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen || m.cursorMenu != 5 {
		t.Fatalf("expected menuScreen with cursorMenu 5 after Esc, got state=%v cursorMenu=%d", m.state, m.cursorMenu)
	}

	// 'q' on configScreen also returns to menuScreen
	m.state = configScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected 'q' in configScreen to return to menuScreen, got %v", m.state)
	}
}

func TestConfigSubmenuActions(t *testing.T) {
	if err := i18n.Init("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })

	root := t.TempDir()
	in, err := instances.Create(root, "ActionTest", "1.20.4", "Vanilla", "", 2048)
	if err != nil {
		t.Fatal(err)
	}

	m := Model{
		state:         configScreen,
		cursorMenu:    5,
		cursorConfig:  0,
		username:      "Player",
		versionSelect: "1.20.4",
		modloader:     "Vanilla",
		memoryMB:      2048,
		dataRoot:      root,
		instance:      in,
		width:         120,
		height:        35,
	}

	// 0: Accounts -> accountsScreen, Esc returns to configScreen
	m.cursorConfig = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected accountsScreen, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != configScreen {
		t.Fatalf("expected return to configScreen, got %v", m.state)
	}

	// 3: Memory -> cycles memory
	m.cursorConfig = 3
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != configScreen || m.memoryMB != 4096 {
		t.Fatalf("expected memoryMB 4096 and configScreen, got mem=%d state=%v", m.memoryMB, m.state)
	}

	// 4: Resets Configs -> enters resetModeScreen, Esc returns to configScreen
	m.cursorConfig = 4
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != resetModeScreen || m.resetMode != resetconfigs.Backup {
		t.Fatalf("expected resetModeScreen, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != configScreen {
		t.Fatalf("expected return to configScreen from resetModeScreen Esc, got %v", m.state)
	}

	// 5: Java -> openJavaPicker, Esc returns to configScreen
	m.cursorConfig = 5
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != javaPickerScreen {
		t.Fatalf("expected javaPickerScreen, got %v", m.state)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != configScreen {
		t.Fatalf("expected return to configScreen from javaPickerScreen Esc, got %v", m.state)
	}

	// 6: Language -> cycles language
	m.cursorConfig = 6
	origLang := i18n.CurrentLanguage()
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != configScreen || i18n.CurrentLanguage() == origLang {
		t.Fatalf("expected language changed and stay in configScreen, got lang=%s state=%v", i18n.CurrentLanguage(), m.state)
	}
	// Restore language
	i18n.SetLanguage(origLang)
	cfg := config.Load()
	cfg.Language = origLang
	config.Save(cfg)

	// 7: Theme -> cycles theme
	m.cursorConfig = 7
	origTheme := CurrentTheme().ID
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != configScreen || CurrentTheme().ID == origTheme {
		t.Fatalf("expected theme changed from %q, got %q", origTheme, CurrentTheme().ID)
	}
	// Restore theme to mctui
	cfg = config.Load()
	cfg.Theme = "mctui"
	config.Save(cfg)
	ApplyTheme(theme.Default())
}

func TestInitialViewLayoutWidth(t *testing.T) {
	if err := i18n.Init("es"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.Init("en") })

	root := t.TempDir()
	in, err := instances.Create(root, "Instalación existente", "1.21", "Fabric", "0.16.0", 4096)
	if err != nil {
		t.Fatal(err)
	}

	m := Model{
		state:         menuScreen,
		cursorMenu:    0,
		username:      "aguZzan",
		versionSelect: "1.21",
		modloader:     "Fabric",
		memoryMB:      4096,
		dataRoot:      root,
		instance:      in,
		width:         160,
		height:        40,
	}

	view := m.View()

	if !strings.Contains(view, "AJUSTES ACTUALES Instalación existente") {
		t.Errorf("banner missing the active instance name next to its label:\n%s", view)
	}

	// 1. Banner must have full "FALTAN ARCHIVOS" on a single line and not break into "ARCHI"
	if strings.Contains(view, "\nARCHI") || strings.Contains(view, "\n ARCHI") {
		t.Errorf("banner broke ARCHIVOS into newline ARCHI:\n%s", view)
	}
	if !strings.Contains(view, "FALTAN ARCHIVOS") {
		t.Errorf("banner missing FALTAN ARCHIVOS:\n%s", view)
	}

	// 2. Center panel must have "Faltan archivos – podría necesitarse conexión" without breaking "conexión" onto a separate line
	if strings.Contains(view, "\nconexión") || strings.Contains(view, "\n conexión") {
		t.Errorf("center panel broke conexión into newline:\n%s", view)
	}

	// 3. View should fit within the terminal boundaries
	w, h := lipgloss.Size(view)
	if w > m.width || h > m.height {
		t.Errorf("view size %dx%d exceeds terminal size %dx%d", w, h, m.width, m.height)
	}

	// 4. On wide terminal (160), total dashboard width must be spacious (> 100 cols)
	if w < 100 {
		t.Errorf("expected spacious layout on wide terminal (w >= 100), got width %d", w)
	}

	// 5. Switching to Roadmap cursor must not change total view height
	m.cursorMenu = 6
	roadmapPreviewView := m.View()
	_, rh := lipgloss.Size(roadmapPreviewView)
	if rh != h {
		t.Errorf("roadmap preview altered view height from %d to %d", h, rh)
	}

	m.state = roadmapScreen
	roadmapScreenView := m.View()
	_, sh := lipgloss.Size(roadmapScreenView)
	if sh != h {
		t.Errorf("roadmap screen altered view height from %d to %d", h, sh)
	}
}
