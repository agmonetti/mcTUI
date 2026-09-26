package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/resetconfigs"
)

func TestMainMenuOptionsAndRendering(t *testing.T) {
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
		state:         menuScreen,
		cursorMenu:    0,
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

	// Main menu must contain the 5 main options in English
	expectedMain := []string{
		"Play (1.20.4)",
		"Worlds",
		"Mods",
		"Change Instances",
		"Configs",
		"Quit",
	}
	for _, opt := range expectedMain {
		if !strings.Contains(view, opt) {
			t.Errorf("main menu view missing expected option %q:\n%s", opt, view)
		}
	}

	// Main menu must NOT contain config sub-options directly
	subOptions := []string{
		"Change Name",
		"Resets Configs",
	}
	for _, sub := range subOptions {
		if strings.Contains(view, "► "+sub) || strings.Contains(view, "  "+sub) {
			t.Errorf("main menu view should not show config sub-option %q:\n%s", sub, view)
		}
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

	// Down arrow moves cursor down to menuOptionsCount-1 (5)
	for expected := 1; expected <= 5; expected++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
		if m.cursorMenu != expected {
			t.Fatalf("step %d: expected cursorMenu %d, got %d", expected, expected, m.cursorMenu)
		}
	}

	// Down when at cursorMenu 5 should stay at 5
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursorMenu != 5 {
		t.Fatalf("expected cursorMenu to stay at 5, got %d", m.cursorMenu)
	}

	// Enter on Quit (cursor 5) quits
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit cmd when pressing Enter on Quit option")
	}

	// Test transitions for options 0, 1, 2, 3:
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

	// 1: Worlds -> worldsScreen
	m.cursorMenu = 1
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

	// 2: Mods -> triggers openMods (modsWorkingScreen)
	m.cursorMenu = 2
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != modsWorkingScreen {
		t.Fatalf("expected modsWorkingScreen for Mods enter, got %v", m.state)
	}
	m.state = menuScreen

	// 3: Change Instances -> instancesScreen
	m.cursorMenu = 3
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

	// 4: Configs -> configScreen
	m.cursorMenu = 4
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != configScreen || m.cursorConfig != 0 {
		t.Fatalf("expected configScreen with cursorConfig 0, got state=%v, cursorConfig=%d", m.state, m.cursorConfig)
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
		cursorMenu:    4,
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

	// Config screen must contain all 7 config options
	expectedConfigs := []string{
		"Change Name",
		"Minecraft: 1.20.4",
		"Modloader: Vanilla",
		"Memory: 2048 MB",
		"Resets Configs",
		"Java: Automatic",
		"Language: English",
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

	// Down navigates through all 7 options (indices 0..6)
	for expected := 1; expected <= 6; expected++ {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
		if m.cursorConfig != expected {
			t.Fatalf("step %d: expected cursorConfig %d, got %d", expected, expected, m.cursorConfig)
		}
	}

	// Down at 6 stays at 6
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.cursorConfig != 6 {
		t.Fatalf("expected cursorConfig to stay at 6, got %d", m.cursorConfig)
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
	if m.state != menuScreen || m.cursorMenu != 4 {
		t.Fatalf("expected menuScreen with cursorMenu 4 after Esc, got state=%v cursorMenu=%d", m.state, m.cursorMenu)
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
		cursorMenu:    4,
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

	// 0: Change Name -> nameScreen, Esc returns to configScreen
	m.cursorConfig = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != nameScreen {
		t.Fatalf("expected nameScreen, got %v", m.state)
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
}
