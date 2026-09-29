package modpacks

import (
	"archive/zip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"mcTUI/internal/instances"
	"mcTUI/internal/operationlock"
)

// UpdatePath describes the three states of a pack-owned path or a new pack
// destination. Empty hashes mean absence. Unrelated local files are never
// enumerated or removed; a local extra is reported if a new pack targets it.
// Every Conflict needs an explicit decision: true replaces the local state,
// false preserves it. A missing local file is also a local change.
type UpdatePath struct {
	Path     string `json:"path"`
	Old      string `json:"old,omitempty"`
	Local    string `json:"local,omitempty"`
	New      string `json:"new,omitempty"`
	Upstream string `json:"upstream"` // added, removed, changed, unchanged
	LocalChange string `json:"local_change"` // added, removed, changed, unchanged
	Conflict bool `json:"conflict"`
}

type UpdatePlan struct {
	Changes []UpdatePath `json:"changes"`
	Conflicts []UpdatePath `json:"conflicts"`
}

const updateJournalName = ".mctui-pack-update.json"

type updateJournal struct {
	Schema int `json:"schema"`
	Stage string `json:"stage"`
	Old Manifest `json:"old"`
	OldAbsent bool `json:"old_absent,omitempty"`
	Next Manifest `json:"next"`
	Changes []updateChange `json:"changes"`
}

type updateChange struct {
	Path string `json:"path"`
	Before string `json:"before,omitempty"`
	After string `json:"after,omitempty"`
}

func guardedPackPath(path string) error {
	if err := ValidPath(path); err != nil { return err }
	part := strings.ToLower(strings.Split(path, "/")[0])
	if part == "saves" || part == "logs" { return fmt.Errorf("pack update cannot touch worlds or logs: %s", path) }
	return nil
}

// localDigest never follows a symlink, including symlinked parent directories.
func localDigest(gameDir, rel string) (string, error) {
	if err := guardedPackPath(rel); err != nil { return "", err }
	path := filepath.Join(gameDir, filepath.FromSlash(rel))
	for p := filepath.Dir(path); p != gameDir; p = filepath.Dir(p) {
		if !strings.HasPrefix(p, gameDir+string(filepath.Separator)) { return "", fmt.Errorf("path outside instance") }
		info, err := os.Lstat(p)
		if os.IsNotExist(err) { return "", nil }
		if err != nil { return "", err }
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 { return "", fmt.Errorf("unsafe parent directory: %s", p) }
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) { return "", nil }
	if err != nil { return "", err }
	if !info.Mode().IsRegular() { return "", fmt.Errorf("unsafe pack destination: %s", path) }
	f, err := os.Open(path)
	if err != nil { return "", err }
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) { return "", fmt.Errorf("pack destination changed: %s", path) }
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil { return "", err }
	return hex.EncodeToString(h.Sum(nil)), nil
}

// inventory computes hashes of overrides from archive bytes; downloads are
// described by the index and verified again by Fetch when staged.
func inventory(ctx context.Context, archive Archive, optional map[string]bool) (map[string]BaseFile, error) {
	files := make(map[string]BaseFile)
	var total int64
	for _, file := range archive.Index.Files {
		if file.Env != nil && (file.Env.Client == "unsupported" || file.Env.Client == "optional" && !optional[file.Path]) { continue }
		if err := guardedPackPath(file.Path); err != nil { return nil, err }
		maxFile := int64(512 << 20)
		if strings.HasSuffix(strings.ToLower(file.Path), ".jar") { maxFile = 250 << 20 }
		if file.FileSize < 0 || file.FileSize > maxFile || total > 4<<30-file.FileSize { return nil, fmt.Errorf("pack download budget exceeded: %s", file.Path) }
		total += file.FileSize
		urls := make([]string, 0, len(file.Downloads))
		for _, raw := range file.Downloads {
			if err := AllowedURL(raw); err != nil { return nil, fmt.Errorf("unsafe URL for %s: %w", file.Path, err) }
			parsed, err := url.Parse(raw)
			if err != nil { return nil, err }
			parsed.RawQuery, parsed.Fragment, parsed.RawFragment = "", "", ""
			parsed.ForceQuery = false
			urls = append(urls, parsed.String())
		}
		files[file.Path] = BaseFile{SHA512: file.Hashes.SHA512, SHA1: file.Hashes.SHA1, Size: file.FileSize, Layer: "downloads", URLs: urls}
	}
	z, err := zip.OpenReader(archive.Path)
	if err != nil { return nil, err }
	defer z.Close()
	entries := make(map[string]*zip.File, len(z.File))
	for _, f := range z.File { entries[f.Name] = f }
	for _, layer := range []string{"overrides", "client-overrides"} {
		for _, override := range archive.Overrides {
			if override.Layer != layer { continue }
			if err := ctx.Err(); err != nil { return nil, err }
			if err := guardedPackPath(override.Path); err != nil { return nil, err }
			f := entries[override.ZipName]
			if f == nil || int64(f.UncompressedSize64) != override.Size { return nil, fmt.Errorf("override changed: %s", override.Path) }
			r, err := f.Open()
			if err != nil { return nil, err }
			h := sha512.New()
			n, copyErr := io.Copy(h, io.LimitReader(r, override.Size+1))
			copyErr = errors.Join(copyErr, r.Close())
			if copyErr != nil { return nil, copyErr }
			if n != override.Size { return nil, fmt.Errorf("override size changed: %s", override.Path) }
			files[override.Path] = BaseFile{SHA512: hex.EncodeToString(h.Sum(nil)), Size: n, Layer: layer}
		}
	}
	return files, nil
}

func makeUpdatePlan(gameDir string, old, next Manifest) (UpdatePlan, error) {
	paths := make(map[string]bool, len(old.Files)+len(next.Files))
	seenCase := make(map[string]string, len(old.Files)+len(next.Files))
	for path := range old.Files { paths[path] = true; seenCase[strings.ToLower(path)] = path }
	for path := range next.Files {
		if prior := seenCase[strings.ToLower(path)]; prior != "" && prior != path { return UpdatePlan{}, fmt.Errorf("pack path case collision: %s and %s", prior, path) }
		paths[path] = true
	}
	keys := make([]string, 0, len(paths))
	for path := range paths { keys = append(keys, path) }
	sort.Strings(keys)
	plan := UpdatePlan{}
	for _, path := range keys {
		local, err := localDigest(gameDir, path)
		if err != nil { return plan, err }
		oldHash, nextHash := old.Files[path].SHA512, next.Files[path].SHA512
		p := UpdatePath{Path: path, Old: oldHash, Local: local, New: nextHash, Upstream: "unchanged", LocalChange: "unchanged"}
		switch { case oldHash == "" && nextHash != "": p.Upstream = "added"; case oldHash != "" && nextHash == "": p.Upstream = "removed"; case oldHash != nextHash: p.Upstream = "changed" }
		switch { case oldHash == "" && local != "": p.LocalChange = "added"; case oldHash != "" && local == "": p.LocalChange = "removed"; case oldHash != local: p.LocalChange = "changed" }
		p.Conflict = p.LocalChange != "unchanged" && (oldHash == "" && local != "" || local != nextHash)
		plan.Changes = append(plan.Changes, p)
		if p.Conflict { plan.Conflicts = append(plan.Conflicts, p) }
	}
	return plan, nil
}

func (s Store) updateBase(ctx context.Context, in instances.Instance, archive Archive, optional map[string]bool) (string, Manifest, Manifest, error) {
	gameDir, dir, err := s.installDestination(in)
	if err != nil { return "", Manifest{}, Manifest{}, err }
	if err := checkPackMetadataDir(dir, false); err != nil { return "", Manifest{}, Manifest{}, err }
	old, err := loadManifest(dir)
	if err != nil { return "", Manifest{}, Manifest{}, err }
	if old.MinecraftVersion != in.MinecraftVersion || old.Loader != in.Loader || old.LoaderVersion != in.LoaderVersion { return "", Manifest{}, Manifest{}, fmt.Errorf("pack runtime does not match instance") }
	current, err := Inspect(archive.Path)
	if err != nil { return "", Manifest{}, Manifest{}, err }
	if current.SHA512 != archive.SHA512 { return "", Manifest{}, Manifest{}, fmt.Errorf("pack archive changed after review") }
	version, loader, loaderVersion, err := runtime(current.Index)
	if err != nil { return "", Manifest{}, Manifest{}, err }
	if version != in.MinecraftVersion || loader != in.Loader || loaderVersion != in.LoaderVersion { return "", Manifest{}, Manifest{}, fmt.Errorf("pack changes Minecraft or loader; import into a new instance") }
	files, err := inventory(ctx, current, optional)
	if err != nil { return "", Manifest{}, Manifest{}, err }
	next := Manifest{SchemaVersion: 1, Name: current.Index.Name, Summary: current.Index.Summary, IndexVersionID: current.Index.VersionID, IndexSHA512: current.SHA512, MinecraftVersion: version, Loader: loader, LoaderVersion: loaderVersion, Files: files, Optional: map[string]bool{}}
	for _, file := range current.Index.Files { if file.Env != nil && file.Env.Client == "optional" { next.Optional[file.Path] = optional[file.Path] } }
	return gameDir, old, next, nil
}

// PlanUpdate compares the old base, disk, and new archive without mutating
// game files. Only paths owned by either pack are examined; new destinations
// already occupied by an unowned local file appear as conflicts.
func (s Store) PlanUpdate(ctx context.Context, in instances.Instance, archive Archive, optional map[string]bool) (UpdatePlan, error) {
	if err := ctx.Err(); err != nil { return UpdatePlan{}, err }
	gameDir, old, next, err := s.updateBase(ctx, in, archive, optional)
	if err != nil { return UpdatePlan{}, err }
	release, err := operationlock.Acquire(gameDir)
	if err != nil { return UpdatePlan{}, err }
	defer release()
	dir := s.packMetadataDir(in, gameDir)
	if err := pendingUpdate(dir); err != nil { return UpdatePlan{}, err }
	latest, err := loadManifest(dir)
	if err != nil { return UpdatePlan{}, err }
	if !reflect.DeepEqual(latest, old) { return UpdatePlan{}, fmt.Errorf("pack manifest changed during review") }
	if err := ctx.Err(); err != nil { return UpdatePlan{}, err }
	return makeUpdatePlan(gameDir, old, next)
}

func pendingUpdate(dir string) error {
	_, err := os.Lstat(filepath.Join(dir, updateJournalName))
	if err == nil { return fmt.Errorf("pack update recovery pending; recover before using this instance") }
	if os.IsNotExist(err) { return nil }
	return err
}

func validateUpdateSource(old Source, source Source) error {
	if source.Type != "local" && source.Type != "url" && source.Type != "modrinth" { return fmt.Errorf("invalid update source") }
	if source.Type == "modrinth" {
		if source.ProjectID == "" || source.VersionID == "" { return fmt.Errorf("API project and version IDs are required") }
		if old.Type == "modrinth" && old.ProjectID != source.ProjectID { return fmt.Errorf("new release belongs to another project") }
	} else if source.ProjectID != "" || source.VersionID != "" { return fmt.Errorf("non-API source cannot claim API identity") }
	return nil
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil { return err }
	defer d.Close()
	return d.Sync()
}

func copyUpdateFile(source, target string) (err error) {
	info, err := os.Lstat(source)
	if err != nil { return err }
	if !info.Mode().IsRegular() { return fmt.Errorf("unsafe update source: %s", source) }
	input, err := os.Open(source)
	if err != nil { return err }
	defer input.Close()
	if opened, e := input.Stat(); e != nil || !os.SameFile(info, opened) { return fmt.Errorf("update source changed: %s", source) }
	output, err := os.CreateTemp(filepath.Dir(target), ".pack-*" )
	if err != nil { return err }
	defer os.Remove(output.Name())
	if _, err = io.Copy(output, input); err != nil { output.Close(); return err }
	if err = output.Sync(); err != nil { output.Close(); return err }
	if err = output.Close(); err != nil { return err }
	if err = os.Rename(output.Name(), target); err != nil { return err }
	return syncDirectory(filepath.Dir(target))
}

func materializeUpdate(ctx context.Context, s Store, archive Archive, gameDir, stage string, optional map[string]bool, expected map[string]BaseFile, progress func(int64, int64)) error {
	var total int64
	for _, file := range archive.Index.Files {
		if file.Env != nil && (file.Env.Client == "unsupported" || file.Env.Client == "optional" && !optional[file.Path]) { continue }
		maxSize := int64(512 << 20)
		if strings.HasSuffix(strings.ToLower(file.Path), ".jar") { maxSize = 250 << 20 }
		if file.FileSize > maxSize || total > 4<<30-file.FileSize { return fmt.Errorf("pack download budget exceeded: %s", file.Path) }
		total += file.FileSize
		for _, raw := range file.Downloads { if err := AllowedURL(raw); err != nil { return fmt.Errorf("unsafe URL for %s: %w", file.Path, err) } }
	}
	if progress != nil { progress(0, total) }
	files := make(map[string]BaseFile)
	var done int64
	for _, file := range archive.Index.Files {
		if err := ctx.Err(); err != nil { return err }
		if file.Env != nil && (file.Env.Client == "unsupported" || file.Env.Client == "optional" && !optional[file.Path]) { continue }
		maxSize := int64(512 << 20)
		if strings.HasSuffix(strings.ToLower(file.Path), ".jar") { maxSize = 250 << 20 }
		tmp, err := Fetch(ctx, s.HTTP, file.Downloads, file.Hashes, file.FileSize, maxSize, stage)
		if err != nil { return fmt.Errorf("download %s: %w", file.Path, err) }
		dest, err := destination(gameDir, file.Path)
		if err == nil { err = os.Rename(tmp, dest) }
		if err != nil { os.Remove(tmp); return err }
		files[file.Path] = expected[file.Path]
		done += file.FileSize
		if progress != nil { progress(done, total) }
	}
	if err := applyOverrides(ctx, archive, gameDir, files); err != nil { return err }
	if len(files) != len(expected) { return fmt.Errorf("staged pack inventory changed") }
	for path, expectedFile := range expected {
		actual, err := localDigest(gameDir, path)
		if err != nil { return err }
		if actual != expectedFile.SHA512 { return fmt.Errorf("staged pack content differs from review: %s", path) }
	}
	return nil
}

// Update commits only after all conflicts have individual decisions, downloads
// are verified, and the reviewed plan still matches disk under the instance
// operation lock. A journal with durable original copies precedes mutations.
func (s Store) Update(ctx context.Context, in instances.Instance, archive Archive, source Source, optional map[string]bool, expected UpdatePlan, decisions map[string]bool, progress func(int64, int64)) (err error) {
	gameDir, old, next, err := s.updateBase(ctx, in, archive, optional)
	if err != nil { return err }
	if err = validateUpdateSource(old.Source, source); err != nil { return err }
	next.Source = source
	if err = next.validate(); err != nil { return err }
	release, err := operationlock.Acquire(gameDir)
	if err != nil { return err }
	defer func() { err = errors.Join(err, release()) }()
	dir := s.packMetadataDir(in, gameDir)
	if err = pendingUpdate(dir); err != nil { return err }
	latest, err := loadManifest(dir)
	if err != nil { return err }
	if !reflect.DeepEqual(latest, old) { return fmt.Errorf("pack manifest changed before update") }
	// A reviewed plan must not authorize different local bytes or pack paths.
	plan, err := makeUpdatePlan(gameDir, old, next)
	if err != nil { return err }
	if !reflect.DeepEqual(plan, expected) { return fmt.Errorf("pack files changed since update review; review again") }
	for _, p := range plan.Conflicts {
		if _, ok := decisions[p.Path]; !ok { return fmt.Errorf("decision required for conflict: %s (%s locally, %s upstream)", p.Path, p.LocalChange, p.Upstream) }
	}
	for path := range decisions {
		found := false
		for _, p := range plan.Conflicts { if p.Path == path { found = true; break } }
		if !found { return fmt.Errorf("unexpected conflict decision: %s", path) }
	}
	stage, err := os.MkdirTemp(dir, ".mctui-pack-stage-")
	if err != nil { return err }
	journalWritten := false
	defer func() { if !journalWritten { err = errors.Join(err, os.RemoveAll(stage)) } }()
	stageGame := filepath.Join(stage, "new")
	if err = os.Mkdir(stageGame, 0700); err != nil { return err }
	if err = materializeUpdate(ctx, s, archive, stageGame, stage, optional, next.Files, progress); err != nil { return err }
	// Recheck both the archive and manifest after the potentially long download.
	current, err := Inspect(archive.Path)
	if err != nil { return err }
	if current.SHA512 != archive.SHA512 { return fmt.Errorf("pack archive changed during update") }
	latest, err = loadManifest(dir)
	if err != nil { return err }
	if !reflect.DeepEqual(latest, old) { return fmt.Errorf("pack manifest changed during update") }
	currentPlan, err := makeUpdatePlan(gameDir, old, next)
	if err != nil { return err }
	if len(currentPlan.Changes) != len(plan.Changes) { return fmt.Errorf("pack paths changed during update") }
	for n := range plan.Changes { if currentPlan.Changes[n] != plan.Changes[n] { return fmt.Errorf("local pack files changed during update: %s", plan.Changes[n].Path) } }
	// Declining ownership of an unowned destination leaves it out of the
	// installed base, even when its bytes happen to match the new pack.
	// A later update must ask again rather than deleting that local file.
	for _, p := range plan.Conflicts {
		if p.Old == "" && p.Local != "" && !decisions[p.Path] { delete(next.Files, p.Path) }
	}
	journal := updateJournal{Schema: 1, Stage: filepath.Base(stage), Old: old, Next: next}
	backupRoot := filepath.Join(stage, "backup")
	if err = os.Mkdir(backupRoot, 0700); err != nil { return err }
	for _, p := range plan.Changes {
		apply := !p.Conflict || decisions[p.Path]
		if !apply || p.Local == p.New { continue }
		journal.Changes = append(journal.Changes, updateChange{Path: p.Path, Before: p.Local, After: p.New})
		if p.Local == "" { continue }
		backup, e := destination(backupRoot, p.Path)
		if e != nil { return e }
		if err = copyUpdateFile(filepath.Join(gameDir, filepath.FromSlash(p.Path)), backup); err != nil { return err }
		actual, e := localDigest(backupRoot, p.Path)
		if e != nil { return e }
		if actual != p.Local { return fmt.Errorf("local pack file changed while backing up: %s", p.Path) }
	}
	// A synced backup file is not enough after a crash: every directory
	// created to reach it must also be durable before publishing the journal.
	backupDirs := map[string]bool{backupRoot: true}
	for _, change := range journal.Changes {
		if change.Before == "" { continue }
		for path := filepath.Dir(filepath.Join(backupRoot, filepath.FromSlash(change.Path))); path != backupRoot && path != stage; path = filepath.Dir(path) {
			backupDirs[path] = true
		}
	}
	dirs := make([]string, 0, len(backupDirs))
	for path := range backupDirs { dirs = append(dirs, path) }
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs { if err = syncDirectory(path); err != nil { return err } }
	if err = ctx.Err(); err != nil { return err }
	if err = syncDirectory(stage); err != nil { return err }
	if err = pendingUpdate(dir); err != nil { return err }
	if err = saveJSON(dir, updateJournalName, journal); err != nil { return err }
	journalWritten = true
	if err = syncDirectory(dir); err != nil { return err }
	for _, change := range journal.Changes {
		if err = ctx.Err(); err != nil { break }
		actual, e := localDigest(gameDir, change.Path)
		if e != nil { err = e; break }
		if actual != change.Before { err = fmt.Errorf("local file changed during publication: %s", change.Path); break }
		path, e := destination(gameDir, change.Path)
		if e != nil { err = e; break }
		if change.After == "" {
			err = os.Remove(path)
			if err == nil { err = syncDirectory(filepath.Dir(path)) }
		} else {
			err = copyUpdateFile(filepath.Join(stageGame, filepath.FromSlash(change.Path)), path)
		}
		if err != nil { break }
	}
	if err == nil { err = saveManifest(dir, next) }
	if err == nil { err = syncDirectory(dir) }
	if err != nil {
		// Do not erase a journal when restoration fails: next startup can retry.
		err = errors.Join(err, recoverUpdateLocked(gameDir, dir))
		return err
	}
	if err = os.Remove(filepath.Join(dir, updateJournalName)); err != nil { return err }
	journalWritten = false
	if err = syncDirectory(dir); err != nil { return err }
	return os.RemoveAll(stage)
}

// Recover rolls back an interrupted update. Absence of a journal is fine for
// ordinary managed instances and legacy instances. A corrupt journal, changed
// files, or missing backup blocks recovery instead of risking local data loss.
func Recover(root string, in instances.Instance) (err error) {
	if in.ID == "" || root == "" || in.Kind != "managed" && in.Kind != "legacy" { return nil }
	s := Store{Root: root}
	if err := s.root(); err != nil { return err }
	stored, err := instances.Read(root, in.ID)
	if err != nil { return err }
	if stored != in { return fmt.Errorf("instance changed since selection") }
	gameDir, err := instances.GameDir(root, stored)
	if err != nil { return err }
	dir := filepath.Dir(gameDir)
	if in.ID == instances.LegacyID {
		dir = filepath.Join(root, "legacy-pack")
		if err := checkPackMetadataDir(dir, false); err != nil {
			if os.IsNotExist(err) { return nil }
			return err
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, updateJournalName)); os.IsNotExist(err) { return nil } else if err != nil { return err }
	release, err := operationlock.Acquire(gameDir)
	if err != nil { return err }
	defer func() { err = errors.Join(err, release()) }()
	return recoverUpdateLocked(gameDir, dir)
}

func recoverUpdateLocked(gameDir, dir string) error {
	journalPath := filepath.Join(dir, updateJournalName)
	info, err := os.Lstat(journalPath)
	if os.IsNotExist(err) { return nil }
	if err != nil { return err }
	if !info.Mode().IsRegular() || info.Size() > 16<<20 { return fmt.Errorf("invalid pack recovery journal") }
	journalFile, err := os.Open(journalPath)
	if err != nil { return err }
	opened, statErr := journalFile.Stat()
	if statErr != nil || !os.SameFile(info, opened) {
		journalFile.Close()
		return fmt.Errorf("pack recovery journal changed while opening")
	}
	bytes, readErr := io.ReadAll(io.LimitReader(journalFile, 16<<20+1))
	err = errors.Join(readErr, journalFile.Close())
	if err != nil { return err }
	if len(bytes) > 16<<20 { return fmt.Errorf("pack recovery journal exceeds size limit") }
	var journal updateJournal
	if err := json.Unmarshal(bytes, &journal); err != nil { return fmt.Errorf("invalid pack recovery journal: %w", err) }
	if (journal.Schema != 1 && journal.Schema != 2) || journal.OldAbsent != (journal.Schema == 2) || !strings.HasPrefix(journal.Stage, ".mctui-pack-stage-") || filepath.Base(journal.Stage) != journal.Stage || !journal.OldAbsent && journal.Old.validate() != nil || journal.Next.validate() != nil { return fmt.Errorf("invalid pack recovery journal") }
	if journal.OldAbsent && (journal.Old.SchemaVersion != 0 || len(journal.Old.Files) != 0) { return fmt.Errorf("invalid absent pack recovery base") }
	current, err := loadManifest(dir)
	if journal.OldAbsent && os.IsNotExist(err) {
		current, err = Manifest{}, nil
	}
	if err != nil { return fmt.Errorf("pack recovery cannot read manifest: %w", err) }
	if !reflect.DeepEqual(current, journal.Old) && !reflect.DeepEqual(current, journal.Next) { return fmt.Errorf("pack recovery blocked by external manifest change") }
	stage := filepath.Join(dir, journal.Stage)
	stageInfo, err := os.Lstat(stage)
	if err != nil { return fmt.Errorf("pack recovery staging unavailable: %w", err) }
	if !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 { return fmt.Errorf("unsafe pack recovery staging") }
	backupRoot := filepath.Join(stage, "backup")
	backupInfo, err := os.Lstat(backupRoot)
	if err != nil { return fmt.Errorf("pack recovery backup directory unavailable: %w", err) }
	if !backupInfo.IsDir() || backupInfo.Mode()&os.ModeSymlink != 0 { return fmt.Errorf("unsafe pack recovery backup directory") }
	seen := make(map[string]bool)
	for _, change := range journal.Changes {
		if err := guardedPackPath(change.Path); err != nil { return err }
		key := strings.ToLower(change.Path)
		if seen[key] || change.Before == change.After || change.Before != "" && !validHash(change.Before, 64) || change.After != "" && !validHash(change.After, 64) ||
			change.After != journal.Next.Files[change.Path].SHA512 || journal.Old.Files[change.Path].SHA512 == "" && journal.Next.Files[change.Path].SHA512 == "" {
			return fmt.Errorf("invalid pack recovery entry: %s", change.Path)
		}
		seen[key] = true
		if change.Before != "" {
			backup, err := localDigest(filepath.Join(stage, "backup"), change.Path)
			if err != nil { return err }
			if backup != change.Before { return fmt.Errorf("missing or changed pack recovery backup: %s", change.Path) }
		}
		local, err := localDigest(gameDir, change.Path)
		if err != nil { return err }
		if local != change.Before && local != change.After { return fmt.Errorf("pack recovery blocked by external change: %s", change.Path) }
	}
	// Validate all entries before changing any destination. Keep backups intact
	// until journal removal, making rollback repeatable across crashes.
	for _, change := range journal.Changes {
		local, err := localDigest(gameDir, change.Path)
		if err != nil { return err }
		if local == change.Before { continue }
		path, err := destination(gameDir, change.Path)
		if err != nil { return err }
		if change.Before == "" {
			if local != "" {
				if err := os.Remove(path); err != nil { return err }
				if err := syncDirectory(filepath.Dir(path)); err != nil { return err }
			}
		} else if err := copyUpdateFile(filepath.Join(stage, "backup", filepath.FromSlash(change.Path)), path); err != nil { return err }
	}
	if journal.OldAbsent {
		if err := os.Remove(filepath.Join(dir, "pack.json")); err != nil && !os.IsNotExist(err) { return err }
	} else if err := saveManifest(dir, journal.Old); err != nil { return err }
	if err := syncDirectory(dir); err != nil { return err }
	if err := os.Remove(journalPath); err != nil { return err }
	if err := syncDirectory(dir); err != nil { return err }
	return os.RemoveAll(stage)
}
