package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFailedDownloadNeverStartsGame(t *testing.T) {
	m := Model{state: downloadScreen, width: 100, height: 30}
	ch := make(chan interface{}, 1)
	ch <- DownloadDoneMsg{Err: errors.New("HTTP 404")}
	close(ch)
	updated, cmd := m.Update(downloadChanMsg{ch: ch, msg: <-ch})
	m = updated.(Model)
	if m.play || cmd != nil || m.dlErr == nil || !strings.Contains(m.View(), "HTTP 404") {
		t.Fatalf("download failure was not shown safely: %+v", m)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).state != menuScreen {
		t.Fatal("cannot return to menu after a failed download")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m = Model{state: downloadScreen, dlContext: ctx}
	updated, cmd = m.Update(downloadChanMsg{msg: DownloadDoneMsg{}})
	m = updated.(Model)
	if m.play || cmd != nil || !errors.Is(m.dlErr, context.Canceled) {
		t.Fatal("late success launched after cancellation")
	}
}
