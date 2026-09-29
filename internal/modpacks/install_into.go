package modpacks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	"mcTUI/internal/instances"
	"mcTUI/internal/operationlock"
)

// installDestination returns a metadata directory separate from the game data.
// Legacy metadata belongs to the application data root, never the parent of
// the user's .minecraft directory.
func (s Store) installDestination(in instances.Instance) (gameDir, metadataDir string, err error) {
	if err = s.root(); err != nil {
		return "", "", err
	}
	if in.Kind != "managed" && (in.Kind != "legacy" || in.ID != instances.LegacyID) {
		return "", "", fmt.Errorf("invalid install destination")
	}
	stored, err := instances.Read(s.Root, in.ID)
	if err != nil {
		return "", "", err
	}
	if stored != in {
		return "", "", fmt.Errorf("instance changed since selection")
	}
	gameDir, err = instances.GameDir(s.Root, in)
	if err != nil {
		return "", "", err
	}
	if in.Kind == "legacy" {
		info, err := os.Lstat(gameDir)
		if err != nil {
			return "", "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("unsafe legacy game directory")
		}
		return gameDir, filepath.Join(s.Root, "legacy-pack"), nil
	}
	return gameDir, filepath.Dir(gameDir), nil
}
func (s Store) packMetadataDir(in instances.Instance, gameDir string) string {
	if in.Kind == "legacy" {
		return filepath.Join(s.Root, "legacy-pack")
	}
	return filepath.Dir(gameDir)
}

// A newly created managed instance may not yet have a minecraft directory.
// Create only that empty directory so its operation lock is shared with all
// other instance operations; revalidate it before opening the lock.
func (s Store) ensureInstallGameDir(in instances.Instance, gameDir string) error {
	if in.Kind != "managed" {
		return nil
	}
	if err := os.Mkdir(gameDir, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	_, err := instances.GameDir(s.Root, in)
	return err
}

func checkPackMetadataDir(dir string, create bool) error {
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe pack metadata directory: %s", dir)
	}
	return nil
}

func absentPack(dir string) error {
	if err := checkPackMetadataDir(dir, false); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := pendingUpdate(dir); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(dir, "pack.json")); err == nil {
		return fmt.Errorf("instance already contains a pack; use update instead")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s Store) installBase(ctx context.Context, in instances.Instance, archive Archive, optional map[string]bool) (string, string, Manifest, error) {
	gameDir, dir, err := s.installDestination(in)
	if err != nil {
		return "", "", Manifest{}, err
	}
	if err := absentPack(dir); err != nil {
		return "", "", Manifest{}, err
	}
	current, err := Inspect(archive.Path)
	if err != nil {
		return "", "", Manifest{}, err
	}
	if current.SHA512 != archive.SHA512 {
		return "", "", Manifest{}, fmt.Errorf("pack archive changed after review")
	}
	version, loader, loaderVersion, err := runtime(current.Index)
	if err != nil {
		return "", "", Manifest{}, err
	}
	if version != in.MinecraftVersion || loader != in.Loader || loaderVersion != in.LoaderVersion {
		return "", "", Manifest{}, fmt.Errorf("pack runtime does not exactly match selected instance; import into a new instance")
	}
	files, err := inventory(ctx, current, optional)
	if err != nil {
		return "", "", Manifest{}, err
	}
	next := Manifest{SchemaVersion: 1, Name: current.Index.Name, Summary: current.Index.Summary,
		IndexVersionID: current.Index.VersionID, IndexSHA512: current.SHA512,
		MinecraftVersion: version, Loader: loader, LoaderVersion: loaderVersion,
		Files: files, Optional: map[string]bool{}}
	for _, file := range current.Index.Files {
		if file.Env != nil && file.Env.Client == "optional" {
			next.Optional[file.Path] = optional[file.Path]
		}
	}
	return gameDir, dir, next, nil
}

// installPlan includes every incoming path. An occupied destination always
// needs explicit consent, even when its bytes match the incoming pack.
func installPlan(gameDir string, next Manifest) (UpdatePlan, error) {
	plan, err := makeUpdatePlan(gameDir, Manifest{Files: map[string]BaseFile{}}, next)
	if err != nil {
		return plan, err
	}
	plan.Conflicts = nil
	for i := range plan.Changes {
		path := &plan.Changes[i]
		path.Conflict = path.Local != ""
		if path.Conflict {
			plan.Conflicts = append(plan.Conflicts, *path)
		}
	}
	return plan, nil
}

// PlanInstall reviews selected incoming paths and every existing destination
// without modifying game files. Installation requires a matching reviewed plan.
func (s Store) PlanInstall(ctx context.Context, in instances.Instance, archive Archive, optional map[string]bool) (UpdatePlan, error) {
	if err := ctx.Err(); err != nil {
		return UpdatePlan{}, err
	}
	gameDir, dir, next, err := s.installBase(ctx, in, archive, optional)
	if err != nil {
		return UpdatePlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return UpdatePlan{}, err
	}
	if err := s.ensureInstallGameDir(in, gameDir); err != nil {
		return UpdatePlan{}, err
	}
	release, err := operationlock.Acquire(gameDir)
	if err != nil {
		return UpdatePlan{}, err
	}
	defer release()
	if err := absentPack(dir); err != nil {
		return UpdatePlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return UpdatePlan{}, err
	}
	return installPlan(gameDir, next)
}

// InstallInto stages and verifies the whole pack before publishing files. The
// durable journal records every changed path and its original bytes, allowing
// rollback after cancellation, publication errors, or an interrupted process.
func (s Store) InstallInto(ctx context.Context, in instances.Instance, archive Archive, source Source, optional map[string]bool, expected UpdatePlan, decisions map[string]bool, progress func(int64, int64)) (err error) {
	gameDir, dir, next, err := s.installBase(ctx, in, archive, optional)
	if err != nil {
		return err
	}
	if err = validateUpdateSource(Source{}, source); err != nil {
		return err
	}
	next.Source = source
	if err = next.validate(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.ensureInstallGameDir(in, gameDir); err != nil {
		return err
	}
	release, err := operationlock.Acquire(gameDir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	if err = absentPack(dir); err != nil {
		return err
	}
	plan, err := installPlan(gameDir, next)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, expected) {
		return fmt.Errorf("pack files changed since install review; review again")
	}
	for _, path := range plan.Conflicts {
		if _, ok := decisions[path.Path]; !ok {
			return fmt.Errorf("decision required for conflict: %s", path.Path)
		}
	}
	conflicts := make(map[string]bool, len(plan.Conflicts))
	for _, path := range plan.Conflicts {
		conflicts[path.Path] = true
	}
	for path := range decisions {
		if !conflicts[path] {
			return fmt.Errorf("unexpected conflict decision: %s", path)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// No metadata is created before the archive, plan, and all decisions pass.
	if err = checkPackMetadataDir(dir, true); err != nil {
		return err
	}
	if err = absentPack(dir); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil {
		return err
	}
	journalWritten := false
	defer func() {
		if !journalWritten {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	stageGame := filepath.Join(stage, "new")
	if err = os.Mkdir(stageGame, 0700); err != nil {
		return err
	}
	if err = materializeUpdate(ctx, s, archive, stageGame, stage, optional, next.Files, progress); err != nil {
		return err
	}
	current, err := Inspect(archive.Path)
	if err != nil {
		return err
	}
	if current.SHA512 != archive.SHA512 {
		return fmt.Errorf("pack archive changed during install")
	}
	stored, err := instances.Read(s.Root, in.ID)
	if err != nil {
		return err
	}
	if stored != in {
		return fmt.Errorf("instance changed during install")
	}
	if err = absentPack(dir); err != nil {
		return err
	}
	currentPlan, err := installPlan(gameDir, next)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentPlan, plan) {
		return fmt.Errorf("local pack files changed during install; review again")
	}
	// Preserved local destinations are not pack-owned. Future updates must
	// obtain new consent instead of assuming ownership over these files.
	for _, path := range plan.Conflicts {
		if !decisions[path.Path] {
			delete(next.Files, path.Path)
		}
	}
	journal := updateJournal{Schema: 2, OldAbsent: true, Stage: filepath.Base(stage), Next: next}
	backupRoot := filepath.Join(stage, "backup")
	if err = os.Mkdir(backupRoot, 0700); err != nil {
		return err
	}
	for _, path := range plan.Changes {
		if path.Conflict && !decisions[path.Path] || path.Local == path.New {
			continue
		}
		journal.Changes = append(journal.Changes, updateChange{Path: path.Path, Before: path.Local, After: path.New})
		if path.Local == "" {
			continue
		}
		backup, e := destination(backupRoot, path.Path)
		if e != nil {
			return e
		}
		if err = copyUpdateFile(filepath.Join(gameDir, filepath.FromSlash(path.Path)), backup); err != nil {
			return err
		}
		actual, e := localDigest(backupRoot, path.Path)
		if e != nil {
			return e
		}
		if actual != path.Local {
			return fmt.Errorf("local pack file changed while backing up: %s", path.Path)
		}
	}
	backupDirs := map[string]bool{backupRoot: true}
	for _, change := range journal.Changes {
		if change.Before == "" {
			continue
		}
		for path := filepath.Dir(filepath.Join(backupRoot, filepath.FromSlash(change.Path))); path != backupRoot && path != stage; path = filepath.Dir(path) {
			backupDirs[path] = true
		}
	}
	dirs := make([]string, 0, len(backupDirs))
	for path := range backupDirs {
		dirs = append(dirs, path)
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs {
		if err = syncDirectory(path); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = syncDirectory(stage); err != nil {
		return err
	}
	if err = absentPack(dir); err != nil {
		return err
	}
	if err = saveJSON(dir, updateJournalName, journal); err != nil {
		return err
	}
	journalWritten = true
	if err = syncDirectory(dir); err != nil {
		return err
	}
	for _, change := range journal.Changes {
		if err = ctx.Err(); err != nil {
			break
		}
		actual, e := localDigest(gameDir, change.Path)
		if e != nil {
			err = e
			break
		}
		if actual != change.Before {
			err = fmt.Errorf("local file changed during publication: %s", change.Path)
			break
		}
		path, e := destination(gameDir, change.Path)
		if e != nil {
			err = e
			break
		}
		err = copyUpdateFile(filepath.Join(stageGame, filepath.FromSlash(change.Path)), path)
		if err != nil {
			break
		}
	}
	if err == nil {
		err = saveManifest(dir, next)
	}
	if err == nil {
		err = syncDirectory(dir)
	}
	if err != nil {
		return errors.Join(err, recoverUpdateLocked(gameDir, dir))
	}
	if err = os.Remove(filepath.Join(dir, updateJournalName)); err != nil {
		return err
	}
	journalWritten = false
	if err = syncDirectory(dir); err != nil {
		return err
	}
	return os.RemoveAll(stage)
}
