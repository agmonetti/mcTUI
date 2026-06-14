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
)

func main() {
	validReleases := mojang.FetchReleases()
	cfg := config.Load()
	javaMajor := java.VersionAt("java")
	roadmapItems := roadmap.Load()

	p := tea.NewProgram(ui.New(validReleases, cfg, roadmapItems, javaMajor), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(ui.Model); ok && m.PlaySelected() {
		launcher.Launch(config.MinecraftDir(), m.Username(), m.VersionSelect(), m.Modloader(), m.MemoryMB())
	}
}
