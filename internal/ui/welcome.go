package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
)

func (m Model) renderWelcomeScreen() string {
	var b strings.Builder

	// Header banner
	header := lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true).
		Render("mcTUI  ─  MINECRAFT TERMINAL LAUNCHER")
	b.WriteString(header + "\n")

	sub := lipgloss.NewStyle().
		Foreground(colorGray).
		Render(i18n.T("welcome.subtitle"))
	b.WriteString(sub + "\n")

	div := lipgloss.NewStyle().
		Foreground(colorDimGray).
		Render(strings.Repeat("─", 60))
	b.WriteString(div + "\n\n")

	// Greeting
	greeting := lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		Render(i18n.T("welcome.title"))
	b.WriteString(greeting + "\n\n")

	// Status header
	statusHdr := lipgloss.NewStyle().
		Foreground(colorWhite).
		Bold(true).
		Render(i18n.T("welcome.status_header"))
	b.WriteString(statusHdr + "\n")

	warnBullet := lipgloss.NewStyle().Foreground(colorAccent).Render("○ ")
	accText := lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("welcome.no_account"))
	b.WriteString("  " + warnBullet + accText + "\n")

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
	b.WriteString("  " + warnBullet + instText + "\n\n")

	// Guide text
	guide := lipgloss.NewStyle().
		Foreground(colorWhite).
		Width(60).
		Render(i18n.T("welcome.guide"))
	b.WriteString(guide + "\n\n")

	b.WriteString(div + "\n")

	// Action buttons
	btnEnter := lipgloss.NewStyle().
		Foreground(colorAccent2).
		Bold(true).
		Render(i18n.T("welcome.continue"))
	btnLang := lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true).
		Render(i18n.T("welcome.language_toggle"))
	btnEsc := lipgloss.NewStyle().
		Foreground(colorDimGray).
		Render(i18n.T("welcome.skip"))

	b.WriteString(btnEnter + "   " + btnLang + "\n" + btnEsc + "\n")

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Width(68).
		Render(b.String())

	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}
