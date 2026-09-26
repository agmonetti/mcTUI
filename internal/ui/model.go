package ui

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/config"
	"mcTUI/internal/instances"
	"mcTUI/internal/java"
	"mcTUI/internal/launcher"
	"mcTUI/internal/mods"
	"mcTUI/internal/resetconfigs"
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
	downloadScreen // nueva pantalla: descarga en tiempo real dentro del TUI
	resetModeScreen
	resetPickerScreen
	resetConfirmScreen
	resetBackupListScreen
	resetReplaceConfirmScreen
	resetRetentionScreen
	resetRestoreConfirmScreen
	resetRunningScreen
	resetResultScreen
	javaPickerScreen
	instancesScreen
	instanceNameScreen
	instanceLoaderScreen
	instanceCopyInspectScreen
	instanceCopySelectScreen
	instanceCopyConfirmScreen
	instanceCopyRunningScreen
	instanceCopyResultScreen
	instanceCopyCleanupConfirmScreen
	instanceTrashScreen
	instanceRemoveChoiceScreen
	instanceRemoveConfirmScreen
	instanceRemoveRunningScreen
	instanceRemoveResultScreen
	gameStartingScreen
	gameRunningScreen
	gameResultScreen
	modsScreen
	modsPickerScreen
	modsConfirmScreen
	modsWorkingScreen
)

// menuOptionsCount must match len(menuOptions) in View(). Centralized
// here so Update()'s cursor bounds check doesn't drift from the menu's
// actual length if options are added/removed.
const menuOptionsCount = 9

// DownloadDoneMsg señala que la descarga finalizó completamente.
type DownloadDoneMsg struct {
	Prepared   launcher.Prepared
	JavaBinary string
	Err        error
}

type GameStartedMsg struct {
	Game *launcher.Game
	Err  error
}

type GameExitedMsg struct {
	Code int
	Err  error
}

// ResetDoneMsg is independent of the download/launch flow.
type ResetDoneMsg struct {
	Result resetconfigs.Result
	Err    error
}

type InstanceCopyInspectDoneMsg struct {
	Items []instances.CopyItem
	Err   error
}

type InstanceCopyDoneMsg struct {
	Result instances.CopyResult
	Err    error
}

type InstanceCopyCleanupDoneMsg struct {
	Err error
}

type InstanceRemovalDoneMsg struct {
	Err error
}

// Model is the root Bubble Tea model for mcTUI.
type LaunchConfig struct {
	GameDir, RuntimeDir          string
	Username, Version, Modloader string
	LoaderVersion                string
	MemoryMB                     int
	JavaPath                     string
}

type Model struct {
	state          screenState
	cursorMenu     int
	cursorVersions int
	cursorWorlds   int

	username              string
	versionSelect         string
	memoryMB              int
	modloader             string
	versions              []string
	worlds                []worlds.Info
	roadmap               []string
	dataRoot              string
	instanceList          []instances.Instance
	instance              instances.Instance
	instanceErr           error
	instanceCursor        int
	instanceRenameID      string
	creatingInstance      bool
	copyingInstance       bool
	creationVersion       string
	creationLoaderVersion string
	copySource            string
	copySourceID          string
	copyLoader            string
	copyItems             []instances.CopyItem
	copySelected          map[string]bool
	copyCursor            int
	copyName              string
	copyStage             string
	copyErr               error
	copyResult            instances.CopyResult
	copyCleaning          bool
	retiredList           []instances.Instance
	retiredCursor         int
	removeTarget          instances.Instance
	removeAction          string
	removeFromTrash       bool
	removeErr             error
	pickerForJava         bool
	pickerForMods         bool
	modStore              mods.Store
	modItems              []mods.Mod
	modCursor             int
	modErr                error
	modBusy               error
	modRequest            uint64
	modTarget             mods.Mod
	launch                LaunchConfig
	latestRelease         updater.Release // zero value = no update available

	// Download progress (usado en downloadScreen)
	dlProgressFrame int
	dlErr           error
	dlFinished      bool
	dlContext       context.Context
	dlCancel        context.CancelFunc

	resetDir         string
	resetInstance    string
	resetBackupRoot  string
	resetFound       []string
	resetMode        resetconfigs.Mode
	resetBackupDir   string
	pickerPath       string
	pickerEntries    []os.DirEntry
	pickerCursor     int
	pickerShowHidden bool
	pickerErr        error
	resetChoice      int
	resetBackups     []resetconfigs.BackupInfo
	resetBackupIndex int
	resetBackup      resetconfigs.BackupInfo
	resetConflicts   []string
	resetReplace     bool
	resetRetention   resetconfigs.RestoreMode
	resetIsRestore   bool
	resetResult      resetconfigs.Result
	resetErr         error
	resetScroll      int

	input           textinput.Model
	gameErr         error
	gameExitCode    int
	gameLog         string
	activeGame      *launcher.Game
	gameKillConfirm bool
	gameKillErr     error

	width             int
	height            int
	javaOverrideMajor int // major version of the selected instance override
}

// New builds the initial model from the loaded config, available versions,
// roadmap content, and latest release info from GitHub.
func New(versions []string, cfg config.Data, roadmap []string, latest updater.Release) Model {
	ti := textinput.New()
	ti.Placeholder = "Type your name..."
	ti.Focus()
	ti.CharLimit = 16
	ti.Width = 30
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorBlue)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorAccent2)

	m := Model{
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
	}
	m.loadInstanceSelection()
	return m
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) loadInstanceSelection() {
	m.dataRoot, m.instanceErr = config.DataDirChecked()
	if m.instanceErr != nil {
		return
	}
	m.instanceList, m.instanceErr = instances.List(m.dataRoot)
	if m.instanceErr != nil {
		return
	}
	m.instance, m.instanceErr = instances.Selected(m.dataRoot)
	if m.instanceErr != nil {
		return
	}
	if m.instance.Kind == "legacy" {
		m.instance.MinecraftVersion = m.versionSelect
		m.instance.Loader = m.modloader
		m.instance.MemoryMB = m.memoryMB
	} else {
		m.versionSelect = m.instance.MinecraftVersion
		m.modloader = m.instance.Loader
		m.memoryMB = m.instance.MemoryMB
	}
	m.javaOverrideMajor = java.VersionAt(m.instance.JavaPath)
}

func (m Model) isManaged() bool { return m.instance.Kind == "managed" }

func (m Model) gameDir() string {
	path, err := instances.GameDir(m.dataRoot, m.instance)
	if err != nil {
		return "(unavailable)"
	}
	return path
}

func (m Model) runtimeDir() string {
	path, err := instances.RuntimeDir(m.dataRoot, m.instance)
	if err != nil {
		return ""
	}
	return path
}

func (m *Model) setInstance(in instances.Instance) error {
	if err := instances.Select(m.dataRoot, in.ID); err != nil {
		return err
	}
	m.instance = in
	if in.Kind == "legacy" {
		cfg := config.Load()
		m.versionSelect, m.modloader, m.memoryMB = cfg.Version, cfg.Modloader, cfg.MemoryMB
	} else {
		m.versionSelect, m.modloader, m.memoryMB = in.MinecraftVersion, in.Loader, in.MemoryMB
	}
	m.javaOverrideMajor = java.VersionAt(in.JavaPath)
	return nil
}

func (m Model) launchConfig() (LaunchConfig, error) {
	if m.dataRoot == "" || m.instance.ID == "" {
		return LaunchConfig{}, fmt.Errorf("no valid instance is selected")
	}
	gameDir, err := instances.GameDir(m.dataRoot, m.instance)
	if err != nil {
		return LaunchConfig{}, err
	}
	runtimeDir, err := instances.RuntimeDir(m.dataRoot, m.instance)
	if err != nil {
		return LaunchConfig{}, err
	}
	if m.isManaged() && (m.versionSelect != m.instance.MinecraftVersion || m.modloader != m.instance.Loader || m.instance.Loader == "Fabric" && m.instance.LoaderVersion == "") {
		return LaunchConfig{}, fmt.Errorf("managed Minecraft and loader versions must match instance metadata")
	}
	return LaunchConfig{GameDir: gameDir, RuntimeDir: runtimeDir, Username: m.username, Version: m.versionSelect, Modloader: m.modloader, LoaderVersion: m.instance.LoaderVersion, MemoryMB: m.memoryMB, JavaPath: m.instance.JavaPath}, nil
}
