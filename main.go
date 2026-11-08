package main

import (
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
	// The updater has a 2s timeout — if GitHub doesn't respond in time,
	// latestRelease is zero-value and the UI shows nothing.
	updateCh := make(chan updater.Release, 1)
	go func() {
		release, _ := updater.Check(Version)
		updateCh <- release
	}()
	latestRelease := <-updateCh

	p := tea.NewProgram(
		ui.New(validReleases, cfg, latestRelease, Version),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("TUI Error: %v\n", err)
		os.Exit(1)
	}
}
