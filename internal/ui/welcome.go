package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
)

// minecraftBannerCompact is the scaled 7-line MINECRAFT dot-art (84 cols wide),
// derived directly from the user's Braille art for comfortable terminal width fitting.
var minecraftBannerCompact = []string{
	"⠀⠀⠀⢠⡤⡤⢤⡄⠀⠀⠀⡤⠤⠤⢤⣤⡤⡤⡤⣤⡤⠤⠤⢤⡀⢠⠤⠤⠤⣤⡤⠤⠤⠤⠤⠤⠤⣤⡤⠤⠤⠤⠤⠤⠤⣤⡤⠤⠤⠤⠤⠤⠤⢤⣤⡤⠤⠤⠤⠤⠤⠤⠤⣤⡤⠤⠤⠤⠤⠤⠤⠤⣤⠤⠤⠤⠤⠤⠤⠤⠤⡄",
	"⠀⠀⢠⠇⠀⠀⠸⢿⣧⡠⠼⠃⠀⠀⣸⡇⠀⠀⢠⣿⠃⠀⠀⠸⢿⣼⠄⠀⠀⣿⠃⠀⠀⢀⣄⣄⣄⣤⣿⠀⠀⠀⢀⣄⣄⣄⣠⣽⡇⠀⠀⢠⣄⠀⠀⠀⢻⡇⠀⢠⣄⠀⣤⣄⠀⢻⣇⠀⠀⠀⣄⣤⣀⣀⣿⣧⣀⣠⠀⠀⠀⢤⣀⡿",
	"⠀⠀⡌⠀⠀⠀⠀⠺⠻⠃⠀⠀⠀⢰⣿⠃⠀⠀⢸⡏⠀⠀⠀⠀⠚⠃⠀⠀⠀⣿⠀⠀⠀⠘⠛⠻⠛⠛⣿⠀⠀⠀⢸⣿⣿⣿⣿⣿⡏⠀⠀⠈⠛⠀⠀⢠⣼⡇⠀⠀⢾⣿⣿⠁⠀⠈⣿⠀⠀⠀⠛⠛⠛⠛⠛⣿⣿⣿⡄⠀⠀⠘⡟⠀",
	"⠀⢸⠁⠀⠀⣠⡄⠀⢠⡤⠀⠀⠀⢸⡏⠀⠀⠀⣾⠇⠀⠀⠀⣿⠀⠀⠀⠀⢰⣿⠀⠀⠀⣤⣤⣤⣤⣤⣿⠀⠀⠀⢸⡟⠛⠛⠛⢻⡇⠀⠀⠀⣤⡄⠀⠀⢸⣷⠀⠀⠈⠀⠈⠀⠀⠀⣿⡄⠀⠀⠀⣤⣤⣤⣤⣿⣿⣿⣷⠀⠀⠀⢳⠀",
	"⢠⠇⠀⠀⢰⣿⣥⣤⣼⡇⠀⠀⠀⣿⠃⠀⠀⢸⡟⠀⠀⠀⢸⣿⣿⠀⠀⠀⣼⡟⠀⠀⠀⠛⠛⠛⠛⠛⣿⠀⠀⠀⠘⠓⠒⠓⠓⢺⡇⠀⠀⠀⣿⡇⠀⠀⠘⣿⠀⠀⠀⢸⣧⠀⠀⠀⢹⣧⠀⠀⠀⣿⣿⣿⠛⠹⣿⣿⣿⡇⠀⠀⠀⣇",
	"⠿⣤⣤⣤⣾⣿⣿⣿⣿⣦⣤⣤⣾⣿⣤⣦⣤⣾⣷⣴⣦⣤⣾⣿⣿⣤⣴⣤⣿⣷⣤⣦⣤⣤⣤⣦⣤⣶⣿⣤⣴⣶⣦⣤⣤⣴⣦⣾⣷⣦⣤⣴⣾⣷⣴⣦⣴⣿⣧⣴⣦⣼⣿⣦⣤⣤⣼⣿⣤⣤⣤⡾⠉⠁⠀⠀⣿⣿⣿⣿⣦⣤⣤⠿",
	"⠀⠈⠻⣿⣿⣿⣿⡿⠉⠉⠻⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠏⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠟⠀⠀⠀⠀⠀⠹⣿⣿⣿⣿⠿⠉⠀",
}

// creeperDotArt is the user's Creeper dot-art.
var creeperDotArt = []string{
	"⠀⠀⡴⠦⠤⠶⠖⠒⠒⠒⠒⠒⠒⠒⠒⠓⢻⠒⠒⠤⠄⣀⡀⠀⠀",
	"⠀⠀⡗⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠈⢹⡆⠀",
	"⠀⠀⡇⢠⠤⡤⠤⣄⠀⠀⢀⡄⠀⠀⢠⠀⢸⠀⠀⠀⠀⠀⠀⢸⡇⠀",
	"⠀⠀⡇⢸⠀⠀⠀⣷⠀⠀⢸⠀⠀⠀⢸⠀⢸⠀⠀⠀⠀⠀⠀⢸⠃⠀",
	"⠀⠀⣷⠘⠦⠤⠤⣧⠄⠄⣼⠦⠀⠀⡾⠀⢸⠀⠀⠀⠀⠀⠀⢸⠀⠀",
	"⠀⠀⣿⠀⠀⢀⣀⣳⠂⠀⢸⣄⣀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⢼⠀⠀",
	"⠀⠀⣿⠀⠀⣿⠀⠀⠀⠀⠀⠀⢨⡇⠀⠀⢸⠀⠀⠀⠀⠀⠀⣿⠀⠀",
	"⠀⠀⢿⠀⠀⢸⠀⣄⣀⣀⣀⡀⠈⡇⠀⠀⢸⠀⠀⠀⠀⠀⠀⣿⠀⠀",
	"⠀⠀⢼⣄⣀⣸⣁⣿⣀⡀⡈⣇⣘⡇⢀⡀⣸⢀⣀⣀⣀⣤⡤⠿⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠉⠉⠉⠉⠉⠉⠉⢹⡇⠉⠀⢰⡇⠀⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇⠀⠀⢠⡇⠀⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡇⠀⠀⠠⡇⠀⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⡇⠀⠀⠀⡇⠀⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠠⡇⠀⠀⠀",
	"⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠐⡇⠀⠀⠀⡇⠀⠀⠀",
	"⠀⠀⠀⠀⠘⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠰⡟⣲⠶⡆",
	"⢀⣀⡤⠖⠊⠉⠙⠒⠒⠶⠤⠤⢀⣀⣀⣀⠀⠐⡇⣀⠴⢚⡏⠀⠀⡇",
	"⢸⠉⠉⠁⠓⠒⠢⠤⠤⠄⣀⣀⣀⠀⢀⣉⠭⢛⡏⠁⠀⢸⡇⠀⠀⡇",
	"⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢹⡏⠀⠀⠈⡇⠀⠀⠈⡇⠀⠀⡇",
	"⢸⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠀⡧⣤⣀⣈⡇⠀⠀⡇",
	"⢘⣇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠀⡇⠀⠀⢈⡇⠀⢀⡇",
	"⠀⡇⠈⠉⠉⠒⠒⠂⠤⠤⢤⣀⣀⡀⡇⠀⠀⠀⡧⠤⠤⢬⠧⠞⠉⠀",
	"⠀⠧⢄⣀⣀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣇⠀⠀⣀⡇⠀⠀⠀⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠈⠉⠁⠒⠒⠦⠤⠤⣀⣫⠴⠚⠉⠀⠀⠀⠀⠀⠀⠀⠀",
}

func (m Model) renderWelcomeScreen() string {
	centerWidth := m.centerWidth()
	panelsWidth := (leftColumnWidth + 2) + centerWidth + 2

	// ── Compact MINECRAFT Dot-Art Banner ──────────────────────────────────────
	bannerStyle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	var bannerSB strings.Builder
	for idx, line := range minecraftBannerCompact {
		bannerSB.WriteString(bannerStyle.Render(line))
		if idx < len(minecraftBannerCompact)-1 {
			bannerSB.WriteString("\n")
		}
	}

	bannerBox := lipgloss.NewStyle().
		Width(panelsWidth).
		Align(lipgloss.Center).
		Render(bannerSB.String())

	// ── 2 Panels Layout (50/50 division) ──────────────────────────────────────
	leftPanelWidth := (panelsWidth - 4) / 2
	rightPanelWidth := panelsWidth - leftPanelWidth - 4

	// Left Panel: Creeper Dot-Art
	creeperLines := creeperDotArt
	panelHeight := len(creeperDotArt)

	var leftSB strings.Builder
	creeperStyle := lipgloss.NewStyle().Foreground(colorAccent2)
	for idx, line := range creeperLines {
		leftSB.WriteString(creeperStyle.Render(line))
		if idx < len(creeperLines)-1 {
			leftSB.WriteString("\n")
		}
	}

	leftPanel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Width(leftPanelWidth).
		Height(panelHeight).
		Align(lipgloss.Center, lipgloss.Center).
		Render(leftSB.String())

	// Right Panel: Welcome Guide & Getting Started
	var rightSB strings.Builder

	welcomeHdr := lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		Render(i18n.T("menu.options") + "  ►  " + i18n.T("welcome.title"))
	divLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", max(20, rightPanelWidth-2)))
	rightSB.WriteString(welcomeHdr + "\n" + divLine + "\n\n")

	statusHdr := lipgloss.NewStyle().
		Foreground(colorWhite).
		Bold(true).
		Render(i18n.T("welcome.status_header"))
	rightSB.WriteString(statusHdr + "\n")

	warnBullet := lipgloss.NewStyle().Foreground(colorAccent).Render("○ ")
	accText := lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("welcome.no_account"))
	rightSB.WriteString("  " + warnBullet + accText + "\n")

	managedCount := 0
	for _, in := range m.instanceList {
		if in.Kind != "legacy" {
			managedCount++
		}
	}
	instText := lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("welcome.no_instances"))
	if managedCount > 0 {
		instText = lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("%d %s", managedCount, i18n.T("welcome.instances_managed")))
	}
	rightSB.WriteString("  " + warnBullet + instText + "\n\n")

	guide := lipgloss.NewStyle().
		Foreground(colorWhite).
		Width(max(20, rightPanelWidth-4)).
		Render(i18n.T("welcome.guide"))
	rightSB.WriteString(guide + "\n\n")

	rightPanel := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorBorder).
		Width(rightPanelWidth).
		Height(panelHeight).
		Padding(0, 1).
		Render(rightSB.String())

	// ── Composition identical to mainView ─────────────────────────────────────
	topPanels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	footerContent := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(i18n.T("welcome.continue")) +
		"  ·  " +
		lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("welcome.language_toggle")) +
		"  ·  " +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("welcome.skip"))

	footerStyle := panelFooter
	var sections []string
	sections = append(sections, bannerBox, "", topPanels, footerStyle.Render(footerContent))

	fullInterface := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fullInterface)
	}
	return "\n" + fullInterface
}
