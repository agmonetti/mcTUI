package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
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
func filesCached(version, modloader string) bool {
	mcDir := config.MinecraftDir()
	if !mojang.ClientJarExists(mcDir, version) {
		return false
	}
	if data, ok := mojang.CachedVersionData(mcDir, version); !ok || data.Downloads.Client.URL == "" || data.AssetIndex.ID == "" {
		return false
	}
	if modloader == "Fabric" && !fabric.ProfileExists(mcDir, version) {
		return false
	}
	return true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
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

	// ── Mensajes de progreso de descarga ───────────────────────────────────
	case downloadChanMsg:
		if m.state != downloadScreen {
			return m, nil
		}
		// Procesa el mensaje portado y encadena la espera del siguiente.
		switch inner := msg.msg.(type) {
		case DownloadProgressMsg:
			m.dlDone = inner.Done
			m.dlFile = inner.File
			return m, nextDownloadCmd(msg.ch)
		case DownloadDoneMsg:
			m.dlFinished = true
			if m.dlContext != nil && m.dlContext.Err() != nil {
				inner.Err = m.dlContext.Err()
			}
			if m.dlCancel != nil {
				m.dlCancel()
				m.dlCancel = nil
			}
			if inner.Err != nil {
				m.dlErr = inner.Err
				return m, nil
			}
			// Descarga completada → lanzar el juego
			m.play = true
			return m, tea.Quit
		}

	case tea.KeyMsg:
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

		case "up", "k":
			if m.state == menuScreen && m.cursorMenu > 0 {
				m.cursorMenu--
			} else if m.state == versionsScreen && m.cursorVersions > 0 {
				m.cursorVersions--
			} else if m.state == worldsScreen && m.cursorWorlds > 0 {
				m.cursorWorlds--
			}

		case "down", "j":
			if m.state == menuScreen && m.cursorMenu < menuOptionsCount-1 {
				m.cursorMenu++
			} else if m.state == versionsScreen && m.cursorVersions < len(m.versions)-1 {
				m.cursorVersions++
			} else if m.state == worldsScreen && m.cursorWorlds < len(m.worlds)-1 {
				m.cursorWorlds++
			}

		case "enter":
			if m.state == menuScreen {
				if m.cursorMenu == 0 {
					m.state = confirmScreen
				} else if m.cursorMenu == 1 {
					m.state = nameScreen
					m.input.SetValue(m.username)
				} else if m.cursorMenu == 2 {
					m.state = versionsScreen
					for i, v := range m.versions {
						if v == m.versionSelect {
							m.cursorVersions = i
							break
						}
					}
				} else if m.cursorMenu == 3 {
					m.worlds = worlds.List(config.MinecraftDir())
					m.state = worldsScreen
					m.cursorWorlds = 0
				} else if m.cursorMenu == 4 {
					if m.modloader == "Vanilla" {
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
					m.save()
				} else if m.cursorMenu == 6 {
					m.resetDir, m.resetFound, m.resetErr = resetconfigs.Inspect()
					m.resetMode = resetconfigs.Backup
					m.resetBackupDir = ""
					m.resetChoice = 0
					m.resetIsRestore = false
					m.resetScroll = 0
					m.state = resetModeScreen
				} else if m.cursorMenu == 7 {
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
				m.versionSelect = m.versions[m.cursorVersions]
				m.save()
				m.state = menuScreen
			} else if m.state == worldsScreen {
				if len(m.worlds) > 0 {
					target := m.worlds[m.cursorWorlds].Version
					for _, v := range m.versions {
						if v == target {
							m.versionSelect = target
							m.save()
							break
						}
					}
				}
				m.state = menuScreen
			} else if m.state == confirmScreen {
				// [Enter] en confirmScreen → iniciar descarga dentro del TUI
				m.state = downloadScreen
				m.dlDone = 0
				m.dlFile = "Initializing..."
				m.dlErr = nil
				m.dlFinished = false
				ctx, cancel := context.WithCancel(context.Background())
				m.dlContext, m.dlCancel = ctx, cancel
				return m, StartDownloadCmd(ctx, m.versionSelect, m.modloader)
			}

		case "y", "Y":
			if m.state == confirmScreen {
				m.state = downloadScreen
				m.dlDone = 0
				m.dlFile = "Initializing..."
				m.dlErr = nil
				m.dlFinished = false
				ctx, cancel := context.WithCancel(context.Background())
				m.dlContext, m.dlCancel = ctx, cancel
				return m, StartDownloadCmd(ctx, m.versionSelect, m.modloader)
			}

		case "n", "N":
			if m.state == confirmScreen {
				m.state = menuScreen
			}

		case "esc":
			if m.state == nameScreen || m.state == versionsScreen || m.state == worldsScreen || m.state == confirmScreen {
				m.state = menuScreen
			}
		}
	}

	if m.state == nameScreen {
		m.input, cmd = m.input.Update(msg)
	}

	return m, cmd
}

// updateResetKey keeps a purge impossible without a mode choice and a fresh
// explicit confirmation. Bubble Tea executes commands asynchronously.
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
				m.resetBackups, m.resetErr = resetconfigs.ListBackups()
				m.resetBackupIndex = 0
				m.resetScroll = 0
				m.state = resetBackupListScreen
			} else if m.resetErr == nil && len(m.resetFound) > 0 {
				m.resetIsRestore = false
				if m.resetChoice == 0 {
					m.resetMode = resetconfigs.Backup
					m.openResetPicker()
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
			mode, base := m.resetMode, m.resetBackupDir
			m.state = resetRunningScreen
			return m, func() tea.Msg {
				result, err := resetconfigs.RunTo(mode, base)
				return ResetDoneMsg{Result: result, Err: err}
			}
		}
	case resetBackupListScreen:
		switch key {
		case "esc", "q":
			m.resetDir, m.resetFound, m.resetErr = resetconfigs.Inspect()
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
				m.resetBackup, m.resetConflicts, err = resetconfigs.RestorePlan(m.resetBackups[m.resetBackupIndex].Path)
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
			name, mode := m.resetBackup.Path, m.resetRetention
			var replaceFolders []string
			if m.resetReplace {
				replaceFolders = append(replaceFolders, m.resetConflicts...)
			}
			m.resetIsRestore = true
			m.state = resetRunningScreen
			return m, func() tea.Msg {
				result, err := resetconfigs.Restore(name, mode, replaceFolders)
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
	config.Save(config.Data{
		Username:  m.username,
		Version:   m.versionSelect,
		Modloader: m.modloader,
		MemoryMB:  m.memoryMB,
	})
}
