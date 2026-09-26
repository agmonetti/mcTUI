// Package resetconfigs resets only mod-related Minecraft folders.
package resetconfigs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mcTUI/internal/config"
	"mcTUI/internal/operationlock"
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
	Restored         FolderStatus = "restored"
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

type BackupInfo struct {
	Name    string
	Path    string
	Created time.Time
	Folders []string
}

type RestoreMode string

const (
	KeepBackup    RestoreMode = "keep"
	ConsumeBackup RestoreMode = "consume"
)

// Inspect checks the same installation that the rest of mcTUI uses.
// It returns the absolute path and existing resettable folders.
func Inspect() (string, []string, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return "", nil, err
	}
	found, err := InspectAt(dir)
	return dir, found, err
}

// InspectAt checks resettable folders under an explicit game directory.
func InspectAt(dir string) ([]string, error) { return inspectAt(dir) }

// Run performs the chosen reset. Call only after explicit user confirmation.
func Run(mode Mode) (Result, error) {
	return RunTo(mode, "")
}

// RunTo creates a timestamped backup in base (empty means the default).
func RunTo(mode Mode, base string) (Result, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return Result{}, err
	}
	return RunAt(dir, mode, base)
}

// RunAt applies a reset to the game directory captured by the caller.
func RunAt(dir string, mode Mode, base string) (Result, error) {
	return RunAtWithBackups(dir, filepath.Join(dir, "mctui-backups"), mode, base)
}

// RunAtWithBackups keeps a managed instance's history outside its game directory.
func RunAtWithBackups(dir, backupRoot string, mode Mode, base string) (result Result, err error) {
	release, err := operationlock.Acquire(dir)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if releaseErr := release(); err == nil {
			err = releaseErr
		}
	}()
	return runAtBaseIn(dir, backupRoot, mode, base)
}

// ListBackups returns valid reset backups, newest first.
func ListBackups() ([]BackupInfo, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return nil, err
	}
	return ListBackupsAt(dir)
}

// ListBackupsAt lists backups associated with an explicit game directory.
func ListBackupsAt(dir string) ([]BackupInfo, error) {
	return ListBackupsAtRoot(dir, filepath.Join(dir, "mctui-backups"))
}

func ListBackupsAtRoot(dir, backupRoot string) ([]BackupInfo, error) {
	return listBackupsIn(dir, backupRoot)
}

// RestorePlan identifies active folders that contain data and would be replaced.
func RestorePlan(name string) (BackupInfo, []string, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return BackupInfo{}, nil, err
	}
	return RestorePlanAt(dir, name)
}

// RestorePlanAt checks conflicts at the explicit game directory.
func RestorePlanAt(dir, name string) (BackupInfo, []string, error) {
	return RestorePlanAtRoot(dir, filepath.Join(dir, "mctui-backups"), name)
}

func RestorePlanAtRoot(dir, backupRoot, name string) (BackupInfo, []string, error) {
	return restorePlanIn(dir, backupRoot, name)
}

func restorePlanAt(dir, name string) (BackupInfo, []string, error) {
	return restorePlanIn(dir, filepath.Join(dir, "mctui-backups"), name)
}

func restorePlanIn(dir, backupRoot, name string) (BackupInfo, []string, error) {
	backup, err := backupInfoIn(dir, backupRoot, name)
	if err != nil {
		return BackupInfo{}, nil, err
	}
	var occupied []string
	for _, folder := range backup.Folders {
		path := filepath.Join(dir, folder)
		exists, err := checkedDir(path, true)
		if err != nil || !exists {
			if err != nil {
				return BackupInfo{}, nil, err
			}
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return BackupInfo{}, nil, err
		}
		if len(entries) > 0 {
			occupied = append(occupied, folder)
		}
	}
	return backup, occupied, nil
}

// Restore restores the selected backup. replaceFolders must name every active
// folder whose non-empty contents the user explicitly approved replacing.
func Restore(name string, mode RestoreMode, replaceFolders []string) (Result, error) {
	dir, err := config.MinecraftDirChecked()
	if err != nil {
		return Result{}, err
	}
	return RestoreAt(dir, name, mode, replaceFolders)
}

// RestoreAt restores only to the game directory captured by the caller.
func RestoreAt(dir, name string, mode RestoreMode, replaceFolders []string) (Result, error) {
	return RestoreAtRoot(dir, filepath.Join(dir, "mctui-backups"), name, mode, replaceFolders)
}

func RestoreAtRoot(dir, backupRoot, name string, mode RestoreMode, replaceFolders []string) (result Result, err error) {
	release, err := operationlock.Acquire(dir)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if releaseErr := release(); err == nil {
			err = releaseErr
		}
	}()
	return restoreIn(dir, backupRoot, name, mode, replaceFolders)
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
	return runAtBase(dir, mode, "")
}

func runAtBase(dir string, mode Mode, base string) (Result, error) {
	return runAtBaseIn(dir, filepath.Join(dir, "mctui-backups"), mode, base)
}

func runAtBaseIn(dir, backupRoot string, mode Mode, base string) (Result, error) {
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
		if base == "" {
			base = backupRoot
			if err := mkdirBackupRoot(base); err != nil {
				return result, err
			}
		} else if err := validateBackupBaseIn(dir, backupRoot, base); err != nil {
			return result, err
		}
		if _, err := checkedDir(base, false); err != nil {
			return result, err
		}
		if err := rememberBackupBaseIn(dir, backupRoot, base); err != nil {
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
			if err := validateBackupBaseIn(dir, backupRoot, base); err != nil {
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

// validateBackupBase refuses links in every ancestor and destinations inside
// folders that a reset moves or deletes.
// mkdirBackupRoot creates only ordinary directories; never follows an existing link.
func mkdirBackupRoot(base string) error {
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return fmt.Errorf("unsafe backup root: %q", base)
	}
	var paths []string
	for path := base; ; path = filepath.Dir(path) {
		paths = append(paths, path)
		if path == filepath.Dir(path) {
			break
		}
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if _, err := checkedDir(paths[i], true); err != nil {
			return err
		}
		if err := os.Mkdir(paths[i], 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if _, err := checkedDir(paths[i], false); err != nil {
			return err
		}
	}
	return nil
}

func validateBackupBase(dir, base string) error {
	return validateBackupBaseIn(dir, filepath.Join(dir, "mctui-backups"), base)
}

func validateBackupBaseIn(dir, backupRoot, base string) error {
	if !filepath.IsAbs(backupRoot) || filepath.Clean(backupRoot) != backupRoot {
		return fmt.Errorf("unsafe backup root: %q", backupRoot)
	}
	if !filepath.IsAbs(base) || filepath.Clean(base) != base {
		return fmt.Errorf("unsafe backup directory: %q", base)
	}
	for path := base; ; path = filepath.Dir(path) {
		if _, err := checkedDir(path, false); err != nil {
			return err
		}
		if path == filepath.Dir(path) {
			break
		}
	}
	for _, name := range targets {
		rel, err := filepath.Rel(filepath.Join(dir, name), base)
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))) {
			return fmt.Errorf("backup directory cannot be inside %s/", name)
		}
	}
	defaultBase := backupRoot
	if rel, err := filepath.Rel(defaultBase, base); err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("backup directory cannot be nested inside %s", defaultBase)
	}
	bases, err := backupBasesIn(dir, backupRoot)
	if err != nil {
		return err
	}
	for _, root := range bases[1:] {
		rel, err := filepath.Rel(root, base)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && strings.HasPrefix(strings.Split(rel, string(os.PathSeparator))[0], "reset-") {
			return fmt.Errorf("backup directory cannot be inside a backup: %s", base)
		}
	}
	return nil
}

func backupBases(dir string) ([]string, error) {
	return backupBasesIn(dir, filepath.Join(dir, "mctui-backups"))
}

func backupBasesIn(dir, backupRoot string) ([]string, error) {
	if _, err := checkedDir(dir, false); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(backupRoot) || filepath.Clean(backupRoot) != backupRoot {
		return nil, fmt.Errorf("unsafe backup root: %q", backupRoot)
	}
	defaultBase := backupRoot
	bases := []string{defaultBase}
	exists, err := checkedDir(defaultBase, true)
	if err != nil || !exists {
		return bases, err
	}
	registry := filepath.Join(defaultBase, "locations.json")
	info, err := os.Lstat(registry)
	if errors.Is(err, os.ErrNotExist) {
		return bases, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe backup location registry: %s", registry)
	}
	data, err := os.ReadFile(registry)
	if err != nil {
		return nil, err
	}
	var external []string
	if err := json.Unmarshal(data, &external); err != nil {
		return nil, err
	}
	return append(bases, external...), nil
}

func rememberBackupBase(dir, base string) error {
	return rememberBackupBaseIn(dir, filepath.Join(dir, "mctui-backups"), base)
}

func rememberBackupBaseIn(dir, backupRoot, base string) error {
	defaultBase := backupRoot
	if base == defaultBase {
		return nil
	}
	if err := mkdirBackupRoot(defaultBase); err != nil {
		return err
	}
	bases, err := backupBasesIn(dir, backupRoot)
	if err != nil || slices.Contains(bases, base) {
		return err
	}
	registry := filepath.Join(defaultBase, "locations.json")
	tmp, err := os.CreateTemp(defaultBase, ".locations-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	data, err := json.Marshal(append(bases[1:], base))
	if err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), registry)
}

func listBackupsAt(dir string) ([]BackupInfo, error) {
	return listBackupsIn(dir, filepath.Join(dir, "mctui-backups"))
}

func listBackupsIn(dir, backupRoot string) ([]BackupInfo, error) {
	bases, err := backupBasesIn(dir, backupRoot)
	if err != nil {
		return nil, err
	}
	var backups []BackupInfo
	for _, base := range bases {
		exists, err := checkedDir(base, true)
		if err != nil {
			return nil, err
		}
		if !exists { // An external drive may be disconnected; keep other backups visible.
			continue
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "reset-") {
				continue
			}
			info, err := backupInfoIn(dir, backupRoot, filepath.Join(base, entry.Name()))
			if err != nil {
				continue // skip invalid entries without blocking the rest
			}
			if len(info.Folders) > 0 {
				backups = append(backups, info)
			}
		}
	}
	slices.SortFunc(backups, func(a, b BackupInfo) int { return b.Created.Compare(a.Created) })
	return backups, nil
}

func backupInfoAt(dir, name string) (BackupInfo, error) {
	return backupInfoIn(dir, filepath.Join(dir, "mctui-backups"), name)
}

func backupInfoIn(dir, backupRoot, name string) (BackupInfo, error) {
	base := backupRoot
	if filepath.IsAbs(name) {
		base = filepath.Dir(name)
	}
	if !strings.HasPrefix(filepath.Base(name), "reset-") || filepath.Base(name) == "reset-" {
		return BackupInfo{}, fmt.Errorf("invalid backup name: %q", name)
	}
	bases, err := backupBasesIn(dir, backupRoot)
	if err != nil {
		return BackupInfo{}, err
	}
	if !slices.Contains(bases, base) {
		return BackupInfo{}, fmt.Errorf("backup location is not registered: %s", base)
	}
	if err := validateBackupBaseIn(dir, backupRoot, base); err != nil {
		return BackupInfo{}, err
	}
	path := filepath.Join(base, filepath.Base(name))
	if _, err := checkedDir(path, false); err != nil {
		return BackupInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return BackupInfo{}, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return BackupInfo{}, err
	}
	backup := BackupInfo{Name: filepath.Base(name), Path: path, Created: info.ModTime()}
	for _, entry := range entries {
		if !slices.Contains(targets, entry.Name()) {
			return BackupInfo{}, fmt.Errorf("unexpected item in backup %s: %s", path, entry.Name())
		}
		if _, err := checkedDir(filepath.Join(path, entry.Name()), false); err != nil {
			return BackupInfo{}, err
		}
		backup.Folders = append(backup.Folders, entry.Name())
	}
	return backup, nil
}

func restoreAt(dir, name string, mode RestoreMode, replaceFolders []string) (Result, error) {
	return restoreIn(dir, filepath.Join(dir, "mctui-backups"), name, mode, replaceFolders)
}

func restoreIn(dir, backupRoot, name string, mode RestoreMode, replaceFolders []string) (Result, error) {
	if mode != KeepBackup && mode != ConsumeBackup {
		return Result{}, fmt.Errorf("unknown restore mode: %q", mode)
	}
	backup, err := backupInfoIn(dir, backupRoot, name)
	if err != nil {
		return Result{}, err
	}
	result := Result{Mode: Backup, BackupPath: backup.Path, Preserved: slices.Clone(Preserved)}
	for _, folder := range backup.Folders {
		dest := filepath.Join(dir, folder)
		exists, err := checkedDir(dest, true)
		if err != nil {
			return result, err
		}
		if exists {
			entries, err := os.ReadDir(dest)
			if err != nil {
				return result, err
			}
			if len(entries) > 0 && !slices.Contains(replaceFolders, folder) {
				return result, fmt.Errorf("%s contains data; explicit replacement confirmation required", folder)
			}
		}
	}
	for _, folder := range backup.Folders {
		source, dest := filepath.Join(backup.Path, folder), filepath.Join(dir, folder)
		status, err := restoreFolderIn(dir, backupRoot, source, dest, mode, slices.Contains(replaceFolders, folder))
		fr := FolderResult{Name: folder, Path: dest, Status: status, Err: err}
		result.Folders = append(result.Folders, fr)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", folder, err))
			break
		}
	}
	if mode == ConsumeBackup && len(result.Errors) == 0 {
		if err := os.Remove(backup.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Errors = append(result.Errors, err)
		}
	}
	return result, nil
}

func restoreFolder(dir, source, dest string, mode RestoreMode, replace bool) (FolderStatus, error) {
	return restoreFolderIn(dir, filepath.Join(dir, "mctui-backups"), source, dest, mode, replace)
}

func restoreFolderIn(dir, backupRoot, source, dest string, mode RestoreMode, replace bool) (FolderStatus, error) {
	if _, err := checkedDir(dir, false); err != nil {
		return Failed, err
	}
	if err := validateBackupBaseIn(dir, backupRoot, filepath.Dir(filepath.Dir(source))); err != nil {
		return Failed, err
	}
	if _, err := checkedDir(source, false); err != nil {
		return Failed, err
	}
	exists, err := checkedDir(dest, true)
	if err != nil {
		return Failed, err
	}
	if exists {
		entries, err := os.ReadDir(dest)
		if err != nil {
			return Failed, err
		}
		if len(entries) > 0 && !replace {
			return Failed, fmt.Errorf("%s gained data after confirmation; refusing replacement", dest)
		}
	}
	if mode == KeepBackup {
		if err := rejectLinks(source); err != nil {
			return Failed, err
		}
	}
	stage, err := os.MkdirTemp(dir, ".mctui-restore-")
	if err != nil {
		return Failed, err
	}
	staged := filepath.Join(stage, filepath.Base(dest))
	if _, err := os.Lstat(dest); err == nil {
		if err := os.Rename(dest, staged); err != nil {
			_ = os.Remove(stage)
			return Failed, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(stage)
		return Failed, err
	}

	if mode == ConsumeBackup {
		err = os.Rename(source, dest)
	} else {
		err = os.CopyFS(dest, os.DirFS(source))
	}
	if err != nil {
		_ = os.RemoveAll(dest)
		if _, statErr := os.Lstat(staged); statErr == nil {
			if rollbackErr := os.Rename(staged, dest); rollbackErr != nil {
				return Failed, fmt.Errorf("restore failed: %v; previous folder remains at %s (rollback failed: %w)", err, staged, rollbackErr)
			}
		}
		_ = os.Remove(stage)
		return Failed, err
	}
	status := Restored
	if mode == ConsumeBackup {
		status = Moved
	}
	if _, statErr := os.Lstat(staged); statErr == nil {
		if err := os.RemoveAll(staged); err != nil {
			return status, fmt.Errorf("restored successfully, but could not remove replaced data at %s: %w", staged, err)
		}
	}
	if err := os.Remove(stage); err != nil {
		return status, fmt.Errorf("restored successfully, but could not clean staging directory %s: %w", stage, err)
	}
	return status, nil
}

func rejectLinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cannot copy backup containing a link: %s", path)
		}
		return nil
	})
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
