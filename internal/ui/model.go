package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/config"
	"mcTUI/internal/updater"
	"mcTUI/internal/worlds"
)

type screenState int

const (
	menuScreen screenState = iota
	nameScreen
	versionsScreen
	worldsScreen
	confirmScreen
)

// menuOptionsCount must match len(menuOptions) in View(). Centralized
// here so Update()'s cursor bounds check doesn't drift from the menu's
// actual length if options are added/removed.
const menuOptionsCount = 7

// Model is the root Bubble Tea model for mcTUI.
type Model struct {
	state          screenState
	cursorMenu     int
	cursorVersions int
	cursorWorlds   int

	username      string
	versionSelect string
	memoryMB      int
	modloader     string
	versions      []string
	worlds        []worlds.Info
	roadmap       []string
	latestRelease updater.Release // zero value = no update available

	input textinput.Model
	play  bool

	width     int
	height    int
	javaMajor int // major version of the default "java" in PATH; 0 = not found
}

// New builds the initial model from the loaded config, available
// versions, roadmap content, detected default Java version, and the
// latest release info from GitHub (for update notifications).
func New(versions []string, cfg config.Data, roadmap []string, javaMajor int, latest updater.Release) Model {
	ti := textinput.New()
	ti.Placeholder = "Type your name..."
	ti.Focus()
	ti.CharLimit = 16
	ti.Width = 30
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorCyan)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorWhite)

	return Model{
		state:         menuScreen,
		cursorMenu:    0,
		username:      cfg.Username,
		versionSelect: cfg.Version,
		modloader:     cfg.Modloader,
		memoryMB:      cfg.MemoryMB,
		versions:      versions,
		roadmap:       roadmap,
		latestRelease: latest,
		input:         ti,
		play:          false,
		javaMajor:     javaMajor,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// PlaySelected reports whether the user chose to launch the game, after
// the Bubble Tea program exits.
func (m Model) PlaySelected() bool {
	return m.play
}

// Username, VersionSelect, Modloader, and MemoryMB expose the final
// values needed by launcher.Launch after the program exits.
func (m Model) Username() string      { return m.username }
func (m Model) VersionSelect() string { return m.versionSelect }
func (m Model) Modloader() string     { return m.modloader }
func (m Model) MemoryMB() int         { return m.memoryMB }
