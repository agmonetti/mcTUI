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
		b.WriteString("RETIRED INSTANCES\n────────────────────────────────────────\n\n")
		if m.removeErr != nil {
			fmt.Fprintf(&b, "Error: %v\n\n", m.removeErr)
		}
		if len(m.retiredList) == 0 {
			b.WriteString("No retired instances.\n")
		}
		start, end := windowAround(m.retiredCursor, len(m.retiredList), 3, 8)
		for i := start; i < end; i++ {
			in := m.retiredList[i]
			marker := "  "
			if i == m.retiredCursor {
				marker = "► "
			}
			fmt.Fprintf(&b, "%s%s — %s %s [%s]\n", marker, in.Name, in.MinecraftVersion, in.Loader, in.ID)
		}
		b.WriteString("\n[↑/↓] Move  [Enter] Restore  [d] Delete permanently  [Esc] Back")
	case instanceRemoveChoiceScreen:
		fmt.Fprintf(&b, "REMOVE INSTANCE\n────────────────────────────────────────\n\n%s [%s]\n\n[s] Soft delete: move to retired instances; keep game data and backups.\n[f] Full delete: permanently erase game data and Reset backups.\n[Esc] Cancel", m.removeTarget.Name, m.removeTarget.ID)
	case instanceRemoveConfirmScreen:
		root, id := m.dataRoot, m.removeTarget.ID
		active := filepath.Join(root, "instances", id)
		retired := filepath.Join(root, "retired", id)
		backup := filepath.Join(root, "backups", id)
		switch m.removeAction {
		case "soft":
			fmt.Fprintf(&b, "CONFIRM SOFT DELETE\n\n%s [%s]\nMove %s\nto %s\nBackups stay at %s.\n", m.removeTarget.Name, id, active, retired, backup)
		case "restore":
			fmt.Fprintf(&b, "CONFIRM RESTORE\n\n%s [%s]\nMove %s\nto %s\n", m.removeTarget.Name, id, retired, active)
		case "delete":
			if m.removeFromTrash {
				active = retired
			}
			fmt.Fprintf(&b, "CONFIRM PERMANENT DELETE\n\n%s [%s]\nPermanently erase:\n  %s\n  %s\nThis cannot be undone.\n", m.removeTarget.Name, id, active, backup)
		}
		b.WriteString("\n[y] Confirm  [n/Esc] Cancel")
	case instanceRemoveRunningScreen:
		b.WriteString("UPDATING INSTANCE\n\nPlease wait; do not close mcTUI.")
	case instanceRemoveResultScreen:
		if m.removeErr != nil {
			fmt.Fprintf(&b, "INSTANCE OPERATION FAILED\n\n%v\n\nCheck instances/ and retired/ before retrying.", m.removeErr)
		} else {
			b.WriteString("INSTANCE UPDATED\n\nOperation completed.")
		}
		b.WriteString("\n\n[Enter/Esc] Back")
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(safeCopyText(b.String()))
}
