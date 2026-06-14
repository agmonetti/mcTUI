package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
	"mcTUI/internal/java"
	"mcTUI/internal/launcher"
	"mcTUI/internal/mojang"
	"mcTUI/internal/roadmap"
	"mcTUI/internal/worlds"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- VISUAL STYLES ---
var (
	colorMagenta = lipgloss.Color("#FF2E93")
	colorCyan    = lipgloss.Color("#00E5FF")
	colorViolet  = lipgloss.Color("#BD00FF")
	colorGreen   = lipgloss.Color("#00FA9A")
	colorWhite   = lipgloss.Color("#ffffff")
	colorDark    = lipgloss.Color("#444444")
	colorGray    = lipgloss.Color("#888888")
	colorRed     = lipgloss.Color("#FF4444")

	panelMenu = lipgloss.NewStyle().
		Width(32).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorViolet).
		Padding(0, 1)

	panelContent = lipgloss.NewStyle().
		Width(48).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(0, 1)

	panelNews = lipgloss.NewStyle().
		Width(40).
		Height(14).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorMagenta).
		Padding(0, 1)

	panelFooter = lipgloss.NewStyle().
		Width(120).
		Height(3).
		Padding(1, 0)

	titleStyle  = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	itemStyle   = lipgloss.NewStyle().Foreground(colorWhite).PaddingLeft(1)
	cursorStyle = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
)

func getAsciiArt() string {
	lines := []string{
		`  __  __      _______ _    _ _____ `,
		` |  \/  |    |__   __| |  | |_   _|`,
		` | \  / | ___   | |  | |  | | | |  `,
		` | |\/| |/ __|  | |  | |  | | | |  `,
		` | |  | | (__   | |  | |__| |_| |_ `,
		` |_|  |_|\___|  |_|   \____/|_____|`,
	}
	colors := []lipgloss.Color{colorCyan, colorCyan, lipgloss.Color("#00BFFF"), colorViolet, colorMagenta, colorMagenta}
	var styledLines []string
	for i, line := range lines {
		styledLines = append(styledLines, lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line))
	}
	return strings.Join(styledLines, "\n")
}

// installationReady reports whether everything needed to launch this
// version + modloader combination already appears to be present locally,
// so launchGame can skip network calls entirely if the user is offline.
	func installationReady(version string, modloader string) bool {
		if !mojang.ClientJarExists(config.MinecraftDir(), version) {
			return false
		}
		if modloader == "Fabric" && !fabric.ProfileExists(config.MinecraftDir(), version) {
		return false
	}
	return true
}

// --- STATES AND MODEL ---
type screenState int

const (
	menuScreen screenState = iota
	nameScreen
	versionsScreen
	worldsScreen
	confirmScreen
)

type model struct {
	state          screenState
	cursorMenu     int
	cursorVersions int
	cursorWorlds   int

	username      string
	versionSelect string
	memoryMB 	  int
	modloader     string
	versions      []string
	worlds        []worlds.Info
	roadmap       []string

	input textinput.Model
	play  bool

	width     int
	height    int
	javaMajor int // major version of the default "java" in PATH; 0 = not found
}

func initialModel(versions []string, cfg config.Data, roadmap []string, javaMajor int) model {
	ti := textinput.New()
	ti.Placeholder = "Type your name..."
	ti.Focus()
	ti.CharLimit = 16
	ti.Width = 30
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorCyan)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorWhite)

	return model{
		state:         menuScreen,
		cursorMenu:    0,
		username:      cfg.Username,
		versionSelect: cfg.Version,
		modloader:     cfg.Modloader,
		memoryMB:      cfg.MemoryMB,
		versions:      versions,
		roadmap:       roadmap,
		input:         ti,
		play:          false,
		javaMajor:     javaMajor,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// menuOptionsCount must match len(menuOptions) in View(). Centralized here
// so Update()'s cursor bounds check doesn't drift from the menu's actual
// length if options are added/removed.
const menuOptionsCount = 7

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
					// FIX (#3): the confirmation screen must trigger whenever
					// ANYTHING required for this version+modloader combo is
					// missing locally — not just the vanilla client.jar.
					// Previously, switching to Fabric for the first time on
					// an already-installed vanilla version would skip the
					// confirmation and silently start downloading Fabric.
					if installationReady(m.versionSelect, m.modloader) {
						m.play = true
						return m, tea.Quit
					} else {
						m.state = confirmScreen
					}
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
					// Toggle Modloader (Vanilla <-> Fabric)
					if m.modloader == "Vanilla" {
						m.modloader = "Fabric"
					} else {
						m.modloader = "Vanilla"
					}
					config.Save(config.Data{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
				} else if m.cursorMenu == 5 {
					// Cycle memory: 1024 -> 2048 -> 4096 -> 6144 -> 8192 -> 1024...
					steps := []int{1024, 2048, 4096, 6144, 8192}
					for i, v := range steps {
						if v == m.memoryMB {
							m.memoryMB = steps[(i+1)%len(steps)]
							break
						}
					}
					// If current value is not in steps (e.g., custom value), default to 2048
					if m.memoryMB != 1024 && m.memoryMB != 2048 && m.memoryMB != 4096 && m.memoryMB != 6144 && m.memoryMB != 8192 {
						m.memoryMB = 2048
					}
					config.Save(config.Data{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
				} else if m.cursorMenu == 6 {
					return m, tea.Quit
				}
			} else if m.state == nameScreen {
				if validUsername(m.input.Value()) {
					m.username = m.input.Value()
					config.Save(config.Data{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
					m.state = menuScreen
				}
			} else if m.state == versionsScreen {
					m.versionSelect = m.versions[m.cursorVersions]
					config.Save(config.Data{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
					m.state = menuScreen
				} else if m.state == worldsScreen {
					if len(m.worlds) > 0 {
						target := m.worlds[m.cursorWorlds].Version
						// Find if the exact version exists in our stable list
						found := false
						for _, v := range m.versions {
							if v == target {
								found = true
								break
							}
						}
						if found {
							m.versionSelect = target
							config.Save(config.Data{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
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

		case "esc":
			if m.state == nameScreen || m.state == versionsScreen || m.state == worldsScreen || m.state == confirmScreen {
				m.state = menuScreen
			}
			
		case "n", "N":
			if m.state == confirmScreen {
				m.state = menuScreen
			}
		}
	}

	if m.state == nameScreen {
		m.input, cmd = m.input.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	asciiHeader := getAsciiArt() + "\n\n"

	menuStr := strings.Builder{}
	menuStr.WriteString(lipgloss.NewStyle().Foreground(colorViolet).Bold(true).Render(" Options") + "\n\n")

	menuOptions := []string{
		fmt.Sprintf("Play (%s)", m.versionSelect),
		"Change Name",
		"Change Version",
		"Worlds",
		fmt.Sprintf("Modloader: %s", lipgloss.NewStyle().Foreground(colorCyan).Render(m.modloader)),
		fmt.Sprintf("Memory: %d MB", m.memoryMB),
		"Quit",
	}

	for i, option := range menuOptions {
		if m.cursorMenu == i {
			menuStr.WriteString(cursorStyle.Render(fmt.Sprintf("▶ %s", option)) + "\n")
		} else {
			menuStr.WriteString(itemStyle.Render(fmt.Sprintf("  %s", option)) + "\n")
		}
	}

	contentStr := strings.Builder{}
	contentStr.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(" mcTUI Launcher") + "\n\n")

	if m.state == menuScreen {
		if m.cursorMenu == 5 {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Memory Configuration") + "\n\n")
			contentStr.WriteString(fmt.Sprintf("Current Allocation : %s MB\n\n", lipgloss.NewStyle().Foreground(colorWhite).Render(fmt.Sprintf("%d", m.memoryMB))))
			contentStr.WriteString("Recommended RAM:\n")
			contentStr.WriteString("  Vanilla 1.20.x    →  1-2 GB (1024-2048 MB)\n")
			contentStr.WriteString("  Fabric/light mods →  2-4 GB (2048-4096 MB)\n")
			contentStr.WriteString("  Heavy modpacks    →  4-8 GB (4096-8192 MB)\n\n")
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Press [Enter] to cycle values."))
		} else if m.cursorMenu == 3 {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Local Worlds") + "\n\n")
			contentStr.WriteString("View and manage your saved singleplayer worlds.\n\n")
			contentStr.WriteString("Selecting a world will automatically change\n")
			contentStr.WriteString("your launcher version to match the world's\n")
			contentStr.WriteString("last played version to prevent corruption.\n\n")
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Press [Enter] to browse saves."))
		} else {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(" Active Session") + "\n\n")
			contentStr.WriteString(fmt.Sprintf("User     : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(m.username)))

			verString := m.versionSelect
			if m.modloader != "Vanilla" {
				verString += fmt.Sprintf(" (%s)", m.modloader)
			}
			contentStr.WriteString(fmt.Sprintf("Version  : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(verString)))

			// FORMATTING THE OS NAME
			osName := runtime.GOOS
			if osName == "darwin" {
				osName = "macOS"
			} else if osName == "windows" {
				osName = "Windows"
			} else if osName == "linux" {
				osName = "Linux"
			}

			contentStr.WriteString("Auth     : Offline (LAN Mode)\n")
			contentStr.WriteString(fmt.Sprintf("OS       : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(osName)))

			required := mojang.RequiredJavaVersion(config.MinecraftDir(), m.versionSelect)
			javaLine := java.FormatInfo(m.javaMajor, required)
			javaStyle := lipgloss.NewStyle().Foreground(colorWhite)
			if required > 0 && m.javaMajor < required {
				javaStyle = lipgloss.NewStyle().Foreground(colorRed)
			}
			contentStr.WriteString(fmt.Sprintf("Java     : %s\n\n", javaStyle.Render(javaLine)))

			if installationReady(m.versionSelect, m.modloader) {
				contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready (offline-capable)"))
			} else {
				contentStr.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("● Not installed — will need network"))
			}
		}

	} else if m.state == nameScreen {
		contentStr.WriteString("New LAN username:\n\n")
		contentStr.WriteString(m.input.View())
		contentStr.WriteString("\n\n")
		if m.input.Value() != "" && !validUsername(m.input.Value()) {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("⚠ Letters, numbers, and underscore only (1-16 chars)"))
		} else {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("Letters, numbers, and underscore only (1-16 chars)"))
		}

	} else if m.state == versionsScreen {
		contentStr.WriteString("Select a stable version:\n\n")

		start := m.cursorVersions - 3
		if start < 0 {
			start = 0
		}
		end := start + 6
		if end > len(m.versions) {
			end = len(m.versions)
			start = end - 6
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < end; i++ {
			if i == m.cursorVersions {
				contentStr.WriteString(cursorStyle.Render(fmt.Sprintf("  ▶ %s", m.versions[i])) + "\n")
			} else {
				contentStr.WriteString(fmt.Sprintf("    %s", m.versions[i]) + "\n")
			}
		}
	} else if m.state == worldsScreen {
		contentStr.WriteString("Select a world to load:\n\n")

		if len(m.worlds) == 0 {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("No worlds found in ~/.minecraft/saves"))
		} else {
			start := m.cursorWorlds - 3
			if start < 0 {
				start = 0
			}
			end := start + 6
			if end > len(m.worlds) {
				end = len(m.worlds)
				start = end - 6
				if start < 0 {
					start = 0
				}
			}

			for i := start; i < end; i++ {
				w := m.worlds[i]

				verStyle := lipgloss.NewStyle().Foreground(colorRed)
				if w.Version == m.versionSelect {
					verStyle = lipgloss.NewStyle().Foreground(colorGreen)
				}

				versionPart := verStyle.Render(" (" + w.Version + ")")
				timePart := ""
				if lastPlayed := worlds.FormatRelativeTime(w.LastPlayed); lastPlayed != "" {
					timePart = " " + lipgloss.NewStyle().Foreground(colorGray).Render("· "+lastPlayed)
				}

				if i == m.cursorWorlds {
					contentStr.WriteString(cursorStyle.Render("  ▶ "+w.LevelName) + versionPart + timePart + "\n")
				} else {
					contentStr.WriteString(itemStyle.Render("    "+w.LevelName) + versionPart + timePart + "\n")
				}
			}
		}
	} else if m.state == confirmScreen {
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ MISSING FILES") + "\n\n")

		if m.modloader == "Fabric" && !fabric.ProfileExists(config.MinecraftDir(), m.versionSelect) {
			contentStr.WriteString(fmt.Sprintf("Fabric libraries for %s are not yet\ncached on your system.\n\n", m.versionSelect))
		} else {
			contentStr.WriteString(fmt.Sprintf("The client.jar file (%s) is not\nfound on your system.\n\n", m.versionSelect))
		}
		contentStr.WriteString("Would you like to download the missing\nfiles from the official Mojang/Fabric\nservers?\n\n")
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("[y/Enter] Yes") + "   " + lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancel"))
	}

	newsStr := strings.Builder{}
	newsStr.WriteString(lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render(" Future Changes") + "\n\n")

	for _, item := range m.roadmap {
		newsStr.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(item) + "\n")
	}
	newsStr.WriteString("\n" + lipgloss.NewStyle().Foreground(colorDark).Render("Stay tuned..."))

	controls := " [↑/↓] Navigate  [Enter] Select  [q] Quit"
	if m.state == nameScreen {
		controls = " [Enter] Save  [Esc] Cancel"
		} else if m.state == versionsScreen {
			controls = " [↑/↓] Move list  [Enter] Choose  [Esc] Back"
		} else if m.state == worldsScreen {
			controls = " [↑/↓] Move list  [Enter] Select  [Esc] Back"
		} else if m.state == confirmScreen {
		controls = " [y] Accept  [n] Cancel"
	}

	separator := lipgloss.NewStyle().Foreground(colorDark).Render(strings.Repeat("─", 120))

	var statusPart string
	if installationReady(m.versionSelect, m.modloader) {
		statusPart = lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready")
	} else {
		statusPart = lipgloss.NewStyle().Foreground(colorRed).Render("● Needs setup")
	}

	userPart := lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("[%s - %s]", m.username, m.versionSelect))
	controlsPart := lipgloss.NewStyle().Foreground(colorDark).Render(controls)

	footerContent := fmt.Sprintf("%s\n%s   %s   %s", separator, statusPart, userPart, controlsPart)

	topPanels := lipgloss.JoinHorizontal(lipgloss.Top,
		panelMenu.Render(menuStr.String()),
		panelContent.Render(contentStr.String()),
		panelNews.Render(newsStr.String()),
	)

	fullInterface := lipgloss.JoinVertical(lipgloss.Center, asciiHeader, topPanels, panelFooter.Render(footerContent))

	if m.width > 0 && m.height > 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, fullInterface)
	}
	return "\n" + fullInterface
}

func main() {
	validReleases := mojang.FetchReleases()
	cfg := config.Load()
	javaMajor := java.VersionAt("java")
	roadmapItems := roadmap.Load()

	p := tea.NewProgram(initialModel(validReleases, cfg, roadmapItems, javaMajor), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(model); ok && m.play {
		launcher.Launch(config.MinecraftDir(), m.username, m.versionSelect, m.modloader, m.memoryMB)
	}
}

// --- FABRIC SUPPORT ---

// fabricProfile holds the resolved data needed to launch Fabric: the main

// validUsername reports whether name is a valid Minecraft username:
// 1-16 characters, letters/digits/underscore only. This matches the
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
