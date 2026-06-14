package ui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
	"mcTUI/internal/java"
	"mcTUI/internal/mojang"
	"mcTUI/internal/worlds"
)

func (m Model) View() string {
	asciiHeader := getAsciiArt() + "\n\n"

	menuStr := strings.Builder{}
	menuStr.WriteString(lipgloss.NewStyle().Foreground(colorViolet).Bold(true).Render(" Options") + "\n\n")

	menuOptions := []string{
		fmt.Sprintf("Play (%s)", m.versionSelect),
		"Change Name",
		"Change Version",
		"Worlds",
		fmt.Sprintf("Modloader: %s", lipgloss.NewStyle().Foreground(colorCyan).Render(m.modloader)),
		fmt.Sprintf("Memory: %d MB", m.memoryMB),
		"Quit",
	}

	for i, option := range menuOptions {
		if m.cursorMenu == i {
			menuStr.WriteString(cursorStyle.Render(fmt.Sprintf("▶ %s", option)) + "\n")
		} else {
			menuStr.WriteString(itemStyle.Render(fmt.Sprintf("  %s", option)) + "\n")
		}
	}

	contentStr := strings.Builder{}
	contentStr.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(" mcTUI Launcher") + "\n\n")

	switch {
	case m.state == menuScreen && m.cursorMenu == 5:
		m.renderMemoryPanel(&contentStr)
	case m.state == menuScreen && m.cursorMenu == 3:
		m.renderWorldsHint(&contentStr)
	case m.state == menuScreen:
		m.renderActiveSession(&contentStr)
	case m.state == nameScreen:
		m.renderNameScreen(&contentStr)
	case m.state == versionsScreen:
		m.renderVersionsScreen(&contentStr)
	case m.state == worldsScreen:
		m.renderWorldsScreen(&contentStr)
	case m.state == confirmScreen:
		m.renderConfirmScreen(&contentStr)
	}

	newsStr := strings.Builder{}
	newsStr.WriteString(lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render(" Future Changes") + "\n\n")
	for _, item := range m.roadmap {
		newsStr.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(item) + "\n")
	}
	newsStr.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDark).Render("Stay tuned..."))

	controls := m.controlsHint()
	
	// Responsive layout logic
	showNews := true
	if m.width > 0 && m.width < 122 {
		showNews = false
	}

	separatorWidth := 120
	if !showNews {
		separatorWidth = 80 // Menu (32) + Content (48)
	}
	separator := lipgloss.NewStyle().Foreground(colorDark).Render(strings.Repeat("─", separatorWidth))

	var statusPart string
	if installationReady(m.versionSelect, m.modloader) {
		statusPart = lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready")
	} else {
		statusPart = lipgloss.NewStyle().Foreground(colorRed).Render("● Needs setup")
	}

	userPart := lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("[%s - %s]", m.username, m.versionSelect))
	controlsPart := lipgloss.NewStyle().Foreground(colorDark).Render(controls)

	footerContent := fmt.Sprintf("%s\n%s   %s   %s", separator, statusPart, userPart, controlsPart)

	var topPanels string
	if showNews {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top,
			panelMenu.Render(menuStr.String()),
			panelContent.Render(contentStr.String()),
			panelNews.Render(newsStr.String()),
		)
	} else {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top,
			panelMenu.Render(menuStr.String()),
			panelContent.Render(contentStr.String()),
		)
	}

	fullInterface := lipgloss.JoinVertical(lipgloss.Center, asciiHeader, topPanels, panelFooter.Render(footerContent))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fullInterface)
	}
	return "\n" + fullInterface
}

func (m Model) controlsHint() string {
	switch m.state {
	case nameScreen:
		return " [Enter] Save  [Esc] Cancel"
	case versionsScreen:
		return " [↑/↓] Move list  [Enter] Choose  [Esc] Back"
	case worldsScreen:
		return " [↑/↓] Move list  [Enter] Select  [Esc] Back"
	case confirmScreen:
		return " [y] Accept  [n] Cancel"
	default:
		return " [↑/↓] Navigate  [Enter] Select  [q] Quit"
	}
}

func (m Model) renderMemoryPanel(c *strings.Builder) {
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Memory Configuration") + "\n\n")
	c.WriteString(fmt.Sprintf("Current Allocation : %s MB\n\n", lipgloss.NewStyle().Foreground(colorWhite).Render(fmt.Sprintf("%d", m.memoryMB))))
	c.WriteString("Recommended RAM:\n")
	c.WriteString("  Vanilla 1.20.x    →  1-2 GB (1024-2048 MB)\n")
	c.WriteString("  Fabric/light mods →  2-4 GB (2048-4096 MB)\n")
	c.WriteString("  Heavy modpacks    →  4-8 GB (4096-8192 MB)\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Press [Enter] to cycle values."))
}

func (m Model) renderWorldsHint(c *strings.Builder) {
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Local Worlds") + "\n\n")
	c.WriteString("View and manage your saved singleplayer worlds.\n\n")
	c.WriteString("Selecting a world will automatically change\n")
	c.WriteString("your launcher version to match the world's\n")
	c.WriteString("last played version to prevent corruption.\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Press [Enter] to browse saves."))
}

func (m Model) renderActiveSession(c *strings.Builder) {
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Active Session") + "\n\n")
	c.WriteString(fmt.Sprintf("User     : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(m.username)))

	verString := m.versionSelect
	if m.modloader != "Vanilla" {
		verString += fmt.Sprintf(" (%s)", m.modloader)
	}
	c.WriteString(fmt.Sprintf("Version  : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(verString)))

	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "macOS"
	case "windows":
		osName = "Windows"
	case "linux":
		osName = "Linux"
	}

	c.WriteString("Auth     : Offline (LAN Mode)\n")
	c.WriteString(fmt.Sprintf("OS       : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(osName)))

	mcDir := config.MinecraftDir()
	required := mojang.RequiredJavaVersion(mcDir, m.versionSelect)
	javaLine := java.FormatInfo(m.javaMajor, required)
	javaStyle := lipgloss.NewStyle().Foreground(colorWhite)
	if required > 0 && m.javaMajor < required {
		javaStyle = lipgloss.NewStyle().Foreground(colorRed)
	}
	c.WriteString(fmt.Sprintf("Java     : %s\n\n", javaStyle.Render(javaLine)))

	if installationReady(m.versionSelect, m.modloader) {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready (offline-capable)"))
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("● Not installed — will need network"))
	}
}

func (m Model) renderNameScreen(c *strings.Builder) {
	c.WriteString("New LAN username:\n\n")
	c.WriteString(m.input.View())
	c.WriteString("\n\n")

	hint := "Letters, numbers, and underscore only (1-16 chars)"
	if m.input.Value() != "" && !validUsername(m.input.Value()) {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ " + hint))
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(hint))
	}
}

func (m Model) renderVersionsScreen(c *strings.Builder) {
	c.WriteString("Select a stable version:\n\n")

	start, end := windowAround(m.cursorVersions, len(m.versions), 3, 6)
	for i := start; i < end; i++ {
		if i == m.cursorVersions {
			c.WriteString(cursorStyle.Render(fmt.Sprintf("  ▶ %s", m.versions[i])) + "\n")
		} else {
			c.WriteString(fmt.Sprintf("    %s", m.versions[i]) + "\n")
		}
	}
}

func (m Model) renderWorldsScreen(c *strings.Builder) {
	c.WriteString("Select a world to load:\n\n")

	if len(m.worlds) == 0 {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("No worlds found in ~/.minecraft/saves"))
		return
	}

	start, end := windowAround(m.cursorWorlds, len(m.worlds), 3, 6)
	for i := start; i < end; i++ {
		w := m.worlds[i]

		verStyle := lipgloss.NewStyle().Foreground(colorRed)
		if w.Version == m.versionSelect {
			verStyle = lipgloss.NewStyle().Foreground(colorGreen)
		}

		versionPart := verStyle.Render(" (" + w.Version + ")")
		timePart := ""
		if lastPlayed := worlds.FormatRelativeTime(w.LastPlayed); lastPlayed != "" {
			timePart = " " + lipgloss.NewStyle().Foreground(colorGray).Render("· "+lastPlayed)
		}

		if i == m.cursorWorlds {
			c.WriteString(cursorStyle.Render("  ▶ "+w.LevelName) + versionPart + timePart + "\n")
		} else {
			c.WriteString(itemStyle.Render("    "+w.LevelName) + versionPart + timePart + "\n")
		}
	}
}

func (m Model) renderConfirmScreen(c *strings.Builder) {
	c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ MISSING FILES") + "\n\n")

	if m.modloader == "Fabric" && !fabric.ProfileExists(config.MinecraftDir(), m.versionSelect) {
		c.WriteString(fmt.Sprintf("Fabric libraries for %s are not yet\ncached on your system.\n\n", m.versionSelect))
	} else {
		c.WriteString(fmt.Sprintf("The client.jar file (%s) is not\nfound on your system.\n\n", m.versionSelect))
	}
	c.WriteString("Would you like to download the missing\nfiles from the official Mojang/Fabric\nservers?\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("[y/Enter] Yes") + "   " + lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancel"))
}

// windowAround computes a [start, end) slice window of size windowSize
// centered (with `before` items before the cursor when possible) around
// cursor, clamped to [0, total).
func windowAround(cursor, total, before, windowSize int) (int, int) {
	start := cursor - before
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > total {
		end = total
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
