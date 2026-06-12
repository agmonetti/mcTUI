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

// --- ESTILOS VISUALES ---
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

	panelContenido = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorCyan).
		Width(54).
		Height(12).
		Padding(0, 1)

	panelInferior = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorWhite).
		Width(84).
		Height(3).
		Padding(0, 1)

	tituloStyle = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	itemStyle   = lipgloss.NewStyle().Foreground(colorWhite).PaddingLeft(1)
	cursorStyle = lipgloss.NewStyle().Foreground(colorMagenta).Bold(true)
)

// --- PERSISTENCIA ---
type ConfigData struct {
	Username string `json:"username"`
	Version  string `json:"version"`
}

func obtenerRutaConfig() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "mctui", "config.json")
}

func cargarConfig() ConfigData {
	ruta := obtenerRutaConfig()
	datos, err := os.ReadFile(ruta)
	
	defaultConfig := ConfigData{Username: "JugadorOffline", Version: "1.20.4"}
	if err != nil {
		return defaultConfig
	}

	var config ConfigData
	json.Unmarshal(datos, &config)

	if config.Username == "" { config.Username = defaultConfig.Username }
	if config.Version == "" { config.Version = defaultConfig.Version }
	return config
}

func guardarConfig(c ConfigData) {
	ruta := obtenerRutaConfig()
	os.MkdirAll(filepath.Dir(ruta), 0755)
	datos, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(ruta, datos, 0644)
}

// --- OBTENCIÓN DE VERSIONES ---
func obtenerReleases() []string {
	resp, err := http.Get("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
	if err != nil {
		return []string{"1.20.4"} // Fallback sin internet inicial
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

// --- ESTADOS Y MODELO ---
type estado int

const (
	pantallaMenu estado = iota
	pantallaNombre
	pantallaVersiones 
)

type model struct {
	estado          estado
	cursorMenu      int
	cursorVersiones int
	
	username        string
	versionSelect   string
	versiones       []string
	
	input           textinput.Model
	jugar           bool
}

func modeloInicial(versiones []string, cfg ConfigData) model {
	ti := textinput.New()
	ti.Placeholder = "Escribe tu nombre..."
	ti.Focus()
	ti.CharLimit = 16
	ti.Width = 30
	ti.PromptStyle = lipgloss.NewStyle().Foreground(colorCyan)
	ti.TextStyle = lipgloss.NewStyle().Foreground(colorWhite)

	return model{
		estado:        pantallaMenu,
		cursorMenu:    0,
		username:      cfg.Username,
		versionSelect: cfg.Version,
		versiones:     versiones,
		input:         ti,
		jugar:         false,
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
			if m.estado == pantallaMenu {
				return m, tea.Quit
			}
		
		case "up", "k":
			if m.estado == pantallaMenu && m.cursorMenu > 0 {
				m.cursorMenu--
			} else if m.estado == pantallaVersiones && m.cursorVersiones > 0 {
				m.cursorVersiones--
			}

		case "down", "j":
			if m.estado == pantallaMenu && m.cursorMenu < 3 { 
				m.cursorMenu++
			} else if m.estado == pantallaVersiones && m.cursorVersiones < len(m.versiones)-1 {
				m.cursorVersiones++
			}

		case "enter":
			if m.estado == pantallaMenu {
				if m.cursorMenu == 0 { 
					m.jugar = true
					return m, tea.Quit
				} else if m.cursorMenu == 1 { 
					m.estado = pantallaNombre
					m.input.SetValue(m.username)
				} else if m.cursorMenu == 2 { 
					m.estado = pantallaVersiones
					for i, v := range m.versiones {
						if v == m.versionSelect {
							m.cursorVersiones = i
							break
						}
					}
				} else if m.cursorMenu == 3 { 
					return m, tea.Quit
				}
			} else if m.estado == pantallaNombre {
				if m.input.Value() != "" {
					m.username = m.input.Value()
					guardarConfig(ConfigData{Username: m.username, Version: m.versionSelect})
				}
				m.estado = pantallaMenu
			} else if m.estado == pantallaVersiones {
				m.versionSelect = m.versiones[m.cursorVersiones]
				guardarConfig(ConfigData{Username: m.username, Version: m.versionSelect})
				m.estado = pantallaMenu
			}

		case "esc":
			if m.estado == pantallaNombre || m.estado == pantallaVersiones {
				m.estado = pantallaMenu
			}
		}
	}

	if m.estado == pantallaNombre {
		m.input, cmd = m.input.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	menuStr := strings.Builder{}
	menuStr.WriteString(tituloStyle.Render("╭ Opciones ╮") + "\n\n")
	
	opcionesMenu := []string{
		fmt.Sprintf("Jugar (%s)", m.versionSelect),
		"Cambiar Nombre",
		"Cambiar Versión",
		"Salir",
	}

	for i, opcion := range opcionesMenu {
		if m.cursorMenu == i {
			menuStr.WriteString(cursorStyle.Render(fmt.Sprintf("▶ %s", opcion)) + "\n")
		} else {
			menuStr.WriteString(itemStyle.Render(fmt.Sprintf("  %s", opcion)) + "\n")
		}
	}

	contenidoStr := strings.Builder{}
	contenidoStr.WriteString(tituloStyle.Render("╭ Launcher TUI ╮") + "\n\n")

	if m.estado == pantallaMenu {
		contenidoStr.WriteString(lipgloss.NewStyle().Foreground(colorCyan).Render("=== INFORMACIÓN DE SESIÓN ===") + "\n\n")
		contenidoStr.WriteString(fmt.Sprintf("Usuario Actual  : %s\n", m.username))
		contenidoStr.WriteString(fmt.Sprintf("Versión Activa  : %s\n", m.versionSelect))
		contenidoStr.WriteString("Autenticación   : Offline (Bypass)\n\n")
		contenidoStr.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Render("El motor de descarga inteligente está\nlisto. Presiona Jugar para iniciar."))
	
	} else if m.estado == pantallaNombre {
		contenidoStr.WriteString("Escribe tu nuevo nombre de usuario\npara el modo multijugador LAN:\n\n")
		contenidoStr.WriteString(m.input.View())
	
	} else if m.estado == pantallaVersiones {
		contenidoStr.WriteString("Selecciona una versión (Estables):\n\n")
		
		inicio := m.cursorVersiones - 3
		if inicio < 0 { inicio = 0 }
		fin := inicio + 7
		if fin > len(m.versiones) { 
			fin = len(m.versiones)
			inicio = fin - 7
			if inicio < 0 { inicio = 0 }
		}

		for i := inicio; i < fin; i++ {
			if i == m.cursorVersiones {
				contenidoStr.WriteString(cursorStyle.Render(fmt.Sprintf("  ▶ %s", m.versiones[i])) + "\n")
			} else {
				contenidoStr.WriteString(fmt.Sprintf("    %s", m.versiones[i]) + "\n")
			}
		}
	}

	footerStr := " [↑/↓] Navegar  [Enter] Seleccionar  [q] Salir  |  mcTUI v1.1"
	if m.estado == pantallaNombre {
		footerStr = " [Enter] Guardar  [Esc] Cancelar  |  Ingreso de texto..."
	} else if m.estado == pantallaVersiones {
		footerStr = " [↑/↓] Mover lista  [Enter] Elegir versión  [Esc] Volver"
	}

	panelSuperior := lipgloss.JoinHorizontal(lipgloss.Top,
		panelMenu.Render(menuStr.String()),
		panelContenido.Render(contenidoStr.String()),
	)
	interfazCompleta := lipgloss.JoinVertical(lipgloss.Left, panelSuperior, panelInferior.Render(footerStr))

	return "\n" + interfazCompleta
}

func main() {
	fmt.Println("Conectando con Mojang para obtener versiones...")
	versionesValidas := obtenerReleases()
	cfg := cargarConfig()

	fmt.Print("\033[H\033[2J")

	p := tea.NewProgram(modeloInicial(versionesValidas, cfg), tea.WithAltScreen())
	
	modeloFinal, err := p.Run()
	if err != nil {
		fmt.Printf("Error en la TUI: %v", err)
		os.Exit(1)
	}

	if m, ok := modeloFinal.(model); ok && m.jugar {
		fmt.Print("\033[H\033[2J") 
		lanzarJuego(m.username, m.versionSelect)
	}
}

// --- EL MOTOR ACTUALIZADO ---
func lanzarJuego(username string, versionBuscada string) {
	fmt.Printf("Iniciando motor para el jugador: %s (Versión: %s)\n", username, versionBuscada)
	fmt.Println("1. Verificando Motor y Librerías...")

	// ESTRUCTURAS CORRECTAMENTE EXPANDIDAS
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

	var urlEspecifica string
	for _, v := range manifest.Versions {
		if v.ID == versionBuscada {
			urlEspecifica = v.URL
			break
		}
	}

	respVersion, _ := http.Get(urlEspecifica)
	bodyVersion, _ := io.ReadAll(respVersion.Body)
	respVersion.Body.Close()

	var data VersionData
	json.Unmarshal(bodyVersion, &data)

	homeDir, _ := os.UserHomeDir()
	
	clientPath := filepath.Join(homeDir, ".minecraft", "versions", versionBuscada, "client.jar")
	if info, err := os.Stat(clientPath); err != nil || info.Size() == 0 {
		os.MkdirAll(filepath.Dir(clientPath), 0755)
		respClient, _ := http.Get(data.Downloads.Client.URL)
		archivoClient, _ := os.Create(clientPath)
		io.Copy(archivoClient, respClient.Body)
		archivoClient.Close()
		respClient.Body.Close()
	}

	var rutasParaClasspath []string
	rutaLibrerias := filepath.Join(homeDir, ".minecraft", "libraries")
	var wg sync.WaitGroup

	descargar := func(url, destino string) {
		defer wg.Done()
		if url == "" { return }
		os.MkdirAll(filepath.Dir(destino), 0755)
		if info, err := os.Stat(destino); err == nil && info.Size() > 0 { return }
		r, err := http.Get(url)
		if err != nil { return }
		defer r.Body.Close()
		a, err := os.Create(destino)
		if err != nil { return }
		defer a.Close()
		io.Copy(a, r.Body)
	}

	for _, lib := range data.Libraries {
		url := lib.Downloads.Artifact.URL
		path := lib.Downloads.Artifact.Path
		if url != "" && path != "" {
			rutaCompleta := filepath.Join(rutaLibrerias, path)
			rutasParaClasspath = append(rutasParaClasspath, rutaCompleta)
			wg.Add(1)
			go descargar(url, rutaCompleta)
		}
	}

	fmt.Println("2. Validando Assets...")
	indexPath := filepath.Join(homeDir, ".minecraft", "assets", "indexes", data.AssetIndex.ID+".json")
	if info, err := os.Stat(indexPath); err != nil || info.Size() == 0 {
		os.MkdirAll(filepath.Dir(indexPath), 0755)
		respIndex, _ := http.Get(data.AssetIndex.URL)
		archivoIndex, _ := os.Create(indexPath)
		io.Copy(archivoIndex, respIndex.Body)
		archivoIndex.Close()
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
			descargar(u, d)
		}(url, dest)
	}

	wg.Wait()
	rutasParaClasspath = append(rutasParaClasspath, clientPath)
	classpathFinal := strings.Join(rutasParaClasspath, ":")

	sessionUUID := uuid.New().String()

	fmt.Println("3. ¡Todo listo! Lanzando Minecraft...")

	cmd := exec.Command("java",
		"-Xmx2G", 
		"-cp", classpathFinal, 
		"net.minecraft.client.main.Main", 
		"--username", username,
		"--version", versionBuscada,
		"--gameDir", filepath.Join(homeDir, ".minecraft"),
		"--assetsDir", filepath.Join(homeDir, ".minecraft", "assets"),
		"--assetIndex", data.AssetIndex.ID,
		"--uuid", sessionUUID, 
		"--accessToken", "0", 
		"--userType", "legacy",
		"--versionType", "release",
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		fmt.Println("\n[!] Error:", err)
	}
}
