package main

import (
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
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

// --- PLATFORM AGNOSTIC PATHS ---

// getAppDataDir returns the base directory used for app-specific data
// (config, roadmap, etc), following OS conventions. subdirs are appended
// to that base, e.g. getAppDataDir("mctui", "config.json").
func getAppDataDir(subdirs ...string) string {
	var base string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			base = appData
		} else {
			homeDir, _ := os.UserHomeDir()
			base = filepath.Join(homeDir, "AppData", "Roaming")
		}
	} else {
		homeDir, _ := os.UserHomeDir()
		base = filepath.Join(homeDir, ".config")
	}
	return filepath.Join(append([]string{base}, subdirs...)...)
}

func getMinecraftDir() string {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, ".minecraft")
		}
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "AppData", "Roaming", ".minecraft")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".minecraft")
}

func getConfigPath() string {
	if runtime.GOOS == "windows" {
		return getAppDataDir("mctui", "config.json")
	}
	return getAppDataDir("mctui", "config.json")
}



// --- PERSISTENCE ---
type ConfigData struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
	MemoryMB  int    `json:"memory_mb"`
}

func loadConfig() ConfigData {
	path := getConfigPath()
	data, err := os.ReadFile(path)

	defaultConfig := ConfigData{Username: "Player", Version: "1.20.4", Modloader: "Vanilla",MemoryMB: 2048}
	if err != nil {
		return defaultConfig
	}

	var config ConfigData
	json.Unmarshal(data, &config)

	if config.Username == "" {
		config.Username = defaultConfig.Username
	}
	if config.Version == "" {
		config.Version = defaultConfig.Version
	}
	if config.Modloader == "" {
		config.Modloader = defaultConfig.Modloader
	}
	
	if config.MemoryMB == 0 {
		config.MemoryMB = defaultConfig.MemoryMB
	}
	
	return config
}

func saveConfig(c ConfigData) {
	path := getConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0644)
}

const roadmapURL = "https://raw.githubusercontent.com/agmonetti/mcTUI/main/roadmap.json"

// embeddedRoadmap is the fallback shown when the remote roadmap can't be
// fetched (offline, GitHub down, etc). Update this whenever you cut a
// release, so offline users still see something reasonably current.
var embeddedRoadmap = []string{
	"• Microsoft Auth",
	"• Custom JVM Arguments",
	"• Expanded UI Themes",
}

var roadmapClient = &http.Client{Timeout: 2 * time.Second}

func loadRoadmap() []string {
	resp, err := roadmapClient.Get(roadmapURL)
	if err != nil || resp == nil {
		return embeddedRoadmap
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return embeddedRoadmap
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return embeddedRoadmap
	}

	var result map[string][]string
	if json.Unmarshal(body, &result) != nil {
		return embeddedRoadmap
	}

	if changes, ok := result["changes"]; ok && len(changes) > 0 {
		return changes
	}
	return embeddedRoadmap
}

// --- VERSION FETCHING & FILE CHECKS ---
func fetchReleases() []string {
	resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil || resp == nil {
		return []string{"1.20.4"}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var manifest struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	json.Unmarshal(body, &manifest)

	var releases []string
	for _, v := range manifest.Versions {
		if v.Type == "release" {
			releases = append(releases, v.ID)
		}
	}
	if len(releases) == 0 {
		return []string{"1.20.4"}
	}
	return releases
}

func clientJarExists(version string) bool {
	path := filepath.Join(getMinecraftDir(), "versions", version, "client.jar")
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// requiredJavaVersion reads the cached version.json for a given Minecraft
// version (if present) and returns its javaVersion.majorVersion, or 0 if
// the file doesn't exist, can't be parsed, or doesn't specify one.
func requiredJavaVersion(version string) int {
	path := filepath.Join(getMinecraftDir(), "versions", version, "version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var v struct {
		JavaVersion struct {
			MajorVersion int `json:"majorVersion"`
		} `json:"javaVersion"`
	}
	if json.Unmarshal(data, &v) != nil {
		return 0
	}
	return v.JavaVersion.MajorVersion
}

// fabricProfilePath returns where we cache the resolved Fabric launch profile
// (mainClass + libraries) for a given Minecraft version, so that a previously
// launched Fabric setup can be detected without hitting the network again.
func fabricProfilePath(version string) string {
	return filepath.Join(getMinecraftDir(), "versions", version, "fabric-profile.json")
}

func fabricProfileExists(version string) bool {
	info, err := os.Stat(fabricProfilePath(version))
	return err == nil && info.Size() > 0
}

// installationReady reports whether everything needed to launch this
// version + modloader combination already appears to be present locally,
// so launchGame can skip network calls entirely if the user is offline.
func installationReady(version string, modloader string) bool {
	if !clientJarExists(version) {
		return false
	}
	if modloader == "Fabric" && !fabricProfileExists(version) {
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
	worlds        []worldInfo
	roadmap       []string

	input textinput.Model
	play  bool

	width     int
	height    int
	javaMajor int // major version of the default "java" in PATH; 0 = not found
}

func initialModel(versions []string, cfg ConfigData, roadmap []string, javaMajor int) model {
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
					m.worlds = listWorlds()
					m.state = worldsScreen
					m.cursorWorlds = 0
				} else if m.cursorMenu == 4 {
					// Toggle Modloader (Vanilla <-> Fabric)
					if m.modloader == "Vanilla" {
						m.modloader = "Fabric"
					} else {
						m.modloader = "Vanilla"
					}
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
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
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
				} else if m.cursorMenu == 6 {
					return m, tea.Quit
				}
			} else if m.state == nameScreen {
				if m.input.Value() != "" {
					m.username = m.input.Value()
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
				}
				m.state = menuScreen
				} else if m.state == versionsScreen {
					m.versionSelect = m.versions[m.cursorVersions]
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
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
							saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader, MemoryMB: m.memoryMB})
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

		case "n", "N", "esc":
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

			required := requiredJavaVersion(m.versionSelect)
			javaLine := formatJavaInfo(m.javaMajor, required)
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
				if lastPlayed := formatRelativeTime(w.LastPlayed); lastPlayed != "" {
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

		if m.modloader == "Fabric" && !fabricProfileExists(m.versionSelect) {
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
	validReleases := fetchReleases()
	cfg := loadConfig()
	javaMajor := checkJavaVersionAt("java")
	roadmap := loadRoadmap()

	p := tea.NewProgram(initialModel(validReleases, cfg, roadmap, javaMajor), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(model); ok && m.play {
		launchGame(m.username, m.versionSelect, m.modloader, m.memoryMB)
	}
}

// formatJavaInfo builds the "Java : ..." line for Active Session,
// cross-checking the installed default against the selected version's
// requirement when known.
func formatJavaInfo(installedMajor int, requiredMajor int) string {
	if installedMajor == 0 {
		return "not found"
	}
	if requiredMajor == 0 {
		return fmt.Sprintf("%d (default)", installedMajor)
	}
	if installedMajor >= requiredMajor {
		return fmt.Sprintf("%d (default, OK)", installedMajor)
	}
	return fmt.Sprintf("%d (default) — needs %d+ ⚠", installedMajor, requiredMajor)
}

// javaCandidate represents a discovered Java installation.
type javaCandidate struct {
	path  string // path to the "java" binary
	major int    // detected major version
}

// findJavaBinary returns the path to a "java" binary that satisfies
// requiredMajor (0 = no requirement, any java works), preferring the
// PATH default if it already qualifies. If no installed JRE satisfies
// the requirement, it returns the PATH default anyway (so the caller can
// fail with a clear message) along with ok=false.
func findJavaBinary(requiredMajor int) (javaCandidate, bool) {
	pathJava := javaCandidate{path: "java", major: checkJavaVersionAt("java")}

	if requiredMajor == 0 || (pathJava.major > 0 && pathJava.major >= requiredMajor) {
		return pathJava, true
	}

	// PATH default doesn't qualify - try Mojang's own bundled runtimes first,
	// since they're purpose-matched to specific MC versions.
	for _, candidate := range scanEmbeddedRuntimes() {
		if candidate.major >= requiredMajor {
			return candidate, true
		}
	}

	// Fall back to generic system-wide JRE/JDK installations.
	for _, candidate := range scanJavaInstallations() {
		if candidate.major >= requiredMajor {
			return candidate, true
		}
	}

	return pathJava, false
}

// checkJavaVersionAt runs "<binaryPath> -version" and parses the major
// version (handling both old "1.8.0_xxx" and new "21.0.x" / "25" formats).
// Returns 0 if it couldn't be determined (e.g. binary not found).
func checkJavaVersionAt(binaryPath string) int {
	cmd := exec.Command(binaryPath, "-version")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return 0
	}

	output := out.String()
	start := strings.Index(output, "\"")
	if start == -1 {
		return 0
	}
	end := strings.Index(output[start+1:], "\"")
	if end == -1 {
		return 0
	}
	versionStr := output[start+1 : start+1+end]

	parts := strings.Split(versionStr, ".")
	if len(parts) == 0 {
		return 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	if major == 1 && len(parts) > 1 {
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			return minor
		}
	}
	return major
}

// scanEmbeddedRuntimes looks for JREs bundled by the official Mojang/Microsoft
// launcher under <mcDir>/runtime/. These are often newer than the user's
// system Java and require no extra installation — if the user has ever
// played via the official launcher, a compatible JRE is frequently already
// sitting here.
//
// Structure: <mcDir>/runtime/<component>/<os-dir>/<component>/bin/java(.exe)
// e.g. .minecraft/runtime/java-runtime-delta/windows-x64/java-runtime-delta/bin/javaw.exe
//
// We glob both variable segments (<component> and <os-dir>) rather than
// hardcoding them, since they vary by platform/architecture and by which
// Minecraft versions were ever installed.
func scanEmbeddedRuntimes() []javaCandidate {
	runtimeRoot := filepath.Join(getMinecraftDir(), "runtime")

	binName := "java"
	if runtime.GOOS == "windows" {
		binName = "java.exe"
	}

	// Pattern: runtime/*/*/*/bin/java(.exe)
	// segments: <component>/<os-dir>/<component again>/bin/<binary>
	pattern := filepath.Join(runtimeRoot, "*", "*", "*", "bin", binName)
	binPaths := globDirs(pattern)

	var candidates []javaCandidate
	for _, binPath := range binPaths {
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		major := checkJavaVersionAt(binPath)
		if major > 0 {
			candidates = append(candidates, javaCandidate{path: binPath, major: major})
		}
	}

	// Sort descending by major version (same as scanJavaInstallations).
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j-1].major < candidates[j].major; j-- {
			candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
		}
	}
	return candidates
}


// scanJavaInstallations looks in OS-conventional locations for installed
// JREs/JDKs and returns them sorted by major version, descending.
func scanJavaInstallations() []javaCandidate {
	var dirs []string

	switch runtime.GOOS {
	case "linux":
		dirs = globDirs("/usr/lib/jvm/*")
	case "darwin":
		dirs = globDirs("/Library/Java/JavaVirtualMachines/*/Contents/Home")

	case "windows":
		dirs = globDirs(`C:\Program Files\Java\*`)
		dirs = append(dirs, globDirs(`C:\Program Files\Eclipse Adoptium\*`)...)
		dirs = append(dirs, globDirs(`C:\Program Files\Microsoft\*`)...)
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			dirs = append(dirs, globDirs(filepath.Join(userProfile, ".jdks", "*"))...)
		}

	}

	var candidates []javaCandidate
	for _, dir := range dirs {
		binName := "java"
		if runtime.GOOS == "windows" {
			binName = "java.exe"
		}
		binPath := filepath.Join(dir, "bin", binName)
		if _, err := os.Stat(binPath); err != nil {
			continue
		}
		major := checkJavaVersionAt(binPath)
		if major > 0 {
			candidates = append(candidates, javaCandidate{path: binPath, major: major})
		}
	}

	// Sort descending by major version (simple insertion sort, small N).
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j-1].major < candidates[j].major; j-- {
			candidates[j-1], candidates[j] = candidates[j], candidates[j-1]
		}
	}
	return candidates
}

// globDirs is a small wrapper around filepath.Glob that returns an empty
// slice instead of an error.
func globDirs(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

// printLogTail prints the last n lines of the file at path. Used to
// surface JVM crash output (e.g. UnsupportedClassVersionError,
// UnsatisfiedLinkError) without requiring the user to find the log
// themselves.
func printLogTail(path string, n int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("(could not read log file:", err, ")")
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := len(lines) - n
	if start < 0 {
		start = 0
	}
	for _, line := range lines[start:] {
		fmt.Println(line)
	}
}

// --- THE GAME LAUNCHING ENGINE ---
//
// IMPORTANT (FIX #2 - panic on offline launches):
// Every http.Get in this function now checks its error AND nils the
// response before touching resp.Body. In Go, if err != nil, resp is
// guaranteed to be nil — calling resp.Body on a nil *http.Response is a
// nil-pointer dereference and crashes the whole process.
//
// We also restructure the flow so that if everything required is already
// cached locally (installationReady == true), we avoid network calls
// entirely and launch directly. If the manifest fetch fails BUT the local
// client.jar + asset index already exist, we fall back to using the cached
// asset index instead of aborting.
func launchGame(username string, targetVersion string, modloader string, memoryMB int) {
	fmt.Print("\033[H\033[2J")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println(" MODE: Offline / LAN")
	fmt.Println(" (The server must have 'online-mode=false' in server.properties)")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println()

	fmt.Printf("Starting engine for player: %s (Version: %s - Modloader: %s)\n", username, targetVersion, modloader)
	fmt.Println("1. Verifying Vanilla Engine and Libraries...")

	type Version struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	type Manifest struct {
		Versions []Version `json:"versions"`
	}

	type VersionData struct {
		AssetIndex struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"assetIndex"`
		JavaVersion struct {
			MajorVersion int `json:"majorVersion"`
		} `json:"javaVersion"`
		Downloads struct {
			Client struct {
				URL string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"client"`
		} `json:"downloads"`
		Libraries []struct {
			Downloads struct {
				Artifact struct {
					Path string `json:"path"`
					URL  string `json:"url"`
					SHA1 string `json:"sha1"`
				} `json:"artifact"`
			} `json:"downloads"`
		} `json:"libraries"`
	}

	mcDir := getMinecraftDir()
	clientPath := filepath.Join(mcDir, "versions", targetVersion, "client.jar")
	haveClient := clientJarExists(targetVersion)

	var manifest Manifest
	var data VersionData
	haveVersionData := false

	// Try to fetch the manifest + version data. This is needed to know the
	// asset index ID, library list, and client download URL. If it fails
	// and we already have a client.jar locally, we try to recover by
	// reading a cached version JSON if one exists; otherwise we abort
	// gracefully (no panic) with a clear message.
	resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil || resp == nil {
		fmt.Println("[!] Could not reach Mojang servers (no internet?).")
	} else {
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr == nil {
			if jsonErr := json.Unmarshal(body, &manifest); jsonErr == nil {
				var specificURL string
				for _, v := range manifest.Versions {
					if v.ID == targetVersion {
						specificURL = v.URL
						break
					}
				}

				if specificURL != "" {
					respVersion, vErr := http.Get(specificURL)
					if vErr != nil || respVersion == nil {
						fmt.Println("[!] Could not fetch version metadata (no internet?).")
					} else {
						bodyVersion, _ := io.ReadAll(respVersion.Body)
						respVersion.Body.Close()
						if json.Unmarshal(bodyVersion, &data) == nil {
							haveVersionData = true
							// Cache the version data locally so a future
							// offline launch can still resolve the asset
							// index ID and library list if needed.
							cachePath := filepath.Join(mcDir, "versions", targetVersion, "version.json")
							os.MkdirAll(filepath.Dir(cachePath), 0755)
							os.WriteFile(cachePath, bodyVersion, 0644)
						}
					}
				} else {
					fmt.Println("[!] Version", targetVersion, "not found in Mojang manifest.")
				}
			}
		}
	}

	// If we couldn't fetch fresh version data from the network, try to use
	// a previously cached copy so an offline launch can still proceed.
	if !haveVersionData {
		cachePath := filepath.Join(mcDir, "versions", targetVersion, "version.json")
		if cached, readErr := os.ReadFile(cachePath); readErr == nil {
			if json.Unmarshal(cached, &data) == nil {
				haveVersionData = true
				fmt.Println("   Using cached version metadata.")
			}
		}
	}

	// If we still have nothing AND no local client.jar, we cannot proceed.
	if !haveVersionData && !haveClient {
		fmt.Println("\n[!] Cannot launch: no internet connection and no local")
		fmt.Println("    installation found for version", targetVersion+".")
		fmt.Println("    Connect to the internet at least once to download it.")
		return
	}

	var classpathEntries []string
	librariesPath := filepath.Join(mcDir, "libraries")
	var wg sync.WaitGroup

	// FIX (#5): cap concurrent downloads so we don't open hundreds of
	// simultaneous connections. Shared across vanilla libs, Fabric libs,
	// and assets (each phase uses its own buffered semaphore below).
	const maxConcurrentDownloads = 20

	// download fetches url into destination. If expectedSHA1 is non-empty,
	// the downloaded content is verified against it; on mismatch, the
	// partial/corrupt file is removed so a future run will retry instead of
	// treating it as already-installed.
	//
	// If destination already exists with size > 0, download skips the
	// fetch entirely (existing behavior) — hash verification only applies
	// to freshly downloaded content, to avoid re-hashing potentially large
	// files (assets, client.jar) on every launch.
	download := func(url, destination, expectedSHA1 string) {
		defer wg.Done()
		if url == "" {
			return
		}
		os.MkdirAll(filepath.Dir(destination), 0755)
		if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
			return
		}
		r, err := http.Get(url)
		if err != nil || r == nil {
			return
		}
		defer r.Body.Close()

		f, err := os.Create(destination)
		if err != nil {
			return
		}

		if expectedSHA1 == "" {
			io.Copy(f, r.Body)
			f.Close()
			return
		}

		// Compute the hash while writing, so we don't need a second pass.
		h := sha1.New()
		_, copyErr := io.Copy(io.MultiWriter(f, h), r.Body)
		f.Close()

		if copyErr != nil {
			os.Remove(destination)
			return
		}

		if fmt.Sprintf("%x", h.Sum(nil)) != expectedSHA1 {
			fmt.Printf("   [!] Checksum mismatch for %s, removing.\n", filepath.Base(destination))
			os.Remove(destination)
		}
	}

	// Download client.jar if missing and we have a URL for it.
	if !haveClient {
		if !haveVersionData || data.Downloads.Client.URL == "" {
			fmt.Println("\n[!] Cannot download client.jar: no version data available.")
			return
		}
		wg.Add(1)
		download(data.Downloads.Client.URL, clientPath, data.Downloads.Client.SHA1)
		wg.Wait() // wait specifically for the client jar before proceeding
	}

	if haveVersionData {
		libSem := make(chan struct{}, maxConcurrentDownloads)
		for _, lib := range data.Libraries {
			url := lib.Downloads.Artifact.URL
			path := lib.Downloads.Artifact.Path
			if url != "" && path != "" {
				fullPath := filepath.Join(librariesPath, path)
				classpathEntries = append(classpathEntries, fullPath)
				wg.Add(1)
				libSem <- struct{}{}
				go func(u, d, sha1 string) {
					defer func() { <-libSem }()
					download(u, d, sha1)
				}(url, fullPath, lib.Downloads.Artifact.SHA1)
			}
		}
	} else {
		fmt.Println("   No version metadata available — skipping library check (offline mode).")
	}

	mainClass := "net.minecraft.client.main.Main"

	if modloader == "Fabric" {
		fmt.Println("-> Fabric injection detected. Resolving profile...")

		profile, ok := resolveFabricProfile(targetVersion, mcDir, librariesPath, &wg, download, maxConcurrentDownloads)
		if ok {
			mainClass = profile.MainClass
			for _, entry := range profile.ClasspathEntries {
				classpathEntries = append(classpathEntries, entry)
			}
		} else {
			fmt.Println("[!] Fabric profile unavailable (offline and not cached, or no")
			fmt.Println("    compatible loader for this version). Falling back to Vanilla.")
		}
	}

	fmt.Println("2. Validating Assets...")
	indexPath := filepath.Join(mcDir, "assets", "indexes", data.AssetIndex.ID+".json")
	haveAssetIndex := false

	if info, err := os.Stat(indexPath); err == nil && info.Size() > 0 {
		haveAssetIndex = true
	} else if haveVersionData && data.AssetIndex.URL != "" {
		os.MkdirAll(filepath.Dir(indexPath), 0755)
		respIndex, iErr := http.Get(data.AssetIndex.URL)
		if iErr != nil || respIndex == nil {
			fmt.Println("[!] Could not download asset index (no internet?). Skipping asset validation.")
		} else {
			indexFile, fErr := os.Create(indexPath)
			if fErr != nil {
				fmt.Println("[!] Could not write asset index:", fErr)
				respIndex.Body.Close()
			} else {
				io.Copy(indexFile, respIndex.Body)
				indexFile.Close()
				respIndex.Body.Close()
				haveAssetIndex = true
			}
		}
	} else {
		fmt.Println("   No asset index available locally and no version data — skipping asset validation (offline mode).")
	}

	if haveAssetIndex {
		indexBytes, _ := os.ReadFile(indexPath)
		var assetIndexData struct {
			Objects map[string]struct {
				Hash string `json:"hash"`
			} `json:"objects"`
		}
		json.Unmarshal(indexBytes, &assetIndexData)

		assetSem := make(chan struct{}, 50)
		for _, obj := range assetIndexData.Objects {
			hash := obj.Hash
			subDir := hash[:2]
			url := "https://resources.download.minecraft.net/" + subDir + "/" + hash
			dest := filepath.Join(mcDir, "assets", "objects", subDir, hash)

			wg.Add(1)
			assetSem <- struct{}{}
			go func(u, d, h string) {
				defer func() { <-assetSem }()
				download(u, d, h)
			}(url, dest, obj.Hash)
		}
	}

	wg.Wait()
	classpathEntries = append(classpathEntries, clientPath)

	finalClasspath := strings.Join(classpathEntries, string(filepath.ListSeparator))

	sessionUUID := uuid.New().String()

	fmt.Println("3. All set! Launching Minecraft...")

	assetIndexID := data.AssetIndex.ID
	if assetIndexID == "" {
		// Best-effort fallback: derive from the index filename if version
		// data wasn't available but a cached index was found.
		if haveAssetIndex {
			base := filepath.Base(indexPath)
			assetIndexID = strings.TrimSuffix(base, ".json")
		}
	}

	javaBinary := "java"
	if haveVersionData && data.JavaVersion.MajorVersion > 0 {
		candidate, ok := findJavaBinary(data.JavaVersion.MajorVersion)
		if !ok {
			fmt.Println()
			fmt.Printf("[!] This version requires Java %d or newer, but no compatible\n", data.JavaVersion.MajorVersion)
			fmt.Println("[!] JRE was found (checked PATH and common install locations).")
			fmt.Println("[!] Install a newer JRE, e.g.:")
			fmt.Println("[!]   Arch:    pacman -S jdk-openjdk")
			fmt.Println("[!]   Debian:  apt install openjdk-21-jre  (or newer)")
			return
		}
		javaBinary = candidate.path
		if candidate.path != "java" {
			fmt.Printf("   Using Java %d from %s (system default did not meet requirement)\n", candidate.major, candidate.path)
		}
	}

	cmd := exec.Command(javaBinary,
		fmt.Sprintf("-Xmx%dM", memoryMB),
		"-cp", finalClasspath,
		mainClass,
		"--username", username,
		"--version", targetVersion,
		"--gameDir", mcDir,
		"--assetsDir", filepath.Join(mcDir, "assets"),
		"--assetIndex", assetIndexID,
		"--uuid", sessionUUID,
		"--accessToken", "0",
		"--userType", "legacy",
		"--versionType", "release",
	)

	logFile, err := os.Create(filepath.Join(mcDir, "mctui_latest.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	err = cmd.Start()
	if err != nil {
		fmt.Println("\n[!] Error starting process:", err)
		return
	}
	
	fmt.Println("   Minecraft is starting in the background...")
	
	// Watch for early crashes (process exits within a few seconds of
	// starting). If that happens, surface the tail of the JVM log instead
	// of leaving the user with no clue what went wrong.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case waitErr := <-done:
		logFile.Close() // flush before reading
		fmt.Println()
		if waitErr != nil {
			fmt.Println("[!] Minecraft exited early with an error.")
		} else {
			fmt.Println("[!] Minecraft exited almost immediately.")
		}
		fmt.Println("[!] Last lines of the log:")
		fmt.Println(strings.Repeat("-", 60))
		printLogTail(filepath.Join(mcDir, "mctui_latest.log"), 25)
		fmt.Println(strings.Repeat("-", 60))

	case <-time.After(8 * time.Second):
		fmt.Println()
		fmt.Println("   Minecraft is running.")
		fmt.Println("   You can close this window — the game will keep running")
		fmt.Println("   in the background as a separate process.")
	}
}

// --- FABRIC SUPPORT ---

// fabricProfile holds the resolved data needed to launch Fabric: the main
// class to invoke instead of the vanilla one, and the extra classpath
// entries (Fabric loader + dependencies).
type fabricProfile struct {
	MainClass        string
	ClasspathEntries []string
}

// resolveFabricProfile tries to obtain the Fabric launch profile for the
// given Minecraft version. It first checks for a cached profile on disk
// (written by a previous successful resolution); if absent, it queries the
// Fabric meta API. All network errors are handled gracefully — on any
// failure this returns ok=false so the caller can fall back to Vanilla
// instead of panicking or aborting the whole launch.
//
// FIX (#5): library downloads triggered here are capped via a semaphore,
// same as vanilla libraries and assets.
func resolveFabricProfile(targetVersion, mcDir, librariesPath string, wg *sync.WaitGroup, download func(url, dest, expectedSHA1 string), maxConcurrent int) (fabricProfile, bool) {
	cachePath := fabricProfilePath(targetVersion)

	// Try cached profile first (enables fully offline Fabric launches).
	if cached, err := os.ReadFile(cachePath); err == nil {
		var p fabricProfile
		if json.Unmarshal(cached, &p) == nil && p.MainClass != "" {
			fmt.Println("   Using cached Fabric profile.")
			return p, true
		}
	}

	loaderURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", targetVersion)
	loaderResp, err := http.Get(loaderURL)
	if err != nil || loaderResp == nil {
		fmt.Println("[!] Could not reach Fabric meta servers (no internet?).")
		return fabricProfile{}, false
	}
	loaderBody, _ := io.ReadAll(loaderResp.Body)
	loaderResp.Body.Close()

	var loaderData []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if jsonErr := json.Unmarshal(loaderBody, &loaderData); jsonErr != nil || len(loaderData) == 0 {
		fmt.Println("[!] Fabric does not have a compatible loader for this version.")
		return fabricProfile{}, false
	}

	latestLoader := loaderData[0].Loader.Version
	fmt.Printf("-> Downloading Fabric bridge libraries v%s...\n", latestLoader)

	profileURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/profile/json", targetVersion, latestLoader)
	profileResp, pErr := http.Get(profileURL)
	if pErr != nil || profileResp == nil {
		fmt.Println("[!] Could not fetch Fabric profile (no internet?).")
		return fabricProfile{}, false
	}
	profileBody, _ := io.ReadAll(profileResp.Body)
	profileResp.Body.Close()

	var rawProfile struct {
		MainClass string `json:"mainClass"`
		Libraries []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"libraries"`
	}
	if jsonErr := json.Unmarshal(profileBody, &rawProfile); jsonErr != nil || rawProfile.MainClass == "" {
		fmt.Println("[!] Fabric profile response was invalid.")
		return fabricProfile{}, false
	}

	result := fabricProfile{MainClass: rawProfile.MainClass}

	fabricSem := make(chan struct{}, maxConcurrent)
	for _, lib := range rawProfile.Libraries {
		parts := strings.Split(lib.Name, ":")
		if len(parts) < 3 {
			continue
		}
		group := strings.ReplaceAll(parts[0], ".", "/")
		artifact := parts[1]
		version := parts[2]
		path := fmt.Sprintf("%s/%s/%s/%s-%s.jar", group, artifact, version, artifact, version)

		baseURL := lib.URL
		if baseURL == "" {
			baseURL = "https://maven.fabricmc.net/"
		}
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}

		fullURL := baseURL + path
		fullDest := filepath.Join(librariesPath, path)

		result.ClasspathEntries = append(result.ClasspathEntries, fullDest)
		wg.Add(1)
		fabricSem <- struct{}{}
		go func(u, d string) {
			defer func() { <-fabricSem }()
			download(u, d, "")
		}(fullURL, fullDest)
	}

	// Cache the resolved profile so future launches can work offline.
	if data, mErr := json.MarshalIndent(result, "", "  "); mErr == nil {
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, data, 0644)
	}

	return result, true
}

// --- WORLD LISTING (NBT level.dat parsing) ---

type worldInfo struct {
	FolderName string
	LevelName  string
	Version    string
	LastPlayed int64
}

func listWorlds() []worldInfo {
	savesDir := filepath.Join(getMinecraftDir(), "saves")
	entries, err := os.ReadDir(savesDir)
	if err != nil {
		return nil
	}

	var worlds []worldInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		levelPath := filepath.Join(savesDir, entry.Name(), "level.dat")
		info, ok := parseLevelDat(levelPath)
		if !ok {
			continue
		}
		info.FolderName = entry.Name()
		worlds = append(worlds, info)
	}

	for i := 1; i < len(worlds); i++ {
		for j := i; j > 0 && worlds[j-1].LastPlayed < worlds[j].LastPlayed; j-- {
			worlds[j-1], worlds[j] = worlds[j], worlds[j-1]
		}
	}
	return worlds
}

const (
	nbtEnd       = 0
	nbtByte      = 1
	nbtShort     = 2
	nbtInt       = 3
	nbtLong      = 4
	nbtFloat     = 5
	nbtDouble    = 6
	nbtByteArray = 7
	nbtString    = 8
	nbtList      = 9
	nbtCompound  = 10
	nbtIntArray  = 11
	nbtLongArray = 12
)

func parseLevelDat(path string) (worldInfo, bool) {
	f, err := os.Open(path)
	if err != nil {
		return worldInfo{}, false
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return worldInfo{}, false
	}
	defer gz.Close()

	r := bufio.NewReader(gz)

	rootType, err := r.ReadByte()
	if err != nil || rootType != nbtCompound {
		return worldInfo{}, false
	}
	if _, ok := readNBTString(r); !ok {
		return worldInfo{}, false
	}

	info := worldInfo{}
	walkCompound(r, []string{}, &info)
	if info.LevelName == "" {
		return worldInfo{}, false
	}
	return info, true
}

func walkCompound(r *bufio.Reader, path []string, info *worldInfo) {
	for {
		tagType, err := r.ReadByte()
		if err != nil || tagType == nbtEnd {
			return
		}
		name, ok := readNBTString(r)
		if !ok {
			return
		}

		switch tagType {
		case nbtCompound:
			if len(path) == 0 && name == "Data" {
				walkCompound(r, []string{"Data"}, info)
			} else if isPath(path, "Data") && name == "Version" {
				walkCompound(r, []string{"Data", "Version"}, info)
			} else {
				walkCompound(r, nil, info) // unrelated compound, don't track path
			}

		case nbtString:
			s, ok := readNBTString(r)
			if !ok {
				return
			}
			if isPath(path, "Data") && name == "LevelName" {
				info.LevelName = s
			} else if isPath(path, "Data", "Version") && name == "Name" {
				info.Version = s
			}

		case nbtLong:
			val, ok := readNBTLong(r)
			if !ok {
				return
			}
			if isPath(path, "Data") && name == "LastPlayed" {
				info.LastPlayed = val
			}

		default:
			if !skipPayload(r, tagType) {
				return
			}
		}
	}
}

func isPath(path []string, want ...string) bool {
	if len(path) != len(want) {
		return false
	}
	for i := range path {
		if path[i] != want[i] {
			return false
		}
	}
	return true
}

func readNBTString(r *bufio.Reader) (string, bool) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return "", false
	}
	n := int(lenBuf[0])<<8 | int(lenBuf[1])
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", false
	}
	return string(buf), true
}

func readNBTLong(r *bufio.Reader) (int64, bool) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(buf[:])), true
}

func skipPayload(r *bufio.Reader, tagType byte) bool {
	switch tagType {
	case nbtByte:
		_, err := r.ReadByte()
		return err == nil
	case nbtShort:
		_, err := r.Discard(2)
		return err == nil
	case nbtInt, nbtFloat:
		_, err := r.Discard(4)
		return err == nil
	case nbtLong, nbtDouble:
		_, err := r.Discard(8)
		return err == nil
	case nbtByteArray:
		n, ok := readNBTInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n))
		return err == nil
	case nbtString:
		_, ok := readNBTString(r)
		return ok
	case nbtIntArray:
		n, ok := readNBTInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n) * 4)
		return err == nil
	case nbtLongArray:
		n, ok := readNBTInt(r)
		if !ok {
			return false
		}
		_, err := r.Discard(int(n) * 8)
		return err == nil
	case nbtList:
		elemType, err := r.ReadByte()
		if err != nil {
			return false
		}
		n, ok := readNBTInt(r)
		if !ok {
			return false
		}
		for i := int32(0); i < n; i++ {
			if elemType == nbtCompound {
				for {
					t, err := r.ReadByte()
					if err != nil {
						return false
					}
					if t == nbtEnd {
						break
					}
					if _, ok := readNBTString(r); !ok {
						return false
					}
					if !skipPayload(r, t) {
						return false
					}
				}
			} else if !skipPayload(r, elemType) {
				return false
			}
		}
		return true
	}
	return false
}

func readNBTInt(r *bufio.Reader) (int32, bool) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, false
	}
	return int32(binary.BigEndian.Uint32(buf[:])), true
}

func formatRelativeTime(ms int64) string {
	if ms == 0 {
		return ""
	}
	t := time.Unix(0, ms*int64(time.Millisecond))
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2, 2006")
	}
}
