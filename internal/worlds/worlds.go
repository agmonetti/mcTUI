// Package worlds reads basic information about a player's singleplayer
// saves (world name, last-played Minecraft version, last-played time) by
// parsing level.dat files via package nbt. It knows nothing about the UI
// or the launcher — it only answers "what worlds exist and what do we
// know about them?".
package worlds

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mcTUI/internal/nbt"
)

// Info holds the subset of level.dat data needed to display a world in
// the launcher: its folder name, display name, the Minecraft version it
// was last played on, and when it was last played.
type Info struct {
	FolderName string
	LevelName  string
	Version    string
	LastPlayed int64 // milliseconds since Unix epoch; 0 if unknown
}

// List scans <mcDir>/saves/*/level.dat and returns Info for each world
// found, sorted by LastPlayed descending (most recently played first;
// worlds with unknown LastPlayed sort last). Worlds whose level.dat can't
// be read or parsed are skipped silently.
func List(mcDir string) []Info {
	savesDir := filepath.Join(mcDir, "saves")
	entries, err := os.ReadDir(savesDir)
	if err != nil {
		return nil
	}

	var result []Info
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
		result = append(result, info)
	}

	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j-1].LastPlayed < result[j].LastPlayed; j-- {
			result[j-1], result[j] = result[j], result[j-1]
		}
	}
	return result
}

// parseLevelDat opens, gunzips, and walks a level.dat file looking for
// Data.LevelName, Data.Version.Name, and Data.LastPlayed. Returns
// ok=false if the file can't be opened/decompressed/parsed, or if
// LevelName was never found (treated as "not a valid world save").
func parseLevelDat(path string) (Info, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, false
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return Info{}, false
	}
	defer gz.Close()

	r := bufio.NewReader(gz)

	if !nbt.ReadRootCompound(r) {
		return Info{}, false
	}

	info := Info{}

	// descendPath defines, for THIS file format, how compound names map
	// to the "path" used by the visitor below: the root's only relevant
	// child is "Data", and within "Data" the only relevant child is
	// "Version". Everything else is walked but produces an empty path
	// (so the visitor below ignores its fields).
	descendPath := func(path []string, name string) []string {
		switch {
		case len(path) == 0 && name == "Data":
			return []string{"Data"}
		case isPath(path, "Data") && name == "Version":
			return []string{"Data", "Version"}
		default:
			return nil
		}
	}

	visit := func(path []string, name string, tagType byte, r *bufio.Reader) bool {
		switch tagType {
		case nbt.TagString:
			s, ok := nbt.ReadString(r)
			if !ok {
				return false
			}
			if isPath(path, "Data") && name == "LevelName" {
				info.LevelName = s
			} else if isPath(path, "Data", "Version") && name == "Name" {
				info.Version = s
			}
			return true

		case nbt.TagLong:
			val, ok := nbt.ReadLong(r)
			if !ok {
				return false
			}
			if isPath(path, "Data") && name == "LastPlayed" {
				info.LastPlayed = val
			}
			return true

		default:
			return nbt.Skip(r, tagType)
		}
	}

	nbt.Walk(r, []string{}, descendPath, visit)

	if info.LevelName == "" {
		return Info{}, false
	}
	return info, true
}

// isPath reports whether path equals want, element by element.
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

// FormatRelativeTime converts a Unix timestamp in milliseconds to a short
// human-readable "time ago" string (e.g. "2h ago", "3d ago"). Returns ""
// if ms is 0 (LastPlayed not present in level.dat). Falls back to an
// absolute date ("Jan 2, 2006") for timestamps older than 30 days.
func FormatRelativeTime(ms int64) string {
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
