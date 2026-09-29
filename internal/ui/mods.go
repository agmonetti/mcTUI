package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/mods"
)

type modsDoneMsg struct {
	Request   uint64
	Items     []mods.Mod
	Err, Busy error
}

type modHistoryMsg struct {
	Request   uint64
	ProjectID string
	Entries   []mods.HistoryEntry
	Err       error
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
	if m.state == modsRollbackScreen {
		switch key {
		case "y", "Y":
			if m.modBusy != nil {
				m.modErr = m.modBusy
				m.state = modsScreen
				return m, nil
			}
			store, project := m.modStore, m.modRollback.ProjectID
			m.modRequest++
			request := m.modRequest
			m.state = modsWorkingScreen
			return m, func() tea.Msg {
				err := store.Rollback(project)
				items, scanErr := store.Scan()
				return modsDoneMsg{request, items, errors.Join(err, scanErr), store.CheckBusy()}
			}
		case "esc", "n", "N":
			m.state = modsScreen
		}
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
		m.state = modrinthHubScreen
	case "up", "k":
		m.modCursor = max(0, m.modCursor-1)
	case "down", "j":
		m.modCursor = min(max(0, len(m.modItems)-1), m.modCursor+1)
	case "r":
		return m.runMods("", mods.Mod{}, "")
	case "c":
		{
			channels := []string{"release", "beta", "alpha"}
			i := 0
			for n, channel := range channels {
				if channel == m.instance.ModChannel {
					i = (n + 1) % len(channels)
					break
				}
			}
			previous := m.instance.ModChannel
			m.instance.ModChannel = channels[i]
			if m.instance.Kind == "managed" {
				if err := instances.UpdateSettings(m.dataRoot, m.instance); err != nil {
					m.modErr = err
				} else {
					m.modErr = nil
				}
			} else {
				cfg := config.Load()
				cfg.ModChannel = m.instance.ModChannel
				m.modErr = config.Save(cfg)
			}
			if m.modErr != nil {
				m.instance.ModChannel = previous
			}
		}
	case "s", "/":
		if m.modBusy != nil {
			m.modErr = m.modBusy
			break
		}
		m.catalogQuery = ""
		m.catalogOffset = 0
		m.catalogResults = modrinth.SearchResponse{}
		m.catalogLoading = true
		m.catalogErr = nil
		m.startCatalogRead()
		m.state = modrinthSearchScreen
		return m, m.catalogSearch()
	case "u":
		if m.modBusy != nil {
			m.modErr = m.modBusy
			break
		}
		m.catalogErr = nil
		m.startCatalogRead()
		m.state = modrinthWorkingScreen
		return m, m.catalogCheckUpdates()
	case "b":
		if m.modBusy != nil {
			m.modErr = m.modBusy
			break
		}
		if len(m.modItems) > 0 && m.modItems[m.modCursor].ProjectID != "" {
			project := m.modItems[m.modCursor].ProjectID
			store := m.modStore
			m.modRequest++
			request := m.modRequest
			m.state = modsWorkingScreen
			return m, func() tea.Msg {
				entries, err := store.History()
				return modHistoryMsg{request, project, entries, err}
			}
		}
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

	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("mods.title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", max(46, m.centerWidth()-4)))
	b.WriteString(title + "\n" + div + "\n\n")

	b.WriteString(labelStyle.Render(i18n.T("common.instance_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorAccent2).Bold(true).Render(modText(m.instance.Name)) + "\n")
	b.WriteString(labelStyle.Render(i18n.T("common.version_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.instance.MinecraftVersion+" / "+m.instance.Loader+" "+m.instance.LoaderVersion)) + "\n")
	pathWidth := max(20, m.centerWidth()-16)
	b.WriteString(labelStyle.Render(i18n.T("common.path_label")) + "  " +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(formatDisplayPath(modText(m.modStore.GameDir), pathWidth)) + "\n\n")

	switch m.state {
	case modsRollbackScreen:
		entry := m.modRollback
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(i18n.T("mods.rollback_title")) + "\n\n")
		b.WriteString(i18n.T("mods.rollback_instance", modText(m.instance.Name)) + "\n")
		b.WriteString(i18n.T("mods.rollback_current", modText(entry.New.File), modText(entry.New.VersionID)) + "\n")
		b.WriteString(i18n.T("mods.rollback_previous", modText(entry.Previous.File), modText(entry.Previous.VersionID)) + "\n")
		b.WriteString(i18n.T("mods.rollback_date", modText(entry.Date)) + "\n")
		backup := filepath.Join(filepath.Dir(m.modStore.Registry), "backups", "mods_replaced", entry.Backup)
		pathWidth := max(20, m.centerWidth()-lipgloss.Width(i18n.T("mods.rollback_backup", ""))-2)
		b.WriteString(i18n.T("mods.rollback_backup", formatDisplayPath(modText(backup), pathWidth)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("mods.rollback_warning")) + "\n\n")
		b.WriteString(i18n.T("mods.rollback_hint"))

	case modsWorkingScreen:
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(i18n.T("mods.processing")) + "\n")

	case modsConfirmScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("mods.confirm_delete_title")) + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("mods.file_label")) + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(m.modTarget.File)) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("mods.confirm_delete_desc")) + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render(i18n.T("mods.delete_hint")) +
			"   " + lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("common.cancel")))

	case modsPickerScreen:
		warn := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 46))
		b.WriteString(warn + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("mods.add_title")) + "\n\n")
		b.WriteString(labelStyle.Render(i18n.T("mods.picker_path")) + "  " +
			lipgloss.NewStyle().Foreground(colorGray).Render(modText(m.pickerPath)) + "\n\n")
		if m.pickerErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(displayError(m.pickerErr))) + "\n\n")
		}
		available := m.centerInnerHeight() - 14
		if m.pickerErr != nil {
			available -= 2
		}
		count := max(1, available)
		start := max(0, m.pickerCursor-count+1)
		for i := start; i < min(len(m.pickerEntries), start+count); i++ {
			entry := m.pickerEntries[i]
			if i == m.pickerCursor {
				b.WriteString(cursorStyle.Render("► "+modText(entry.Name())) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+modText(entry.Name())) + "\n")
			}
		}
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDimGray).Render(i18n.T("mods.picker_hint")))

	default:
		if m.modBusy != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("mods.locked", modText(displayError(m.modBusy)))) + "\n\n")
		}
		if m.modErr != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ "+modText(displayError(m.modErr))) + "\n\n")
		}
		if len(m.modItems) == 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+i18n.T("mods.empty")) + "\n\n")
		}
		available := m.centerInnerHeight() - 11
		if m.modBusy != nil {
			available -= 2
		}
		if m.modErr != nil {
			available -= 2
		}
		if len(m.modItems) == 0 {
			available -= 2
		}
		count := max(1, available)
		start := max(0, m.modCursor-count+1)
		for i := start; i < min(len(m.modItems), start+count); i++ {
			item := m.modItems[i]

			stateStr := lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("mods.disabled"))
			if item.Enabled {
				stateStr = lipgloss.NewStyle().Foreground(colorGreen).Render(i18n.T("mods.enabled"))
			}
			var flags []string
			if item.Metadata.ID == "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("mods.unknown")))
			}
			if item.Problem != "" {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render(i18n.T("mods.corrupt")))
			}
			if item.Duplicate {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorRed).Render(i18n.T("mods.duplicate")))
			}
			if item.Changed {
				flags = append(flags, lipgloss.NewStyle().Foreground(colorAccent).Render(i18n.T("mods.modified")))
			}

			nameStr := item.Metadata.Name
			if nameStr == "" {
				nameStr = item.File
			}
			row := stateStr + "  " + lipgloss.NewStyle().Foreground(colorWhite).Render(modText(nameStr))
			if len(flags) > 0 {
				row += "  " + strings.Join(flags, " / ")
			}
			rowWidth := max(44, m.centerWidth()-6)
			if i == m.modCursor {
				b.WriteString(cursorStyle.Render("► ") + lipgloss.NewStyle().MaxWidth(rowWidth).Render(row) + "\n")
			} else {
				b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  ") + lipgloss.NewStyle().MaxWidth(rowWidth).Render(row) + "\n")
			}
		}
		b.WriteString("\n")
		footerRaw := i18n.T("mods.footer", len(m.modItems), i18n.T("modrinth.channel."+m.catalogChannel()))
		for _, fl := range strings.Split(footerRaw, "\n") {
			b.WriteString(lipgloss.NewStyle().Foreground(colorDimGray).Render("  "+fl) + "\n")
		}
	}
	return b.String()
}
