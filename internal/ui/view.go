package ui

import (
	"fmt"
	"runtime"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/mojang"
	"mcTUI/internal/resetconfigs"
	"mcTUI/internal/theme"
	"mcTUI/internal/updater"
	"mcTUI/internal/worlds"
)

func (m Model) View() string {
	if m.state == welcomeScreen {
		return m.renderWelcomeScreen()
	}
	if m.state == javaPickerScreen {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, m.pickerPanelContent())
	}
	// ── Download screen completo (tapa todo el TUI) ──────────────────────────
	if m.state == downloadScreen {
		return m.renderDownloadScreen()
	}
	if m.state >= gameStartingScreen && m.state <= gameResultScreen {
		return m.renderGameScreen()
	}
	resetActive := m.state >= resetModeScreen && m.state <= resetResultScreen
	if resetActive && m.compactReset() {
		content := m.resetPanelContent()
		if m.state == resetPickerScreen {
			content = m.pickerPanelContent()
		}
		header := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("  mcTUI  LAUNCHER") + "\n" +
			lipgloss.NewStyle().Foreground(colorBlue).Render("  "+i18n.T("menu.options")+"  ► "+i18n.T("menu.reset")) + "\n\n"
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, header+content)
	}

	instancesActive := m.state == instancesScreen || m.state == instanceNameScreen ||
		m.state == instanceLoaderScreen ||
		(m.state >= instanceCopyInspectScreen && m.state <= instanceCopyCleanupConfirmScreen) ||
		(m.state >= instanceTrashScreen && m.state <= instanceRemoveResultScreen)
	modsActive := m.state >= modsScreen && m.state <= modrinthWorkingScreen
	packsActive := m.state >= modrinthHubScreen && m.state <= packWorkingScreen || m.state == modpackScreen

	hasUpdate := m.latestRelease.TagName != ""

	centerWidth := m.centerWidth()
	panelsWidth := (leftColumnWidth + 2) + centerWidth + 2

	// ── Topbar de tabs ───────────────────────────────────────────────────────
	topBar := renderTopBar(m.cursorMenu, hasUpdate, m.state, panelsWidth)

	// ── Featured Banner status ───────────────────────────────────────────────
	cached := filesCached(m.runtimeDir(), m.versionSelect, m.modloader, m.instance.LoaderVersion)

	// ── Panel izquierdo: Profile Card + menú ────────────────────────────────
	profileCard := renderProfileCard(m.username, m.versionSelect, m.modloader, m.currentAccountType())

	menuStr := strings.Builder{}
	isConfigScreen := m.state == configScreen || m.state == accountsScreen || m.state == deviceCodeScreen
	menuHeader := i18n.T("menu.options")
	if isConfigScreen {
		menuHeader = i18n.T("menu.configs")
	}
	header := lipgloss.NewStyle().
		Foreground(colorBlue).Bold(true).Render(menuHeader)
	divLine := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 31))
	menuStr.WriteString(header + "\n" + divLine + "\n")

	javaLabel := i18n.T("menu.java_automatic")
	if m.instance.JavaPath != "" {
		if m.javaOverrideMajor > 0 {
			javaLabel = i18n.T("menu.java_manual", m.javaOverrideMajor)
		} else {
			javaLabel = i18n.T("menu.java_unavailable")
		}
	}
	loaderLabel := m.modloader
	if m.isManaged() && m.instance.LoaderVersion != "" {
		loaderLabel += " " + m.instance.LoaderVersion
	}
	versionLabel := versionOption(m.instance, m.versionSelect)

	// ── Main menu ─────────────────────────────────────────────────────────────
	menuOptions := []string{
		i18n.T("menu.play", m.versionSelect),
		i18n.T("menu.accounts_simple"),
		i18n.T("menu.worlds"),
		i18n.T("menu.modrinth"),
		i18n.T("menu.change_instances"),
		i18n.T("menu.configs"),
		i18n.T("menu.roadmap"),
		i18n.T("menu.quit"),
	}

	showConfigs := isConfigScreen || resetActive
	if showConfigs {
		appVer := m.appVersion
		if appVer == "" {
			appVer = "dev"
		}
		var updateOpt string
		if m.latestRelease.TagName != "" {
			updateOpt = i18n.T("menu.update_available", m.latestRelease.TagName)
		} else {
			updateOpt = i18n.T("menu.update_up_to_date")
		}
		configOptions := []string{
			i18n.T("menu.accounts_simple"),
			versionLabel,
			i18n.T("menu.modloader", lipgloss.NewStyle().Foreground(colorAccent2).Render(loaderLabel)),
			i18n.T("menu.memory", m.memoryMB),
			i18n.T("menu.reset"),
			i18n.T("menu.java", javaLabel),
			i18n.T("menu.language", i18n.T("menu.language_name")),
			i18n.T("menu.theme", CurrentTheme().Name),
			updateOpt,
		}
		for i, option := range configOptions {
			if m.cursorConfig == i && !instancesActive && !modsActive && !packsActive {
				line := lipgloss.NewStyle().
					Foreground(colorAccent).Bold(true).
					Render(fmt.Sprintf("► %s", option))
				menuStr.WriteString(lipgloss.NewStyle().MaxWidth(33).Render(line) + "\n")
			} else {
				line := lipgloss.NewStyle().Foreground(colorGray).
					Render(fmt.Sprintf("  %s", option))
				menuStr.WriteString(lipgloss.NewStyle().MaxWidth(33).Render(line) + "\n")
			}
		}
	} else {
		for i, option := range menuOptions {
			isActive := m.cursorMenu == i && !instancesActive && !modsActive && !packsActive
			if isActive {
				line := lipgloss.NewStyle().
					Foreground(colorAccent).Bold(true).
					Render(fmt.Sprintf("► %s", option))
				menuStr.WriteString(lipgloss.NewStyle().MaxWidth(33).Render(line) + "\n")
			} else {
				line := lipgloss.NewStyle().Foreground(colorGray).
					Render(fmt.Sprintf("  %s", option))
				menuStr.WriteString(lipgloss.NewStyle().MaxWidth(33).Render(line) + "\n")
			}
		}
	}

	leftBorderColor := colorBorder
	if m.state == menuScreen || resetActive || isConfigScreen || m.state == roadmapScreen {
		leftBorderColor = colorAccent
	}
	leftPanel := lipgloss.NewStyle().
		Width(leftColumnWidth).Height(14).
		Border(lipgloss.NormalBorder()).
		BorderForeground(leftBorderColor).
		Padding(0, 1).
		Render(menuStr.String())

	// Stack: profile card encima del panel de menú
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, profileCard, leftPanel)

	// ── Panel central: detalle ───────────────────────────────────────────────
	contentStr := strings.Builder{}
	switch {
	case m.state == accountsScreen:
		contentStr.WriteString(m.renderAccountsContent())
	case m.state == deviceCodeScreen:
		contentStr.WriteString(m.renderDeviceCodeContent())
	case m.state == resetPickerScreen:
		contentStr.WriteString(m.pickerPanelContent())
	case resetActive:
		contentStr.WriteString(m.resetPanelContent())
	case instancesActive:
		contentStr.WriteString(m.renderInstancesContent())
	case m.state == modpackScreen:
		contentStr.WriteString(m.renderModpacks())
	case packsActive:
		if m.state >= packSearchScreen && m.state <= packReviewScreen {
			contentStr.WriteString(m.renderPackCatalog())
		} else {
			contentStr.WriteString(m.renderPacks())
		}
	case modsActive:
		if m.state >= modrinthSearchScreen {
			contentStr.WriteString(m.renderCatalog())
		} else {
			contentStr.WriteString(m.renderMods())
		}
	case isConfigScreen && m.cursorConfig == 3:
		m.renderMemoryPanel(&contentStr)
	case isConfigScreen && m.cursorConfig == 7:
		m.renderThemePanel(&contentStr)
	case isConfigScreen && m.cursorConfig == 8:
		m.renderUpdatePanel(&contentStr)
	case isConfigScreen:
		m.renderActiveSession(&contentStr)
	case m.state == updateScreen:
		m.renderUpdatePanel(&contentStr)
	case m.state == menuScreen && m.cursorMenu == 1:
		contentStr.WriteString(m.renderAccountsContent())
	case m.state == menuScreen && m.cursorMenu == 2:
		m.renderWorldsHint(&contentStr)
	case m.state == roadmapScreen || (m.state == menuScreen && m.cursorMenu == 6):
		m.renderRoadmapContent(&contentStr)
	case m.state == menuScreen:
		m.renderActiveSession(&contentStr)
	case m.state == nameScreen:
		m.renderNameScreen(&contentStr)
	case m.state == versionsScreen:
		m.renderVersionsScreen(&contentStr)
	case m.state == worldsScreen:
		m.renderWorldsScreen(&contentStr)
	case m.state >= worldCopyDestinationScreen && m.state <= worldCopyResultScreen:
		m.renderWorldCopyScreen(&contentStr)
	case m.state == confirmScreen:
		m.renderConfirmScreen(&contentStr)
	}
	content := contentStr.String()
	centerStyle := panelContent.Copy().Width(centerWidth)
	if resetActive || instancesActive || modsActive || packsActive {
		if m.width > 0 {
			panelHeight := 20
			if modsActive || packsActive {
				panelHeight = max(20, panelHeight)
				// Height is only a minimum in Lip Gloss: long catalog pages
				// must not grow the panel and push the footer off the terminal.
				content = lipgloss.NewStyle().MaxWidth(centerWidth).MaxHeight(panelHeight).Render(content)
			}
			centerStyle = centerStyle.Height(panelHeight)
		} else {
			centerStyle = centerStyle.UnsetWidth().UnsetHeight()
		}
	}
	centerPanel := centerStyle.Render(content)

	// ── Footer ───────────────────────────────────────────────────────────────
	var statusPart string
	if cached {
		statusPart = lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("status.files_present"))
	} else {
		statusPart = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("status.files_missing"))
	}
	userPart := lipgloss.NewStyle().Foreground(colorGray).
		Render(fmt.Sprintf("[%s · %s]", m.username, m.versionSelect))
	var controls string
	if !modsActive && !packsActive {
		controls = m.controlsHint()
	}

	var updatePart string
	if hasUpdate {
		updatePart = "   " + lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(
			i18n.T("footer.update", m.latestRelease.TagName),
		)
	}
	footerContent := fmt.Sprintf("%s   %s", statusPart, userPart)
	if controls != "" {
		footerContent += "\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(controls)
	}
	footerContent += updatePart
	if m.width > 0 {
		footerContent = lipgloss.NewStyle().MaxWidth(m.width).Render(footerContent)
	}

	// ── Composición de paneles ────────────────────────────────────────────────
	topPanels := lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, centerPanel)

	compactLayout := resetActive || modsActive || packsActive
	var sections []string
	if (modsActive || packsActive) && m.height > 0 && m.height < 36 {
		sections = []string{topBar}
	} else {
		sections = []string{getAsciiArt()}
		if !compactLayout || m.height >= 37 {
			sections = append(sections, "")
		}
		sections = append(sections, topBar)
		if !compactLayout || m.height >= 36 {
			sections = append(sections, "")
		}
		sections = append(sections, renderFeaturedBanner(m.instance.Name, m.versionSelect, m.modloader, m.memoryMB, cached, panelsWidth))
		if !compactLayout || m.height >= 35 {
			sections = append(sections, "")
		}
	}
	footerStyle := panelFooter
	if controls == "" {
		footerStyle = footerStyle.UnsetHeight()
	}
	sections = append(sections, topPanels, footerStyle.Render(footerContent))
	fullInterface := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fullInterface)
	}
	return "\n" + fullInterface
}

// ── Reset screens ────────────────────────────────────────────────────────────
func (m Model) resetLines() []string {
	preserved := i18n.T("reset.preserved")
	worldsWarning := i18n.T("reset.worlds_warning")
	switch m.state {
	case resetModeScreen, resetConfirmScreen:
		if m.resetErr != nil {
			return []string{i18n.T("reset.installation"), i18n.T("reset.instance", m.resetInstance), i18n.T("reset.directory", m.resetDir), "", i18n.T("reset.cannot_inspect", displayError(m.resetErr))}
		}
		var lines []string
		if len(m.resetFound) == 0 {
			lines = []string{
				i18n.T("reset.installation"),
				i18n.T("reset.instance", m.resetInstance),
				i18n.T("reset.directory", m.resetDir),
				"",
				i18n.T("reset.status"),
				i18n.T("reset.nothing_found"),
				"",
			}
		} else {
			lines = []string{
				i18n.T("reset.installation"),
				i18n.T("reset.instance", m.resetInstance),
				i18n.T("reset.directory", m.resetDir),
				i18n.T("reset.close_game"),
				"",
				i18n.T("reset.affected"),
			}
			for _, name := range m.resetFound {
				lines = append(lines, "  - "+name+"/")
			}
			lines = append(lines, "", preserved, worldsWarning, "")
		}
		if m.state == resetModeScreen {
			lines = append(lines, i18n.T("reset.actions"))
			choices := []string{i18n.T("reset.backup_action"), i18n.T("reset.purge_action"), i18n.T("reset.browse_action")}
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
			return append([]string{i18n.T("reset.permanent_warning"), ""}, lines...)
		}
		dest := m.resetBackupDir
		if dest == "" {
			dest = m.resetBackupRoot
		}
		return append(lines, i18n.T("reset.backup_destination", dest), i18n.T("reset.move_question"))
	case resetBackupListScreen:
		lines := []string{i18n.T("reset.instance", m.resetInstance), i18n.T("reset.backups_in", m.resetBackupRoot)}
		if m.resetErr != nil {
			return append(lines, i18n.T("reset.cannot_list", displayError(m.resetErr)))
		}
		if len(m.resetBackups) == 0 {
			return append(lines, i18n.T("reset.no_backups"))
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
		lines := []string{i18n.T("reset.instance", m.resetInstance), i18n.T("reset.directory", m.resetDir), i18n.T("reset.backup", m.resetBackup.Name), i18n.T("reset.folders_to_restore", strings.Join(m.resetBackup.Folders, ", ")),
			i18n.T("reset.replaced_in")}
		for _, folder := range m.resetConflicts {
			lines = append(lines, "  - "+folder+"/")
		}
		return append(lines, i18n.T("reset.removes_current"))
	case resetRetentionScreen:
		lines := []string{i18n.T("reset.instance", m.resetInstance), i18n.T("reset.directory", m.resetDir), i18n.T("reset.restore_backup", m.resetBackup.Name), i18n.T("reset.folders", strings.Join(m.resetBackup.Folders, ", "))}
		if m.resetReplace {
			lines = append(lines, i18n.T("reset.confirmed_replace", strings.Join(m.resetConflicts, ", ")))
		}
		keep, consume := "  ", "  "
		if m.resetRetention == resetconfigs.KeepBackup {
			keep = "> "
		} else if m.resetRetention == resetconfigs.ConsumeBackup {
			consume = "> "
		}
		lines = append(lines, i18n.T("reset.after_restore"), keep+i18n.T("reset.keep_backup"),
			consume+i18n.T("reset.consume_backup"))
		return lines
	case resetRestoreConfirmScreen:
		text := i18n.T("reset.keep_question")
		if m.resetRetention == resetconfigs.ConsumeBackup {
			text = i18n.T("reset.consume_warning")
		}
		return []string{i18n.T("reset.instance", m.resetInstance), i18n.T("reset.directory", m.resetDir), i18n.T("reset.restore", m.resetBackup.Name), i18n.T("reset.folders", strings.Join(m.resetBackup.Folders, ", ")), text}
	case resetRunningScreen:
		if m.resetIsRestore {
			return []string{i18n.T("reset.restoring", m.resetBackup.Name), i18n.T("instances.wait")}
		}
		return []string{i18n.T("reset.resetting", m.resetDir), i18n.T("reset.wait_safe")}
	case resetResultScreen:
		lines := []string{i18n.T("reset.directory", m.resetDir)}
		if m.resetIsRestore {
			lines = append(lines, i18n.T("reset.restore_backup", m.resetBackup.Name))
			if len(m.resetResult.Errors) == 0 && m.resetErr == nil {
				if m.resetRetention == resetconfigs.KeepBackup {
					lines = append(lines, i18n.T("reset.original_kept", m.resetBackup.Path))
				} else {
					lines = append(lines, i18n.T("reset.original_consumed"))
				}
			}
		} else if m.resetMode == resetconfigs.Purge {
			lines = append(lines, i18n.T("reset.mode_purge"))
		} else {
			lines = append(lines, i18n.T("reset.mode_backup"))
		}
		if m.resetErr != nil {
			lines = append(lines, i18n.T("reset.cannot_start", displayError(m.resetErr)))
		} else if m.resetResult.NothingToReset {
			lines = append(lines, i18n.T("reset.nothing_created"))
		} else if len(m.resetResult.Errors) > 0 {
			operation := "reset.incomplete"
			if m.resetIsRestore {
				operation = "reset.restore_incomplete"
			}
			lines = append(lines, i18n.T(operation))
		} else if m.resetIsRestore {
			lines = append(lines, i18n.T("reset.restored_success"))
		} else {
			lines = append(lines, i18n.T("reset.completed_success"))
		}
		for _, err := range m.resetResult.Errors {
			lines = append(lines, i18n.T("reset.error", displayError(err)))
		}
		for _, folder := range m.resetResult.Folders {
			lines = append(lines, "", folder.Name+"/: "+i18n.T("reset.folder_"+string(folder.Status)), i18n.T("reset.path", folder.Path))
			if folder.Err != nil {
				lines = append(lines, i18n.T("reset.indented_error", displayError(folder.Err)))
			}
			for _, path := range folder.Removed {
				lines = append(lines, i18n.T("reset.removed", path))
			}
		}
		if m.resetResult.BackupPath != "" && !m.resetIsRestore {
			lines = append(lines, "", i18n.T("reset.backup", m.resetResult.BackupPath))
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
	title := i18n.T("reset.title")
	if m.state == resetResultScreen {
		title = i18n.T("reset.result_title")
		if m.resetIsRestore {
			title = i18n.T("reset.restore_result_title")
		}
	} else if m.state == resetRunningScreen {
		title = i18n.T("reset.running_title")
		if m.resetIsRestore {
			title = i18n.T("reset.restore_running_title")
		}
	} else if m.state == resetConfirmScreen {
		title = i18n.T("reset.confirm_title")
	} else if m.state == resetReplaceConfirmScreen || m.state == resetRestoreConfirmScreen {
		title = i18n.T("reset.confirm_restore_title")
	} else if m.state == resetBackupListScreen {
		title = i18n.T("reset.history_title")
	}
	lines := m.resetDisplayLines()
	start := min(m.resetScroll, m.resetMaxScroll())
	end := min(len(lines), start+m.resetVisibleLines())
	footer := i18n.T("common.back")
	if m.state == resetModeScreen {
		footer = i18n.T("reset.mode_hint")
	} else if m.state == resetBackupListScreen {
		footer = i18n.T("reset.history_hint")
	} else if m.state == resetResultScreen {
		footer = i18n.T("reset.result_hint")
	} else if m.state == resetRunningScreen {
		footer = i18n.T("reset.wait_hint")
	} else if m.state == resetConfirmScreen {
		footer = i18n.T("reset.move_hint")
		if m.resetMode == resetconfigs.Purge {
			footer = i18n.T("reset.purge_hint")
		}
	} else if m.state == resetReplaceConfirmScreen {
		footer = i18n.T("reset.replace_hint")
	} else if m.state == resetRestoreConfirmScreen {
		footer = i18n.T("reset.restore_hint")
	} else if m.state == resetRetentionScreen {
		footer = i18n.T("reset.retention_hint")
	}
	if end < len(lines) {
		key := "PgDn"
		if m.state == resetResultScreen {
			key = "↓"
		}
		footer = i18n.T("reset.more_lines", len(lines)-end, key, footer)
	} else if start > 0 {
		footer = i18n.T("reset.previous_lines", start, footer)
	}
	var styledLines []string
	for _, line := range m.resetLines() {
		style := lipgloss.NewStyle().Foreground(colorWhite)
		switch {
		case line == i18n.T("reset.installation") || line == i18n.T("reset.affected") || line == i18n.T("reset.actions") || line == i18n.T("reset.status"):
			style = style.Foreground(colorBlue).Bold(true)
		case strings.Contains(line, i18n.T("reset.purge_action")):
			style = style.Foreground(colorRed).Bold(strings.HasPrefix(line, "> "))
		case strings.HasPrefix(line, "> "):
			style = style.Foreground(colorAccent2).Bold(true)
		case strings.HasPrefix(line, i18n.T("reset.permanent_warning")) || line == i18n.T("reset.removes_current") || line == i18n.T("reset.incomplete") || line == i18n.T("reset.restore_incomplete") || strings.HasPrefix(line, i18n.T("reset.error", "")) || strings.HasPrefix(line, i18n.T("reset.cannot_inspect", "")) || strings.HasPrefix(line, i18n.T("reset.cannot_start", "")):
			style = style.Foreground(colorRed).Bold(true)
		case line == i18n.T("reset.completed_success") || line == i18n.T("reset.restored_success") || line == i18n.T("reset.nothing_created") || strings.HasSuffix(line, "/: "+i18n.T("reset.folder_moved")) || strings.HasSuffix(line, "/: "+i18n.T("reset.folder_restored")) || strings.HasSuffix(line, "/: "+i18n.T("reset.folder_deleted")):
			style = style.Foreground(colorGreen).Bold(true)
		case line == i18n.T("reset.preserved") || line == i18n.T("reset.worlds_warning") || line == i18n.T("reset.close_game"):
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
	titleText := i18n.T("status.verifying")
	description := i18n.T("status.preparing")
	if m.dlStage.Stage != "" {
		titleText = i18n.T("status.downloading")
		description = i18n.T("status.stage." + m.dlStage.Stage)
	}
	title := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(titleText)
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 60))

	const width, segment = 36, 8
	var progress string
	if m.dlStage.Total > 0 {
		done := min(m.dlStage.Done, m.dlStage.Total)
		filled := done * width / m.dlStage.Total
		progress = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(strings.Repeat("━", filled)) +
			lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", width-filled)) +
			"  " + fmt.Sprintf(i18n.T("status.files_progress"), done*100/m.dlStage.Total, done, m.dlStage.Total)
	} else {
		position := m.dlProgressFrame % (width - segment + 1)
		progress = lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", position)) +
			lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(strings.Repeat("━", segment)) +
			lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", width-position-segment))
	}

	var errLine string
	if m.dlErr != nil {
		errLine = "\n" + lipgloss.NewStyle().Foreground(colorRed).Bold(true).
			Render("⚠ "+displayError(m.dlErr))
	}

	hintText := i18n.T("status.download_hint")
	if m.dlFinished && m.dlErr != nil {
		hintText = i18n.T("status.download_error_hint")
	}
	hint := lipgloss.NewStyle().Foreground(colorDimGray).Render(hintText)

	body := fmt.Sprintf("%s\n%s\n\n%s\n%s%s\n\n%s",
		title, div, description, progress, errLine, hint)

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

func (m Model) renderGameScreen() string {
	status := i18n.T("status.starting")
	hint := i18n.T("status.game_wait")
	statusColor := colorAccent
	switch m.state {
	case gameRunningScreen:
		status = i18n.T("status.game_running")
		statusColor = colorGreen
		if m.gameKillConfirm {
			hint = i18n.T("status.force_close_question")
		} else {
			hint = i18n.T("status.force_close_hint")
		}
		if m.gameKillErr != nil {
			hint = i18n.T("status.kill_error", displayError(m.gameKillErr)) + "\n" + hint
			statusColor = colorRed
		}
	case gameResultScreen:
		status = i18n.T("status.game_exited", m.gameExitCode)
		statusColor = colorAccent
		if m.gameErr != nil {
			status = i18n.T("status.game_failed", displayError(m.gameErr))
			statusColor = colorRed
		}
		hint = i18n.T("status.game_back")
	}

	contentWidth := 64
	if m.width > 0 {
		contentWidth = max(1, min(contentWidth, m.width-10))
	}
	lines := wrapResetLines([]string{status}, contentWidth)
	styled := []string{lipgloss.NewStyle().Foreground(statusColor).Bold(true).Render(strings.Join(lines, "\n")), ""}
	for _, line := range wrapResetLines([]string{i18n.T("status.instance", m.launch.GameDir)}, contentWidth) {
		styled = append(styled, lipgloss.NewStyle().Foreground(colorWhite).Render(line))
	}
	if m.gameLog != "" {
		for _, line := range wrapResetLines([]string{i18n.T("status.log", m.gameLog)}, contentWidth) {
			styled = append(styled, lipgloss.NewStyle().Foreground(colorGray).Render(line))
		}
	}
	styled = append(styled, "")
	for _, line := range wrapResetLines(strings.Split(hint, "\n"), contentWidth) {
		styled = append(styled, lipgloss.NewStyle().Foreground(colorDimGray).Render(line))
	}
	body := strings.Join(styled, "\n")
	panel := lipgloss.NewStyle().Width(contentWidth).Border(lipgloss.NormalBorder()).BorderForeground(colorAccent).Padding(1, 3).Render(body)
	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
	}
	return "\n" + panel
}

func (m Model) controlsHint() string {
	if m.state == menuScreen {
		return " " + i18n.T("footer.controls")
	}
	switch m.state {
	case welcomeScreen:
		return i18n.T("welcome.help")
	case accountsScreen:
		return i18n.T("accounts.help")
	case deviceCodeScreen:
		if m.deviceCodeErr != nil {
			return i18n.T("msa.cancel")
		}
		return i18n.T("msa.copy") + "   " + i18n.T("msa.open_browser") + "   " + i18n.T("msa.cancel")
	case nameScreen:
		return i18n.T("footer.save")
	case configScreen, versionsScreen:
		return i18n.T("footer.choose")
	case roadmapScreen:
		return i18n.T("roadmap.hint")
	case worldsScreen:
		return i18n.T("worldcopy.worlds_hint")
	case worldCopyDestinationScreen:
		return i18n.T("worldcopy.destination_hint")
	case worldCopyConfirmScreen:
		return i18n.T("worldcopy.confirm_hint")
	case worldCopyRunningScreen:
		return i18n.T("worldcopy.running_hint")
	case worldCopyResultScreen:
		return i18n.T("worldcopy.result_hint")
	case confirmScreen:
		return i18n.T("footer.accept")
	case resetModeScreen, resetBackupListScreen, resetRetentionScreen:
		return i18n.T("footer.continue")
	case resetPickerScreen:
		return i18n.T("footer.picker")
	case javaPickerScreen:
		return i18n.T("footer.java")
	case resetResultScreen:
		return i18n.T("footer.scroll")
	case resetRunningScreen:
		return i18n.T("footer.wait")
	case resetConfirmScreen, resetReplaceConfirmScreen, resetRestoreConfirmScreen:
		return " " + i18n.T("common.confirm") + "  " + i18n.T("common.cancel")
	case modrinthHubScreen:
		return i18n.T("packs.hub_hint")
	case modpackScreen:
		return i18n.T("modpacks.footer_hint")
	case packListScreen:
		return i18n.T("packs.list_hint")
	case packPickerScreen:
		return i18n.T("packs.picker_hint")
	case packSearchScreen:
		return i18n.T("modrinth.search_hint")
	case packDetailScreen:
		return i18n.T("modrinth.detail_hint")
	case packReviewScreen:
		return i18n.T("modrinth.review_hint")
	case packConfirmScreen:
		return i18n.T("packs.confirm_hint")
	case modsScreen:
		return i18n.T("footer.mods")
	case modsPickerScreen:
		return i18n.T("mods.picker_hint")
	case modsConfirmScreen:
		return " " + i18n.T("mods.delete_hint") + "  " + i18n.T("common.cancel")
	case modsRollbackScreen:
		return " " + i18n.T("mods.rollback_hint")
	case modrinthSearchScreen:
		return i18n.T("modrinth.search_hint")
	case modrinthDetailScreen:
		return i18n.T("modrinth.detail_hint")
	case modrinthReviewScreen:
		return i18n.T("modrinth.review_hint")
	case modrinthUpdatesScreen:
		return i18n.T("modrinth.updates_hint")
	default:
		return i18n.T("footer.default")
	}
}

func (m Model) renderMemoryPanel(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.memory_title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	c.WriteString(labelStyle.Render(i18n.T("menu.allocation")) +
		valueStyle.Render(fmt.Sprintf("%d MB", m.memoryMB)) + "\n\n")

	c.WriteString(labelStyle.Render(i18n.T("menu.recommended_ram")) + "\n")
	rows := []struct{ label, gb, mb string }{
		{"Vanilla 1.20.x", "1-2 GB", "1024-2048 MB"},
		{i18n.T("menu.light_mods"), "2-4 GB", "2048-4096 MB"},
		{i18n.T("menu.heavy_modpacks"), "4-8 GB", "4096-8192 MB"},
	}
	for _, r := range rows {
		line := fmt.Sprintf("  %-18s %-6s (%s)", r.label, r.gb, r.mb)
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(line) + "\n")
	}
	c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("menu.memory_hint")))
}

func (m Model) renderWorldsHint(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.worlds_title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	text := i18n.T("menu.worlds_intro")
	if m.isManaged() {
		text += i18n.T("menu.worlds_managed")
	} else {
		text += i18n.T("menu.worlds_legacy")
	}

	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(text) + "\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("menu.worlds_hint")))
}

func (m Model) renderThemePanel(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.theme_panel_title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	curr := CurrentTheme()
	c.WriteString(labelStyle.Render("THEME") + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(curr.Name) + "\n")
	if curr.Description != "" {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(curr.Description) + "\n\n")
	} else {
		c.WriteString("\n")
	}

	swatch := func(label, hex string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)).Render("■ ") +
			lipgloss.NewStyle().Foreground(colorGray).Render(label) + " " +
			lipgloss.NewStyle().Foreground(colorDimGray).Render(hex)
	}

	c.WriteString(swatch("Accent", curr.Accent) + "   " + swatch("Accent2", curr.Accent2) + "\n")
	c.WriteString(swatch("Border", curr.Border) + "   " + swatch("BorderHi", curr.BorderHi) + "\n")
	c.WriteString(swatch("Text", curr.White) + "     " + swatch("Muted", curr.Gray) + "\n")
	c.WriteString(swatch("Green", curr.Green) + "    " + swatch("Red", curr.Red) + "   " + swatch("Blue", curr.Blue) + "\n\n")

	c.WriteString(div + "\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("menu.theme_hint")) + "\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(formatDisplayPath(theme.ThemesDir(), 44)))
}

func (m Model) renderUpdatePanel(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("update.title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	c.WriteString(labelStyle.Render(i18n.T("update.current_version", m.appVersion)) + "\n\n")

	latestTag := m.latestRelease.TagName
	if latestTag == "" {
		upToDateText := i18n.T("update.up_to_date")
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("  ✔ "+upToDateText) + "\n\n")
		return
	}

	c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).
		Render(fmt.Sprintf("  ⬆ %s", i18n.T("update.latest_version", latestTag))) + "\n")

	targetAsset := updater.TargetAssetName(runtime.GOOS, runtime.GOARCH)
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).
		Render(fmt.Sprintf("  • %s", i18n.T("update.platform", targetAsset, runtime.GOARCH))) + "\n\n")

	switch m.updateStage {
	case "idle":
		if m.latestRelease.Body != "" {
			c.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).
				Render("  " + i18n.T("update.release_notes")) + "\n")
			lines := formatReleaseNotes(m.latestRelease.Body, 40)
			visible := 5
			maxScroll := max(0, len(lines)-visible)
			start := min(m.updateScroll, maxScroll)
			if start < 0 {
				start = 0
			}
			if start > 0 {
				c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Render("    ↑") + "\n")
			}
			end := min(len(lines), start+visible)
			for _, line := range lines[start:end] {
				c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(fmt.Sprintf("    %s", line)) + "\n")
			}
			if end < len(lines) {
				c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Render("    ↓") + "\n")
			}
			c.WriteString("\n")
		}
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
			Render("  "+i18n.T("update.status_idle")) + "\n")
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).
			Render("  "+i18n.T("update.prompt_install")) + "\n")

	case "checksums":
		c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).
			Render("  ⏳ "+i18n.T("update.status_checksums")) + "\n")

	case "downloading":
		pct := int(m.updateProgress * 100)
		barWidth := 24
		filled := int(float64(barWidth) * m.updateProgress)
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		c.WriteString(lipgloss.NewStyle().Foreground(colorAccent).
			Render(fmt.Sprintf("  [%s] %3d%%", bar, pct)) + "\n")
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).
			Render(fmt.Sprintf("  %s", i18n.T("update.status_downloading", pct))) + "\n")

	case "verifying":
		c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).
			Render("  🔒 "+i18n.T("update.status_verifying")) + "\n")

	case "installing":
		c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).
			Render("  📦 "+i18n.T("update.status_installing")) + "\n")

	case "done":
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
			Render("  ✔ "+i18n.T("update.status_success", latestTag)) + "\n\n")
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).
			Render("  ✔ SHA-256 Verified OK") + "\n\n")
		c.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).
			Render("  "+i18n.T("update.prompt_restart")) + "\n")

	case "error":
		errMsg := "Unknown error"
		if m.updateErr != nil {
			errMsg = m.updateErr.Error()
		}
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).
			Render(fmt.Sprintf("  ✖ %s", i18n.T("update.error_prefix", errMsg))) + "\n\n")
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).
			Render("  [Esc] "+i18n.T("msa.cancel")) + "\n")
	}
}

// formatReleaseNotes converts the common Markdown in GitHub release notes to
// compact plain text and wraps every line to fit the updater panel.
func formatReleaseNotes(body string, width int) []string {
	width = max(1, width)
	var result []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimLeft(line, "# ")
		line = strings.TrimLeft(line, "-*+ ")
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "__", "")
		line = strings.ReplaceAll(line, "`", "")
		if line == "" {
			continue
		}

		var wrapped strings.Builder
		lineWidth := 0
		for _, word := range strings.Fields(line) {
			wordWidth := runewidth.StringWidth(word)
			if lineWidth > 0 && lineWidth+1+wordWidth > width {
				result = append(result, wrapped.String())
				wrapped.Reset()
				lineWidth = 0
			}
			if lineWidth > 0 {
				wrapped.WriteByte(' ')
				lineWidth++
			}
			wrapped.WriteString(word)
			lineWidth += wordWidth
		}
		if wrapped.Len() > 0 {
			result = append(result, wrapped.String())
		}
	}
	return result
}

func (m Model) renderRoadmapContent(c *strings.Builder) {
	width := 46
	if w := m.centerWidth() - 4; w > width {
		width = w
	}

	titleText := i18n.T("roadmap.title")
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(titleText)

	statusText := i18n.T("roadmap.offline")
	statusColor := colorGray
	if m.roadmapOnline {
		statusText = i18n.T("roadmap.synced")
		statusColor = colorGreen
	}
	if m.roadmapLoading {
		statusText = "..."
	}
	statusBadge := lipgloss.NewStyle().Foreground(statusColor).Render(statusText)

	topPad := width - runewidth.StringWidth(titleText) - runewidth.StringWidth(statusText)
	if topPad < 1 {
		topPad = 1
	}
	header := title + strings.Repeat(" ", topPad) + statusBadge
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", width))

	c.WriteString(header + "\n" + div + "\n")

	items := m.roadmapData.Items
	if len(items) == 0 {
		c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGray).Render("  No roadmap items.") + "\n\n")
		return
	}

	const maxVisible = 10
	startIdx := 0
	if len(items) > maxVisible {
		startIdx = m.roadmapCursor - maxVisible/2
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+maxVisible > len(items) {
			startIdx = len(items) - maxVisible
		}
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(items) {
		endIdx = len(items)
	}

	for i := startIdx; i < endIdx; i++ {
		item := items[i]
		isSel := m.state == roadmapScreen && m.roadmapCursor == i

		badgeLabel := item.Status
		badgeColor := colorBlue
		switch item.Status {
		case "in_progress":
			badgeLabel = i18n.T("roadmap.status_in_progress")
			badgeColor = colorAccent
		case "done", "completed":
			badgeLabel = i18n.T("roadmap.status_done")
			badgeColor = colorGreen
		default:
			badgeLabel = i18n.T("roadmap.status_planned")
			badgeColor = colorBlue
		}

		badgeStr := "[" + badgeLabel + "]"
		badgeWidth := runewidth.StringWidth(badgeStr)

		prefix := "  "
		if isSel {
			prefix = cursorStyle.Render("► ")
		}

		availTitleWidth := width - 2 - badgeWidth - 2
		if availTitleWidth < 10 {
			availTitleWidth = 10
		}

		titleStr := runewidth.Truncate(item.Title(), availTitleWidth, "…")
		titleWidth := runewidth.StringWidth(titleStr)

		padLen := width - 2 - titleWidth - badgeWidth
		if padLen < 1 {
			padLen = 1
		}

		itemTitleStyle := lipgloss.NewStyle().Foreground(colorGray)
		if isSel {
			itemTitleStyle = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
		}

		renderedBadge := lipgloss.NewStyle().Foreground(badgeColor).Bold(isSel).Render(badgeStr)

		c.WriteString(prefix + itemTitleStyle.Render(titleStr) + strings.Repeat(" ", padLen) + renderedBadge + "\n")
	}

	// Pad remaining lines up to maxVisible to strictly fix vertical height
	for i := endIdx - startIdx; i < maxVisible; i++ {
		c.WriteString("\n")
	}

	c.WriteString(div + "\n")
	if m.state == roadmapScreen {
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("roadmap.hint")) + "\n")
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("footer.default")) + "\n")
	}
}

func (m Model) renderActiveSession(c *strings.Builder) {
	titleText := i18n.T("menu.session_title")
	if m.state == configScreen {
		titleText = i18n.T("topbar.configs")
	}
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(titleText)
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

	runtimeDir := m.runtimeDir()
	required := mojang.RequiredJavaVersion(runtimeDir, m.versionSelect)
	javaLine := i18n.T("menu.java_auto_check")
	if required > 0 {
		javaLine = i18n.T("menu.java_auto_required", required)
	}
	javaColor := colorWhite
	javaIncompatible := false
	if m.instance.JavaPath != "" {
		if m.javaOverrideMajor == 0 {
			javaLine = i18n.T("menu.java_unavailable")
			javaIncompatible = true
		} else {
			javaLine = i18n.T("menu.java_manual_session", m.javaOverrideMajor)
			javaIncompatible = required > 0 && m.javaOverrideMajor < required
		}
	}
	if javaIncompatible {
		javaColor = colorRed
	}

	authLabel := i18n.T("menu.offline_auth")
	authColor := colorGray
	if m.currentAccountType() == "microsoft" {
		authLabel = i18n.T("menu.online_auth")
		authColor = colorGreen
	}

	rows := []struct {
		label, val string
		vc         lipgloss.Color
	}{
		{i18n.T("common.instance_label"), m.instance.Name, colorWhite},
		{i18n.T("menu.user_label"), m.username, colorWhite},

		{i18n.T("common.version_label"), verString, colorWhite},
		{i18n.T("menu.auth_label"), authLabel, authColor},
		{i18n.T("menu.os_label"), osName, colorWhite},
		{i18n.T("menu.java_label"), javaLine, javaColor},
	}

	for _, r := range rows {
		c.WriteString(fmt.Sprintf("%s%s  %s\n",
			pad,
			labelStyle.Render(r.label),
			lipgloss.NewStyle().Foreground(r.vc).Render(r.val),
		))
	}

	if javaIncompatible && required > 0 {
		c.WriteString(pad + "          " +
			lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("menu.java_too_old", required)) + "\n")
	}

	c.WriteString("\n")

	if filesCached(runtimeDir, m.versionSelect, m.modloader, m.instance.LoaderVersion) {
		c.WriteString(pad + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("menu.files_present")))
	} else {
		c.WriteString(pad + lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("menu.files_missing")))
	}
	c.WriteString("\n\n" + pad + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("menu.game_data", m.gameDir())))
	if m.instanceErr != nil {
		c.WriteString("\n\n" + lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.instanceErr)))
	}
}

func (m Model) renderNameScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.change_name_title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	c.WriteString(labelStyle.Render(i18n.T("menu.username_title")) + "\n\n")
	c.WriteString(m.input.View())
	c.WriteString("\n\n")

	hint := i18n.T("menu.username_hint")
	if m.input.Value() != "" && !validUsername(m.input.Value()) {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ " + hint))
	} else {
		c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(hint))
	}
}

func (m Model) renderVersionsScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.select_version"))
	if m.creatingInstance {
		title = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.select_new_version"))
	}
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
	if m.creatingInstance {
		c.WriteString("\n" + i18n.T("menu.new_version_hint"))
	}
}

func (m Model) renderWorldsScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.select_world"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")

	if len(m.worlds) == 0 {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("menu.no_worlds")))
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
	c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("worldcopy.worlds_hint")))
}

func (m Model) renderConfirmScreen(c *strings.Builder) {
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("menu.verify_installation"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	c.WriteString(title + "\n" + div + "\n\n")
	width := max(20, m.centerWidth()-6)
	launch, err := m.launchConfig()
	if err != nil {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(displayError(err)) + "\n\n")
	} else {
		javaLabel := i18n.T("menu.java_automatic")
		if launch.JavaPath != "" && m.javaOverrideMajor > 0 {
			javaLabel = i18n.T("menu.java_manual", m.javaOverrideMajor)
		} else if launch.JavaPath != "" {
			javaLabel = i18n.T("menu.java_unavailable")
		}
		details := strings.TrimSpace(i18n.T("menu.verify_details", m.instance.Name, launch.GameDir, launch.RuntimeDir, javaLabel))
		for _, line := range strings.Split(details, "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				label := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(parts[0] + ":")
				wrapped := wrapResetLines([]string{strings.TrimLeft(parts[1], " ")}, max(10, width-lipgloss.Width(parts[0])-2))
				for i, part := range wrapped {
					if i == 0 {
						c.WriteString(label + " " + lipgloss.NewStyle().Foreground(colorWhite).Render(part) + "\n")
					} else {
						c.WriteString("  " + lipgloss.NewStyle().Foreground(colorWhite).Render(part) + "\n")
					}
				}
			} else {
				c.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(strings.Join(wrapResetLines([]string{line}, width), "\n")) + "\n")
			}
		}
	}
	if m.dlErr != nil {
		c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorRed).Render(strings.Join(wrapResetLines([]string{displayError(m.dlErr)}, width), "\n")) + "\n")
	}
	c.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGray).Render(strings.Join(wrapResetLines([]string{i18n.T("menu.verify_hint")}, width), "\n")) + "\n\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("menu.yes_hint")) +
		"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("common.cancel")))
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

func versionOption(in instances.Instance, version string) string {
	if in.Kind == "managed" {
		return i18n.T("menu.version_fixed", version)
	}
	return i18n.T("menu.change_version")
}

// renderInstancesContent is the unified dispatcher for all instance sub-screens.
// It returns bare styled content that View() feeds into the center panel.
func (m Model) renderInstancesContent() string {
	switch {
	case m.state == instancesScreen:
		return m.renderInstancesListContent()
	case m.state == instanceNameScreen:
		return m.renderInstanceNameContent()
	case m.state == instanceLoaderScreen:
		return m.renderInstanceLoaderContent()
	case m.state >= instanceCopyInspectScreen && m.state <= instanceCopyCleanupConfirmScreen:
		return m.renderInstanceCopyContent()
	case m.state >= instanceTrashScreen && m.state <= instanceRemoveResultScreen:
		return m.renderRemovalScreen()
	}
	return ""
}

func (m Model) renderInstancesListContent() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.select_title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	b.WriteString(title + "\n" + div + "\n\n")
	if m.instanceErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.instanceErr)) + "\n\n")
	}
	if len(m.instanceList) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.empty")) + "\n")
	} else {
		start, end := windowAround(m.instanceCursor, len(m.instanceList), 3, 8)
		for i := start; i < end; i++ {
			in := m.instanceList[i]
			loader := in.Loader
			if in.LoaderVersion != "" {
				loader += " " + in.LoaderVersion
			}
			line := fmt.Sprintf("%s — %s %s", in.Name, in.MinecraftVersion, loader)
			var suffix string
			if in.ID == m.instance.ID {
				suffix = "  " + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("instances.active"))
			}
			if i == m.instanceCursor {
				b.WriteString(cursorStyle.Render("► "+line) + suffix + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+line) + suffix + "\n")
			}
		}
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(
		i18n.T("instances.list_hint"),
	))
	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.private_data")))
	return b.String()
}

func (m Model) renderInstanceNameContent() string {
	title := i18n.T("instances.new_vanilla")
	if m.creationLoaderVersion != "" {
		title = i18n.T("instances.new_fabric")
	}
	if m.copyingInstance {
		title = i18n.T("instances.copy_from_legacy")
		if m.copySourceID != instances.LegacyID {
			title = i18n.T("instances.duplicate")
		}
	} else if m.instanceRenameID != "" {
		title = i18n.T("instances.rename")
	}
	var b strings.Builder
	styledTitle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(title)
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	b.WriteString(styledTitle + "\n" + div + "\n\n")
	b.WriteString(labelStyle.Render(i18n.T("instances.name_label")) + "  " + m.input.View() + "\n")
	if m.instanceErr != nil {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.instanceErr)) + "\n")
	}
	if m.instanceRenameID != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.save_hint")))
	} else if m.copyingInstance {
		b.WriteString("\n" + labelStyle.Render(i18n.T("instances.minecraft_label")) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Render(m.creationVersion+" ("+m.copyLoader+")") + "\n")
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.choose_hint")))
	} else {
		loader := "Vanilla"
		if m.creationLoaderVersion != "" {
			loader = "Fabric " + m.creationLoaderVersion
		}
		b.WriteString("\n" + labelStyle.Render(i18n.T("instances.minecraft_label")) + "  " +
			lipgloss.NewStyle().Foreground(colorAccent2).Render(m.creationVersion+" ("+loader+")") + "\n")
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.create_hint")))
	}
	return b.String()
}

func (m Model) renderInstanceLoaderContent() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.finding_loader"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	b.WriteString(title + "\n" + div + "\n\n")
	b.WriteString(labelStyle.Render(i18n.T("instances.minecraft_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorAccent2).Render(m.creationVersion) + "\n\n")
	if m.instanceErr != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.instanceErr)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.retry_hint")))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.checking_fabric")) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.esc_cancel")))
	}
	return b.String()
}

func (m Model) renderInstanceCopyContent() string {
	var b strings.Builder
	title := i18n.T("instances.copy_from_legacy")
	if m.copySourceID != instances.LegacyID {
		title = i18n.T("instances.duplicate")
	}
	styledTitle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(title)
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	footer := lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("common.back"))

	switch m.state {
	case instanceCopyInspectScreen:
		b.WriteString(styledTitle + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.inspect_copy")) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render(i18n.T("instances.please_wait")))
		footer = lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.inspection"))

	case instanceCopySelectScreen:
		b.WriteString(styledTitle + "\n" + div + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.source_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(safeCopyText(m.copySource)) + "\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.dest_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(safeCopyText(m.dataRoot)+"/instances/<new-id>/minecraft") + "\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.minecraft_plain")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.creationVersion+" ("+m.copyLoader+")") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.choose_items")) + "\n")
		selected := false
		for i, item := range m.copyItems {
			selected = selected || m.copySelected[item.Name]
			mark := lipgloss.NewStyle().Foreground(colorDimGray).Render("[ ]")
			if m.copySelected[item.Name] {
				mark = lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("[x]")
			}
			status := formatCopySize(item.Bytes)
			if item.Error != "" {
				status = i18n.T("instances.unavailable", item.Error)
			} else if !item.Exists {
				status = i18n.T("instances.not_present")
			}
			nameStr := item.Name + "  " + lipgloss.NewStyle().Foreground(colorDimGray).Render("("+status+")")
			if i == m.copyCursor {
				b.WriteString(cursorStyle.Render("► ") + mark + " " + lipgloss.NewStyle().Foreground(colorWhite).Render(nameStr) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  ") + mark + " " + lipgloss.NewStyle().Foreground(colorGray).Render(nameStr) + "\n")
			}
		}
		if !selected {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("instances.select_item")))
		}
		footer = lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.copy_select_hint"))

	case instanceCopyConfirmScreen:
		cTitle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.confirm_copy"))
		b.WriteString(cTitle + "\n" + div + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.source_plain")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(safeCopyText(m.copySource)) + "\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.name_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(m.copyName) + "\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.ver_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.creationVersion+" ("+m.copyLoader+")") + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.content_label")) + "\n")
		var total int64
		for _, item := range m.copyItems {
			if m.copySelected[item.Name] {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(colorGray).Render(
					fmt.Sprintf("- %s (%s)", item.Name, formatCopySize(item.Bytes))) + "\n")
				total += item.Bytes
			}
		}
		b.WriteString("\n" + labelStyle.Render(i18n.T("instances.total_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(formatCopySize(total)) + "\n")
		sourceLabel := i18n.T("instances.source_name")
		if m.copySourceID == instances.LegacyID {
			sourceLabel = i18n.T("instances.legacy_name")
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGray).Render(
			i18n.T("instances.source_safe", sourceLabel)))
		footer = lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("instances.copy_confirm_hint")) +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.no_back"))

	case instanceCopyRunningScreen:
		opTitle := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(i18n.T("instances.copying"))
		if m.copyCleaning {
			opTitle = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.removing_partial"))
		}
		b.WriteString(opTitle + "\n" + div + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("instances.stage_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(safeCopyText(m.copyStage)) + "\n")
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.do_not_close")))
		footer = lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.operation_progress"))

	case instanceCopyResultScreen:
		resTitle := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("instances.copy_result"))
		b.WriteString(resTitle + "\n" + div + "\n\n")
		if m.copyResult.Instance.ID != "" {
			gameDir, _ := instances.GameDir(m.dataRoot, m.copyResult.Instance)
			b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render(i18n.T("instances.copy_completed")) + "\n\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.instance_plain")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.copyResult.Instance.Name) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.game_dir_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(safeCopyText(gameDir)) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.copied_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(strings.Join(m.copyResult.Copied, ", ")) + "\n")
			if m.copyErr != nil {
				b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("instances.select_failed", displayError(m.copyErr))) + "\n")
			}
		} else if m.copyErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.copyErr)) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.partial_removed")) + "\n")
		}
		if m.copyStage != "" {
			b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.partial_retained", safeCopyText(m.copyStage))) + "\n")
			footer = lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.discard_hint"))
		} else {
			footer = lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.continue_hint"))
		}

	case instanceCopyCleanupConfirmScreen:
		cTitle := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.confirm_cleanup"))
		b.WriteString(cTitle + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.remove_partial_question")) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(safeCopyText(m.copyStage)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.remove_partial_desc")))
		footer = lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.delete_partial_hint")) +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.keep_hint"))
	}

	b.WriteString("\n\n" + footer)
	return b.String()
}

func safeCopyText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return '�'
		}
		return r
	}, text)
}

func formatCopySize(size int64) string {
	if size < 1<<10 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1<<20 {
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	}
	return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
}
