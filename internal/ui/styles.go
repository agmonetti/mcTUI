// Package ui implements the Bubble Tea model, update, and view for the
// mcTUI interface.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
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
func renderFeaturedBanner(version, modloader string, memoryMB int, cached bool, width int) string {
	statusStr := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("status.files_present_short"))
	if !cached {
		statusStr = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("status.files_missing"))
	}

	label := lipgloss.NewStyle().Foreground(colorGray).Render
	val := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render

	sep := lipgloss.NewStyle().Foreground(colorDimGray).Render("  │  ")

	content := label(i18n.T("banner.setup")) + "    " +
		label(i18n.T("banner.version")+" ") + val(version) +
		sep +
		label(i18n.T("banner.loader")+" ") + val(modloader) +
		sep +
		label(i18n.T("banner.memory")+" ") + val(fmt.Sprintf("%d MB", memoryMB)) +
		sep +
		statusStr

	bannerWidth := 78
	if width > 0 {
		bannerWidth = max(70, width-2)
	}
	innerWidth := max(1, bannerWidth-4)
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorBorderHi).
		Width(bannerWidth).
		Padding(0, 2).
		Render(lipgloss.NewStyle().MaxWidth(innerWidth).Render(content))
}

// renderTopBar renders the CS:GO-style navigation tab bar.
// cursorMenu drives highlighting for menu-bound tabs; state overrides
// for sub-screens that are reached via keyboard shortcuts (i, m).
func renderTopBar(cursorMenu int, hasUpdate bool, state screenState, width int) string {
	type tabEntry struct {
		label string
		idx   int
	}
	tabs := []tabEntry{
		{i18n.T("topbar.play"), 0},
		{i18n.T("topbar.worlds"), 1},
		{i18n.T("topbar.mods"), 2},
		{i18n.T("topbar.instances"), 3},
		{i18n.T("topbar.configs"), 4},
		{i18n.T("topbar.quit"), 5},
	}

	// Determine which tab to highlight.
	activeIdx := cursorMenu
	instancesActive := state == instancesScreen || state == instanceNameScreen ||
		state == instanceLoaderScreen ||
		(state >= instanceCopyInspectScreen && state <= instanceCopyCleanupConfirmScreen) ||
		(state >= instanceTrashScreen && state <= instanceRemoveResultScreen)
	modsActive := state >= modsScreen && state <= modrinthWorkingScreen
	configsActive := state == configScreen || state == javaPickerScreen ||
		(state >= resetModeScreen && state <= resetResultScreen) ||
		(!instancesActive && (state == nameScreen || state == versionsScreen))
	if instancesActive {
		activeIdx = 3
	} else if modsActive {
		activeIdx = 2
	} else if state == worldsScreen {
		activeIdx = 1
	} else if state == confirmScreen {
		activeIdx = 0
	} else if configsActive {
		activeIdx = 4
	}

	var parts []string
	for _, t := range tabs {
		label := t.label
		// Badge de update en el tab PLAY
		if t.idx == 0 && hasUpdate {
			label += lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(" [!]")
		}
		if activeIdx == t.idx {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabInactive.Render(label))
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Center, parts...)
	barWidth := 82
	if width > 0 {
		barWidth = max(82, width)
	}
	divider := lipgloss.NewStyle().
		Foreground(colorBorder).
		Render(strings.Repeat("─", barWidth))

	return lipgloss.NewStyle().MaxWidth(barWidth).Render(bar) + "\n" + divider
}
