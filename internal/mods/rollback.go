package mods

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type HistoryEntry struct {
	Date      string `json:"date"`
	ProjectID string `json:"project_id"`
	Previous  Record `json:"previous"`
	New       Record `json:"new"`
	Backup    string `json:"backup"`
}

func (s Store) backupDir() string {
	return filepath.Join(filepath.Dir(s.Registry), "backups", "mods_replaced")
}
func (s Store) history() ([]HistoryEntry, error) {
	path := filepath.Join(s.backupDir(), "history.json")
	if err := safePath(path); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > maxRegistry {
		return nil, fmt.Errorf("history too large")
	}
	var entries []HistoryEntry
	err = json.Unmarshal(b, &entries)
	return entries, err
}
func (s Store) saveHistory(entries []HistoryEntry) error {
	dir := s.backupDir()
	if err := safePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxRegistry {
		return fmt.Errorf("history too large")
	}
	f, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, "history.json"))
}
func (s Store) History() ([]HistoryEntry, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := s.recoverIfPending(); err != nil {
		return nil, err
	}
	return s.history()
}
func (s Store) prune(entries []HistoryEntry) ([]HistoryEntry, error) {
	var size int64
	kept := make([]HistoryEntry, 0, len(entries))
	var discarded []string
	seen := map[string]bool{}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !validName(e.Previous.File) || strings.ContainsAny(e.Backup, `/\`) || e.Backup == "" {
			return nil, fmt.Errorf("invalid backup history")
		}
		path := filepath.Join(s.backupDir(), e.Backup)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		key := e.ProjectID
		if key == "" {
			key = e.Previous.File
		}
		if seen[key] || !info.Mode().IsRegular() || size+info.Size() > 500<<20 {
			discarded = append(discarded, path)
			continue
		}
		seen[key] = true
		size += info.Size()
		kept = append(kept, e)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	// Persist the retained set before deleting backups: a failed deletion may leave
	// an orphan, but must never leave history pointing at a missing rollback file.
	if err := s.saveHistory(kept); err != nil {
		return nil, err
	}
	for _, path := range discarded {
		if err := os.Remove(path); err != nil {
			return kept, err
		}
	}
	return kept, nil
}

// Rollback restores the previous JAR only if the installed version still matches its recorded snapshot.
func (s Store) Rollback(projectID string) error {
	release, err := s.acquire()
	if err != nil {
		return err
	}
	defer release()
	r, err := s.load()
	if err != nil {
		return err
	}
	entries, err := s.history()
	if err != nil {
		return err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.ProjectID != projectID {
			continue
		}
		if !validName(e.Previous.File) || !validName(e.New.File) || strings.ContainsAny(e.Backup, `/\`) {
			return fmt.Errorf("invalid backup history")
		}
		backup := filepath.Join(s.backupDir(), e.Backup)
		prev, err := snapshot(backup)
		if err != nil {
			return err
		}
		if prev.SHA256 != e.Previous.SHA256 {
			return fmt.Errorf("backup changed")
		}
		current := filepath.Join(s.GameDir, "mods", e.New.File)
		now, err := snapshot(current)
		if err != nil {
			return err
		}
		if now.SHA256 != e.New.SHA256 {
			return fmt.Errorf("installed mod changed; preserved")
		}
		if e.Previous.File != e.New.File {
			if err = s.collision(e.Previous.File); err != nil {
				return err
			}
		}
		// Keep the current file until the old one has been safely restored.
		swap := filepath.Join(s.backupDir(), ".rollback-"+e.Backup)
		if err = os.Rename(current, swap); err != nil {
			return err
		}
		dest := filepath.Join(s.GameDir, "mods", e.Previous.File)
		if err = os.Rename(backup, dest); err != nil {
			return errors.Join(err, os.Rename(swap, current))
		}
		rec, err := snapshot(dest)
		if err != nil {
			return errors.Join(err, os.Rename(dest, backup), os.Rename(swap, current))
		}
		rec.Origin = e.Previous.Origin
		rec.ProjectID = e.Previous.ProjectID
		rec.VersionID = e.Previous.VersionID
		rec.Channel = e.Previous.Channel
		rec.Hashes = e.Previous.Hashes
		rec.RequiredBy = e.Previous.RequiredBy
		rec.InstalledAt = e.Previous.InstalledAt
		original := registry{SchemaVersion: r.SchemaVersion, Mods: append([]Record(nil), r.Mods...)}
		put(&r, e.New.File, &rec)
		if err = s.save(r); err == nil {
			entries = append(entries[:i], entries[i+1:]...)
			err = s.saveHistory(entries)
		}
		if err != nil {
			return errors.Join(err, os.Rename(dest, backup), os.Rename(swap, current), s.save(original))
		}
		return os.Remove(swap)
	}
	return fmt.Errorf("no backup for project %s", projectID)
}
