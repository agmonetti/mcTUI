// Package config handles persistence of user settings (username, selected
// version, modloader, memory allocation) and resolves OS-specific paths
// for config files and the Minecraft directory.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Data is the persisted user configuration, stored as JSON under
// AppDataDir("mctui", "config.json").
type Data struct {
	Username  string `json:"username"`
	Version   string `json:"version"`
	Modloader string `json:"modloader"`
	MemoryMB  int    `json:"memory_mb"`
}

// defaults returns the configuration used when no config file exists yet,
// or to fill in missing/zero fields from a partially-populated one.
func defaults() Data {
	return Data{
		Username:  "Player",
		Version:   "1.20.4",
		Modloader: "Vanilla",
		MemoryMB:  2048,
	}
}

// AppDataDir returns the base directory used for app-specific data,
// following OS conventions (Roaming AppData on Windows, ~/.config
// elsewhere). subdirs are appended to that base, e.g.
// AppDataDir("mctui", "config.json").
func AppDataDir(subdirs ...string) string {
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

// MinecraftDir returns the path to the .minecraft directory, following
// the same conventions as the official launcher.
func MinecraftDir() string {
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

// path returns the location of the config file.
func path() string {
	return AppDataDir("mctui", "config.json")
}

// Load reads the config file, falling back to defaults() for missing
// fields (or the whole file, if it doesn't exist or can't be parsed).
func Load() Data {
	data, err := os.ReadFile(path())
	def := defaults()
	if err != nil {
		return def
	}

	var cfg Data
	json.Unmarshal(data, &cfg)

	if cfg.Username == "" {
		cfg.Username = def.Username
	}
	if cfg.Version == "" {
		cfg.Version = def.Version
	}
	if cfg.Modloader == "" {
		cfg.Modloader = def.Modloader
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = def.MemoryMB
	}
	return cfg
}

// Save writes cfg to the config file, creating parent directories as
// needed.
func Save(cfg Data) {
	p := path()
	os.MkdirAll(filepath.Dir(p), 0755)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(p, data, 0644)
}
