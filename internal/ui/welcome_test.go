package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/auth"
	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/updater"
)

func TestWelcomeScreenFirstRunFlow(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)
	_ = i18n.Init("en")

	// Fresh installation: empty config, no accounts
	cfg := config.Data{}
	m := New([]string{"1.20.4"}, cfg, updater.Release{})

	if m.state != welcomeScreen {
		t.Fatalf("expected fresh binary to start on welcomeScreen, got %v", m.state)
	}

	view := m.View()
	if !strings.Contains(view, "WELCOME TO mcTUI") {
		t.Fatalf("missing welcome headline in view: %s", view)
	}
	if !strings.Contains(view, "No accounts configured") {
		t.Fatalf("missing no accounts notice in view: %s", view)
	}
	if !strings.Contains(view, "0 managed instances") {
		t.Fatalf("missing no instances notice in view: %s", view)
	}

	// Pressing Enter transitions to accountsScreen
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected Enter on welcomeScreen to open accountsScreen, got %v", m.state)
	}

	// Esc returns to menuScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected Esc on accountsScreen to return to menuScreen, got %v", m.state)
	}

	// In menuScreen, profile card shows no account
	menuView := m.View()
	if !strings.Contains(menuView, "No account") {
		t.Fatalf("expected profile card to display 'No account', got: %s", menuView)
	}

	// Trying to launch (Play, cursor 0) without an account redirects back to accountsScreen
	m.cursorMenu = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected Play without account to redirect to accountsScreen, got %v", m.state)
	}
}

func TestWelcomeScreenSkipToMenu(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = i18n.Init("en")

	m := Model{
		state:       welcomeScreen,
		authStorage: &auth.Storage{},
		width:       100,
		height:      30,
	}

	// Pressing Esc directly from welcomeScreen skips to menuScreen
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen {
		t.Fatalf("expected Esc on welcomeScreen to skip to menuScreen, got %v", m.state)
	}
}

func TestWelcomeScreenLanguageToggle(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)
	_ = i18n.Init("en")

	m := Model{
		state:       welcomeScreen,
		authStorage: &auth.Storage{},
		width:       100,
		height:      30,
	}

	view := m.View()
	if !strings.Contains(view, "WELCOME TO mcTUI") {
		t.Fatalf("expected English welcome text, got %s", view)
	}

	// Pressing L switches to Spanish
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updated.(Model)
	esView := m.View()
	if !strings.Contains(esView, "BIENVENIDO A mcTUI") {
		t.Fatalf("expected Spanish welcome text after toggle, got %s", esView)
	}

	// Pressing L again switches back to English
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	m = updated.(Model)
	enView := m.View()
	if !strings.Contains(enView, "WELCOME TO mcTUI") {
		t.Fatalf("expected English welcome text after second toggle, got %s", enView)
	}
}
