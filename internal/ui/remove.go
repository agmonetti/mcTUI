package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
)

func (m Model) updateRemovalKey(key string) (tea.Model, tea.Cmd) {
	switch m.state {
	case instanceTrashScreen:
		switch key {
		case "up", "k":
			m.retiredCursor = max(0, m.retiredCursor-1)
		case "down", "j":
			m.retiredCursor = min(max(0, len(m.retiredList)-1), m.retiredCursor+1)
		case "enter", "d":
			if len(m.retiredList) > 0 {
				m.removeTarget = m.retiredList[m.retiredCursor]
				m.removeFromTrash = true
				m.removeAction = "restore"
				if key == "d" {
					m.removeAction = "delete"
				}
				m.state = instanceRemoveConfirmScreen
			}
		case "esc":
			m.state = instancesScreen
		}
	case instanceRemoveChoiceScreen:
		switch key {
		case "s":
			m.removeAction = "soft"
			m.state = instanceRemoveConfirmScreen
		case "f":
			m.removeAction = "delete"
			m.state = instanceRemoveConfirmScreen
		case "esc":
			m.state = instancesScreen
		}
	case instanceRemoveConfirmScreen:
		switch key {
		case "y":
			root, id, action, fromTrash := m.dataRoot, m.removeTarget.ID, m.removeAction, m.removeFromTrash
			m.state = instanceRemoveRunningScreen
			return m, func() tea.Msg {
				var err error
				switch action {
				case "soft":
					err = instances.Retire(root, id)
				case "restore":
					err = instances.RestoreRetired(root, id)
				case "delete":
					if !fromTrash {
						err = instances.Retire(root, id)
					}
					if err == nil {
						err = instances.PurgeRetired(root, id)
					}
				}
				return InstanceRemovalDoneMsg{Err: err}
			}
		case "n", "esc":
			if m.removeFromTrash {
				m.state = instanceTrashScreen
			} else {
				m.state = instanceRemoveChoiceScreen
			}
		}
	case instanceRemoveResultScreen:
		if key == "enter" || key == "esc" {
			m.state = instancesScreen
		}
	}
	return m, nil
}

func (m Model) renderRemovalScreen() string {
	var b strings.Builder

	switch m.state {
	case instanceTrashScreen:
		title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.retired_title"))
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		if m.removeErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+displayError(m.removeErr)) + "\n\n")
		}
		if len(m.retiredList) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.no_retired")) + "\n")
		}
		start, end := windowAround(m.retiredCursor, len(m.retiredList), 3, 8)
		for i := start; i < end; i++ {
			in := m.retiredList[i]
			line := fmt.Sprintf("%s — %s %s", in.Name, in.MinecraftVersion, in.Loader)
			idStr := lipgloss.NewStyle().Foreground(colorDimGray).Render("[" + in.ID + "]")
			if i == m.retiredCursor {
				b.WriteString(cursorStyle.Render("► "+line) + "  " + idStr + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+line) + "  " + idStr + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.trash_hint")))

	case instanceRemoveChoiceScreen:
		title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.remove_title"))
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(m.removeTarget.Name) +
			"  " + lipgloss.NewStyle().Foreground(colorDimGray).Render("["+m.removeTarget.ID+"]") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render(i18n.T("instances.soft_delete")) +
			lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.soft_desc")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.full_delete")) +
			lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.full_desc")) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.esc_cancel")))

	case instanceRemoveConfirmScreen:
		root, id := m.dataRoot, m.removeTarget.ID
		active := filepath.Join(root, "instances", id)
		retired := filepath.Join(root, "retired", id)
		backup := filepath.Join(root, "backups", id)
		switch m.removeAction {
		case "soft":
			title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.confirm_soft"))
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.move_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(active) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.to_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(retired) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.backups_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.preserved_at", backup)) + "\n")
		case "restore":
			title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("instances.confirm_restore"))
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.move_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(retired) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.to_label")) + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(active) + "\n")
		case "delete":
			src := active
			if m.removeFromTrash {
				src = retired
			}
			title := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.confirm_delete"))
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.irreversible")) + "\n\n")
			b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.erase_label")) + "  " + lipgloss.NewStyle().Foreground(colorRed).Render(src) + "\n")
			b.WriteString(labelStyle.Render(i18n.T("instances.erase_label")) + "  " + lipgloss.NewStyle().Foreground(colorRed).Render(backup) + "\n")
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("common.confirm")) +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("common.cancel")))

	case instanceRemoveRunningScreen:
		title := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(i18n.T("instances.updating"))
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.wait")))

	case instanceRemoveResultScreen:
		if m.removeErr != nil {
			title := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("instances.failed"))
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(displayError(m.removeErr)) + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.check_paths")))
		} else {
			title := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("instances.updated"))
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("instances.completed")))
		}
		b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("instances.result_back")))
	}

	return b.String()
}
