// Package roadmap fetches and manages the dynamic roadmap data
// for mcTUI, with offline fallback and internationalization.
package roadmap

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/i18n"
)

// RemoteURL is the endpoint hosting the official roadmap.json file.
const RemoteURL = "https://raw.githubusercontent.com/agmonetti/mcTUI/main/roadmap.json"

// Item represents a single feature or planned milestone in the roadmap.
type Item struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // "planned", "in_progress", "done"
	TitleEN string `json:"title_en"`
	TitleES string `json:"title_es"`
	DescEN  string `json:"desc_en"`
	DescES  string `json:"desc_es"`
}

// Title returns the item's title in the currently active UI language.
func (i Item) Title() string {
	if strings.HasPrefix(i18n.CurrentLanguage(), "es") && i.TitleES != "" {
		return i.TitleES
	}
	if i.TitleEN != "" {
		return i.TitleEN
	}
	return i.ID
}

// Description returns the item's description in the currently active UI language.
func (i Item) Description() string {
	if strings.HasPrefix(i18n.CurrentLanguage(), "es") && i.DescES != "" {
		return i.DescES
	}
	return i.DescEN
}

// Roadmap contains the collection of roadmap items and metadata.
type Roadmap struct {
	Version   string `json:"version"`
	UpdatedAt string `json:"updated_at"`
	Items     []Item `json:"items"`
}

// LoadedMsg is delivered to Bubble Tea when the roadmap finishes loading.
type LoadedMsg struct {
	Data   Roadmap
	Online bool
	Err    error
}

// Embedded returns the fallback roadmap bundled into the binary.
func Embedded() Roadmap {
	return Roadmap{
		Version:   "2.0",
		UpdatedAt: "2026-11-08",
		Items: []Item{
			{
				ID:      "neoforge",
				Status:  "planned",
				TitleEN: "NeoForge & Quilt Support",
				TitleES: "Soporte NeoForge y Quilt",
			},
			{
				ID:      "mod_toggle",
				Status:  "in_progress",
				TitleEN: "Enable / Disable Mods (.disabled)",
				TitleES: "Activar / Desactivar Mods (.disabled)",
			},
			{
				ID:      "java_downloader",
				Status:  "planned",
				TitleEN: "Auto OpenJDK JRE Downloader",
				TitleES: "Descargador automático de OpenJDK",
			},
			{
				ID:      "crash_analyzer",
				Status:  "planned",
				TitleEN: "Crash Log Analyzer & Diagnostic",
				TitleES: "Analizador y diagnóstico de crashes",
			},
			{
				ID:      "server_favorites",
				Status:  "planned",
				TitleEN: "Server Favorites & Direct Ping",
				TitleES: "Favoritos de servidores y ping",
			},
			{
				ID:      "instance_clone",
				Status:  "planned",
				TitleEN: "1-Click Instance Cloning",
				TitleES: "Clonación de instancias en 1 clic",
			},
			{
				ID:      "curseforge_import",
				Status:  "planned",
				TitleEN: "CurseForge Modpack Import (.zip)",
				TitleES: "Importar modpacks CurseForge (.zip)",
			},
			{
				ID:      "custom_jvm_args",
				Status:  "planned",
				TitleEN: "Custom JVM Arguments & Flags",
				TitleES: "Argumentos y flags JVM personalizados",
			},
			{
				ID:      "screenshots_browser",
				Status:  "planned",
				TitleEN: "In-App Screenshots Manager",
				TitleES: "Gestor de capturas de pantalla",
			},
		},
	}
}

var httpClient = &http.Client{Timeout: 2500 * time.Millisecond}

// Fetch attempts to retrieve the latest roadmap from the remote repository.
// If the network request fails, it gracefully falls back to the embedded roadmap.
func Fetch() (Roadmap, bool, error) {
	resp, err := httpClient.Get(RemoteURL)
	if err != nil {
		return Embedded(), false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Embedded(), false, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Embedded(), false, err
	}

	var rm Roadmap
	if err := json.Unmarshal(body, &rm); err != nil {
		return Embedded(), false, err
	}

	if len(rm.Items) == 0 {
		return Embedded(), false, fmt.Errorf("empty items in remote roadmap")
	}

	return rm, true, nil
}

// FetchCmd returns a Bubble Tea command that fetches the roadmap asynchronously.
func FetchCmd() tea.Cmd {
	return func() tea.Msg {
		data, online, err := Fetch()
		return LoadedMsg{
			Data:   data,
			Online: online,
			Err:    err,
		}
	}
}
