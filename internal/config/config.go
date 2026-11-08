// Package config handles persistence of user settings (username, selected
// version, modloader, memory allocation, language) and resolves OS-specific paths
// for config files and the Minecraft directory.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"mcTUI/internal/i18n"
)

// Data is the persisted user configuration, stored as JSON under
// AppDataDir("mctui", "config.json").
type Data struct {
	Username   string `json:"username"`
	Version    string `json:"version"`
	Modloader  string `json:"modloader"`
	MemoryMB   int    `json:"memory_mb"`
	Language   string `json:"language"`
	ModChannel string `json:"mod_channel,omitempty"`
}

// defaults returns the configuration used when no config file exists yet,
// or to fill in missing/zero fields from a partially-populated one.
func defaults() Data {
	return Data{
		Username:   "",
		Version:    "1.20.4",
		Modloader:  "Vanilla",
		MemoryMB:   2048,
		Language:   i18n.DetectSystemLanguage(),
		ModChannel: "release",
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

// DataDirChecked resolves the root for managed instances and shared runtime.
// It never falls back to the working directory when the home is unavailable.
func DataDirChecked() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("LOCALAPPDATA must be absolute: %q", base)
		}
		return filepath.Join(base, "mctui"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "mctui"), nil
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("XDG_DATA_HOME must be absolute: %q", base)
		}
		return filepath.Join(base, "mctui"), nil
	}
}

// MinecraftDir returns the existing launcher path. Destructive operations
// should use MinecraftDirChecked instead.
func MinecraftDir() string {
	dir, _ := MinecraftDirChecked()
	return dir
}

// MinecraftDirChecked resolves the Minecraft directory without falling back
// to a relative path when the user's home directory is unavailable.
func MinecraftDirChecked() (string, error) {
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, ".minecraft"), nil
		}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(homeDir, "AppData", "Roaming", ".minecraft"), nil
	}
	return filepath.Join(homeDir, ".minecraft"), nil
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
	if cfg.Language == "" {
		cfg.Language = def.Language
	}
	if cfg.ModChannel != "beta" && cfg.ModChannel != "alpha" {
		cfg.ModChannel = def.ModChannel
	}
	return cfg
}

// Save writes cfg to the config file, creating parent directories as
// needed.
func Save(cfg Data) error {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}
