package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
	"mcTUI/internal/i18n"
	"mcTUI/internal/instances"
	"mcTUI/internal/launcher"
	"mcTUI/internal/mojang"
	"mcTUI/internal/resetconfigs"
	"mcTUI/internal/worlds"
)

// validUsername reports whether name is a valid Minecraft username: 1-16
// characters, letters/digits/underscore only. This matches the
// validation Minecraft itself performs — usernames that fail this check
// cause a cryptic "Invalid characters in username" error deep into the
// launch process instead of being rejected up front.
func validUsername(name string) bool {
	if len(name) == 0 || len(name) > 16 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '_' {
			return false
		}
	}
	return true
}

// filesCached is only a display hint. Nonempty files are not evidence of a
// valid installation; Play always runs the shared verifier asynchronously.
func filesCached(runtimeDir, version, modloader string, loaderVersion ...string) bool {
	mcDir := runtimeDir
	if !mojang.ClientJarExists(mcDir, version) {
		return false
	}
	if data, ok := mojang.CachedVersionData(mcDir, version); !ok || data.Downloads.Client.URL == "" || data.AssetIndex.ID == "" {
		return false
	}
	if modloader == "Fabric" && !fabric.ProfileExists(mcDir, version, loaderVersion...) {
		return false
	}
	return true
}

func startLaunchDownload(ctx context.Context, launch LaunchConfig) tea.Cmd {
	if launch.LoaderVersion == "" {
		return StartDownloadCmd(ctx, launch.RuntimeDir, launch.Version, launch.Modloader, launch.JavaPath)
	}
	return func() tea.Msg {
		prepared, err := launcher.PrepareWithLoaderContext(ctx, launch.RuntimeDir, launch.Version, launch.Modloader, launch.LoaderVersion, nil)
		var javaBinary string
		if err == nil {
			javaBinary, err = launcher.ResolvePreparedJava(launch.RuntimeDir, prepared, launch.JavaPath)
		}
		return DownloadDoneMsg{Prepared: prepared, JavaBinary: javaBinary, Err: err}
	}
}

func fabricLoaderCmd(version string, request uint64) tea.Cmd {
	return func() tea.Msg {
		loader, err := fabric.LatestLoader(context.Background(), version)
		return FabricLoaderDoneMsg{Request: request, MinecraftVersion: version, LoaderVersion: loader, Err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case FabricLoaderDoneMsg:
		if m.state != instanceLoaderScreen || m.creationVersion != msg.MinecraftVersion || m.fabricLoaderRequest != msg.Request {
			return m, nil
		}
		if msg.Err != nil {
			m.instanceErr = msg.Err
			return m, nil
		}
		m.creationLoaderVersion = msg.LoaderVersion
		m.instanceErr = nil
		m.state = instanceNameScreen
		m.input.SetValue("")
		m.input.CharLimit = 80
		m.input.Placeholder = i18n.T("input.instance_placeholder")
		m.input.Focus()
		return m, nil
	case modsDoneMsg:
		if msg.Request != m.modRequest || m.state != modsWorkingScreen {
			return m, nil
		}
		m.modItems, m.modErr, m.modBusy = msg.Items, msg.Err, msg.Busy
		m.modCursor = min(m.modCursor, max(0, len(m.modItems)-1))
		m.state = modsScreen
		return m, nil
	case downloadAnimationTickMsg:
		if m.state == downloadScreen && !m.dlFinished {
			m.dlProgressFrame++
			return m, downloadAnimationTickCmd()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case ResetDoneMsg:
		if m.state == resetRunningScreen {
			m.resetResult = msg.Result
			m.resetErr = msg.Err
			m.resetScroll = 0
			m.state = resetResultScreen
		}
		return m, nil

	case InstanceCopyInspectDoneMsg:
		if m.state == instanceCopyInspectScreen {
			if msg.Err != nil {
				m.copyErr = msg.Err
				m.state = instanceCopyResultScreen
			} else {
				m.copyItems = msg.Items
				m.copySelected = make(map[string]bool)
				m.copyCursor = 0
				m.copyErr = nil
				m.state = instanceCopySelectScreen
			}
		}
		return m, nil

	case InstanceCopyDoneMsg:
		if m.state == instanceCopyRunningScreen && !m.copyCleaning {
			m.copyResult, m.copyErr, m.copyStage = msg.Result, msg.Err, msg.Result.StagingPath
			if msg.Err == nil {
				if err := m.setInstance(msg.Result.Instance); err != nil {
					m.copyErr = err
					m.state = instanceCopyResultScreen
				} else {
					m.instanceList, m.instanceErr = instances.List(m.dataRoot)
					m.copyingInstance = false
					m.state = instanceCopyResultScreen
				}
			} else {
				m.state = instanceCopyResultScreen
			}
		}
		return m, nil

	case InstanceCopyCleanupDoneMsg:
		if m.state == instanceCopyRunningScreen && m.copyCleaning {
			m.copyCleaning = false
			m.copyErr = msg.Err
			if msg.Err == nil {
				m.copyStage = ""
			}
			m.state = instanceCopyResultScreen
		}
		return m, nil

	case InstanceRemovalDoneMsg:
		if m.state == instanceRemoveRunningScreen {
			m.removeErr = msg.Err
			m.instanceList, m.instanceErr = instances.List(m.dataRoot)
			var listErr error
			m.retiredList, listErr = instances.RetiredList(m.dataRoot)
			if m.removeErr == nil {
				m.removeErr = listErr
			}
			m.instanceCursor, m.retiredCursor = 0, 0
			if selected, err := instances.Selected(m.dataRoot); err == nil {
				m.instance = selected
				if selected.Kind == "legacy" {
					cfg := config.Load()
					m.versionSelect, m.modloader, m.memoryMB = cfg.Version, cfg.Modloader, cfg.MemoryMB
				}
			}
			m.state = instanceRemoveResultScreen
		}
		return m, nil

	case GameStartedMsg:
		if m.state != gameStartingScreen {
			return m, nil
		}
		if msg.Err != nil {
			m.gameErr = msg.Err
			m.activeGame = nil
			m.state = gameResultScreen
			return m, nil
		}
		m.activeGame = msg.Game
		m.gameKillConfirm = false
		m.gameKillErr = nil
		m.gameLog = msg.Game.LogPath
		m.state = gameRunningScreen
		return m, func() tea.Msg {
			code, err := msg.Game.Wait()
			return GameExitedMsg{Code: code, Err: err}
		}

	case GameExitedMsg:
		if m.state == gameRunningScreen {
			m.activeGame = nil
			m.gameKillConfirm = false
			m.gameExitCode, m.gameErr = msg.Code, msg.Err
			m.state = gameResultScreen
		}
		return m, nil

	case tea.MouseMsg:
		if m.state >= resetModeScreen && m.state <= resetResultScreen {
			switch msg.Button {
			case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
				if m.state == resetPickerScreen {
					key := "up"
					if msg.Button == tea.MouseButtonWheelDown {
						key = "down"
					}
					return m.updateResetPicker(key)
				}
				if msg.Button == tea.MouseButtonWheelUp {
					m.resetScroll = max(0, m.resetScroll-1)
				} else {
					m.resetScroll = min(m.resetMaxScroll(), m.resetScroll+1)
				}
				return m, nil
			}
		}

	case DownloadDoneMsg:
		if m.state != downloadScreen {
			return m, nil
		}
		m.dlFinished = true
		if m.dlContext != nil && m.dlContext.Err() != nil {
			msg.Err = m.dlContext.Err()
		}
		if m.dlCancel != nil {
			m.dlCancel()
			m.dlCancel = nil
		}
		if msg.Err != nil {
			m.dlErr = msg.Err
			return m, nil
		}
		// Launch only the verified artifacts; never prepare a second time.
		launch := m.launch
		m.gameErr, m.gameLog = nil, ""
		m.state = gameStartingScreen
		return m, func() tea.Msg {
			game, err := launcher.StartPrepared(launch.GameDir, launch.RuntimeDir, msg.JavaBinary, launch.Username, launch.Version, launch.MemoryMB, msg.Prepared)
			return GameStartedMsg{Game: game, Err: err}
		}

	case tea.KeyMsg:
		if m.state == gameStartingScreen {
			return m, nil // wait for Java; never detach or kill it implicitly
		}
		if m.state == gameRunningScreen {
			if m.gameKillConfirm {
				switch msg.String() {
				case "y", "Y":
					m.gameKillConfirm = false
					if m.activeGame != nil {
						if err := m.activeGame.Kill(); err != nil {
							m.gameKillErr = err
						}
					}
					return m, nil
				case "n", "N", "esc":
					m.gameKillConfirm = false
					return m, nil
				default:
					return m, nil
				}
			} else {
				switch msg.String() {
				case "k", "K":
					m.gameKillConfirm = true
					m.gameKillErr = nil
					return m, nil
				default:
					return m, nil // wait for Java; never detach or kill it implicitly
				}
			}
		}
		if m.state == gameResultScreen {
			if msg.String() == "enter" || msg.String() == "esc" {
				m.state = menuScreen
			}
			return m, nil
		}
		if m.state >= modsScreen && m.state <= modsWorkingScreen {
			return m.updateModsKey(msg.String())
		}
		if m.state == javaPickerScreen {
			return m.updateJavaPicker(msg.String())
		}
		if m.state >= instanceTrashScreen && m.state <= instanceRemoveResultScreen {
			return m.updateRemovalKey(msg.String())
		}
		if m.state >= resetModeScreen && m.state <= resetResultScreen {
			return m.updateResetKey(msg.String())
		}
		// A failed download stays visible until the user returns to the menu.
		if m.state == downloadScreen {
			switch msg.String() {
			case "ctrl+c":
				if m.dlCancel != nil {
					m.dlCancel()
				}
				return m, tea.Quit
			case "esc":
				if !m.dlFinished && m.dlCancel != nil {
					m.dlCancel()
				}
				if m.dlFinished && m.dlErr != nil {
					m.state = menuScreen
				}
			}
			return m, cmd
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == menuScreen {
				return m, tea.Quit
			}

		case "m":
			if m.state == menuScreen {
				return m.openMods()
			}
		case "i":
			if m.state == menuScreen {
				m.instanceList, m.instanceErr = instances.List(m.dataRoot)
				m.instanceCursor = 0
				for i, in := range m.instanceList {
					if in.ID == m.instance.ID {
						m.instanceCursor = i
						break
					}
				}
				m.state = instancesScreen
			}

		case "up", "k":
			if m.state == instanceCopySelectScreen && m.copyCursor > 0 {
				m.copyCursor--
			} else if m.state == instancesScreen && m.instanceCursor > 0 {
				m.instanceCursor--
			} else if m.state == menuScreen && m.cursorMenu > 0 {
				m.cursorMenu--
			} else if m.state == versionsScreen && m.cursorVersions > 0 {
				m.cursorVersions--
			} else if m.state == worldsScreen && m.cursorWorlds > 0 {
				m.cursorWorlds--
			}

		case "down", "j":
			if m.state == instanceCopySelectScreen && m.copyCursor+1 < len(m.copyItems) {
				m.copyCursor++
			} else if m.state == instancesScreen && m.instanceCursor+1 < len(m.instanceList) {
				m.instanceCursor++
			} else if m.state == menuScreen && m.cursorMenu < menuOptionsCount-1 {
				m.cursorMenu++
			} else if m.state == versionsScreen && m.cursorVersions < len(m.versions)-1 {
				m.cursorVersions++
			} else if m.state == worldsScreen && m.cursorWorlds < len(m.worlds)-1 {
				m.cursorWorlds++
			}

		case "enter":
			if m.state == instanceCopyResultScreen {
				if m.copyErr == nil && m.copyResult.Instance.ID != "" {
					m.state = menuScreen
				} else {
					m.copyingInstance = false
					m.state = instancesScreen
					m.instanceList, m.instanceErr = instances.List(m.dataRoot)
				}
			} else if m.state == instanceCopySelectScreen {
				selected := false
				for _, item := range m.copyItems {
					selected = selected || m.copySelected[item.Name]
				}
				if selected {
					m.state = instanceCopyConfirmScreen
				}
			} else if m.state == instanceLoaderScreen {
				if m.instanceErr != nil {
					m.instanceErr = nil
					m.fabricLoaderRequest++
					return m, fabricLoaderCmd(m.creationVersion, m.fabricLoaderRequest)
				}
			} else if m.state == instanceNameScreen {
				name := strings.TrimSpace(m.input.Value())
				if m.copyingInstance {
					if name == "" || len(name) > 80 {
						m.instanceErr = fmt.Errorf("%s", i18n.T("error.instance_name_length"))
					} else {
						m.copyName = name
						m.copyErr = nil
						m.state = instanceCopyInspectScreen
						source := m.copySource
						return m, func() tea.Msg {
							items, err := instances.InspectCopySource(source)
							return InstanceCopyInspectDoneMsg{Items: items, Err: err}
						}
					}
				} else if m.instanceRenameID != "" {
					m.instanceErr = instances.Rename(m.dataRoot, m.instanceRenameID, name)
					if m.instanceErr == nil {
						m.instanceList, m.instanceErr = instances.List(m.dataRoot)
						for i, in := range m.instanceList {
							if in.ID == m.instanceRenameID {
								m.instanceCursor = i
								if m.instance.ID == in.ID {
									m.instance = in
								}
								break
							}
						}
						m.instanceRenameID = ""
						m.state = instancesScreen
					}
				} else {
					loader := "Vanilla"
					if m.creationLoaderVersion != "" {
						loader = "Fabric"
					}
					in, err := instances.Create(m.dataRoot, name, m.creationVersion, loader, m.creationLoaderVersion, m.memoryMB)
					if err != nil {
						m.instanceErr = err
					} else if err = m.setInstance(in); err != nil {
						m.instanceErr = err
					} else {
						m.input.CharLimit = 16
						m.input.Placeholder = i18n.T("input.username_placeholder")
						m.instanceList, m.instanceErr = instances.List(m.dataRoot)
						m.creatingInstance = false
						m.creationVersion, m.creationLoaderVersion = "", ""
						m.state = menuScreen
					}
				}
			} else if m.state == instancesScreen && len(m.instanceList) > 0 {
				if err := m.setInstance(m.instanceList[m.instanceCursor]); err != nil {
					m.instanceErr = err
				} else {
					m.instanceErr = nil
					m.state = menuScreen
				}
			} else if m.state == menuScreen {
				if m.cursorMenu == 0 {
					m.state = confirmScreen
				} else if m.cursorMenu == 1 {
					m.state = nameScreen
					m.input.CharLimit = 16
					m.input.Placeholder = i18n.T("input.username_placeholder")
					m.input.SetValue(m.username)
				} else if m.cursorMenu == 2 {
					if m.isManaged() {
						m.instanceErr = fmt.Errorf("%s", i18n.T("error.version_fixed"))
					} else {
						m.state = versionsScreen
						for i, v := range m.versions {
							if v == m.versionSelect {
								m.cursorVersions = i
								break
							}
						}
					}
				} else if m.cursorMenu == 3 {
					gameDir, err := instances.GameDir(m.dataRoot, m.instance)
					if err != nil {
						m.instanceErr = err
					} else {
						m.worlds = worlds.List(gameDir)
					}
					m.state = worldsScreen
					m.cursorWorlds = 0
				} else if m.cursorMenu == 4 {
					if m.isManaged() {
						m.instanceErr = fmt.Errorf("%s", i18n.T("error.fabric_version_required"))
					} else if m.modloader == "Vanilla" {
						m.modloader = "Fabric"
					} else {
						m.modloader = "Vanilla"
					}
					m.save()
				} else if m.cursorMenu == 5 {
					steps := []int{1024, 2048, 4096, 6144, 8192}
					for i, v := range steps {
						if v == m.memoryMB {
							m.memoryMB = steps[(i+1)%len(steps)]
							break
						}
					}
					if m.memoryMB != 1024 && m.memoryMB != 2048 && m.memoryMB != 4096 && m.memoryMB != 6144 && m.memoryMB != 8192 {
						m.memoryMB = 2048
					}
					if m.isManaged() {
						updated := m.instance
						updated.MemoryMB = m.memoryMB
						m.instanceErr = instances.UpdateSettings(m.dataRoot, updated)
						if m.instanceErr == nil {
							m.instance = updated
						} else {
							m.memoryMB = m.instance.MemoryMB
							return m, nil
						}
					} else {
						m.save()
					}
				} else if m.cursorMenu == 6 {
					m.resetDir, m.resetBackupRoot, m.resetErr = "", "", nil
					m.resetInstance = m.instance.Name
					if m.resetInstance == "" {
						m.resetInstance = i18n.T("instances.legacy_name")
					}
					if m.instance.ID == "" { // legacy-only models used before instance selection is loaded
						m.resetDir, m.resetErr = config.MinecraftDirChecked()
						m.resetBackupRoot = filepath.Join(m.resetDir, "mctui-backups")
					} else {
						m.resetDir, m.resetErr = instances.GameDir(m.dataRoot, m.instance)
						if m.resetErr == nil {
							m.resetBackupRoot, m.resetErr = instances.BackupDir(m.dataRoot, m.instance)
						}
					}
					if m.resetErr == nil && m.isManaged() {
						if err := os.Mkdir(m.resetDir, 0700); err != nil && !os.IsExist(err) {
							m.resetErr = err
						} else {
							_, m.resetErr = instances.GameDir(m.dataRoot, m.instance)
						}
					}
					if m.resetErr == nil {
						m.resetFound, m.resetErr = resetconfigs.InspectAt(m.resetDir)
					}
					m.resetMode = resetconfigs.Backup
					m.resetBackupDir = ""
					m.resetChoice = 0
					m.resetIsRestore = false
					m.resetScroll = 0
					m.state = resetModeScreen
				} else if m.cursorMenu == 7 {
					if m.isManaged() {
						m.openJavaPicker()
					} else {
						m.instanceErr = fmt.Errorf("%s", i18n.T("error.java_managed_only"))
					}
				} else if m.cursorMenu == 8 {
					i18n.NextLanguage()
					cfg := config.Load()
					cfg.Language = i18n.CurrentLanguage()
					config.Save(cfg)
				} else if m.cursorMenu == 9 {
					return m, tea.Quit
				}
			} else if m.state == nameScreen {
				if validUsername(m.input.Value()) {
					m.username = m.input.Value()
					m.save()
					m.state = menuScreen
				}
				// else: stay on nameScreen, showing the validation hint
			} else if m.state == versionsScreen {
				version := m.versions[m.cursorVersions]
				if m.creatingInstance || m.copyingInstance {
					m.creationVersion = version
					m.creationLoaderVersion = ""
					m.state = instanceNameScreen
					m.instanceErr = nil
					m.input.SetValue("")
					m.input.CharLimit = 80
					m.input.Placeholder = i18n.T("input.instance_placeholder")
					m.input.Focus()
				} else {
					m.versionSelect = version
					m.save()
					m.state = menuScreen
				}
			} else if m.state == worldsScreen {
				if len(m.worlds) > 0 {
					target := m.worlds[m.cursorWorlds].Version
					if !m.isManaged() {
						for _, v := range m.versions {
							if v == target {
								m.versionSelect = target
								m.save()
								break
							}
						}
					}
				}
				m.state = menuScreen
			} else if m.state == confirmScreen {
				// Capture target instance and paths before starting async preparation.
				launch, err := m.launchConfig()
				if err != nil {
					m.dlErr = err
					return m, nil
				}
				m.launch = launch
				m.state = downloadScreen
				m.dlProgressFrame = 0
				m.dlErr = nil
				m.dlFinished = false
				ctx, cancel := context.WithCancel(context.Background())
				m.dlContext, m.dlCancel = ctx, cancel
				return m, tea.Batch(downloadAnimationTickCmd(), startLaunchDownload(ctx, launch))
			}

		case "y", "Y":
			if m.state == instanceCopyConfirmScreen {
				return m.startInstanceCopy()
			} else if m.state == instanceCopyCleanupConfirmScreen {
				root, stage := m.dataRoot, m.copyStage
				m.copyCleaning = true
				m.state = instanceCopyRunningScreen
				return m, func() tea.Msg {
					return InstanceCopyCleanupDoneMsg{Err: instances.DiscardCopyStage(root, stage)}
				}
			} else if m.state == confirmScreen {
				launch, err := m.launchConfig()
				if err != nil {
					m.dlErr = err
					return m, nil
				}
				m.launch = launch
				m.state = downloadScreen
				m.dlProgressFrame = 0
				m.dlErr = nil
				m.dlFinished = false
				ctx, cancel := context.WithCancel(context.Background())
				m.dlContext, m.dlCancel = ctx, cancel
				return m, tea.Batch(downloadAnimationTickCmd(), startLaunchDownload(ctx, launch))
			}

		case "f", "F":
			if m.state == versionsScreen && m.creatingInstance && len(m.versions) > 0 {
				m.creationVersion = m.versions[m.cursorVersions]
				m.creationLoaderVersion = ""
				m.instanceErr = nil
				m.state = instanceLoaderScreen
				m.fabricLoaderRequest++
				return m, fabricLoaderCmd(m.creationVersion, m.fabricLoaderRequest)
			}

		case "n", "N":
			if m.state == confirmScreen {
				m.state = menuScreen
			} else if m.state == instanceCopyConfirmScreen {
				m.state = instanceCopySelectScreen
			} else if m.state == instanceCopyCleanupConfirmScreen {
				m.state = instanceCopyResultScreen
			} else if m.state == instancesScreen {
				m.instanceErr = nil
				m.instanceRenameID = ""
				m.copyingInstance = false
				m.creatingInstance = true
				m.creationLoaderVersion = ""
				m.state = versionsScreen
				for i, version := range m.versions {
					if version == m.versionSelect {
						m.cursorVersions = i
						break
					}
				}
				return m, nil
			}

		case "c", "C":
			if m.state == instancesScreen {
				if m.instanceCursor < 0 || m.instanceCursor >= len(m.instanceList) {
					m.instanceErr = fmt.Errorf("%s", i18n.T("error.select_instance_copy"))
					return m, nil
				}
				in, err := instances.Read(m.dataRoot, m.instanceList[m.instanceCursor].ID)
				if err == nil {
					m.copySource, err = instances.GameDir(m.dataRoot, in)
				}
				if err != nil {
					m.instanceErr = err
					return m, nil
				}
				m.copySourceID = in.ID
				m.copyLoader = in.Loader
				m.instanceErr = nil
				m.creatingInstance = false
				m.copyingInstance = true
				m.input.SetValue("")
				m.input.CharLimit = 80
				m.input.Placeholder = i18n.T("input.instance_placeholder")
				m.input.Focus()
				if in.ID == instances.LegacyID {
					m.copyLoader = "Vanilla"
					m.creationVersion = ""
					m.state = versionsScreen
					for i, version := range m.versions {
						if version == m.versionSelect {
							m.cursorVersions = i
							break
						}
					}
				} else {
					m.creationVersion = in.MinecraftVersion
					m.state = instanceNameScreen
				}
				return m, nil
			}

		case "t", "T":
			if m.state == instancesScreen {
				m.retiredList, m.removeErr = instances.RetiredList(m.dataRoot)
				if m.removeErr == nil {
					m.retiredCursor = 0
					m.state = instanceTrashScreen
				}
				return m, nil
			}

		case "r", "R":
			if m.state == instancesScreen && len(m.instanceList) > 0 {
				in := m.instanceList[m.instanceCursor]
				if in.Kind != "managed" {
					m.instanceErr = fmt.Errorf("%s", i18n.T("error.legacy_rename"))
				} else {
					m.instanceErr = nil
					m.instanceRenameID = in.ID
					m.state = instanceNameScreen
					m.input.SetValue(in.Name)
					m.input.CharLimit = 80
					m.input.Placeholder = i18n.T("input.instance_placeholder")
					m.input.Focus()
				}
				return m, nil
			}

		case " ":
			if m.state == instanceCopySelectScreen && len(m.copyItems) > 0 {
				item := m.copyItems[m.copyCursor]
				if item.Exists && item.Error == "" {
					m.copySelected[item.Name] = !m.copySelected[item.Name]
				}
			} else if m.state == instanceCopyConfirmScreen {
				m.state = instanceCopySelectScreen
			}

		case "d", "D":
			if m.state == instancesScreen && m.instanceCursor >= 0 && m.instanceCursor < len(m.instanceList) {
				in := m.instanceList[m.instanceCursor]
				if in.ID == instances.LegacyID {
					m.instanceErr = fmt.Errorf("%s", i18n.T("error.legacy_remove"))
				} else {
					m.removeTarget = in
					m.removeFromTrash = false
					m.removeErr = nil
					m.state = instanceRemoveChoiceScreen
				}
				return m, nil
			} else if m.state == instanceCopyResultScreen && m.copyStage != "" {
				m.state = instanceCopyCleanupConfirmScreen
			}

		case "esc":
			if m.state == instanceCopySelectScreen {
				m.state = instanceNameScreen
			} else if m.state == instanceCopyConfirmScreen {
				m.state = instanceCopySelectScreen
			} else if m.state == instanceCopyCleanupConfirmScreen {
				m.state = instanceCopyResultScreen
			} else if m.state == instanceCopyResultScreen {
				m.copyingInstance = false
				if m.copyErr == nil && m.copyResult.Instance.ID != "" {
					m.state = menuScreen
				} else {
					m.state = instancesScreen
					m.instanceList, m.instanceErr = instances.List(m.dataRoot)
				}
			} else if m.state == instancesScreen {
				m.state = menuScreen
				m.instanceErr = nil
			} else if m.state == instanceLoaderScreen {
				m.state = versionsScreen
				m.instanceErr = nil
			} else if m.state == instanceNameScreen {
				m.state = instancesScreen
				m.instanceErr = nil
				m.instanceRenameID = ""
				m.creatingInstance = false
				m.copyingInstance = false
				m.creationVersion, m.creationLoaderVersion = "", ""
				m.input.CharLimit = 16
				m.input.Placeholder = i18n.T("input.username_placeholder")
			} else if m.state == versionsScreen {
				if m.creatingInstance || m.copyingInstance {
					m.creatingInstance = false
					m.copyingInstance = false
					m.state = instancesScreen
				} else {
					m.state = menuScreen
				}
			} else if m.state == nameScreen || m.state == worldsScreen || m.state == confirmScreen {
				m.state = menuScreen
			}
		}
	}

	if m.state == nameScreen || m.state == instanceNameScreen {
		m.input, cmd = m.input.Update(msg)
	}

	return m, cmd
}

// updateResetKey keeps a purge impossible without a mode choice and a fresh
// explicit confirmation. Bubble Tea executes commands asynchronously.
func (m Model) startInstanceCopy() (tea.Model, tea.Cmd) {
	selected := make([]string, 0, len(m.copySelected))
	for _, item := range m.copyItems {
		if m.copySelected[item.Name] {
			selected = append(selected, item.Name)
		}
	}
	root, source, sourceID, name, version, memory := m.dataRoot, m.copySource, m.copySourceID, m.copyName, m.creationVersion, m.memoryMB
	m.copyErr = nil
	m.copyStage = ""
	m.copyResult = instances.CopyResult{}
	m.copyCleaning = false
	m.state = instanceCopyRunningScreen
	return m, func() tea.Msg {
		var result instances.CopyResult
		var err error
		if sourceID == instances.LegacyID {
			result, err = instances.CopyFromLegacy(root, source, name, version, selected, memory)
		} else {
			result, err = instances.CopyInstance(root, sourceID, name, selected)
		}
		return InstanceCopyDoneMsg{Result: result, Err: err}
	}
}

func (m Model) updateResetKey(key string) (tea.Model, tea.Cmd) {
	if m.state != resetPickerScreen && m.state != resetRunningScreen {
		switch key {
		case "pgdown":
			m.resetScroll = min(m.resetMaxScroll(), m.resetScroll+m.resetVisibleLines())
			return m, nil
		case "pgup":
			m.resetScroll = max(0, m.resetScroll-m.resetVisibleLines())
			return m, nil
		}
	}
	if m.state == resetConfirmScreen || m.state == resetReplaceConfirmScreen || m.state == resetRestoreConfirmScreen || m.state == resetResultScreen {
		switch key {
		case "up", "k":
			m.resetScroll = max(0, m.resetScroll-1)
			return m, nil
		case "down", "j":
			m.resetScroll = min(m.resetMaxScroll(), m.resetScroll+1)
			return m, nil
		}
	}
	switch m.state {
	case resetModeScreen:
		switch key {
		case "esc", "q":
			m.state = menuScreen
		case "up", "k":
			if m.resetChoice > 0 {
				m.resetChoice--
				m.resetMode = []resetconfigs.Mode{resetconfigs.Backup, resetconfigs.Purge, resetconfigs.Backup}[m.resetChoice]
				m.scrollToBackupSelection()
			} else if m.resetScroll > 0 {
				m.resetScroll--
			}
		case "down", "j":
			if m.resetChoice < 2 {
				m.resetChoice++
				m.resetMode = []resetconfigs.Mode{resetconfigs.Backup, resetconfigs.Purge, resetconfigs.Backup}[m.resetChoice]
				m.scrollToBackupSelection()
			} else if m.resetScroll < m.resetMaxScroll() {
				m.resetScroll++
			}
		case "enter":
			if m.resetChoice == 2 {
				m.resetBackups, m.resetErr = resetconfigs.ListBackupsAtRoot(m.resetDir, m.resetBackupRoot)
				m.resetBackupIndex = 0
				m.resetScroll = 0
				m.state = resetBackupListScreen
			} else if m.resetErr == nil && len(m.resetFound) > 0 {
				m.resetIsRestore = false
				if m.resetChoice == 0 {
					m.resetMode = resetconfigs.Backup
					if m.isManaged() {
						m.resetBackupDir = ""
						m.state = resetConfirmScreen // managed history stays under backups/<id>
					} else {
						m.openResetPicker()
					}
				} else {
					m.resetMode = resetconfigs.Purge
					m.state = resetConfirmScreen
				}
			}
		}
	case resetPickerScreen:
		return m.updateResetPicker(key)
	case resetConfirmScreen:
		switch key {
		case "esc", "n", "N":
			m.state = resetModeScreen
		case "y", "Y":
			dir, root, mode, base := m.resetDir, m.resetBackupRoot, m.resetMode, m.resetBackupDir
			m.state = resetRunningScreen
			return m, func() tea.Msg {
				result, err := resetconfigs.RunAtWithBackups(dir, root, mode, base)
				return ResetDoneMsg{Result: result, Err: err}
			}
		}
	case resetBackupListScreen:
		switch key {
		case "esc", "q":
			m.resetFound, m.resetErr = resetconfigs.InspectAt(m.resetDir)
			m.state = resetModeScreen
		case "up", "k":
			if m.resetBackupIndex > 0 {
				m.resetBackupIndex--
				m.scrollToBackupSelection()
			} else if m.resetScroll > 0 {
				m.resetScroll--
			}
		case "down", "j":
			if m.resetBackupIndex+1 < len(m.resetBackups) {
				m.resetBackupIndex++
				m.scrollToBackupSelection()
			} else if m.resetScroll < m.resetMaxScroll() {
				m.resetScroll++
			}
		case "enter":
			if m.resetErr == nil && len(m.resetBackups) > 0 {
				var err error
				m.resetBackup, m.resetConflicts, err = resetconfigs.RestorePlanAtRoot(m.resetDir, m.resetBackupRoot, m.resetBackups[m.resetBackupIndex].Path)
				m.resetErr = err
				m.resetReplace = false
				m.resetRetention = ""
				m.resetScroll = 0
				if err == nil && len(m.resetConflicts) > 0 {
					m.state = resetReplaceConfirmScreen
				} else if err == nil {
					m.state = resetRetentionScreen
				}
			}
		}
	case resetReplaceConfirmScreen:
		switch key {
		case "esc", "n", "N":
			m.state = resetBackupListScreen
		case "y", "Y":
			m.resetReplace = true
			m.state = resetRetentionScreen
		}
	case resetRetentionScreen:
		switch key {
		case "esc", "q":
			m.state = resetBackupListScreen
		case "up", "k":
			if m.resetRetention == resetconfigs.KeepBackup {
				m.resetRetention = resetconfigs.ConsumeBackup
			} else {
				m.resetRetention = resetconfigs.KeepBackup
			}
		case "down", "j":
			if m.resetRetention == resetconfigs.ConsumeBackup {
				m.resetRetention = resetconfigs.KeepBackup
			} else {
				m.resetRetention = resetconfigs.ConsumeBackup
			}
		case "enter":
			if m.resetRetention != "" {
				m.state = resetRestoreConfirmScreen
			}
		}
	case resetRestoreConfirmScreen:
		switch key {
		case "esc", "n", "N":
			m.state = resetRetentionScreen
		case "y", "Y":
			dir, root, name, mode := m.resetDir, m.resetBackupRoot, m.resetBackup.Path, m.resetRetention
			var replaceFolders []string
			if m.resetReplace {
				replaceFolders = append(replaceFolders, m.resetConflicts...)
			}
			m.resetIsRestore = true
			m.state = resetRunningScreen
			return m, func() tea.Msg {
				result, err := resetconfigs.RestoreAtRoot(dir, root, name, mode, replaceFolders)
				return ResetDoneMsg{Result: result, Err: err}
			}
		}
	case resetResultScreen:
		switch key {
		case "esc", "enter", "q":
			m.state = menuScreen
		}
	}
	return m, nil
}

// save persists the model's current settings to disk.
func (m Model) save() {
	cfg := config.Load()
	cfg.Username = m.username
	if !m.isManaged() {
		cfg.Version, cfg.Modloader, cfg.MemoryMB = m.versionSelect, m.modloader, m.memoryMB
	}
	config.Save(cfg)
}
