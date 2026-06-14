// Package ui implements the Bubble Tea model, update, and view for the
// mcTUI interface.
package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorMagenta = lipgloss.Color("#FF2E93")
	colorCyan    = lipgloss.Color("#00E5FF")
	colorViolet  = lipgloss.Color("#BD00FF")
	colorGreen   = lipgloss.Color("#00FA9A")
	colorWhite   = lipgloss.Color("#ffffff")
	colorDark    = lipgloss.Color("#444444")
	colorGray    = lipgloss.Color("#888888")
	colorRed     = lipgloss.Color("#FF4444")

	panelMenu = lipgloss.NewStyle().
		Width(32).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorViolet).
		Padding(0, 1)

	panelContent = lipgloss.NewStyle().
		Width(48).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(0, 1)

	panelNews = lipgloss.NewStyle().
		Width(40).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMagenta).
		Padding(0, 1)

	panelFooter = lipgloss.NewStyle().
		Height(3).
		Padding(1, 0)

	itemStyle   = lipgloss.NewStyle().Foreground(colorWhite).PaddingLeft(1)
	cursorStyle = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
)

// getAsciiArt renders the mcTUI logo with a per-line color gradient.
func getAsciiArt() string {
	lines := []string{
		`  __  __      _______ _    _ _____ `,
		` |  \/  |    |__   __| |  | |_   _|`,
		` | \  / | ___   | |  | |  | | | |  `,
		` | |\/| |/ __|  | |  | |  | | | |  `,
		` | |  | | (__   | |  | |__| |_| |_ `,
		` |_|  |_|\___|  |_|   \____/|_____|`,
	}
	colors := []lipgloss.Color{colorCyan, colorCyan, lipgloss.Color("#00BFFF"), colorViolet, colorMagenta, colorMagenta}
	var styledLines []string
	for i, line := range lines {
		styledLines = append(styledLines, lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line))
	}
	return strings.Join(styledLines, "\n")
}
