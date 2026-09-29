package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
	"mcTUI/internal/worlds"
)

func (m Model) updateWorldCopy(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	if done, ok := msg.(WorldCopyDoneMsg); ok {
		if m.state == worldCopyRunningScreen {
			m.worldCopyFolder, m.worldCopyErr = done.Folder, done.Err
			m.state = worldCopyResultScreen
		}
		return m, nil, true
	}
	if m.state < worldCopyDestinationScreen || m.state > worldCopyResultScreen {
		return m, nil, false
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil, false
	}
	switch m.state {
	case worldCopyDestinationScreen:
		switch key.String() {
		case "up", "k":
			if m.worldCopyCursor > 0 {
				m.worldCopyCursor--
			}
		case "down", "j":
			if m.worldCopyCursor+1 < len(m.worldCopyTargets) {
				m.worldCopyCursor++
			}
		case "enter":
			if len(m.worldCopyTargets) > 0 {
				m.worldCopyDestination = m.worldCopyTargets[m.worldCopyCursor]
				m.state = worldCopyConfirmScreen
			}
		case "esc":
			m.state = worldsScreen
		case "ctrl+c":
			return m, tea.Quit, true
		}
	case worldCopyConfirmScreen:
		switch key.String() {
		case "y", "Y", "enter":
			root, source, destination, folder := m.dataRoot, m.instance, m.worldCopyDestination, m.worldCopySource.FolderName
			m.state = worldCopyRunningScreen
			return m, func() tea.Msg {
				name, err := worlds.Copy(root, source, destination, folder)
				return WorldCopyDoneMsg{Folder: name, Err: err}
			}, true
		case "n", "N", "esc":
			m.state = worldCopyDestinationScreen
		case "ctrl+c":
			return m, tea.Quit, true
		}
	case worldCopyResultScreen:
		switch key.String() {
		case "enter", "esc":
			m.state = worldsScreen
		case "ctrl+c":
			return m, tea.Quit, true
		}
	case worldCopyRunningScreen:
		// Keep the screen and process alive until the copy reports completion.
	}
	return m, nil, true
}

func (m Model) renderWorldCopyScreen(c *strings.Builder) {
	title := i18n.T("worldcopy.select_destination")
	switch m.state {
	case worldCopyConfirmScreen:
		title = i18n.T("worldcopy.confirm_title")
	case worldCopyRunningScreen:
		title = i18n.T("worldcopy.running_title")
	case worldCopyResultScreen:
		title = i18n.T("worldcopy.result_title")
	}
	c.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(title) + "\n")
	c.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46)) + "\n\n")
	if m.worldCopyErr != nil {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(safeCopyText(displayError(m.worldCopyErr))) + "\n")
		return
	}
	if m.state == worldCopyResultScreen {
		c.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render(i18n.T("worldcopy.success", safeCopyText(m.worldCopyFolder), safeCopyText(m.worldCopyDestination.Name))))
		return
	}
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("worldcopy.source", safeCopyText(m.worldCopySource.LevelName), safeCopyText(m.worldCopySource.FolderName), safeCopyText(m.instance.Name))) + "\n\n")
	if m.state == worldCopyDestinationScreen {
		if len(m.worldCopyTargets) == 0 {
			c.WriteString(i18n.T("worldcopy.no_destinations"))
			return
		}
		start, end := windowAround(m.worldCopyCursor, len(m.worldCopyTargets), 3, 6)
		for i := start; i < end; i++ {
			in := m.worldCopyTargets[i]
			line := safeCopyText(in.Name) + " (" + safeCopyText(in.MinecraftVersion) + ")"
			if i == m.worldCopyCursor {
				c.WriteString(cursorStyle.Render("► "+line) + "\n")
			} else {
				c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+line) + "\n")
			}
		}
		return
	}
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("worldcopy.destination", safeCopyText(m.worldCopyDestination.Name), safeCopyText(m.worldCopyDestination.MinecraftVersion))) + "\n\n")
	if m.state == worldCopyRunningScreen {
		c.WriteString(i18n.T("worldcopy.working"))
		return
	}
	if m.worldCopySource.Version == "" || m.worldCopyDestination.MinecraftVersion == "" || m.worldCopySource.Version != m.worldCopyDestination.MinecraftVersion {
		c.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("worldcopy.version_warning", safeCopyText(m.worldCopySource.Version), safeCopyText(m.worldCopyDestination.MinecraftVersion))) + "\n\n")
	}
	c.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("worldcopy.safety_warning")) + "\n\n")
	c.WriteString(i18n.T("worldcopy.confirm_prompt"))
}
