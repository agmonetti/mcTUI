package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/launcher"
)

// StartDownloadCmd prepares the requested version using the same engine as
// Launch; the runtime and selection are fixed when the command starts.
func StartDownloadCmd(version, modloader string) tea.Cmd {
	return func() tea.Msg {
		ch := startDownload(version, modloader)
		return downloadChanMsg{ch: ch, msg: <-ch}
	}
}

type downloadChanMsg struct {
	ch  <-chan interface{}
	msg interface{}
}

func nextDownloadCmd(ch <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			msg = DownloadDoneMsg{Err: fmt.Errorf("preparation stopped without a result")}
		}
		return downloadChanMsg{ch: ch, msg: msg}
	}
}

func startDownload(version, modloader string) <-chan interface{} {
	ch := make(chan interface{}, 64)
	go func() {
		defer close(ch)
		runtimeDir, err := config.MinecraftDirChecked()
		if err != nil {
			ch <- DownloadDoneMsg{Err: err}
			return
		}
		done := 0
		_, err = launcher.Prepare(runtimeDir, version, modloader, func(file string) {
			done++
			ch <- DownloadProgressMsg{Done: done, File: file}
		})
		ch <- DownloadDoneMsg{Err: err}
	}()
	return ch
}
