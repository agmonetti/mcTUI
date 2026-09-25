package ui

import (
	"fmt"
	"path/filepath"
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
	resetActive := m.state >= resetModeScreen && m.state <= resetResultScreen
	if resetActive && m.compactReset() {
		content := m.resetPanelContent()
		if m.state == resetPickerScreen {
			content = m.pickerPanelContent()
		}
		header := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("  mcTUI  LAUNCHER") + "\n" +
			lipgloss.NewStyle().Foreground(colorBlue).Render("  OPTIONS  ► Resets Configs") + "\n\n"
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, header+content)
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
	if m.state == menuScreen || resetActive {
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
	case m.state == resetPickerScreen:
		contentStr.WriteString(m.pickerPanelContent())
	case resetActive:
		contentStr.WriteString(m.resetPanelContent())
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
	centerStyle := panelContent
	if resetActive {
		centerStyle = panelContent.Copy().Width(max(50, m.width-68)).Height(max(16, min(40, m.height-17)))
	}
	centerPanel := centerStyle.Render(contentStr.String())

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
	if resetActive || (m.width > 0 && m.width < 122) {
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

	sections := []string{logo}
	if !resetActive || m.height >= 37 {
		sections = append(sections, "")
	}
	sections = append(sections, topBar)
	if !resetActive || m.height >= 36 {
		sections = append(sections, "")
	}
	sections = append(sections, banner)
	if !resetActive || m.height >= 35 {
		sections = append(sections, "")
	}
	sections = append(sections, topPanels, panelFooter.Render(footerContent))
	fullInterface := lipgloss.JoinVertical(lipgloss.Left, sections...)

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
		if m.resetErr != nil {
			return []string{"INSTALLATION", "Minecraft directory: " + m.resetDir, "", "Cannot inspect installation: " + m.resetErr.Error()}
		}
		var lines []string
		if len(m.resetFound) == 0 {
			lines = []string{
				"INSTALLATION",
				"Minecraft directory: " + m.resetDir,
				"",
				"STATUS",
				"Nothing to reset (mods/ and config/ are empty or missing).",
				"",
			}
		} else {
			lines = []string{
				"INSTALLATION",
				"Minecraft directory: " + m.resetDir,
				"Close Minecraft before resetting; open files may be locked.",
				"",
				"AFFECTED FOLDERS",
			}
			for _, name := range m.resetFound {
				lines = append(lines, "  - "+name+"/")
			}
			lines = append(lines, "", preserved, worldsWarning, "")
		}
		if m.state == resetModeScreen {
			lines = append(lines, "ACTIONS")
			choices := []string{"Reset and create backup", "Reset without backup (delete permanently)", "Browse and restore backups"}
			for i, choice := range choices {
				prefix := "  "
				if i == m.resetChoice {
					prefix = "> "
				}
				lines = append(lines, prefix+choice)
			}
			return lines
		}
		if m.resetMode == resetconfigs.Purge {
			return append([]string{"PERMANENT DELETION: the affected folders cannot be restored by mcTUI.", ""}, lines...)
		}
		dest := m.resetBackupDir
		if dest == "" {
			dest = filepath.Join(m.resetDir, "mctui-backups")
		}
		return append(lines, "Backup destination: "+dest, "Move affected folders into a new timestamped backup?")
	case resetBackupListScreen:
		lines := []string{"Backups in: " + filepath.Join(m.resetDir, "mctui-backups")}
		if m.resetErr != nil {
			return append(lines, "Cannot list backups: "+m.resetErr.Error())
		}
		if len(m.resetBackups) == 0 {
			return append(lines, "No backups found.")
		}
		for i, backup := range m.resetBackups {
			prefix := "  "
			if i == m.resetBackupIndex {
				prefix = "> "
			}
			lines = append(lines, fmt.Sprintf("%s%s  %s  [%s]", prefix, backup.Path, backup.Created.Format("2006-01-02 15:04:05"), strings.Join(backup.Folders, ", ")))
		}
		return lines
	case resetReplaceConfirmScreen:
		lines := []string{"Backup: " + m.resetBackup.Name, "Folders to restore: " + strings.Join(m.resetBackup.Folders, ", "),
			"Existing content will be replaced in:"}
		for _, folder := range m.resetConflicts {
			lines = append(lines, "  - "+folder+"/")
		}
		return append(lines, "This permanently removes the current contents of those folders.")
	case resetRetentionScreen:
		lines := []string{"Restore backup: " + m.resetBackup.Name, "Folders: " + strings.Join(m.resetBackup.Folders, ", ")}
		if m.resetReplace {
			lines = append(lines, "Confirmed to replace current contents of: "+strings.Join(m.resetConflicts, ", "))
		}
		keep, consume := "  ", "  "
		if m.resetRetention == resetconfigs.KeepBackup {
			keep = "> "
		} else if m.resetRetention == resetconfigs.ConsumeBackup {
			consume = "> "
		}
		lines = append(lines, "After restoring this backup:", keep+"Keep original backup",
			consume+"Move it into Minecraft and remove it from history")
		return lines
	case resetRestoreConfirmScreen:
		text := "Keep the original backup after restoring?"
		if m.resetRetention == resetconfigs.ConsumeBackup {
			text = "The selected backup will be moved into Minecraft and removed from history."
		}
		return []string{"Restore: " + m.resetBackup.Name, "Folders: " + strings.Join(m.resetBackup.Folders, ", "), text}
	case resetRunningScreen:
		if m.resetIsRestore {
			return []string{"Restoring " + m.resetBackup.Name + "...", "Please wait; do not close mcTUI."}
		}
		return []string{"Resetting " + m.resetDir + "...", "Please wait. Do not close mcTUI or Minecraft files may be left incomplete."}
	case resetResultScreen:
		lines := []string{"Minecraft directory: " + m.resetDir}
		if m.resetIsRestore {
			lines = append(lines, "Restore backup: "+m.resetBackup.Name)
			if len(m.resetResult.Errors) == 0 && m.resetErr == nil {
				if m.resetRetention == resetconfigs.KeepBackup {
					lines = append(lines, "Original backup kept: "+m.resetBackup.Path)
				} else {
					lines = append(lines, "Original backup consumed and removed from history.")
				}
			}
		} else if m.resetMode == resetconfigs.Purge {
			lines = append(lines, "Mode: permanent deletion")
		} else {
			lines = append(lines, "Mode: backup")
		}
		if m.resetErr != nil {
			lines = append(lines, "Operation could not start: "+m.resetErr.Error())
		} else if m.resetResult.NothingToReset {
			lines = append(lines, "Nothing to reset; no backup was created.")
		} else if len(m.resetResult.Errors) > 0 {
			operation := "Reset"
			if m.resetIsRestore {
				operation = "Restore"
			}
			lines = append(lines, operation+" incomplete; check each folder below.")
		} else if m.resetIsRestore {
			lines = append(lines, "Backup restored successfully.")
		} else {
			lines = append(lines, "Resets Configs completed successfully.")
		}
		for _, err := range m.resetResult.Errors {
			lines = append(lines, "Error: "+err.Error())
		}
		for _, folder := range m.resetResult.Folders {
			lines = append(lines, "", folder.Name+"/: "+string(folder.Status), "  Path: "+folder.Path)
			if folder.Err != nil {
				lines = append(lines, "  Error: "+folder.Err.Error())
			}
			for _, path := range folder.Removed {
				lines = append(lines, "  Removed: "+path)
			}
		}
		if m.resetResult.BackupPath != "" && !m.resetIsRestore {
			lines = append(lines, "", "Backup: "+m.resetResult.BackupPath)
		}
		return lines
	}
	return nil
}

func (m Model) compactReset() bool {
	return m.width > 0 && m.height > 0 && (m.width < 122 || m.height < 34)
}

func (m Model) resetWidth() int {
	if m.compactReset() {
		return max(32, m.width-4)
	}
	if m.width > 0 {
		return max(32, min(120, m.width-75))
	}
	return 50
}

func (m Model) resetVisibleLines() int {
	if m.compactReset() {
		return max(4, m.height-9)
	}
	return max(7, min(40, m.height-23))
}

// Long paths must remain visible, not be silently truncated by the panel.
func (m Model) resetDisplayLines() []string {
	return wrapResetLines(m.resetLines(), m.resetWidth())
}

func wrapResetLines(raw []string, width int) []string {
	var lines []string
	for _, line := range raw {
		part := ""
		for _, r := range line {
			if lipgloss.Width(part+string(r)) > width {
				if i := strings.LastIndexByte(part, ' '); i > 2 {
					lines = append(lines, strings.TrimRight(part[:i], " "))
					part = strings.TrimLeft(part[i+1:], " ")
				} else {
					lines = append(lines, part)
					part = ""
				}
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

func (m *Model) scrollToBackupSelection() {
	for i, line := range m.resetDisplayLines() {
		if strings.HasPrefix(line, "> ") {
			if i < m.resetScroll {
				m.resetScroll = i
			} else if i >= m.resetScroll+m.resetVisibleLines() {
				m.resetScroll = i - m.resetVisibleLines() + 1
			}
			m.resetScroll = max(0, min(m.resetMaxScroll(), m.resetScroll))
			return
		}
	}
}

func (m Model) resetPanelContent() string {
	title := "RESETS CONFIGS"
	if m.state == resetResultScreen {
		title = "RESET RESULT"
		if m.resetIsRestore {
			title = "RESTORE RESULT"
		}
	} else if m.state == resetRunningScreen {
		title = "RESET IN PROGRESS"
		if m.resetIsRestore {
			title = "RESTORE IN PROGRESS"
		}
	} else if m.state == resetConfirmScreen {
		title = "CONFIRM RESET"
	} else if m.state == resetReplaceConfirmScreen || m.state == resetRestoreConfirmScreen {
		title = "CONFIRM RESTORE"
	} else if m.state == resetBackupListScreen {
		title = "BACKUP HISTORY"
	}
	lines := m.resetDisplayLines()
	start := min(m.resetScroll, m.resetMaxScroll())
	end := min(len(lines), start+m.resetVisibleLines())
	footer := "[Esc] Back"
	if m.state == resetModeScreen {
		footer = "[↑/↓] Choose  [Enter] Continue  [Esc] Cancel"
	} else if m.state == resetBackupListScreen {
		footer = "[↑/↓] Select backup  [Enter] Restore  [Esc] Back"
	} else if m.state == resetResultScreen {
		footer = "[↑/↓] Scroll  [Enter/Esc] Main menu"
	} else if m.state == resetRunningScreen {
		footer = "Waiting for operation to finish..."
	} else if m.state == resetConfirmScreen {
		footer = "[y] Move to backup  [n/Esc] Cancel"
		if m.resetMode == resetconfigs.Purge {
			footer = "[y] Delete permanently  [n/Esc] Cancel"
		}
	} else if m.state == resetReplaceConfirmScreen {
		footer = "[y] Confirm replacement  [n/Esc] Cancel"
	} else if m.state == resetRestoreConfirmScreen {
		footer = "[y] Confirm restore  [n/Esc] Cancel"
	} else if m.state == resetRetentionScreen {
		footer = "[↑/↓] Select  [Enter] Continue  [Esc] Cancel"
	}
	if end < len(lines) {
		key := "PgDn"
		if m.state == resetResultScreen {
			key = "↓"
		}
		footer = fmt.Sprintf("↓ %d more lines [%s]  %s", len(lines)-end, key, footer)
	} else if start > 0 {
		footer = fmt.Sprintf("↑ %d previous lines [PgUp]  %s", start, footer)
	}
	var styledLines []string
	for _, line := range m.resetLines() {
		style := lipgloss.NewStyle().Foreground(colorWhite)
		switch {
		case line == "INSTALLATION" || line == "AFFECTED FOLDERS" || line == "ACTIONS" || line == "STATUS":
			style = style.Foreground(colorBlue).Bold(true)
		case strings.Contains(line, "Reset without backup"):
			style = style.Foreground(colorRed).Bold(strings.HasPrefix(line, "> "))
		case strings.HasPrefix(line, "> "):
			style = style.Foreground(colorAccent2).Bold(true)
		case strings.Contains(line, "PERMANENT DELETION") || strings.Contains(line, "permanently removes") || strings.Contains(line, "incomplete") || strings.HasPrefix(line, "Error:") || strings.HasPrefix(line, "Cannot "):
			style = style.Foreground(colorRed).Bold(true)
		case strings.Contains(line, "completed successfully") || strings.Contains(line, "restored successfully") || strings.HasPrefix(line, "Nothing to reset") || strings.HasSuffix(line, "/: moved") || strings.HasSuffix(line, "/: restored") || strings.HasSuffix(line, "/: deleted"):
			style = style.Foreground(colorGreen).Bold(true)
		case strings.HasPrefix(line, "Preserved:") || strings.HasPrefix(line, "Worlds are preserved") || strings.HasPrefix(line, "Close Minecraft"):
			style = style.Foreground(colorGray)
		}
		for _, part := range wrapResetLines([]string{line}, m.resetWidth()) {
			styledLines = append(styledLines, style.Render(part))
		}
	}
	return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n" +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46)) + "\n\n" +
		strings.Join(styledLines[start:end], "\n") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorAccent2).Render(footer)
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
	case resetModeScreen, resetBackupListScreen, resetRetentionScreen:
		return " [↑/↓] Choose  [Enter] Continue  [Esc] Back"
	case resetPickerScreen:
		return " [←/→] Browse  [Ctrl+H] Hidden  [Enter] Select  [d] Default  [Esc] Back"
	case resetResultScreen:
		return " [↑/↓] Scroll  [Enter/Esc] Menu"
	case resetRunningScreen:
		return " Please wait…"
	case resetConfirmScreen, resetReplaceConfirmScreen, resetRestoreConfirmScreen:
		return " [y] Confirm  [n/Esc] Cancel"
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
