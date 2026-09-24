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
	"mcTUI/internal/resetconfigs"
	"mcTUI/internal/worlds"
)

func (m Model) View() string {
	// ── Download screen completo (tapa todo el TUI) ──────────────────────────
	if m.state == downloadScreen {
		return m.renderDownloadScreen()
	}
	if m.state >= resetModeScreen && m.state <= resetResultScreen {
		return m.renderResetScreen()
	}

	hasUpdate := m.latestRelease.TagName != ""

	// ── Logo ─────────────────────────────────────────────────────────────────
	logo := getAsciiArt()

	// ── Topbar de tabs ───────────────────────────────────────────────────────
	topBar := renderTopBar(m.cursorMenu, hasUpdate)

	// ── Featured Banner ──────────────────────────────────────────────────────
	ready := installationReady(m.versionSelect, m.modloader)
	banner := renderFeaturedBanner(m.versionSelect, m.modloader, m.memoryMB, ready)

	// ── Panel izquierdo: Profile Card + menú ────────────────────────────────
	profileCard := renderProfileCard(m.username, m.versionSelect, m.modloader)

	menuStr := strings.Builder{}
	header := lipgloss.NewStyle().
		Foreground(colorBlue).Bold(true).Render("OPTIONS")
	divLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 24))
	menuStr.WriteString(header + "\n" + divLine + "\n\n")

	menuOptions := []string{
		fmt.Sprintf("Play (%s)", m.versionSelect),
		"Change Name",
		"Change Version",
		"Worlds",
		fmt.Sprintf("Modloader: %s", lipgloss.NewStyle().Foreground(colorAccent2).Render(m.modloader)),
		fmt.Sprintf("Memory: %d MB", m.memoryMB),
		"Resets Configs",
		"Quit",
	}

	for i, option := range menuOptions {
		if m.cursorMenu == i {
			line := lipgloss.NewStyle().
				Foreground(colorAccent).Bold(true).
				Render(fmt.Sprintf("► %s", option))
			menuStr.WriteString(line + "\n")
		} else {
			line := lipgloss.NewStyle().Foreground(colorGray).
				Render(fmt.Sprintf("  %s", option))
			menuStr.WriteString(line + "\n")
		}
	}

	leftBorderColor := colorBorder
	if m.state == menuScreen {
		leftBorderColor = colorAccent
	}
	leftPanel := lipgloss.NewStyle().
		Width(28).Height(14).
		Border(lipgloss.NormalBorder()).
		BorderForeground(leftBorderColor).
		Padding(0, 1).
		Render(menuStr.String())

	// Stack: profile card encima del panel de menú
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, profileCard, leftPanel)

	// ── Panel central: detalle ───────────────────────────────────────────────
	contentStr := strings.Builder{}
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
	centerPanel := panelContent.Render(contentStr.String())

	// ── Panel derecho: roadmap ───────────────────────────────────────────────
	newsStr := strings.Builder{}
	newsHeader := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("FUTURE CHANGES")
	newsDivLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 31))
	newsStr.WriteString(newsHeader + "\n" + newsDivLine + "\n\n")

	const newsContentWidth = 32
	for _, item := range m.roadmap {
		for _, line := range wrapText(item, newsContentWidth) {
			newsStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("· "+line) + "\n")
		}
	}
	if hasUpdate {
		newsStr.WriteString("\n" + lipgloss.NewStyle().
			Foreground(colorAccent2).Bold(true).
			Render(fmt.Sprintf("⬆ %s available!", m.latestRelease.TagName)) + "\n")
	}
	newsStr.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("Stay tuned..."))

	// ── Layout responsive ────────────────────────────────────────────────────
	showNews := true
	if m.width > 0 && m.width < 122 {
		showNews = false
	}

	separatorWidth := 118
	if !showNews {
		separatorWidth = 82
	}
	separator := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", separatorWidth))

	// ── Footer ───────────────────────────────────────────────────────────────
	var statusPart string
	if ready {
		statusPart = lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("● READY")
	} else {
		statusPart = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("● NEEDS SETUP")
	}
	userPart := lipgloss.NewStyle().Foreground(colorGray).
		Render(fmt.Sprintf("[%s · %s]", m.username, m.versionSelect))
	controls := m.controlsHint()
	controlsPart := lipgloss.NewStyle().Foreground(colorDimGray).Render(controls)

	var updatePart string
	if hasUpdate {
		updatePart = "   " + lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(
			fmt.Sprintf("⬆ %s — github.com/agmonetti/mcTUI/releases", m.latestRelease.TagName),
		)
	}
	footerContent := fmt.Sprintf("%s\n%s   %s   %s%s",
		separator, statusPart, userPart, controlsPart, updatePart)

	// ── Composición de paneles ────────────────────────────────────────────────
	var topPanels string
	if showNews {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top,
			leftColumn,
			centerPanel,
			panelNews.Render(newsStr.String()),
		)
	} else {
		topPanels = lipgloss.JoinHorizontal(lipgloss.Top,
			leftColumn,
			centerPanel,
		)
	}

	fullInterface := lipgloss.JoinVertical(lipgloss.Left,
		logo,
		"",
		topBar,
		"",
		banner,
		"",
		topPanels,
		panelFooter.Render(footerContent),
	)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fullInterface)
	}
	return "\n" + fullInterface
}

// ── Reset screens ────────────────────────────────────────────────────────────
func (m Model) resetLines() []string {
	preserved := "Preserved: saves/, versions/, assets/, libraries/, shaderpacks/"
	worldsWarning := "Worlds are preserved, but do not load mod-dependent worlds until those mods are restored to prevent block/item loss."
	switch m.state {
	case resetModeScreen, resetConfirmScreen:
		lines := []string{
			"Minecraft directory: " + m.resetDir,
			"Close Minecraft before resetting; open files may be locked.",
			"Affected folders:",
		}
		for _, name := range m.resetFound {
			lines = append(lines, "  - "+name+"/")
		}
		lines = append(lines, preserved, worldsWarning, "")
		if m.resetErr != nil {
			return append(lines, "Cannot reset: "+m.resetErr.Error(), "[Esc] Back")
		}
		if len(m.resetFound) == 0 {
			return append(lines, "Nothing to reset.", "[Esc] Back")
		}
		if m.state == resetModeScreen {
			selectedBackup, selectedPurge := "  ", "  "
			if m.resetMode == resetconfigs.Backup {
				selectedBackup = "> "
			} else {
				selectedPurge = "> "
			}
			return append(lines,
				"How should Resets Configs handle the old files?",
				selectedBackup+"Reset and create backup (move files; manual restore only)",
				selectedPurge+"Reset without backup (delete permanently)",
				"[↑/↓] Choose  [Enter] Continue  [Esc] Cancel")
		}
		if m.resetMode == resetconfigs.Purge {
			return append(lines, "PERMANENT DELETION: the affected folders cannot be restored by mcTUI.",
				"[y] Delete permanently  [n/Esc] Cancel (Enter does not confirm)")
		}
		return append(lines, "Move affected folders into a new timestamped backup?",
			"[y] Move to backup  [n/Esc] Cancel (Enter does not confirm)")
	case resetRunningScreen:
		return []string{"Resetting " + m.resetDir + "...", "Please wait. Do not close mcTUI or Minecraft files may be left incomplete."}
	case resetResultScreen:
		lines := []string{"Minecraft directory: " + m.resetDir}
		if m.resetMode == resetconfigs.Purge {
			lines = append(lines, "Mode: permanent deletion")
		} else {
			lines = append(lines, "Mode: backup")
		}
		if m.resetErr != nil {
			lines = append(lines, "Reset could not start: "+m.resetErr.Error())
		} else if m.resetResult.NothingToReset {
			lines = append(lines, "Nothing to reset; no backup was created.")
		} else if len(m.resetResult.Errors) > 0 {
			lines = append(lines, "Reset incomplete; check each folder below. No automatic rollback.")
		} else {
			lines = append(lines, "Resets Configs completed successfully.")
		}
		for _, folder := range m.resetResult.Folders {
			lines = append(lines, "", folder.Name+"/: "+string(folder.Status), "  Source: "+folder.Path)
			if folder.Err != nil {
				lines = append(lines, "  Error: "+folder.Err.Error())
			}
			for _, path := range folder.Removed {
				lines = append(lines, "  Removed: "+path)
			}
		}
		if m.resetResult.BackupPath != "" {
			lines = append(lines, "", "Backup: "+m.resetResult.BackupPath, "Restore manually; mcTUI does not provide undo.")
		}
		return append(lines, "", preserved, worldsWarning)
	}
	return nil
}

func (m Model) resetWidth() int {
	if m.width > 0 {
		return max(32, min(80, m.width-12))
	}
	return 80
}

func (m Model) resetVisibleLines() int {
	if m.height > 0 {
		return max(4, m.height-9)
	}
	return 18
}

// Long paths must remain visible, not be silently truncated by the panel.
func (m Model) resetDisplayLines() []string {
	var lines []string
	width := m.resetWidth()
	for _, line := range m.resetLines() {
		part := ""
		for _, r := range line {
			if lipgloss.Width(part+string(r)) > width {
				lines = append(lines, part)
				part = ""
			}
			part += string(r)
		}
		lines = append(lines, part)
	}
	return lines
}

func (m Model) resetMaxScroll() int {
	return max(0, len(m.resetDisplayLines())-m.resetVisibleLines())
}

func (m Model) renderResetScreen() string {
	title := "RESETS CONFIGS"
	if m.state == resetResultScreen {
		title = "RESET RESULT"
	} else if m.state == resetRunningScreen {
		title = "RESET IN PROGRESS"
	} else if m.state == resetConfirmScreen {
		title = "CONFIRM RESET"
	}
	lines := m.resetDisplayLines()
	start := min(m.resetScroll, m.resetMaxScroll())
	end := min(len(lines), start+m.resetVisibleLines())
	footer := "[Esc] Back"
	if m.state == resetResultScreen {
		footer = "[↑/↓] Scroll  [Enter/Esc] Main menu"
	} else if m.state == resetRunningScreen {
		footer = "Waiting for reset to finish..."
	}
	body := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n" +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", m.resetWidth())) + "\n\n" +
		lipgloss.NewStyle().Foreground(colorWhite).Render(strings.Join(lines[start:end], "\n")) + "\n\n" +
		lipgloss.NewStyle().Foreground(colorAccent2).Render(footer)
	panel := lipgloss.NewStyle().Width(m.resetWidth()+4).Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).Padding(1, 2).Render(body)
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
	}
	return "\n" + panel
}

// ── Download Screen ───────────────────────────────────────────────────────────
func (m Model) renderDownloadScreen() string {
	title := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("DOWNLOADING FILES")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 60))

	pct := 0.0
	if m.dlTotal > 0 {
		pct = float64(m.dlDone) / float64(m.dlTotal) * 100
	}

	bar := renderProgressBar(m.dlDone, m.dlTotal, 48)

	fileLabel := lipgloss.NewStyle().Foreground(colorGray).Render("FILE  ")
	fileVal := lipgloss.NewStyle().Foreground(colorWhite).Render(m.dlFile)

	countLabel := lipgloss.NewStyle().Foreground(colorGray).Render("DONE  ")
	countVal := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).
		Render(fmt.Sprintf("%d / %d", m.dlDone, m.dlTotal))

	pctLabel := lipgloss.NewStyle().Foreground(colorGray).Render("PCT   ")
	pctVal := lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).
		Render(fmt.Sprintf("%.0f%%", pct))

	var errLine string
	if m.dlErr != nil {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colorRed).Bold(true).
			Render("⚠ "+m.dlErr.Error())
	}

	hint := lipgloss.NewStyle().Foreground(colorDimGray).
		Render("[Ctrl+C] Abort")

	body := fmt.Sprintf(
		"%s\n%s\n\n%s\n\n%s%s\n%s%s\n%s%s\n\n%s%s",
		title, div,
		bar,
		fileLabel, fileVal,
		countLabel, countVal,
		pctLabel, pctVal,
		errLine, hint,
	)

	panel := lipgloss.NewStyle().
		Width(64).
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Render(body)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
	}
	return "\n" + panel
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
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("MEMORY CONFIGURATION")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	c.WriteString(labelStyle.Render("CURRENT ALLOCATION  ") +
		valueStyle.Render(fmt.Sprintf("%d MB", m.memoryMB)) + "\n\n")

	c.WriteString(labelStyle.Render("RECOMMENDED RAM") + "\n")
	rows := []struct{ label, gb, mb string }{
		{"Vanilla 1.20.x", "1-2 GB", "1024-2048 MB"},
		{"Fabric/light mods", "2-4 GB", "2048-4096 MB"},
		{"Heavy modpacks", "4-8 GB", "4096-8192 MB"},
	}
	for _, r := range rows {
		line := fmt.Sprintf("  %-18s %-6s (%s)", r.label, r.gb, r.mb)
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(line) + "\n")
	}
	c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("Press [Enter] to cycle values."))
}

func (m Model) renderWorldsHint(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("LOCAL WORLDS")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	text := "View and manage your saved singleplayer\n" +
		"worlds.\n\n" +
		"Selecting a world will automatically\n" +
		"change your launcher version to match\n" +
		"the world's last played version to\n" +
		"prevent corruption."

	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(text) + "\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("Press [Enter] to browse saves."))
}

func (m Model) renderActiveSession(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("ACTIVE SESSION")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	pad := "  "

	osName := runtime.GOOS
	switch osName {
	case "darwin":
		osName = "macOS"
	case "windows":
		osName = "Windows"
	case "linux":
		osName = "Linux"
	}

	verString := m.versionSelect
	if m.modloader != "Vanilla" {
		verString += fmt.Sprintf(" (%s)", m.modloader)
	}

	mcDir := config.MinecraftDir()
	required := mojang.RequiredJavaVersion(mcDir, m.versionSelect)
	javaLine := java.FormatInfo(m.javaMajor, required)
	javaColor := colorWhite
	if required > 0 && m.javaMajor < required {
		javaColor = colorRed
	}

	rows := []struct {
		label, val string
		vc         lipgloss.Color
	}{
		{"USER      ", m.username, colorWhite},
		{"VERSION   ", verString, colorWhite},
		{"AUTH      ", "Offline (LAN Mode)", colorGray},
		{"OS        ", osName, colorWhite},
		{"JAVA      ", javaLine, javaColor},
	}

	for _, r := range rows {
		c.WriteString(fmt.Sprintf("%s%s  %s\n",
			pad,
			labelStyle.Render(r.label),
			lipgloss.NewStyle().Foreground(r.vc).Render(r.val),
		))
	}

	if required > 0 && m.javaMajor < required {
		c.WriteString(pad + "          " +
			lipgloss.NewStyle().Foreground(colorDimGray).Render("adoptium.net/temurin") + "\n")
	}

	c.WriteString("\n")

	if installationReady(m.versionSelect, m.modloader) {
		c.WriteString(pad + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("● READY — offline-capable"))
	} else {
		c.WriteString(pad + lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("● NOT INSTALLED — will need network"))
	}
}

func (m Model) renderNameScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("CHANGE NAME")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	c.WriteString(labelStyle.Render("NEW LAN USERNAME") + "\n\n")
	c.WriteString(m.input.View())
	c.WriteString("\n\n")

	hint := "Letters, numbers, and underscore only (1-16 chars)"
	if m.input.Value() != "" && !validUsername(m.input.Value()) {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ " + hint))
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(hint))
	}
}

func (m Model) renderVersionsScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("SELECT VERSION")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	start, end := windowAround(m.cursorVersions, len(m.versions), 3, 6)
	for i := start; i < end; i++ {
		if i == m.cursorVersions {
			c.WriteString(cursorStyle.Render(fmt.Sprintf("► %s", m.versions[i])) + "\n")
		} else {
			c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("  %s", m.versions[i])) + "\n")
		}
	}
}

func (m Model) renderWorldsScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("SELECT WORLD")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	if len(m.worlds) == 0 {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("No worlds found in ~/.minecraft/saves"))
		return
	}

	start, end := windowAround(m.cursorWorlds, len(m.worlds), 3, 6)
	for i := start; i < end; i++ {
		w := m.worlds[i]

		verColor := colorRed
		if w.Version == m.versionSelect {
			verColor = colorGreen
		}

		versionPart := lipgloss.NewStyle().Foreground(verColor).Render(" (" + w.Version + ")")
		timePart := ""
		if lastPlayed := worlds.FormatRelativeTime(w.LastPlayed); lastPlayed != "" {
			timePart = " " + lipgloss.NewStyle().Foreground(colorDimGray).Render("· "+lastPlayed)
		}

		if i == m.cursorWorlds {
			c.WriteString(cursorStyle.Render("► "+w.LevelName) + versionPart + timePart + "\n")
		} else {
			c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+w.LevelName) + versionPart + timePart + "\n")
		}
	}
}

func (m Model) renderConfirmScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ MISSING FILES")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	if m.modloader == "Fabric" && !fabric.ProfileExists(config.MinecraftDir(), m.versionSelect) {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(
			fmt.Sprintf("Fabric libraries for %s are not yet\ncached on your system.", m.versionSelect)) + "\n\n")
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(
			fmt.Sprintf("The client.jar file (%s) is not\nfound on your system.", m.versionSelect)) + "\n\n")
	}

	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(
		"Would you like to download the missing\nfiles from the official Mojang/Fabric\nservers?") + "\n\n")

	c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("[y/Enter] YES") +
		"   " +
		lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancel"))
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

// wrapText breaks s into lines of at most width characters, breaking
// only at spaces (never mid-word), so long roadmap items wrap cleanly
// instead of being cut by the panel's fixed width.
func wrapText(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) <= width {
			current += " " + w
		} else {
			lines = append(lines, current)
			current = w
		}
	}
	lines = append(lines, current)
	return lines
}
