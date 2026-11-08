package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/auth"
)

func TestAccountsScreenNavigationAndSwitch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	storage := &auth.Storage{
		ActiveAccountID: "acc-1",
		Accounts: []auth.Account{
			{
				ID:        "acc-1",
				Username:  "PlayerOffline",
				Type:      auth.AccountTypeOffline,
				CreatedAt: time.Now(),
			},
			{
				ID:        "acc-2",
				Username:  "MicrosoftUser",
				Type:      auth.AccountTypeMicrosoft,
				CreatedAt: time.Now(),
			},
		},
	}

	ti := textinput.New()
	ti.Focus()

	m := Model{
		state:          accountsScreen,
		authStorage:    storage,
		username:       "PlayerOffline",
		accountsCursor: 0,
		input:          ti,
		width:          120,
		height:         35,
	}

	view := m.View()
	if !strings.Contains(view, "PlayerOffline") || !strings.Contains(view, "MicrosoftUser") {
		t.Fatalf("accounts screen missing accounts in view: %s", view)
	}
	if !strings.Contains(view, "[Microsoft]") || !strings.Contains(view, "[Offline]") {
		t.Fatalf("accounts screen missing account tags in view: %s", view)
	}

	// Move cursor down to second account
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.accountsCursor != 1 {
		t.Fatalf("expected accountsCursor 1, got %d", m.accountsCursor)
	}

	// Press Enter to activate second account
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.authStorage.ActiveAccountID != "acc-2" {
		t.Fatalf("expected active account acc-2, got %s", m.authStorage.ActiveAccountID)
	}
	if m.username != "MicrosoftUser" {
		t.Fatalf("expected username MicrosoftUser, got %s", m.username)
	}

	// Press 'o' to trigger offline name screen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = updated.(Model)
	if m.state != nameScreen {
		t.Fatalf("expected state nameScreen, got %v", m.state)
	}

	// Press 'esc' to return to accountsScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected state accountsScreen after Esc, got %v", m.state)
	}

	// Press 'esc' to return to menuScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != menuScreen || m.cursorMenu != 1 {
		t.Fatalf("expected state menuScreen with cursorMenu 1 after Esc, got state=%v cursorMenu=%d", m.state, m.cursorMenu)
	}
}

func TestDeviceCodeScreenRendering(t *testing.T) {
	resp := &auth.DeviceCodeResponse{
		DeviceCode:      "dev-123",
		UserCode:        "ABCD-WXYZ",
		VerificationURI: "https://microsoft.com/link",
		ExpiresIn:       900,
		Interval:        5,
	}

	m := Model{
		state:               deviceCodeScreen,
		deviceCodeResp:      resp,
		deviceCodeExpiresIn: 890,
		deviceCodeTick:      1,
		width:               120,
		height:              35,
	}

	view := m.View()
	if !strings.Contains(view, "ABCD-WXYZ") {
		t.Fatalf("device code screen missing user code: %s", view)
	}
	if !strings.Contains(view, "https://microsoft.com/link") {
		t.Fatalf("device code screen missing verification URI: %s", view)
	}

	// Press 'c' to copy code
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(Model)
	// Even if system clipboard fails in headless CI, key is handled cleanly

	// Press 'esc' to cancel and return to accountsScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.state != accountsScreen {
		t.Fatalf("expected cancel to return to accountsScreen, got %v", m.state)
	}
}
