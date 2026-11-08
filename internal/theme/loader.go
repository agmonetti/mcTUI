package theme

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"mcTUI/internal/config"
)

// ThemesDir returns the standard directory where custom user themes are stored.
func ThemesDir() string {
	return config.AppDataDir("mctui", "themes")
}

// LoadUserThemes reads all valid .json theme files located in the given directory.
// Invalid or unreadable files are skipped gracefully.
func LoadUserThemes(dir string) []Theme {
	if strings.TrimSpace(dir) == "" {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var themes []Theme
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var t Theme
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}

		// If ID is missing, default to the file stem
		if strings.TrimSpace(t.ID) == "" {
			t.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		t.Normalize()
		themes = append(themes, t)
	}

	return themes
}

// List returns all available themes (built-in themes followed by any valid custom user themes).
func List(customThemesDir string) []Theme {
	builtins := BuiltinThemes()
	if strings.TrimSpace(customThemesDir) == "" {
		customThemesDir = ThemesDir()
	}

	userThemes := LoadUserThemes(customThemesDir)
	if len(userThemes) == 0 {
		return builtins
	}

	// Avoid duplicates if a user theme defines the same ID as a built-in
	idMap := make(map[string]bool)
	var all []Theme
	for _, b := range builtins {
		idMap[strings.ToLower(b.ID)] = true
		all = append(all, b)
	}

	for _, u := range userThemes {
		lowID := strings.ToLower(u.ID)
		if idMap[lowID] {
			// Replace built-in with custom theme override
			for idx, existing := range all {
				if strings.ToLower(existing.ID) == lowID {
					all[idx] = u
					break
				}
			}
		} else {
			all = append(all, u)
			idMap[lowID] = true
		}
	}

	return all
}

// Get resolves a theme by ID from either custom themes or builtins.
// If the ID is empty or not found, it returns the default theme.
func Get(id string, customThemesDir string) Theme {
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return Default()
	}

	all := List(customThemesDir)
	for _, t := range all {
		if strings.ToLower(t.ID) == id {
			return t
		}
	}

	return Default()
}
