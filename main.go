package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"strconv"

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

func getRoadmapPath() string {
	if runtime.GOOS == "windows" {
		return getAppDataDir("mctui", "roadmap.json")
	}
	return getAppDataDir("mctui", "roadmap.json")
}

// --- PERSISTENCE ---
type ConfigData struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
}

func loadConfig() ConfigData {
	path := getConfigPath()
	data, err := os.ReadFile(path)

	defaultConfig := ConfigData{Username: "OfflinePlayer", Version: "1.20.4", Modloader: "Vanilla"}
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
	return config
}

func saveConfig(c ConfigData) {
	path := getConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0644)
}

// --- EXTERNAL ROADMAP LOGIC ---
func loadRoadmap() []string {
	path := getRoadmapPath()
	data, err := os.ReadFile(path)

	defaultChanges := []string{
		"• Microsoft Auth",
		"• Custom JVM Arguments",
		"• Expanded UI Themes",
	}

	if err != nil {
		os.MkdirAll(filepath.Dir(path), 0755)
		wrapped := map[string][]string{"changes": defaultChanges}
		jsonData, _ := json.MarshalIndent(wrapped, "", "  ")
		os.WriteFile(path, jsonData, 0644)
		return defaultChanges
	}

	var result map[string][]string
	if err := json.Unmarshal(data, &result); err != nil {
		return defaultChanges // Fallback if JSON is malformed
	}

	if changes, ok := result["changes"]; ok && len(changes) > 0 {
		return changes
	}
	return defaultChanges
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
	confirmScreen
)

type model struct {
	state          screenState
	cursorMenu     int
	cursorVersions int

	username      string
	versionSelect string
	modloader     string
	versions      []string
	roadmap       []string

	input textinput.Model
	play  bool

	width  int
	height int
}

func initialModel(versions []string, cfg ConfigData, roadmap []string) model {
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
		versions:      versions,
		roadmap:       roadmap,
		input:         ti,
		play:          false,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

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
			}

		case "down", "j":
			if m.state == menuScreen && m.cursorMenu < 4 {
				m.cursorMenu++
			} else if m.state == versionsScreen && m.cursorVersions < len(m.versions)-1 {
				m.cursorVersions++
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
					// Toggle Modloader (Vanilla <-> Fabric)
					if m.modloader == "Vanilla" {
						m.modloader = "Fabric"
					} else {
						m.modloader = "Vanilla"
					}
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader})
				} else if m.cursorMenu == 4 {
					return m, tea.Quit
				}
			} else if m.state == nameScreen {
				if m.input.Value() != "" {
					m.username = m.input.Value()
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader})
				}
				m.state = menuScreen
			} else if m.state == versionsScreen {
				m.versionSelect = m.versions[m.cursorVersions]
				saveConfig(ConfigData{Username: m.username, Version: m.versionSelect, Modloader: m.modloader})
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
			if m.state == nameScreen || m.state == versionsScreen || m.state == confirmScreen {
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
		fmt.Sprintf("Modloader: %s", lipgloss.NewStyle().Foreground(colorCyan).Render(m.modloader)),
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
		contentStr.WriteString(fmt.Sprintf("OS       : %s\n\n", lipgloss.NewStyle().Foreground(colorWhite).Render(osName)))

		if installationReady(m.versionSelect, m.modloader) {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready (offline-capable)"))
		} else {
			contentStr.WriteString(lipgloss.NewStyle().Foreground(colorRed).Render("● Not installed — will need network"))
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
	} else if m.state == confirmScreen {
		controls = " [y] Accept  [n] Cancel"
	}

	separator := lipgloss.NewStyle().Foreground(colorDark).Render(strings.Repeat("─", 120))
	statusPart := lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready")
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
	roadmap := loadRoadmap()

	p := tea.NewProgram(initialModel(validReleases, cfg, roadmap), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(model); ok && m.play {
		launchGame(m.username, m.versionSelect, m.modloader)
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
func launchGame(username string, targetVersion string, modloader string) {
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
			} `json:"client"`
		} `json:"downloads"`
		Libraries []struct {
			Downloads struct {
				Artifact struct {
					Path string `json:"path"`
					URL  string `json:"url"`
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

	// Download client.jar if missing and we have a URL for it.
	if !haveClient {
		if !haveVersionData || data.Downloads.Client.URL == "" {
			fmt.Println("\n[!] Cannot download client.jar: no version data available.")
			return
		}
		os.MkdirAll(filepath.Dir(clientPath), 0755)
		respClient, cErr := http.Get(data.Downloads.Client.URL)
		if cErr != nil || respClient == nil {
			fmt.Println("\n[!] Error downloading client.jar:", cErr)
			return
		}
		clientFile, fErr := os.Create(clientPath)
		if fErr != nil {
			fmt.Println("\n[!] Error creating client.jar file:", fErr)
			respClient.Body.Close()
			return
		}
		io.Copy(clientFile, respClient.Body)
		clientFile.Close()
		respClient.Body.Close()
	}

	var classpathEntries []string
	librariesPath := filepath.Join(mcDir, "libraries")
	var wg sync.WaitGroup

	// FIX (#5): cap concurrent downloads so we don't open hundreds of
	// simultaneous connections. Shared across vanilla libs, Fabric libs,
	// and assets (each phase uses its own buffered semaphore below).
	const maxConcurrentDownloads = 20

	download := func(url, destination string) {
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
		defer f.Close()
		io.Copy(f, r.Body)
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
				go func(u, d string) {
					defer func() { <-libSem }()
					download(u, d)
				}(url, fullPath)
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
			go func(u, d string) {
				defer func() { <-assetSem }()
				download(u, d)
			}(url, dest)
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
	if haveVersionData && data.JavaVersion.MajorVersion > 0 {
		installedJava := checkJavaVersion()
		if installedJava > 0 && installedJava < data.JavaVersion.MajorVersion {
			fmt.Println()
			fmt.Printf("[!] This version requires Java %d or newer, but found Java %d.\n", data.JavaVersion.MajorVersion, installedJava)
			fmt.Println("[!] Install a newer JRE and make sure it's first in your PATH.")
			fmt.Println("[!] On Arch: pacman -S jdk-openjdk (or jreXX-openjdk for the specific version)")
			return
		}
		if installedJava == 0 {
			fmt.Println("[!] Warning: could not determine installed Java version. Proceeding anyway...")
		}
	}

	cmd := exec.Command("java",
		"-Xmx2G",
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
}

// --- FABRIC SUPPORT ---

// fabricProfile holds the resolved data needed to launch Fabric: the main
// class to invoke instead of the vanilla one, and the extra classpath
// entries (Fabric loader + dependencies).
type fabricProfile struct {
	MainClass        string
	ClasspathEntries []string
}


// checkJavaVersion runs "java -version" and parses the major version number
// (handling both old "1.8.0_xxx" and new "21.0.x" / "25" formats). It
// returns the detected major version, or 0 if it couldn't be determined
// (e.g. java is not installed) — callers should treat 0 as "unknown,
// proceed anyway" rather than blocking the launch.
func checkJavaVersion() int {
	cmd := exec.Command("java", "-version")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out // java -version prints to stderr
	if err := cmd.Run(); err != nil {
		return 0
	}

	output := out.String()
	// Look for a quoted version string, e.g. "21.0.3" or "1.8.0_392"
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

	// Old versioning scheme: "1.8" means Java 8, "1.7" means Java 7, etc.
	if major == 1 && len(parts) > 1 {
		minor, err := strconv.Atoi(parts[1])
		if err == nil {
			return minor
		}
	}

	return major
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
func resolveFabricProfile(targetVersion, mcDir, librariesPath string, wg *sync.WaitGroup, download func(url, dest string), maxConcurrent int) (fabricProfile, bool) {
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
			download(u, d)
		}(fullURL, fullDest)
	}

	// Cache the resolved profile so future launches can work offline.
	if data, mErr := json.MarshalIndent(result, "", "  "); mErr == nil {
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, data, 0644)
	}

	return result, true
}
