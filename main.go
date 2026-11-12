package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/mojang"
	"mcTUI/internal/ui"
	"mcTUI/internal/updater"
)

// Version is set at build time via -ldflags:
//
//	go build -ldflags="-X main.Version=v1.4.0" ...
//
// Defaults to "dev" for local builds, which disables update checks.
var Version = "dev"

func main() {
	validReleases := mojang.FetchReleases()
	cfg := config.Load()
	if err := i18n.Init(cfg.Language); err != nil {
		fmt.Fprintf(os.Stderr, "i18n initialization failed: %v\n", err)
		os.Exit(1)
	}

	// Check for updates concurrently so startup isn't delayed.
	// If GitHub does not respond, latestRelease stays zero-value and the UI
	// shows nothing. A rate limit with a failed gh fallback adds a quiet
	// notice instead of claiming the app is up to date.
	updateCh := make(chan updater.CheckResult, 1)
	go func() {
		updateCh <- updater.CheckStatus(Version)
	}()
	result := <-updateCh

	model := ui.New(validReleases, cfg, result.Release, Version)
	if errors.Is(result.Err, updater.ErrRateLimited) {
		model = model.WithUpdateCheckError(result.Err)
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("TUI Error: %v\n", err)
		os.Exit(1)
	}
}
