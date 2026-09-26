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
	lines := []string{"Mods locales — " + modText(m.instance.Name), modText(m.instance.MinecraftVersion + " / " + m.instance.Loader + " " + m.instance.LoaderVersion), modText(m.modStore.GameDir), "Aviso: retirar/desactivar mods puede dañar mundos dependientes.", "Cierra Minecraft externo: su ejecución no está verificada."}
	switch m.state {
	case modsWorkingScreen:
		lines = append(lines, "Procesando archivos locales…")
	case modsConfirmScreen:
		lines = append(lines, "¿Eliminar físicamente "+modText(m.modTarget.File)+"?", "Config y dependencias se conservan. [y] Confirmar / [n/Esc] Cancelar")
	case modsPickerScreen:
		lines = append(lines, "Agregar .jar: "+modText(m.pickerPath), "[↑/↓] Elegir [→/Enter] Abrir/elegir [←] Padre [Ctrl+H] Ocultos [Esc] Volver")
		if m.pickerErr != nil {
			lines = append(lines, modText(m.pickerErr.Error()))
		}
		count := max(1, m.height-len(lines)-2)
		start := max(0, m.pickerCursor-count+1)
		for i := start; i < min(len(m.pickerEntries), start+count); i++ {
			prefix := "  "
			if i == m.pickerCursor {
				prefix = "> "
			}
			lines = append(lines, prefix+modText(m.pickerEntries[i].Name()))
		}
	default:
		lines = append(lines, "[Space/Enter] Alternar [a] Agregar [d] Retirar [r] Refrescar [Esc] Volver")
		if m.modBusy != nil {
			lines = append(lines, "BLOQUEADO: "+modText(m.modBusy.Error()))
		}
		if m.modErr != nil {
			lines = append(lines, "Error: "+modText(m.modErr.Error()))
		}
		count := max(1, m.height-len(lines)-3)
		start := max(0, m.modCursor-count+1)
		for i := start; i < min(len(m.modItems), start+count); i++ {
			item := m.modItems[i]
			state := "Deshabilitado"
			if item.Enabled {
				state = "Habilitado"
			}
			if item.Metadata.ID == "" {
				state += " / Desconocido"
			}
			if item.Problem != "" {
				state += " / Corrupto"
			}
			if item.Duplicate {
				state += " / Duplicado"
			}
			if item.Changed {
				state += " / Modificado externamente"
			}
			origin := "Externo/no gestionado"
			if item.Managed {
				origin = item.Origin
			}
			prefix := "  "
			if i == m.modCursor {
				prefix = "> "
			}
			lines = append(lines, prefix+modText(fmt.Sprintf("%s [%s] %s %s %s (%s)", item.File, state, item.Metadata.Name, item.Metadata.Version, item.Metadata.Environment, origin)))
		}
		lines = append(lines, fmt.Sprintf("%d archivos · ↑/↓ desplazar", len(m.modItems)))
	}
	width := max(20, m.width)
	for i := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(width).Render(lines[i])
	}
	return strings.Join(lines, "\n")
}
