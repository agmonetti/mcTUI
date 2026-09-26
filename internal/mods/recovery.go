package mods

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type installStep struct {
	New    Record  `json:"new"`
	Old    *Record `json:"old,omitempty"`
	Backup string  `json:"backup,omitempty"`
	Temp   string  `json:"temp"`
}

type installPending struct {
	Registry registry       `json:"registry"`
	History  []HistoryEntry `json:"history"`
	Steps    []installStep  `json:"steps"`
}

func (s Store) pendingPath() string { return filepath.Join(s.backupDir(), "install.pending.json") }

func (s Store) recoverIfPending() error {
	if _, err := os.Lstat(s.pendingPath()); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	release, err := s.acquire()
	if err != nil {
		return err
	}
	return release()
}

func (s Store) savePending(p installPending) error {
	dir := s.backupDir()
	if err := safePath(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(data) > 3*maxRegistry {
		return fmt.Errorf("recovery journal too large")
	}
	f, err := os.CreateTemp(dir, ".pending-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), s.pendingPath())
}

// recoverInstall runs only under the instance lock. A changed file stops recovery without deleting it.
func (s Store) recoverInstall() error {
	path := s.pendingPath()
	if err := safePath(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) > 3*maxRegistry {
		return fmt.Errorf("recovery journal too large: %s", path)
	}
	var pending installPending
	if err = json.Unmarshal(data, &pending); err != nil {
		return fmt.Errorf("invalid recovery journal %s: %w", path, err)
	}
	if pending.Registry.SchemaVersion != 1 && pending.Registry.SchemaVersion != 2 || len(pending.Steps) == 0 {
		return fmt.Errorf("invalid recovery journal: %s", path)
	}
	for _, step := range pending.Steps {
		if !validName(step.New.File) || !strings.HasPrefix(step.Temp, ".tmp-") || strings.ContainsAny(step.Temp, `/\`) || step.New.SHA256 == "" {
			return fmt.Errorf("invalid recovery step: %s", path)
		}
		if step.Old != nil && (!validName(step.Old.File) || step.Backup == "" || strings.ContainsAny(step.Backup, `/\`) || !strings.HasSuffix(step.Backup, "_"+step.Old.File)) {
			return fmt.Errorf("invalid recovery backup: %s", path)
		}
	}
	modsDir := filepath.Join(s.GameDir, "mods")
	type recoveryStep struct {
		backupExists, tempExists, removeNew, newExists bool
	}
	states := make([]recoveryStep, len(pending.Steps))
	// Preflight the entire batch: a bad backup or a changed JAR in a later step
	// must not leave earlier steps half-restored.
	for i := len(pending.Steps) - 1; i >= 0; i-- {
		step := pending.Steps[i]
		state := &states[i]
		newPath := filepath.Join(modsDir, step.New.File)
		backup := filepath.Join(s.backupDir(), step.Backup)
		if step.Old != nil {
			_, err = os.Lstat(backup)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			state.backupExists = err == nil
			if state.backupExists {
				old, scanErr := snapshot(backup)
				if scanErr != nil || old.SHA256 != step.Old.SHA256 {
					return fmt.Errorf("recovery blocked by changed backup %s: %v", backup, scanErr)
				}
			}
		}
		temp := filepath.Join(modsDir, step.Temp)
		if _, err = os.Lstat(temp); err == nil {
			staged, scanErr := snapshot(temp)
			if scanErr != nil || staged.SHA256 != step.New.SHA256 {
				return fmt.Errorf("recovery blocked by changed temporary %s: %v", temp, scanErr)
			}
			state.tempExists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// A remaining staged file means this step was not published; leave any external destination alone.
		state.removeNew = !state.tempExists && (step.Old == nil || state.backupExists || step.Old.File != step.New.File)
		if state.removeNew {
			if _, err = os.Lstat(newPath); err == nil {
				now, scanErr := snapshot(newPath)
				if scanErr != nil || now.SHA256 != step.New.SHA256 {
					return fmt.Errorf("recovery blocked by changed file %s: %v", newPath, scanErr)
				}
				state.newExists = true
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if step.Old != nil {
			oldPath := filepath.Join(modsDir, step.Old.File)
			if state.backupExists {
				if _, err = os.Lstat(oldPath); err == nil && !(oldPath == newPath && state.newExists) {
					return fmt.Errorf("recovery destination occupied: %s", oldPath)
				} else if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
			} else {
				old, scanErr := snapshot(oldPath)
				if scanErr != nil || old.SHA256 != step.Old.SHA256 {
					return fmt.Errorf("recovery missing original %s: %v", oldPath, scanErr)
				}
			}
		}
	}
	for i := len(pending.Steps) - 1; i >= 0; i-- {
		step, state := pending.Steps[i], states[i]
		newPath := filepath.Join(modsDir, step.New.File)
		if state.removeNew && state.newExists {
			if err = os.Remove(newPath); err != nil {
				return err
			}
		}
		if state.backupExists {
			if err = os.Rename(filepath.Join(s.backupDir(), step.Backup), filepath.Join(modsDir, step.Old.File)); err != nil {
				return err
			}
		}
		if state.tempExists {
			if err = os.Remove(filepath.Join(modsDir, step.Temp)); err != nil {
				return err
			}
		}
	}
	if err = s.save(pending.Registry); err != nil {
		return err
	}
	if err = s.saveHistory(pending.History); err != nil {
		return err
	}
	return os.Remove(path)
}
