package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/java"
	"mcTUI/internal/mojang"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Adapted from msTTui's directorypicker: embedded in the dashboard panel,
// with mcTUI colors and directory-only selection (no separate popup).
func (m *Model) openResetPicker() {
	m.pickerForJava = false
	m.pickerForMods = false
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
			} else if m.pickerForMods && entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".jar") {
				m.pickerEntries = append(m.pickerEntries, entry)
			} else if m.pickerForPacks && entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(entry.Name()), ".zip") {
				m.pickerEntries = append(m.pickerEntries, entry)
			} else if m.pickerForJava && isJavaEntry(m.pickerPath, entry) {
				m.pickerEntries = append(m.pickerEntries, entry)
			}
		}
	}
	m.pickerCursor = 0
}

func (m *Model) openJavaPicker() {
	m.pickerForJava = true
	m.pickerForMods = false
	m.pickerShowHidden = true
	if m.instance.JavaPath != "" {
		m.pickerPath = filepath.Dir(m.instance.JavaPath)
	} else if path, err := exec.LookPath("java"); err == nil {
		m.pickerPath = filepath.Dir(path)
	} else if home, err := os.UserHomeDir(); err == nil {
		m.pickerPath = home
	} else {
		m.pickerPath = string(filepath.Separator)
	}
	m.loadPickerDir()
	m.state = javaPickerScreen
}

func isJavaEntry(dir string, entry os.DirEntry) bool {
	name := "java"
	if runtime.GOOS == "windows" {
		name = "java.exe"
	}
	if entry.Name() != name || entry.IsDir() {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, entry.Name()))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0
}

func (m Model) updateJavaPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.state = configScreen
	case "d":
		in := m.instance
		in.JavaPath = ""
		m.pickerErr = instances.UpdateSettings(m.dataRoot, in)
		if m.pickerErr == nil {
			m.instance = in
			m.javaOverrideMajor = 0
			m.state = configScreen
		}
	case "up", "k":
		m.pickerCursor = max(0, m.pickerCursor-1)
	case "down", "j":
		m.pickerCursor = min(max(0, len(m.pickerEntries)-1), m.pickerCursor+1)
	case "left", "h":
		if parent := filepath.Dir(m.pickerPath); parent != m.pickerPath {
			m.pickerPath = parent
			m.loadPickerDir()
		}
	case "right", "l", "enter":
		if len(m.pickerEntries) > 0 {
			entry := m.pickerEntries[m.pickerCursor]
			path := filepath.Join(m.pickerPath, entry.Name())
			if entry.IsDir() {
				m.pickerPath = path
				m.loadPickerDir()
			} else if key == "enter" {
				major := java.VersionAt(path)
				required := mojang.RequiredJavaVersion(m.runtimeDir(), m.versionSelect)
				switch {
				case major == 0:
					m.pickerErr = fmt.Errorf("%s", i18n.T("error.java_not_executable", path))
				case required > 0 && major < required:
					m.pickerErr = fmt.Errorf("%s", i18n.T("error.java_too_old", major, m.versionSelect, required))
				default:
					in := m.instance
					in.JavaPath = path
					if err := instances.UpdateSettings(m.dataRoot, in); err != nil {
						m.pickerErr = err
					} else {
						m.instance = in
						m.javaOverrideMajor = major
						m.state = configScreen
					}
				}
			}
		}
	}
	return m, nil
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
	hidden := i18n.T("reset.hidden_off")
	if m.pickerShowHidden {
		hidden = i18n.T("reset.hidden_on")
	}
	lines := []string{i18n.T("reset.choose_backup"), i18n.T("reset.current", m.pickerPath),
		i18n.T("reset.use_default", m.resetBackupRoot), "",
		i18n.T("reset.directories_hint"), "[Ctrl+H] " + hidden}
	if m.pickerForJava {
		required := mojang.RequiredJavaVersion(m.runtimeDir(), m.versionSelect)
		requirement := i18n.T("reset.java_requirement")
		if required > 0 {
			requirement = i18n.T("reset.requires_java", m.versionSelect, required)
		}
		lines = []string{i18n.T("reset.choose_java"), i18n.T("reset.current", m.pickerPath),
			i18n.T("reset.java_auto"), requirement, "",
			i18n.T("reset.directories_hint"), i18n.T("reset.java_select")}
	}
	if m.pickerErr != nil {
		return append(lines, i18n.T("reset.error", displayError(m.pickerErr)))
	}
	if len(m.pickerEntries) == 0 {
		return append(lines, i18n.T("reset.no_dirs"))
	}
	visible := max(3, min(7, m.height-32))
	start := max(0, m.pickerCursor-visible+2)
	end := min(len(m.pickerEntries), start+visible)
	if start > 0 {
		lines = append(lines, i18n.T("reset.previous_folders", start))
	}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.pickerCursor {
			prefix = "> "
		}
		icon := "▸ "
		if m.pickerForJava && !m.pickerEntries[i].IsDir() {
			icon = "● "
		}
		lines = append(lines, prefix+icon+m.pickerEntries[i].Name())
	}
	if end < len(m.pickerEntries) {
		lines = append(lines, i18n.T("reset.more_folders", len(m.pickerEntries)-end))
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
		case strings.HasPrefix(line, i18n.T("reset.error", "")):
			lines[i] = lipgloss.NewStyle().Foreground(colorRed).Render(line)
		case strings.HasPrefix(line, "[d]") || strings.HasPrefix(line, "[Ctrl+H]"):
			lines[i] = lipgloss.NewStyle().Foreground(colorAccent).Render(line)
		case slices.Contains([]string{i18n.T("reset.choose_backup"), i18n.T("reset.directories_hint")}, line):
			lines[i] = lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(line)
		}
	}
	title, controls := i18n.T("reset.picker_title"), i18n.T("reset.picker_controls")
	if m.pickerForJava {
		title, controls = i18n.T("reset.java_picker_title"), i18n.T("reset.java_picker_controls")
	}
	return lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render(title) + "\n" +
		lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 62)) + "\n\n" +
		strings.Join(lines, "\n") + "\n\n" +
		lipgloss.NewStyle().Foreground(colorAccent2).Render(controls)
}
