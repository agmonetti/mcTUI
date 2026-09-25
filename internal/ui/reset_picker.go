package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Adapted from msTTui's directorypicker: embedded in the dashboard panel,
// with mcTUI colors and directory-only selection (no separate popup).
func (m *Model) openResetPicker() {
	m.pickerPath = m.resetDir
	m.pickerShowHidden = false
	m.pickerCursor = 0
	m.loadPickerDir()
	m.state = resetPickerScreen
}

func (m *Model) loadPickerDir() {
	entries, err := os.ReadDir(m.pickerPath)
	m.pickerErr = err
	m.pickerEntries = nil
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && (m.pickerShowHidden || !strings.HasPrefix(entry.Name(), ".")) { // excludes symlinks; engine checks ancestors again
				m.pickerEntries = append(m.pickerEntries, entry)
			}
		}
	}
	m.pickerCursor = 0
}

func (m Model) updateResetPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.state = resetModeScreen
	case "d":
		m.resetBackupDir = ""
		m.state = resetConfirmScreen
	case "ctrl+h":
		m.pickerShowHidden = !m.pickerShowHidden
		m.loadPickerDir()
	case "enter":
		if m.pickerErr == nil {
			m.resetBackupDir = m.pickerPath
			m.state = resetConfirmScreen
		}
	case "up", "k":
		m.pickerCursor = max(0, m.pickerCursor-1)
	case "down", "j":
		if len(m.pickerEntries) > 0 {
			m.pickerCursor = min(len(m.pickerEntries)-1, m.pickerCursor+1)
		}
	case "right", "l":
		if len(m.pickerEntries) > 0 {
			m.pickerPath = filepath.Join(m.pickerPath, m.pickerEntries[m.pickerCursor].Name())
			m.loadPickerDir()
		}
	case "left", "h":
		if parent := filepath.Dir(m.pickerPath); parent != m.pickerPath {
			m.pickerPath = parent
			m.loadPickerDir()
		}
	case "home":
		m.pickerCursor = 0
	case "end":
		m.pickerCursor = max(0, len(m.pickerEntries)-1)
	}
	return m, nil
}

func (m Model) pickerLines() []string {
	hidden := "hidden off"
	if m.pickerShowHidden {
		hidden = "hidden on"
	}
	lines := []string{"Choose backup location", "Current: " + m.pickerPath,
		"[d] Use default: " + filepath.Join(m.resetDir, "mctui-backups"), "",
		"DIRECTORIES  [→] Open  [←] Parent", "[Ctrl+H] " + hidden}
	if m.pickerErr != nil {
		return append(lines, "Error: "+m.pickerErr.Error())
	}
	if len(m.pickerEntries) == 0 {
		return append(lines, "  (no subdirectories)")
	}
	visible := max(3, min(7, m.height-32))
	start := max(0, m.pickerCursor-visible+2)
	end := min(len(m.pickerEntries), start+visible)
	if start > 0 {
		lines = append(lines, fmt.Sprintf("↑ %d previous folders", start))
	}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.pickerCursor {
			prefix = "> "
		}
		lines = append(lines, prefix+"▸ "+m.pickerEntries[i].Name())
	}
	if end < len(m.pickerEntries) {
		lines = append(lines, fmt.Sprintf("↓ %d more folders", len(m.pickerEntries)-end))
	}
	return lines
}

func (m Model) pickerPanelContent() string {
	lines := wrapResetLines(m.pickerLines(), m.resetWidth())
	visible := m.resetVisibleLines()
	if len(lines) > visible {
		lines = lines[:visible]
	}
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "> "):
			lines[i] = lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(line)
		case strings.HasPrefix(line, "Error:"):
			lines[i] = lipgloss.NewStyle().Foreground(colorRed).Render(line)
		case strings.HasPrefix(line, "[d]") || strings.HasPrefix(line, "[Ctrl+H]"):
			lines[i] = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		case slices.Contains([]string{"Choose backup location", "DIRECTORIES  [→] Open  [←] Parent"}, line):
			lines[i] = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(line)
		}
	}
	return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("BACKUP DESTINATION") + "\n" +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 62)) + "\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorAccent2).Render("[↑/↓] Navigate  [Ctrl+H] Hidden  [Enter] Select  [Esc] Cancel")
}
