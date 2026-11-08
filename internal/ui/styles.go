// Package ui implements the Bubble Tea model, update, and view for the
// mcTUI interface.
package ui

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
	"mcTUI/internal/theme"
)

// Current active theme and palette colors
var (
	currentTheme = theme.Default()

	// Fondo y estructura
	colorBorder   = lipgloss.Color(currentTheme.Border)
	colorBorderHi = lipgloss.Color(currentTheme.BorderHi)

	// Texto
	colorWhite   = lipgloss.Color(currentTheme.White)
	colorGray    = lipgloss.Color(currentTheme.Gray)
	colorDimGray = lipgloss.Color(currentTheme.DimGray)

	// Acentos
	colorAccent  = lipgloss.Color(currentTheme.Accent)
	colorAccent2 = lipgloss.Color(currentTheme.Accent2)

	// Status
	colorGreen = lipgloss.Color(currentTheme.Green)
	colorRed   = lipgloss.Color(currentTheme.Red)
	colorBlue  = lipgloss.Color(currentTheme.Blue)

	// Tab
	colorTabActive = lipgloss.Color(currentTheme.TabActive)
	colorTabInact  = lipgloss.Color(currentTheme.TabInact)

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

// CurrentTheme returns the active theme.
func CurrentTheme() theme.Theme {
	return currentTheme
}

// ApplyTheme updates all palette colors and derived Lipgloss styles
// to match the provided theme.
func ApplyTheme(t theme.Theme) {
	t.Normalize()
	currentTheme = t

	colorBorder = lipgloss.Color(t.Border)
	colorBorderHi = lipgloss.Color(t.BorderHi)
	colorWhite = lipgloss.Color(t.White)
	colorGray = lipgloss.Color(t.Gray)
	colorDimGray = lipgloss.Color(t.DimGray)
	colorAccent = lipgloss.Color(t.Accent)
	colorAccent2 = lipgloss.Color(t.Accent2)
	colorGreen = lipgloss.Color(t.Green)
	colorRed = lipgloss.Color(t.Red)
	colorBlue = lipgloss.Color(t.Blue)
	colorTabActive = lipgloss.Color(t.TabActive)
	colorTabInact = lipgloss.Color(t.TabInact)

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

	itemStyle = lipgloss.NewStyle().Foreground(colorWhite)
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
}

func formatDisplayPath(path string, maxWidth int) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home) {
		path = "~" + strings.TrimPrefix(path, home)
	}
	w := runewidth.StringWidth(path)
	if w <= maxWidth {
		return path
	}
	base := filepath.Base(path)
	baseWidth := runewidth.StringWidth(base)
	if baseWidth+6 >= maxWidth {
		return runewidth.Truncate(path, maxWidth, "…")
	}
	headLimit := maxWidth - baseWidth - 4
	head := runewidth.Truncate(filepath.Dir(path), headLimit, "")
	return head + "/…/" + base
}

// ── Logo ──────────────────────────────────────────────────────────────────────
func getAsciiArt() string {
	top := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("  mcTUI  LAUNCHER")

	sub := lipgloss.NewStyle().
		Foreground(colorGray).
		Render("  Minecraft Terminal UI")

	line := lipgloss.NewStyle().
		Foreground(colorDimGray).
		Render(strings.Repeat("─", 52))

	return top + "\n" + sub + "\n" + line
}

const leftColumnWidth = 35

// ── Profile Card ──────────────────────────────────────────────────────────────
// Mini card estilo CS:GO: badge de rango + username + version activa.
func renderProfileCard(username, version, modloader, accountType string) string {
	rankBadge := "[OPERATOR]"
	if accountType == "microsoft" {
		rankBadge = "[OPERATOR · MSA]"
	}
	rank := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render(rankBadge)

	displayName := username
	nameColor := colorWhite
	if displayName == "" {
		displayName = i18n.T("accounts.no_profile")
		nameColor = colorGray
	}

	user := lipgloss.NewStyle().
		Foreground(nameColor).
		Bold(username != "").
		Render(displayName)

	verStr := version
	if modloader != "Vanilla" {
		verStr += " · " + modloader
	}
	ver := lipgloss.NewStyle().Foreground(colorGray).Render(verStr)

	divLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 31))

	card := rank + "\n" + user + "\n" + ver + "\n" + divLine
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Width(leftColumnWidth).
		Padding(0, 1).
		Render(card)
}

// ── Featured Banner ───────────────────────────────────────────────────────────
// Franja horizontal que muestra la configuración activa, estilo CS:GO featured offer.
func renderFeaturedBanner(instanceName, version, modloader string, memoryMB int, cached bool, width int) string {
	statusStr := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("status.files_present_short"))
	if !cached {
		statusStr = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("status.files_missing"))
	}

	label := lipgloss.NewStyle().Foreground(colorGray).Render
	val := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render

	sep := lipgloss.NewStyle().Foreground(colorDimGray).Render("  │  ")

	content := label(i18n.T("banner.setup")) + " " + val(modText(instanceName)) + "    " +
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
		{i18n.T("topbar.accounts"), 1},
		{i18n.T("topbar.worlds"), 2},
		{i18n.T("topbar.modrinth"), 3},
		{i18n.T("topbar.instances"), 4},
		{i18n.T("topbar.configs"), 5},
		{i18n.T("topbar.quit"), 6},
	}

	// Determine which tab to highlight.
	activeIdx := cursorMenu
	instancesActive := state == instancesScreen || state == instanceNameScreen ||
		state == instanceLoaderScreen ||
		(state >= instanceCopyInspectScreen && state <= instanceCopyCleanupConfirmScreen) ||
		(state >= instanceTrashScreen && state <= instanceRemoveResultScreen)
	modsActive := state >= modsScreen && state <= modrinthWorkingScreen || state >= modrinthHubScreen && state <= packWorkingScreen || state == modpackScreen
	configsActive := state == configScreen || state == javaPickerScreen ||
		(state >= resetModeScreen && state <= resetResultScreen) ||
		(!instancesActive && (state == nameScreen || state == versionsScreen))
	accountsActive := state == accountsScreen || state == deviceCodeScreen
	if instancesActive {
		activeIdx = 4
	} else if modsActive {
		activeIdx = 3
	} else if state == worldsScreen || (state >= worldCopyDestinationScreen && state <= worldCopyResultScreen) {
		activeIdx = 2
	} else if accountsActive {
		activeIdx = 1
	} else if state == confirmScreen {
		activeIdx = 0
	} else if configsActive {
		activeIdx = 5
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
