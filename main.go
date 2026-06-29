package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/java"
	"mcTUI/internal/launcher"
	"mcTUI/internal/mojang"
	"mcTUI/internal/roadmap"
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
	javaMajor := java.VersionAt("java")
	roadmapItems := roadmap.Load()

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
		ui.New(validReleases, cfg, roadmapItems, javaMajor, latestRelease),
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(ui.Model); ok && m.PlaySelected() {
		launcher.Launch(config.MinecraftDir(), m.Username(), m.VersionSelect(), m.Modloader(), m.MemoryMB())
	}
}
