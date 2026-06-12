package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// --- VISUAL STYLES ---
var (
	colorMagenta = lipgloss.Color("#ff00ff")
	colorCyan    = lipgloss.Color("#00ffff")
	colorYellow  = lipgloss.Color("#ffff00")
	colorWhite   = lipgloss.Color("#ffffff")

	panelMenu = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorMagenta).
		Width(26).
		Height(12).
		Padding(0, 1)

	panelContent = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorCyan).
		Width(54).
		Height(12).
		Padding(0, 1)

	panelFooter = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorWhite).
		Width(84).
		Height(3).
		Padding(0, 1)

	titleStyle  = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	itemStyle   = lipgloss.NewStyle().Foreground(colorWhite).PaddingLeft(1)
	cursorStyle = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
)

// --- PERSISTENCE ---
type ConfigData struct {
	Username string `json:"username"`
	Version  string `json:"version"`
}

func getConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "mctui", "config.json")
}

func loadConfig() ConfigData {
	path := getConfigPath()
	data, err := os.ReadFile(path)

	defaultConfig := ConfigData{Username: "OfflinePlayer", Version: "1.20.4"}
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
	return config
}

func saveConfig(c ConfigData) {
	path := getConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0644)
}

// --- VERSION FETCHING ---
func fetchReleases() []string {
	resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
		return []string{"1.20.4"} // Fallback without internet initially
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
	return releases
}

// --- STATES AND MODEL ---
type screenState int

const (
	menuScreen    screenState = iota
	nameScreen
	versionsScreen
)

type model struct {
	state          screenState
	cursorMenu     int
	cursorVersions int

	username      string
	versionSelect string
	versions      []string

	input textinput.Model
	play  bool
}

func initialModel(versions []string, cfg ConfigData) model {
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
		versions:      versions,
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
			if m.state == menuScreen && m.cursorMenu < 3 {
				m.cursorMenu++
			} else if m.state == versionsScreen && m.cursorVersions < len(m.versions)-1 {
				m.cursorVersions++
			}

		case "enter":
			if m.state == menuScreen {
				if m.cursorMenu == 0 {
					m.play = true
					return m, tea.Quit
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
					return m, tea.Quit
				}
			} else if m.state == nameScreen {
				if m.input.Value() != "" {
					m.username = m.input.Value()
					saveConfig(ConfigData{Username: m.username, Version: m.versionSelect})
				}
				m.state = menuScreen
			} else if m.state == versionsScreen {
				m.versionSelect = m.versions[m.cursorVersions]
				saveConfig(ConfigData{Username: m.username, Version: m.versionSelect})
				m.state = menuScreen
			}

		case "esc":
			if m.state == nameScreen || m.state == versionsScreen {
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
	menuStr := strings.Builder{}
	menuStr.WriteString(titleStyle.Render("╭ Options ╮") + "\n\n")

	menuOptions := []string{
		fmt.Sprintf("Play (%s)", m.versionSelect),
		"Change Name",
		"Change Version",
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
	contentStr.WriteString(titleStyle.Render("╭ mcTUI Launcher ╮") + "\n\n")

	if m.state == menuScreen {
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Render("=== ACTIVE SESSION ===") + "\n\n")
		contentStr.WriteString(fmt.Sprintf("User     : %s\n", m.username))
		contentStr.WriteString(fmt.Sprintf("Version  : %s\n", m.versionSelect))
		contentStr.WriteString("Auth     : Offline (Bypass)\n\n")
		contentStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render("● System ready."))

	} else if m.state == nameScreen {
		contentStr.WriteString("New LAN username:\n\n")
		contentStr.WriteString(m.input.View())

	} else if m.state == versionsScreen {
		contentStr.WriteString("Select a stable version:\n\n")

		start := m.cursorVersions - 3
		if start < 0 {
			start = 0
		}
		end := start + 7
		if end > len(m.versions) {
			end = len(m.versions)
			start = end - 7
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
	}

	footerStr := " [↑/↓] Navigate  [Enter] Select  [q] Quit"
	if m.state == nameScreen {
		footerStr = " [Enter] Save  [Esc] Cancel"
	} else if m.state == versionsScreen {
		footerStr = " [↑/↓] Move list  [Enter] Choose  [Esc] Back"
	}

	topPanels := lipgloss.JoinHorizontal(lipgloss.Top,
		panelMenu.Render(menuStr.String()),
		panelContent.Render(contentStr.String()),
	)
	fullInterface := lipgloss.JoinVertical(lipgloss.Left, topPanels, panelFooter.Render(footerStr))

	return "\n" + fullInterface
}

func main() {
	fmt.Println("Connecting to Mojang to fetch versions...")
	validReleases := fetchReleases()
	cfg := loadConfig()

	fmt.Print("\033[H\033[2J")

	p := tea.NewProgram(initialModel(validReleases, cfg), tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("TUI Error: %v", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(model); ok && m.play {
		fmt.Print("\033[H\033[2J")
		launchGame(m.username, m.versionSelect)
	}
}

// --- THE GAME LAUNCHING ENGINE ---
func launchGame(username string, targetVersion string) {
	fmt.Printf("Starting engine for player: %s (Version: %s)\n", username, targetVersion)
	fmt.Println("1. Verifying Engine and Libraries...")

	// CORRECTLY EXPANDED STRUCTURES
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

	resp, _ := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var manifest Manifest
	json.Unmarshal(body, &manifest)

	var specificURL string
	for _, v := range manifest.Versions {
		if v.ID == targetVersion {
			specificURL = v.URL
			break
		}
	}

	respVersion, _ := http.Get(specificURL)
	bodyVersion, _ := io.ReadAll(respVersion.Body)
	respVersion.Body.Close()

	var data VersionData
	json.Unmarshal(bodyVersion, &data)

	homeDir, _ := os.UserHomeDir()

	clientPath := filepath.Join(homeDir, ".minecraft", "versions", targetVersion, "client.jar")
	if info, err := os.Stat(clientPath); err != nil || info.Size() == 0 {
		os.MkdirAll(filepath.Dir(clientPath), 0755)
		respClient, _ := http.Get(data.Downloads.Client.URL)
		clientFile, _ := os.Create(clientPath)
		io.Copy(clientFile, respClient.Body)
		clientFile.Close()
		respClient.Body.Close()
	}

	var classpathEntries []string
	librariesPath := filepath.Join(homeDir, ".minecraft", "libraries")
	var wg sync.WaitGroup

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
		if err != nil {
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

	for _, lib := range data.Libraries {
		url := lib.Downloads.Artifact.URL
		path := lib.Downloads.Artifact.Path
		if url != "" && path != "" {
			fullPath := filepath.Join(librariesPath, path)
			classpathEntries = append(classpathEntries, fullPath)
			wg.Add(1)
			go download(url, fullPath)
		}
	}

	fmt.Println("2. Validating Assets...")
	indexPath := filepath.Join(homeDir, ".minecraft", "assets", "indexes", data.AssetIndex.ID+".json")
	if info, err := os.Stat(indexPath); err != nil || info.Size() == 0 {
		os.MkdirAll(filepath.Dir(indexPath), 0755)
		respIndex, _ := http.Get(data.AssetIndex.URL)
		indexFile, _ := os.Create(indexPath)
		io.Copy(indexFile, respIndex.Body)
		indexFile.Close()
		respIndex.Body.Close()
	}

	indexBytes, _ := os.ReadFile(indexPath)
	var assetIndexData struct {
		Objects map[string]struct {
			Hash string `json:"hash"`
		} `json:"objects"`
	}
	json.Unmarshal(indexBytes, &assetIndexData)

	sem := make(chan struct{}, 50)
	for _, obj := range assetIndexData.Objects {
		hash := obj.Hash
		subDir := hash[:2]
		url := "https://resources.download.minecraft.net/" + subDir + "/" + hash
		dest := filepath.Join(homeDir, ".minecraft", "assets", "objects", subDir, hash)

		wg.Add(1)
		sem <- struct{}{}
		go func(u, d string) {
			defer func() { <-sem }()
			download(u, d)
		}(url, dest)
	}

	wg.Wait()
	classpathEntries = append(classpathEntries, clientPath)
	finalClasspath := strings.Join(classpathEntries, ":")

	sessionUUID := uuid.New().String()

	fmt.Println("3. All set! Launching Minecraft...")

	cmd := exec.Command("java",
		"-Xmx2G",
		"-cp", finalClasspath,
		"net.minecraft.client.main.Main",
		"--username", username,
		"--version", targetVersion,
		"--gameDir", filepath.Join(homeDir, ".minecraft"),
		"--assetsDir", filepath.Join(homeDir, ".minecraft", "assets"),
		"--assetIndex", data.AssetIndex.ID,
		"--uuid", sessionUUID,
		"--accessToken", "0",
		"--userType", "legacy",
		"--versionType", "release",
	)

	logFile, err := os.Create(filepath.Join(homeDir, ".minecraft", "mctui_latest.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		// We don't use defer logFile.Close() here because the child process will keep it open
	}

	// NEW: We use Start() instead of Run()
	// Start launches the process in the Linux kernel and returns control to Go instantly.
	err = cmd.Start()
	if err != nil {
		fmt.Println("\n[!] Error starting process:", err)
		return
	}

}
