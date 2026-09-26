package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mcTUI/internal/instances"
	"mcTUI/internal/mods"
)

type modsDoneMsg struct {
	Request   uint64
	Items     []mods.Mod
	Err, Busy error
}

func (m Model) openMods() (tea.Model, tea.Cmd) {
	dir, err := instances.GameDir(m.dataRoot, m.instance)
	m.modErr = err
	m.modItems = nil
	m.modCursor = 0
	m.state = modsScreen
	if err != nil {
		return m, nil
	}
	m.modStore = mods.Store{GameDir: dir, Registry: filepath.Join(m.dataRoot, "instances", m.instance.ID, "mods.json")}
	return m.runMods("", mods.Mod{}, "")
}
func (m Model) runMods(action string, target mods.Mod, source string) (tea.Model, tea.Cmd) {
	m.modRequest++
	request := m.modRequest
	store := m.modStore
	m.state = modsWorkingScreen
	return m, func() tea.Msg {
		var err error
		switch action {
		case "add":
			err = store.Add(source)
		case "toggle":
			err = store.Toggle(target)
		case "remove":
			err = store.Remove(target, true)
		}
		items, scanErr := store.Scan()
		return modsDoneMsg{request, items, errors.Join(err, scanErr), store.CheckBusy()}
	}
}
func (m Model) updateModsKey(key string) (tea.Model, tea.Cmd) {
	if m.state == modsWorkingScreen {
		return m, nil
	}
	if m.state == modsConfirmScreen {
		switch key {
		case "y", "Y":
			return m.runMods("remove", m.modTarget, "")
		case "esc", "n", "N":
			m.state = modsScreen
		}
		return m, nil
	}
	if m.state == modsPickerScreen {
		switch key {
		case "esc", "q":
			m.state = modsScreen
			m.pickerForMods = false
		case "up", "k":
			m.pickerCursor = max(0, m.pickerCursor-1)
		case "down", "j":
			m.pickerCursor = min(max(0, len(m.pickerEntries)-1), m.pickerCursor+1)
		case "left", "h":
			m.pickerPath = filepath.Dir(m.pickerPath)
			m.loadPickerDir()
		case "ctrl+h":
			m.pickerShowHidden = !m.pickerShowHidden
			m.loadPickerDir()
		case "enter", "right", "l":
			if len(m.pickerEntries) > 0 {
				e := m.pickerEntries[m.pickerCursor]
				p := filepath.Join(m.pickerPath, e.Name())
				if e.IsDir() {
					m.pickerPath = p
					m.loadPickerDir()
				} else if key == "enter" {
					m.pickerForMods = false
					return m.runMods("add", mods.Mod{}, p)
				}
			}
		}
		return m, nil
	}
	switch key {
	case "esc", "q":
		m.state = menuScreen
	case "up", "k":
		m.modCursor = max(0, m.modCursor-1)
	case "down", "j":
		m.modCursor = min(max(0, len(m.modItems)-1), m.modCursor+1)
	case "r":
		return m.runMods("", mods.Mod{}, "")
	case "a":
		if m.modBusy != nil {
			m.modErr = m.modBusy
			break
		}
		m.pickerForJava = false
		m.pickerForMods = true
		m.pickerShowHidden = false
		m.pickerPath, m.pickerErr = os.UserHomeDir()
		if m.pickerErr != nil {
			m.modErr = m.pickerErr
			break
		}
		m.loadPickerDir()
		m.state = modsPickerScreen
	case " ", "enter", "d":
		if m.modBusy != nil {
			m.modErr = m.modBusy
			break
		}
		if len(m.modItems) == 0 {
			break
		}
		target := m.modItems[m.modCursor]
		if key == "d" {
			m.modTarget = target
			m.state = modsConfirmScreen
		} else {
			return m.runMods("toggle", target, "")
		}
	}
	return m, nil
}
func modText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func (m Model) renderMods() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("LOCAL MODS")
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
	b.WriteString(title + "\n" + div + "\n\n")

	b.WriteString(labelStyle.Render("INSTANCE  ") + "  " +
		lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(m.instance.Name)) + "\n")
	b.WriteString(labelStyle.Render("VERSION   ") + "  " +
		lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.instance.MinecraftVersion+" / "+m.instance.Loader+" "+m.instance.LoaderVersion)) + "\n")
	b.WriteString(labelStyle.Render("PATH      ") + "  " +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(modText(m.modStore.GameDir)) + "\n\n")

	switch m.state {
	case modsWorkingScreen:
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("Processing local files…") + "\n")

	case modsConfirmScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("CONFIRM DELETE") + "\n\n")
		b.WriteString(labelStyle.Render("File: ") + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(m.modTarget.File)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Config and dependencies are preserved.") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("[y] Delete") +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancel"))

	case modsPickerScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render("ADD MOD (.jar)") + "\n\n")
		b.WriteString(labelStyle.Render("PATH  ") + "  " +
			lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.pickerPath)) + "\n\n")
		if m.pickerErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(m.pickerErr.Error())) + "\n\n")
		}
		count := max(1, m.height-12)
		start := max(0, m.pickerCursor-count+1)
		for i := start; i < min(len(m.pickerEntries), start+count); i++ {
			entry := m.pickerEntries[i]
			if i == m.pickerCursor {
				b.WriteString(cursorStyle.Render("► "+modText(entry.Name())) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+modText(entry.Name())) + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render("[↑/↓] Navigate  [→/Enter] Open/Select  [←] Parent  [Ctrl+H] Hidden  [Esc] Back"))

	default:
		if m.modBusy != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ LOCKED: "+modText(m.modBusy.Error())) + "\n\n")
		}
		if m.modErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(m.modErr.Error())) + "\n\n")
		}
		count := max(1, m.height-14)
		start := max(0, m.modCursor-count+1)
		for i := start; i < min(len(m.modItems), start+count); i++ {
			item := m.modItems[i]

			stateStr := lipgloss.NewStyle().Foreground(colorRed).Render("Disabled")
			if item.Enabled {
				stateStr = lipgloss.NewStyle().Foreground(colorGreen).Render("Enabled")
			}
			var flags []string
			if item.Metadata.ID == "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorGray).Render("Unknown"))
			}
			if item.Problem != "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("Corrupt"))
			}
			if item.Duplicate {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorRed).Render("Duplicate"))
			}
			if item.Changed {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorAccent).Render("Modified"))
			}

			nameStr := item.Metadata.Name
			if nameStr == "" {
				nameStr = item.File
			}
			row := stateStr + "  " + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(nameStr))
			if len(flags) > 0 {
				row += "  " + strings.Join(flags, " / ")
			}
			if i == m.modCursor {
				b.WriteString(cursorStyle.Render("► ") + lipgloss.NewStyle().MaxWidth(44).Render(row) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  ") + lipgloss.NewStyle().MaxWidth(44).Render(row) + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(
			fmt.Sprintf("%d files · [Space/Enter] Toggle  [a] Add  [d] Remove  [r] Refresh  [Esc] Back",
				len(m.modItems))))
	}

	return b.String()
}
