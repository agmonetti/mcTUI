package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
	"mcTUI/internal/mojang"
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

// installationReady reports whether everything needed to launch this
// version + modloader combination already appears to be present locally,
// so launcher.Launch can skip network calls entirely if the user is
// offline.
func installationReady(version, modloader string) bool {
	mcDir := config.MinecraftDir()
	if !mojang.ClientJarExists(mcDir, version) {
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

	case tea.KeyMsg:
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
					if installationReady(m.versionSelect, m.modloader) {
						m.play = true
						return m, tea.Quit
					}
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
				m.play = true
				return m, tea.Quit
			}

		case "y", "Y":
			if m.state == confirmScreen {
				m.play = true
				return m, tea.Quit
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

// save persists the model's current settings to disk.
func (m Model) save() {
	config.Save(config.Data{
		Username:  m.username,
		Version:   m.versionSelect,
		Modloader: m.modloader,
		MemoryMB:  m.memoryMB,
	})
}
