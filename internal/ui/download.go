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
type downloadStageMsg struct {
	Stage string
	Done  int
	Total int
}

// publishDownloadStage keeps the newest snapshot without blocking the worker.
func publishDownloadStage(events chan downloadStageMsg, stage string, done, total int) {
	if events == nil {
		return
	}
	next := downloadStageMsg{Stage: stage, Done: done, Total: total}
	select {
	case events <- next:
	default:
		select {
		case <-events:
		default:
		}
		select {
		case events <- next:
		default:
		}
	}
}

func prepareLaunch(ctx context.Context, launch LaunchConfig, events chan downloadStageMsg) tea.Msg {
	prepared, err := launcher.PrepareWithStageProgress(ctx, launch.RuntimeDir, launch.Version, launch.Modloader, launch.LoaderVersion,
		func(stage string, done, total int) { publishDownloadStage(events, stage, done, total) })
	if err != nil {
		return DownloadDoneMsg{Err: err}
	}
	publishDownloadStage(events, "java", 0, 0)
	javaBinary, err := launcher.ResolvePreparedJava(launch.RuntimeDir, prepared, launch.JavaPath)
	return DownloadDoneMsg{Prepared: prepared, JavaBinary: javaBinary, Err: err}
}

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
