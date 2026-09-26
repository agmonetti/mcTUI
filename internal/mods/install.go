package mods

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"mcTUI/internal/modrinth"
)

// Install stages and verifies every selected file before publishing anything. A changed local JAR requires explicit consent.
func (s Store) Install(ctx context.Context, c *modrinth.Client, plan modrinth.InstallPlan, optional []string, overwriteChanged bool, progress modrinth.Progress) error {
	release, err := s.acquire()
	if err != nil {
		return err
	}
	defer release()
	r, err := s.load()
	if err != nil {
		return err
	}
	originalRegistry := registry{SchemaVersion: r.SchemaVersion, Mods: append([]Record(nil), r.Mods...)}
	if len(plan.IncompatibleMods) > 0 {
		return fmt.Errorf("incompatible mods: %v", plan.IncompatibleMods)
	}
	items := append([]modrinth.PlanItem(nil), plan.Targets...)
	if len(items) == 0 {
		targetFile, ok := plan.TargetMod.PrimaryFile()
		if !ok {
			return fmt.Errorf("target has no primary file")
		}
		items = append(items, modrinth.PlanItem{Version: plan.TargetMod, File: targetFile})
	}
	items = append(items, plan.RequiredDeps...)
	items = append(items, plan.SelectedOptional(optional)...)
	selectedIDs := map[string]bool{}
	for _, item := range items {
		selectedIDs[item.Version.ProjectID] = true
	}
	for _, item := range items {
		for _, forbidden := range item.IncompatibleIDs {
			if selectedIDs[forbidden] {
				return fmt.Errorf("%s is incompatible with selected mod %s", item.Version.ProjectID, forbidden)
			}
			for _, rec := range r.Mods {
				if rec.Enabled && rec.ProjectID == forbidden {
					return fmt.Errorf("%s is incompatible with installed mod %s", item.Version.ProjectID, forbidden)
				}
			}
		}
	}
	// The review was made against an earlier snapshot. Recheck the whole selected
	// graph under the instance lock before creating staging files or changing metadata.
	for _, forbidden := range plan.IncompatibleIDs {
		for _, rec := range r.Mods {
			if rec.Enabled && rec.ProjectID == forbidden {
				return fmt.Errorf("installation plan is stale: incompatible mod %s is installed", forbidden)
			}
		}
	}
	for _, item := range items {
		var current *Record
		for i := range r.Mods {
			if r.Mods[i].Enabled && r.Mods[i].ProjectID == item.Version.ProjectID {
				if current != nil {
					return fmt.Errorf("installation plan is stale: duplicate project %s", item.Version.ProjectID)
				}
				current = &r.Mods[i]
			}
		}
		version := ""
		if current != nil {
			version = current.VersionID
		}
		if version != item.InstalledVersion || item.AlreadyInstalled != (current != nil && version == item.Version.ID) {
			return fmt.Errorf("installation plan is stale: %s changed since review", item.Version.ProjectID)
		}
		if item.AlreadyInstalled {
			actual, e := snapshot(filepath.Join(s.GameDir, "mods", current.File))
			if e != nil || actual.SHA256 != current.SHA256 || actual.Mtime != current.Mtime {
				return fmt.Errorf("installation plan is stale: installed dependency %s changed: %v", item.Version.ProjectID, e)
			}
		}
	}
	// Reconcile owners for updated projects; the new plan, not stale metadata, defines required_by.
	for _, item := range items {
		if item.AlreadyInstalled {
			continue
		}
		for i := range r.Mods {
			r.Mods[i].RequiredBy = slices.DeleteFunc(r.Mods[i].RequiredBy, func(id string) bool { return id == item.Version.ProjectID })
		}
	}
	dir := filepath.Join(s.GameDir, "mods")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	type staged struct {
		item modrinth.PlanItem
		temp string
		old  *Record
		seen Record
		rec  Record
	}
	stagedFiles := []staged{}
	defer func() {
		for _, v := range stagedFiles {
			os.Remove(v.temp)
		}
	}()
	used := map[string]bool{}
	var completed int64
	for _, item := range items {
		if item.AlreadyInstalled {
			for i := range r.Mods {
				if r.Mods[i].ProjectID == item.Version.ProjectID && r.Mods[i].VersionID == item.Version.ID {
					for _, id := range item.RequiredBy {
						if selectedIDs[id] && !slices.Contains(r.Mods[i].RequiredBy, id) {
							r.Mods[i].RequiredBy = append(r.Mods[i].RequiredBy, id)
						}
					}
				}
			}
			continue
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		name := item.File.Filename
		if !validName(name) || !slices.Contains([]string{"release", "beta", "alpha"}, item.Version.Channel) {
			return fmt.Errorf("invalid Modrinth filename or channel: %q", name)
		}
		if used[name] {
			return fmt.Errorf("duplicate destination %s", name)
		}
		used[name] = true
		var old *Record
		var seen Record
		for _, v := range r.Mods {
			if v.ProjectID == item.Version.ProjectID && v.ProjectID != "" {
				copy := v
				old = &copy
				break
			}
		}
		if old != nil {
			seen, err = snapshot(filepath.Join(dir, old.File))
			if err != nil {
				return err
			}
			if (seen.SHA256 != old.SHA256 || seen.Mtime != old.Mtime) && !overwriteChanged {
				return fmt.Errorf("local mod %s modified; explicit overwrite confirmation required", old.File)
			}
		}
		if old == nil {
			if err = s.collision(name); err != nil {
				return err
			}
		} else if old.File != name {
			if err = s.collision(name); err != nil {
				return err
			}
		}
		var report modrinth.Progress
		var received int64
		if progress != nil {
			base := completed
			report = func(n, _ int64) {
				received = n
				progress(base+n, base+item.File.Size)
			}
		}
		temp, stageErr := c.Stage(ctx, item.File, dir, report)
		if stageErr != nil {
			return stageErr
		}
		completed += received
		stagedFiles = append(stagedFiles, staged{item: item, temp: temp, old: old, seen: seen})
		if _, err = Inspect(temp); err != nil {
			return fmt.Errorf("invalid JAR %s: %w", name, err)
		}
		rec, scanErr := snapshot(temp)
		if scanErr != nil {
			return scanErr
		}
		v := &stagedFiles[len(stagedFiles)-1]
		rec.File = name
		rec.Enabled = true
		rec.Origin = "modrinth"
		rec.ProjectID = item.Version.ProjectID
		rec.VersionID = item.Version.ID
		rec.Channel = item.Version.Channel
		rec.Hashes = Hashes{SHA512: item.File.Hashes.SHA512, SHA256: rec.SHA256, SHA1: item.File.Hashes.SHA1}
		for _, id := range item.RequiredBy {
			if selectedIDs[id] {
				rec.RequiredBy = append(rec.RequiredBy, id)
			}
		}
		if old != nil {
			for _, id := range old.RequiredBy {
				// Updated parents have new edges in the plan; only retain owners outside this batch.
				updated := false
				for _, planned := range items {
					if planned.Version.ProjectID == id && !planned.AlreadyInstalled {
						updated = true
						break
					}
				}
				if !updated && !slices.Contains(rec.RequiredBy, id) {
					rec.RequiredBy = append(rec.RequiredBy, id)
				}
			}
		}
		rec.InstalledAt = time.Now().UTC().Format(time.RFC3339)
		v.rec = rec
	}
	// External launchers do not hold our lock: never publish over a JAR changed during staging,
	// even when the user approved a modification observed at review time.
	for _, v := range stagedFiles {
		if v.old != nil {
			now, checkErr := snapshot(filepath.Join(dir, v.old.File))
			if checkErr != nil || now.SHA256 != v.seen.SHA256 || now.Mtime != v.seen.Mtime {
				return fmt.Errorf("local mod %s changed during download: %v", v.old.File, checkErr)
			}
		} else if checkErr := s.collision(v.rec.File); checkErr != nil {
			return checkErr
		}
	}
	history, err := s.history()
	if err != nil {
		return err
	}
	pending := installPending{Registry: originalRegistry, History: append([]HistoryEntry(nil), history...)}
	for i, v := range stagedFiles {
		step := installStep{New: v.rec, Old: v.old, Temp: filepath.Base(v.temp)}
		if v.old != nil {
			// The journal and rollback must verify the bytes actually moved, not an
			// older registry hash when overwriting an explicitly approved local edit.
			previous := *v.old
			previous.SHA256, previous.Mtime = v.seen.SHA256, v.seen.Mtime
			previous.Hashes.SHA512 = v.seen.Hashes.SHA512
			if previous.SHA256 != v.old.SHA256 {
				previous.Origin = "external"
				previous.ProjectID, previous.VersionID, previous.Channel = "", "", ""
			}
			step.Old = &previous
			step.Backup = fmt.Sprintf("%s-%d_%s", time.Now().UTC().Format("20060102T150405.000000000"), i, v.old.File)
			if _, statErr := os.Lstat(filepath.Join(s.backupDir(), step.Backup)); !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("backup destination unavailable: %s: %v", step.Backup, statErr)
			}
		}
		pending.Steps = append(pending.Steps, step)
	}
	if len(pending.Steps) > 0 {
		if err = s.savePending(pending); err != nil {
			return err
		}
	}
	restore := func(cause error) error { return errors.Join(cause, s.recoverInstall()) }
	for _, step := range pending.Steps {
		if err = ctx.Err(); err != nil {
			return restore(err)
		}
		if step.Old != nil {
			if err = os.Rename(filepath.Join(dir, step.Old.File), filepath.Join(s.backupDir(), step.Backup)); err != nil {
				return restore(err)
			}
			history = append(history, HistoryEntry{Date: time.Now().UTC().Format(time.RFC3339), ProjectID: step.New.ProjectID, Previous: *step.Old, New: step.New, Backup: step.Backup})
		}
		if err = os.Rename(filepath.Join(dir, step.Temp), filepath.Join(dir, step.New.File)); err != nil {
			return restore(err)
		}
		actual, scanErr := snapshot(filepath.Join(dir, step.New.File))
		if scanErr != nil {
			return restore(scanErr)
		}
		step.New.Mtime = actual.Mtime
		if step.Old != nil {
			put(&r, step.Old.File, nil)
		}
		put(&r, step.New.File, &step.New)
	}
	if err = s.save(r); err != nil {
		if len(pending.Steps) > 0 {
			return fmt.Errorf("registry write failed: %w", restore(err))
		}
		return err
	}
	if len(pending.Steps) > 0 {
		if err = s.saveHistory(history); err != nil {
			return fmt.Errorf("backup history write failed: %w", restore(err))
		}
		if err = os.Remove(s.pendingPath()); err != nil {
			return fmt.Errorf("recovery journal removal failed: %w", restore(err))
		}
		// Commit completed; only now may old backups be pruned.
		if len(history) > 0 {
			if _, err = s.prune(history); err != nil {
				return fmt.Errorf("JARs installed but backup retention failed: %w", err)
			}
		}
	}
	return nil
}
