// Package ui implements the Bubble Tea model, update, and view for the
// mcTUI interface.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Paleta CS:GO estilo antiguo ──────────────────────────────────────────────
var (
	// Fondo y estructura
	colorBorder   = lipgloss.Color("#30363d")
	colorBorderHi = lipgloss.Color("#4a90a4") // azul acero highlight

	// Texto
	colorWhite   = lipgloss.Color("#e6edf3")
	colorGray    = lipgloss.Color("#8b949e")
	colorDimGray = lipgloss.Color("#3d4450")

	// Acento naranja/dorado CS:GO
	colorAccent  = lipgloss.Color("#c8a951") // dorado CS:GO
	colorAccent2 = lipgloss.Color("#e8b84b") // dorado más brillante

	// Status
	colorGreen = lipgloss.Color("#3fb950")
	colorRed   = lipgloss.Color("#f85149")
	colorBlue  = lipgloss.Color("#4a90a4") // azul acero

	// Tab
	colorTabActive = lipgloss.Color("#c8a951")
	colorTabInact  = lipgloss.Color("#8b949e")

	// ── Panels ──────────────────────────────────────────────────────────────
	panelMenu = lipgloss.NewStyle().
			Width(28).
			Height(16).
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelContent = lipgloss.NewStyle().
			Width(50).
			Height(16).
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelNews = lipgloss.NewStyle().
			Width(36).
			Height(16).
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	panelFooter = lipgloss.NewStyle().
			Height(3).
			Padding(1, 0)

	// ── Estilos de texto ────────────────────────────────────────────────────
	itemStyle   = lipgloss.NewStyle().Foreground(colorWhite)
	cursorStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	tabActive = lipgloss.NewStyle().
			Foreground(colorTabActive).
			Bold(true).
			Underline(true).
			Padding(0, 2)

	tabInactive = lipgloss.NewStyle().
			Foreground(colorTabInact).
			Padding(0, 2)

	labelStyle = lipgloss.NewStyle().Foreground(colorGray)
	valueStyle = lipgloss.NewStyle().Foreground(colorAccent2)
)

// ── Logo ──────────────────────────────────────────────────────────────────────
func getAsciiArt() string {
	top := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("  mcTUI  LAUNCHER")

	sub := lipgloss.NewStyle().
		Foreground(colorGray).
		Render("  Minecraft Terminal UI  ·  Offline-first")

	line := lipgloss.NewStyle().
		Foreground(colorDimGray).
		Render(strings.Repeat("─", 52))

	return top + "\n" + sub + "\n" + line
}

// ── Profile Card ──────────────────────────────────────────────────────────────
// Mini card estilo CS:GO: badge de rango + username + version activa.
func renderProfileCard(username, version, modloader string) string {
	rank := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("[OPERATOR]")

	user := lipgloss.NewStyle().
		Foreground(colorWhite).
		Bold(true).
		Render(username)

	verStr := version
	if modloader != "Vanilla" {
		verStr += " · " + modloader
	}
	ver := lipgloss.NewStyle().Foreground(colorGray).Render(verStr)

	divLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 24))

	card := rank + "\n" + user + "\n" + ver + "\n" + divLine
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Width(28).
		Padding(0, 1).
		Render(card)
}

// ── Featured Banner ───────────────────────────────────────────────────────────
// Franja horizontal que muestra la configuración activa, estilo CS:GO featured offer.
func renderFeaturedBanner(version, modloader string, memoryMB int, ready bool) string {
	statusStr := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("● READY")
	if !ready {
		statusStr = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("● NEEDS SETUP")
	}

	label := lipgloss.NewStyle().Foreground(colorGray).Render
	val := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render

	sep := lipgloss.NewStyle().Foreground(colorDimGray).Render("  │  ")

	content := label("CURRENT SETUP") + "    " +
		label("VER ") + val(version) +
		sep +
		label("MOD ") + val(modloader) +
		sep +
		label("RAM ") + val(fmt.Sprintf("%d MB", memoryMB)) +
		sep +
		statusStr

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorBorderHi).
		Width(118).
		Padding(0, 2).
		Render(content)
}

// ── Topbar tipo CS:GO ─────────────────────────────────────────────────────────
func renderTopBar(cursorMenu int, hasUpdate bool) string {
	tabs := []struct {
		label string
		idx   int
	}{
		{"PLAY", 0},
		{"SET NAME", 1},
		{"VERSION", 2},
		{"WORLDS", 3},
		{"MODLOADER", 4},
		{"MEMORY", 5},
		{"RESET", 6},
		{"QUIT", 7},
	}

	var parts []string
	for _, t := range tabs {
		label := t.label
		// Badge de update en el tab PLAY
		if t.idx == 0 && hasUpdate {
			label += lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(" [!]")
		}
		if cursorMenu == t.idx {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabInactive.Render(label))
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	divider := lipgloss.NewStyle().
		Foreground(colorBorder).
		Render(strings.Repeat("─", 120))

	return bar + "\n" + divider
}

// ── Progress Bar ASCII ────────────────────────────────────────────────────────
// Renderiza una barra de progreso estilo CS:GO: [■■■■■░░░░░] 52%
func renderProgressBar(current, total int, width int) string {
	if total == 0 {
		total = 1
	}
	pct := float64(current) / float64(total)
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}

	bar := lipgloss.NewStyle().Foreground(colorAccent).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("░", width-filled))

	pctStr := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).
		Render(fmt.Sprintf(" %3.0f%%", pct*100))

	return "[" + bar + "]" + pctStr
}
