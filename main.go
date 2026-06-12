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
	colorMagenta = lipgloss.Color("#FF2E93")
	colorCyan    = lipgloss.Color("#00E5FF")
	colorViolet  = lipgloss.Color("#BD00FF")
	colorGreen   = lipgloss.Color("#00FA9A")
	colorWhite   = lipgloss.Color("#ffffff")
	colorDark    = lipgloss.Color("#444444")
	colorGray    = lipgloss.Color("#888888")
	colorRed     = lipgloss.Color("#FF4444")

	panelMenu = lipgloss.NewStyle().
		Width(30).
		Height(11).
		Padding(0, 2)

	panelContent = lipgloss.NewStyle().
		Width(42).
		Height(11).
		Padding(0, 2)

	panelNews = lipgloss.NewStyle().
		Width(30).
		Height(11).
		Padding(0, 2)

	panelFooter = lipgloss.NewStyle().
		Width(102).
		Height(3).
		Padding(0, 2)

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

// --- PERSISTENCE ---
type ConfigData struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
}

func getConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "mctui", "config.json")
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

// --- LÓGICA DE ROADMAP EXTERNO ---
func getRoadmapPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "mctui", "roadmap.json")
}

func loadRoadmap() []string {
	path := getRoadmapPath()
	data, err := os.ReadFile(path)

	// Cambios por defecto si el archivo no existe
	defaultChanges := []string{
		"• Microsoft Auth",
		"• Custom JVM Arguments",
		"• Expanded UI Themes",
	}

	if err != nil {
		// Creamos el archivo automáticamente para facilitarle la edición al usuario
		os.MkdirAll(filepath.Dir(path), 0755)
		wrapped := map[string][]string{"changes": defaultChanges}
		jsonData, _ := json.MarshalIndent(wrapped, "", "  ")
		os.WriteFile(path, jsonData, 0644)
		return defaultChanges
	}

	var result map[string][]string
	if err := json.Unmarshal(data, &result); err != nil {
		return defaultChanges // Fallback si el JSON está mal formateado
	}

	if changes, ok := result["changes"]; ok && len(changes) > 0 {
		return changes
	}
	return defaultChanges
}

// --- VERSION FETCHING & FILE CHECKS ---
func fetchReleases() []string {
	resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
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
	return releases
}

func clientJarExists(version string) bool {
	homeDir, _ := os.UserHomeDir()
	path := filepath.Join(homeDir, ".minecraft", "versions", version, "client.jar")
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
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
	roadmap       []string // NUEVO: Lista dinámica cargada desde el JSON

	input textinput.Model
	play  bool
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
		roadmap:       roadmap, // NUEVO: Asignación
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
			if m.state == menuScreen && m.cursorMenu < 4 {
				m.cursorMenu++
			} else if m.state == versionsScreen && m.cursorVersions < len(m.versions)-1 {
				m.cursorVersions++
			}

		case "enter":
			if m.state == menuScreen {
				if m.cursorMenu == 0 {
					if clientJarExists(m.versionSelect) {
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
	menuStr.WriteString(lipgloss.NewStyle().Foreground(colorViolet).Bold(true).Render("─ Options ────────────────────") + "\n\n")

	menuOptions := []string{
		fmt.Sprintf("Play (%s)", m.versionSelect),
		"Change Name",
		"Change Version",
		fmt.Sprintf("Toggle Modloader : [ %s ]", lipgloss.NewStyle().Foreground(colorCyan).Render(m.modloader)),
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
	contentStr.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("─ mcTUI Launcher ─────────────────────") + "\n\n")

	if m.state == menuScreen {
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("─ Active Session ─") + "\n\n")
		contentStr.WriteString(fmt.Sprintf("User     : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(m.username)))
		
		verString := m.versionSelect
		if m.modloader != "Vanilla" {
			verString += fmt.Sprintf(" (%s)", m.modloader)
		}
		contentStr.WriteString(fmt.Sprintf("Version  : %s\n", lipgloss.NewStyle().Foreground(colorWhite).Render(verString)))
		contentStr.WriteString("Auth     : Offline (LAN Mode)\n\n")

	} else if m.state == nameScreen {
		contentStr.WriteString("New LAN username:\n\n")
		contentStr.WriteString(m.input.View())

	} else if m.state == versionsScreen {
		contentStr.WriteString("Select a stable version:\n\n")

		start := m.cursorVersions - 3
		if start < 0 { start = 0 }
		end := start + 6
		if end > len(m.versions) {
			end = len(m.versions)
			start = end - 6
			if start < 0 { start = 0 }
		}

		for i := start; i < end; i++ {
			if i == m.cursorVersions {
				contentStr.WriteString(cursorStyle.Render(fmt.Sprintf("  ▶ %s", m.versions[i])) + "\n")
			} else {
				contentStr.WriteString(fmt.Sprintf("    %s", m.versions[i]) + "\n")
			}
		}
	} else if m.state == confirmScreen {
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorRed).Bold(true).Render("⚠ ARCHIVO FALTANTE") + "\n\n")
		contentStr.WriteString(fmt.Sprintf("El archivo client.jar (%s) no se\nencuentra en tu sistema.\n\n", m.versionSelect))
		contentStr.WriteString("¿Deseas descargarlo desde los\nservidores de Mojang?\n\n")
		contentStr.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Render("[y/Enter] Sí") + "   " + lipgloss.NewStyle().Foreground(colorGray).Render("[n/Esc] Cancelar"))
	}

	// --- PANEL DERECHO: NOTICIAS DINÁMICAS ---
	newsStr := strings.Builder{}
	newsStr.WriteString(lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render("─ Future Changes ─────────") + "\n\n")
	
	// CORRECCIÓN: Leemos directamente del slice dinámico cargado por el modelo
	for _, item := range m.roadmap {
		// Si el usuario no le puso viñeta en el JSON, se la agregamos dinámicamente si queremos,
		// o imprimimos el texto directo. Vamos con texto directo para máxima flexibilidad:
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

	separator := lipgloss.NewStyle().Foreground(colorDark).Render(strings.Repeat("─", 94))
	statusPart := lipgloss.NewStyle().Foreground(colorGreen).Render("● Ready")
	userPart := lipgloss.NewStyle().Foreground(colorGray).Render(fmt.Sprintf("[%s - %s]", m.username, m.versionSelect))
	controlsPart := lipgloss.NewStyle().Foreground(colorDark).Render(controls)

	footerContent := fmt.Sprintf("%s\n%s   %s   %s", separator, statusPart, userPart, controlsPart)

	topPanels := lipgloss.JoinHorizontal(lipgloss.Top,
		panelMenu.Render(menuStr.String()),
		panelContent.Render(contentStr.String()),
		panelNews.Render(newsStr.String()),
	)
	
	fullInterface := lipgloss.JoinVertical(lipgloss.Left, asciiHeader, topPanels, panelFooter.Render(footerContent))

	return "\n" + fullInterface
}

func main() {
	validReleases := fetchReleases()
	cfg := loadConfig()
	roadmap := loadRoadmap() // NUEVO: Carga del archivo dinámico

	// NUEVO: Pasamos el roadmap como tercer argumento
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
func launchGame(username string, targetVersion string, modloader string) {
	fmt.Print("\033[H\033[2J") // Limpiar consola
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println(" MODO: Offline / LAN")
	fmt.Println(" (El servidor debe tener 'online-mode=false' en server.properties)")
	fmt.Println(strings.Repeat("=", 75))
	fmt.Println()
	
	fmt.Printf("Iniciando motor para el jugador: %s (Versión: %s - Modloader: %s)\n", username, targetVersion, modloader)
	fmt.Println("1. Verificando Motor y Librerías Vanilla...")

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
		if url == "" { return }
		os.MkdirAll(filepath.Dir(destination), 0755)
		if info, err := os.Stat(destination); err == nil && info.Size() > 0 { return }
		r, err := http.Get(url)
		if err != nil { return }
		defer r.Body.Close()
		f, err := os.Create(destination)
		if err != nil { return }
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

	// --- INYECCIÓN DE FABRIC ---
	mainClass := "net.minecraft.client.main.Main"

	if modloader == "Fabric" {
		fmt.Println("-> Detectada inyección Fabric. Interceptando manifiesto...")
		
		loaderURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", targetVersion)
		loaderResp, err := http.Get(loaderURL)
		if err == nil {
			defer loaderResp.Body.Close()
			loaderBody, _ := io.ReadAll(loaderResp.Body)
			var loaderData []struct {
				Loader struct {
					Version string `json:"version"`
				} `json:"loader"`
			}
			json.Unmarshal(loaderBody, &loaderData)
			
			if len(loaderData) > 0 {
				latestLoader := loaderData[0].Loader.Version
				fmt.Printf("-> Descargando librerías puente de Fabric v%s...\n", latestLoader)
				
				profileURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/profile/json", targetVersion, latestLoader)
				profileResp, _ := http.Get(profileURL)
				defer profileResp.Body.Close()
				profileBody, _ := io.ReadAll(profileResp.Body)

				var profile struct {
					MainClass string `json:"mainClass"`
					Libraries []struct {
						Name string `json:"name"`
						URL  string `json:"url"`
					} `json:"libraries"`
				}
				json.Unmarshal(profileBody, &profile)
				
				mainClass = profile.MainClass

				for _, lib := range profile.Libraries {
					parts := strings.Split(lib.Name, ":")
					if len(parts) >= 3 {
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
						
						classpathEntries = append(classpathEntries, fullDest)
						wg.Add(1)
						go download(fullURL, fullDest)
					}
				}
			} else {
				fmt.Println("[!] Fabric no tiene un loader compatible para esta versión. Cayendo a Vanilla.")
			}
		}
	}

	fmt.Println("2. Validando Assets...")
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
		mainClass,
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
	}

	err = cmd.Start()
	if err != nil {
		fmt.Println("\n[!] Error starting process:", err)
		return
	}
}
