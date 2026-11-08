package ui

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/auth"
	"mcTUI/internal/config"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/java"
	"mcTUI/internal/launcher"
	"mcTUI/internal/modrinth"
	"mcTUI/internal/mods"
	"mcTUI/internal/packs"
	"mcTUI/internal/resetconfigs"
	"mcTUI/internal/roadmap"
	"mcTUI/internal/theme"
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
	modsRollbackScreen
	modrinthSearchScreen
	modrinthDetailScreen
	modrinthReviewScreen
	modrinthUpdatesScreen
	modrinthWorkingScreen
	configScreen
	modrinthHubScreen
	packListScreen
	packPickerScreen
	packConfirmScreen
	packSearchScreen
	packDetailScreen
	packReviewScreen
	packWorkingScreen
	modpackScreen
	accountsScreen
	deviceCodeScreen
	welcomeScreen
	roadmapScreen
)

// World-copy states are kept outside the instance-copy range.
const (
	worldCopyDestinationScreen screenState = 1000 + iota
	worldCopyConfirmScreen
	worldCopyRunningScreen
	worldCopyResultScreen
)

// menuOptionsCount must match len(menuOptions) in View().
const menuOptionsCount = 8

// configOptionsCount must match len(configOptions) in View() for the config submenu.
const configOptionsCount = 8

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

type FabricLoaderDoneMsg struct {
	Request          uint64
	MinecraftVersion string
	LoaderVersion    string
	Err              error
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
type WorldCopyDoneMsg struct {
	Folder string
	Err    error
}

// Model is the root Bubble Tea model for mcTUI.
type LaunchConfig struct {
	GameDir, RuntimeDir          string
	Username, Version, Modloader string
	LoaderVersion                string
	MemoryMB                     int
	JavaPath                     string
	UUID                         string
	AccessToken                  string
	UserType                     string
}

type Model struct {
	state          screenState
	cursorMenu     int
	cursorConfig   int
	cursorVersions int
	cursorWorlds   int

	authStorage         *auth.Storage
	accountsCursor      int
	accountsFromConfig  bool
	deviceCodeResp      *auth.DeviceCodeResponse
	deviceCodeCancel    context.CancelFunc
	deviceCodeErr       error
	deviceCodeCopied    bool
	deviceCodeExpiresIn int
	deviceCodeTick      int

	username             string
	versionSelect        string
	memoryMB             int
	modloader            string
	versions             []string
	worlds               []worlds.Info
	worldCopySource      worlds.Info
	worldCopyTargets     []instances.Instance
	worldCopyCursor      int
	worldCopyDestination instances.Instance
	worldCopyFolder      string
	worldCopyErr         error

	dataRoot               string
	instanceList           []instances.Instance
	instance               instances.Instance
	instanceErr            error
	instanceCursor         int
	instanceRenameID       string
	creatingInstance       bool
	copyingInstance        bool
	creationVersion        string
	creationLoaderVersion  string
	fabricLoaderRequest    uint64
	copySource             string
	copySourceID           string
	copyLoader             string
	copyItems              []instances.CopyItem
	copySelected           map[string]bool
	copyCursor             int
	copyName               string
	copyStage              string
	copyErr                error
	copyResult             instances.CopyResult
	copyCleaning           bool
	retiredList            []instances.Instance
	retiredCursor          int
	removeTarget           instances.Instance
	removeAction           string
	removeFromTrash        bool
	removeErr              error
	pickerForJava          bool
	pickerForMods          bool
	pickerForPacks         bool
	modStore               mods.Store
	modItems               []mods.Mod
	modCursor              int
	modErr                 error
	modBusy                error
	modRequest             uint64
	modTarget              mods.Mod
	modRollback            mods.HistoryEntry
	catalogQuery           string
	catalogResults         modrinth.SearchResponse
	catalogCursor          int
	catalogOffset          int
	catalogSortIndex       int
	catalogLoading         bool
	catalogPreviousScreen  screenState
	catalogRequest         uint64
	catalogProject         modrinth.Project
	catalogVersion         modrinth.Version
	catalogPlan            modrinth.InstallPlan
	catalogOptional        map[string]bool
	catalogUpdates         []modrinth.Version
	catalogUpdateRecords   []mods.Record
	catalogSelected        map[string]bool
	catalogCursorUpdate    int
	catalogErr             error
	catalogCancel          context.CancelFunc
	catalogContext         context.Context
	catalogEvents          chan tea.Msg
	catalogDownloaded      int64
	catalogTotal           int64
	catalogModifiedConfirm bool
	hubCursor              int
	packKind               packs.Kind
	packStore              packs.Store
	packItems              []packs.Pack
	packCursor             int
	packTarget             string
	packErr                error
	packRequest            uint64
	packEngine             string
	packQuery              string
	packResults            modrinth.SearchResponse
	packSearchCursor       int
	packOffset             int
	packSortIndex          int
	packProject            modrinth.Project
	packVersion            modrinth.Version
	packBusy               bool
	packContext            context.Context
	packCancel             context.CancelFunc
	packReturn             screenState
	mp                     modpackState
	launch                 LaunchConfig
	latestRelease          updater.Release // zero value = no update available

	// Download progress (usado en downloadScreen)
	dlProgressFrame int
	dlStage         downloadStageMsg
	dlEvents        chan downloadStageMsg
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

	roadmapData    roadmap.Roadmap
	roadmapCursor  int
	roadmapOnline  bool
	roadmapLoading bool
}

// New builds the initial model from the loaded config, available versions,
// and latest release info from GitHub.
func New(versions []string, cfg config.Data, latest updater.Release) Model {
	activeTheme := theme.Get(cfg.Theme, "")
	ApplyTheme(activeTheme)

	ti := textinput.New()
	ti.Placeholder = i18n.T("input.username_placeholder")
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
		latestRelease: latest,
		input:         ti,
		roadmapData:   roadmap.Embedded(),
	}
	m.loadInstanceSelection()
	m.loadAccounts()
	if m.authStorage == nil || len(m.authStorage.Accounts) == 0 {
		m.state = welcomeScreen
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, roadmap.FetchCmd())
}

func (m *Model) loadAccounts() {
	storage, err := auth.LoadStorage()
	if err == nil && storage != nil {
		m.authStorage = storage
		if act := storage.GetActive(); act != nil {
			m.username = act.Username
		} else if m.username != "" {
			offID := auth.OfflineUUID(m.username)
			m.authStorage.AddAccount(auth.Account{
				ID:       offID,
				Username: m.username,
				Type:     auth.AccountTypeOffline,
			}, true)
		}
	}
}

func (m Model) currentAccountType() string {
	if m.authStorage != nil {
		if act := m.authStorage.GetActive(); act != nil {
			return string(act.Type)
		}
	}
	return "offline"
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
		return i18n.T("common.unavailable")
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
		return LaunchConfig{}, fmt.Errorf("%s", i18n.T("error.no_instance_selected"))
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
		return LaunchConfig{}, fmt.Errorf("%s", i18n.T("error.managed_versions_mismatch"))
	}

	var accUUID, token, userType string
	if m.authStorage != nil {
		if act := m.authStorage.GetActive(); act != nil {
			accUUID = act.ID
			token = act.AccessToken
			if act.Type == auth.AccountTypeMicrosoft {
				userType = "msa"
			} else {
				userType = "legacy"
			}
		}
	}
	if userType == "" {
		userType = "legacy"
	}
	if accUUID == "" {
		accUUID = auth.OfflineUUID(m.username)
	}

	return LaunchConfig{
		GameDir:       gameDir,
		RuntimeDir:    runtimeDir,
		Username:      m.username,
		Version:       m.versionSelect,
		Modloader:     m.modloader,
		LoaderVersion: m.instance.LoaderVersion,
		MemoryMB:      m.memoryMB,
		JavaPath:      m.instance.JavaPath,
		UUID:          accUUID,
		AccessToken:   token,
		UserType:      userType,
	}, nil
}

func (m Model) centerWidth() int {
	if m.width > 0 {
		return min(80, max(46, m.width-41))
	}
	return 76
}

func (m Model) panelsWidth() int {
	return (leftColumnWidth + 2) + m.centerWidth() + 2
}

func (m Model) centerInnerHeight() int {
	if m.width > 0 {
		return max(14, min(38, m.height-19))
	}
	return m.height - 4
}

func (m Model) catalogWidth() int {
	w := m.centerWidth() - 2
	if w < 78 && m.width >= 100 {
		w = 78
	}
	if w < 60 {
		w = 60
	}
	return w
}
