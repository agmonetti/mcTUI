// Package resetconfigs resets only mod-related Minecraft folders.
package resetconfigs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"mcTUI/internal/config"
)

type Mode string

const (
	Backup Mode = "backup"
	Purge  Mode = "purge"
)

type FolderStatus string

const (
	Untouched        FolderStatus = "untouched"
	Moved            FolderStatus = "moved"
	Deleted          FolderStatus = "deleted"
	PartiallyDeleted FolderStatus = "partially_deleted"
	Failed           FolderStatus = "failed"
)

var targets = []string{"mods", "config"}
var Preserved = []string{"saves", "versions", "assets", "libraries", "shaderpacks"}

type FolderResult struct {
	Name    string
	Path    string
	Status  FolderStatus
	Removed []string // Paths observed missing after a failed deletion, relative to Path.
	Err     error
}

type Result struct {
	Mode           Mode
	BackupPath     string
	Folders        []FolderResult
	Preserved      []string
	Errors         []error
	NothingToReset bool
}

// Inspect checks the same installation that the rest of mcTUI uses.
// It returns the absolute path and existing resettable folders.
func Inspect() (string, []string, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return "", nil, err
	}
	found, err := inspectAt(dir)
	return dir, found, err
}

// Run performs the chosen reset. Call only after explicit user confirmation.
func Run(mode Mode) (Result, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return Result{}, err
	}
	return runAt(dir, mode)
}

// checkedDir rejects links (including Windows junctions reported as links by
// os.Lstat) and paths that are not ordinary directories.
func checkedDir(path string, missingOK bool) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && missingOK {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("%s is not a regular directory (link or unexpected file)", path)
	}
	return true, nil
}

func inspectAt(dir string) ([]string, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || filepath.Dir(dir) == dir {
		return nil, fmt.Errorf("unsafe Minecraft path: %q", dir)
	}
	if _, err := checkedDir(dir, false); err != nil {
		return nil, err
	}
	var found []string
	for _, name := range targets {
		exists, err := checkedDir(filepath.Join(dir, name), true)
		if err != nil {
			return nil, err
		}
		if exists {
			found = append(found, name)
		}
	}
	if _, err := checkedDir(filepath.Join(dir, "mctui-backups"), true); err != nil {
		return nil, err
	}
	return found, nil
}

func runAt(dir string, mode Mode) (Result, error) {
	if mode != Backup && mode != Purge {
		return Result{}, fmt.Errorf("unknown reset mode: %q", mode)
	}
	found, err := inspectAt(dir)
	if err != nil {
		return Result{}, err
	}
	result := Result{Mode: mode, Preserved: slices.Clone(Preserved), NothingToReset: len(found) == 0}
	for _, name := range targets {
		result.Folders = append(result.Folders, FolderResult{Name: name, Path: filepath.Join(dir, name), Status: Untouched})
	}
	if result.NothingToReset {
		return result, nil
	}
	if mode == Backup {
		base := filepath.Join(dir, "mctui-backups")
		if _, err := checkedDir(base, true); err != nil {
			return result, err
		}
		if err := os.Mkdir(base, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return result, err
		}
		if _, err := checkedDir(base, false); err != nil {
			return result, err
		}
		for n := 0; ; n++ {
			name := "reset-" + time.Now().Format("2006-01-02-150405")
			if n > 0 {
				name = fmt.Sprintf("%s-%d", name, n)
			}
			path := filepath.Join(base, name)
			if err := os.Mkdir(path, 0700); errors.Is(err, os.ErrExist) {
				if _, err := checkedDir(path, false); err != nil {
					return result, err
				}
				continue
			} else if err != nil {
				return result, err
			}
			result.BackupPath = path
			break
		}
	}
	for i := range result.Folders {
		folder := &result.Folders[i]
		if !slices.Contains(found, folder.Name) {
			continue
		}
		// Check again immediately before each mutation: the UI's inspection
		// and even this run's initial inspection may be stale.
		if _, err := checkedDir(dir, false); err != nil {
			folder.Err = err
		} else if _, err := checkedDir(folder.Path, false); err != nil {
			folder.Err = err
		} else if mode == Backup {
			if _, err := checkedDir(filepath.Join(dir, "mctui-backups"), false); err != nil {
				folder.Err = err
			} else if _, err := checkedDir(result.BackupPath, false); err != nil {
				folder.Err = err
			} else {
				dest := filepath.Join(result.BackupPath, folder.Name)
				if _, err := os.Lstat(dest); err == nil {
					folder.Err = fmt.Errorf("backup destination already exists: %s", dest)
				} else if !errors.Is(err, os.ErrNotExist) {
					folder.Err = err
				} else {
					folder.Err = os.Rename(folder.Path, dest)
					_, sourceErr := os.Lstat(folder.Path)
					_, destErr := checkedDir(dest, false)
					if errors.Is(sourceErr, os.ErrNotExist) && destErr == nil {
						folder.Status = Moved
					}
					if folder.Err == nil && folder.Status != Moved {
						folder.Err = fmt.Errorf("could not verify move from %s to %s (source: %v, destination: %v)", folder.Path, dest, sourceErr, destErr)
					}
				}
			}
		} else {
			var before []string
			before, folder.Err = snapshot(folder.Path)
			if folder.Err == nil {
				folder.Err = os.RemoveAll(folder.Path)
				for _, name := range before {
					if _, err := os.Lstat(filepath.Join(folder.Path, name)); errors.Is(err, os.ErrNotExist) {
						folder.Removed = append(folder.Removed, name)
					} else if err != nil && folder.Err == nil {
						folder.Err = err
					}
				}
				if _, err := os.Lstat(folder.Path); err == nil && folder.Err == nil {
					folder.Err = fmt.Errorf("source still exists after deletion: %s", folder.Path)
				} else if err != nil && !errors.Is(err, os.ErrNotExist) && folder.Err == nil {
					folder.Err = err
				}
				if folder.Err == nil {
					folder.Status = Deleted
				} else if len(folder.Removed) > 0 {
					folder.Status = PartiallyDeleted
				}
			}
		}
		if folder.Err != nil {
			if folder.Status == Untouched {
				folder.Status = Failed
			}
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", folder.Name, folder.Err))
			break // Never attempt the next folder after an error.
		}
	}
	return result, nil
}

func snapshot(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, err
}
