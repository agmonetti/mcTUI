package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/i18n"
	"mcTUI/internal/launcher"
)

type downloadAnimationTickMsg struct{}

func downloadAnimationTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return downloadAnimationTickMsg{} })
}

// StartDownloadCmd prepares the requested version asynchronously using the shared launcher.
func StartDownloadCmd(ctx context.Context, runtimeDir, version, modloader, javaPath string) tea.Cmd {
	return func() tea.Msg {
		if runtimeDir == "" {
			return DownloadDoneMsg{Err: fmt.Errorf("%s", i18n.T("error.runtime_directory_empty"))}
		}
		prepared, err := launcher.PrepareContext(ctx, runtimeDir, version, modloader, nil)
		var javaBinary string
		if err == nil {
			javaBinary, err = launcher.ResolvePreparedJava(runtimeDir, prepared, javaPath)
		}
		return DownloadDoneMsg{Prepared: prepared, JavaBinary: javaBinary, Err: err}
	}
}
