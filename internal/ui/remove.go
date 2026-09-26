package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
		title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("RETIRED INSTANCES")
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		if m.removeErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+m.removeErr.Error()) + "\n\n")
		}
		if len(m.retiredList) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("No retired instances.") + "\n")
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
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[↑/↓] Move  [Enter] Restore  [d] Delete permanently  [Esc] Back"))

	case instanceRemoveChoiceScreen:
		title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("REMOVE INSTANCE")
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(m.removeTarget.Name) +
			"  " + lipgloss.NewStyle().Foreground(colorDimGray).Render("["+m.removeTarget.ID+"]") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render("[s] Soft delete") +
			lipgloss.NewStyle().Foreground(colorGray).Render(" — move to retired; keeps game data and backups") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("[f] Full delete") +
			lipgloss.NewStyle().Foreground(colorGray).Render(" — permanently erase game data and backups") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("[Esc] Cancel"))

	case instanceRemoveConfirmScreen:
		root, id := m.dataRoot, m.removeTarget.ID
		active := filepath.Join(root, "instances", id)
		retired := filepath.Join(root, "retired", id)
		backup := filepath.Join(root, "backups", id)
		switch m.removeAction {
		case "soft":
			title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("CONFIRM SOFT DELETE")
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(labelStyle.Render("INSTANCE  ") + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render("MOVE      ") + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(active) + "\n")
			b.WriteString(labelStyle.Render("TO        ") + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(retired) + "\n")
			b.WriteString(labelStyle.Render("BACKUPS   ") + "  " + lipgloss.NewStyle().Foreground(colorGray).Render("Preserved at "+backup) + "\n")
		case "restore":
			title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("CONFIRM RESTORE")
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(labelStyle.Render("INSTANCE  ") + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render("MOVE      ") + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(retired) + "\n")
			b.WriteString(labelStyle.Render("TO        ") + "  " + lipgloss.NewStyle().Foreground(colorGray).Render(active) + "\n")
		case "delete":
			src := active
			if m.removeFromTrash {
				src = retired
			}
			title := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("CONFIRM PERMANENT DELETE")
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("This cannot be undone.") + "\n\n")
			b.WriteString(labelStyle.Render("INSTANCE  ") + "  " + lipgloss.NewStyle().Foreground(colorAccent2).Render(m.removeTarget.Name) + "\n")
			b.WriteString(labelStyle.Render("ERASE     ") + "  " + lipgloss.NewStyle().Foreground(colorRed).Render(src) + "\n")
			b.WriteString(labelStyle.Render("ERASE     ") + "  " + lipgloss.NewStyle().Foreground(colorRed).Render(backup) + "\n")
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("[y] Confirm") +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancel"))

	case instanceRemoveRunningScreen:
		title := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("UPDATING INSTANCE")
		div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(title + "\n" + div + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Please wait; do not close mcTUI."))

	case instanceRemoveResultScreen:
		if m.removeErr != nil {
			title := lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("INSTANCE OPERATION FAILED")
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render(m.removeErr.Error()) + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Check instances/ and retired/ before retrying."))
		} else {
			title := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("INSTANCE UPDATED")
			div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
			b.WriteString(title + "\n" + div + "\n\n")
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Operation completed successfully."))
		}
		b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[Enter/Esc] Back"))
	}

	return b.String()
}
