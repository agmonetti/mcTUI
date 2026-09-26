package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/launcher"
)

func TestDownloadProgressIsIndeterminateBarWithoutFileName(t *testing.T) {
	m := Model{state: downloadScreen}
	before := m.View()
	if strings.Contains(before, "DONE") || strings.Contains(before, "library.jar") || !strings.Contains(before, "Preparing game files") {
		t.Fatalf("download view includes redundant detail or lacks status: %s", before)
	}
	updated, tick := m.Update(downloadAnimationTickMsg{})
	m = updated.(Model)
	if tick == nil || m.dlProgressFrame != 1 || m.View() == before {
		t.Fatal("horizontal progress bar did not advance")
	}
}

func TestPreparedGameReturnsToTUI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell Java stub is Unix-only")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			fmt.Fprint(w, `{"objects":{}}`)
		} else {
			fmt.Fprint(w, "client")
		}
	}))
	defer server.Close()
	game, runtimeDir := t.TempDir(), t.TempDir()
	versionDir := filepath.Join(runtimeDir, "versions", "1.20.4")
	if err := os.MkdirAll(versionDir, 0700); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"assetIndex":{"id":"test","url":%q,"sha1":"62ea787c1f800c091b98678b050453a5ae59d7bc"},"downloads":{"client":{"url":%q,"sha1":"d2a04d71301a8915217dd5faf81d12cffd6cd958"}}}`,
		server.URL+"/index", server.URL+"/client") // Mojang SHA-1 fixture
	if err := os.WriteFile(filepath.Join(versionDir, "version.json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := launcher.Prepare(runtimeDir, "1.20.4", "Vanilla", nil)
	if err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(java, []byte("#!/bin/sh\necho launched-from-tui\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{state: downloadScreen, launch: LaunchConfig{GameDir: game, RuntimeDir: runtimeDir, Username: "Player", Version: "1.20.4", MemoryMB: 1024}}
	updated, start := m.Update(DownloadDoneMsg{Prepared: prepared, JavaBinary: java})
	m = updated.(Model)
	if m.state != gameStartingScreen || start == nil {
		t.Fatal("prepared game did not start from TUI")
	}
	updated, wait := m.Update(start())
	m = updated.(Model)
	if m.state != gameRunningScreen || wait == nil {
		t.Fatalf("Java not supervised by TUI: state=%v err=%v", m.state, m.gameErr)
	}
	updated, _ = m.Update(wait())
	m = updated.(Model)
	if m.state != gameResultScreen || m.gameErr != nil || m.gameExitCode != 0 || !strings.Contains(m.View(), "exited") {
		t.Fatalf("game result not shown: state=%v err=%v code=%d", m.state, m.gameErr, m.gameExitCode)
	}
	if data, err := os.ReadFile(m.gameLog); err != nil || !strings.Contains(string(data), "launched-from-tui") {
		t.Fatalf("game log: %q, %v", data, err)
	}
}

func TestGameLifecycleKeepsTUIOpen(t *testing.T) {
	m := Model{state: downloadScreen, launch: LaunchConfig{GameDir: t.TempDir(), RuntimeDir: t.TempDir()}, width: 100, height: 30}
	updated, start := m.Update(DownloadDoneMsg{})
	m = updated.(Model)
	if m.state != gameStartingScreen || start == nil {
		t.Fatal("successful preparation did not hand off to the TUI launch flow")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if updated.(Model).state != gameStartingScreen {
		t.Fatal("quit detached Java while starting")
	}
	updated, _ = m.Update(start()) // incomplete fixture must fail before starting a process
	m = updated.(Model)
	if m.state != gameResultScreen || m.gameErr == nil || !strings.Contains(m.View(), "could not start") {
		t.Fatalf("start failure was not shown: state=%v err=%v", m.state, m.gameErr)
	}
	m.state = gameRunningScreen
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if updated.(Model).state != gameRunningScreen {
		t.Fatal("ctrl+c detached a running game")
	}
	updated, _ = m.Update(GameExitedMsg{Code: 7, Err: errors.New("exit status 7")})
	m = updated.(Model)
	if m.state != gameResultScreen || m.gameExitCode != 7 || m.gameErr == nil || !strings.Contains(m.gameErr.Error(), "exit status 7") {
		t.Fatalf("process result not visible: state=%v code=%d", m.state, m.gameExitCode)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != menuScreen {
		t.Fatal("cannot return to menu after game exits")
	}
}

func TestFailedDownloadNeverStartsGame(t *testing.T) {
	m := Model{state: downloadScreen, width: 100, height: 30}
	updated, cmd := m.Update(DownloadDoneMsg{Err: errors.New("HTTP 404")})
	m = updated.(Model)
	if cmd != nil || m.dlErr == nil || !strings.Contains(m.View(), "HTTP 404") {
		t.Fatalf("download failure was not shown safely: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).state != menuScreen {
		t.Fatal("cannot return to menu after a failed download")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m = Model{state: downloadScreen, dlContext: ctx}
	updated, cmd = m.Update(DownloadDoneMsg{})
	m = updated.(Model)
	if cmd != nil || !errors.Is(m.dlErr, context.Canceled) {
		t.Fatal("late success launched after cancellation")
	}
}

func TestGameRunningForcedCloseFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell Java stub is Unix-only")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			fmt.Fprint(w, `{"objects":{}}`)
		} else {
			fmt.Fprint(w, "client")
		}
	}))
	defer server.Close()
	game, runtimeDir := t.TempDir(), t.TempDir()
	versionDir := filepath.Join(runtimeDir, "versions", "1.20.4")
	if err := os.MkdirAll(versionDir, 0700); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf(`{"assetIndex":{"id":"test","url":%q,"sha1":"62ea787c1f800c091b98678b050453a5ae59d7bc"},"downloads":{"client":{"url":%q,"sha1":"d2a04d71301a8915217dd5faf81d12cffd6cd958"}}}`,
		server.URL+"/index", server.URL+"/client")
	if err := os.WriteFile(filepath.Join(versionDir, "version.json"), []byte(meta), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := launcher.Prepare(runtimeDir, "1.20.4", "Vanilla", nil)
	if err != nil {
		t.Fatal(err)
	}
	java := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(java, []byte("#!/bin/sh\nsleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	m := Model{state: downloadScreen, launch: LaunchConfig{GameDir: game, RuntimeDir: runtimeDir, Username: "Player", Version: "1.20.4", MemoryMB: 1024}}
	updated, start := m.Update(DownloadDoneMsg{Prepared: prepared, JavaBinary: java})
	m = updated.(Model)
	updated, wait := m.Update(start())
	m = updated.(Model)
	if m.state != gameRunningScreen || wait == nil || m.activeGame == nil {
		t.Fatalf("game not running: state=%v activeGame=%v", m.state, m.activeGame)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	if !m.gameKillConfirm || !strings.Contains(m.View(), "Force close Java?") {
		t.Fatalf("kill confirmation prompt not shown: confirm=%v view=%s", m.gameKillConfirm, m.View())
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)
	if m.gameKillConfirm || strings.Contains(m.View(), "Force close Java?") {
		t.Fatal("confirmation was not canceled by 'n'")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.gameKillConfirm {
		t.Fatal("confirmation was not canceled by 'esc'")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = updated.(Model)
	if m.gameKillConfirm {
		t.Fatal("confirmation flag not cleared after confirming")
	}
	msg := wait()
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.state != gameResultScreen {
		t.Fatalf("killed game did not transition to result screen: state=%v", m.state)
	}
}
