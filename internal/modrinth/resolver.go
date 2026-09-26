package modrinth

import (
	"context"
	"fmt"
	"slices"
)

type PlanItem struct {
	Version          Version
	File             File
	RequiredBy       []string
	AlreadyInstalled bool
	// InstalledVersion is the version observed at review time; publication must reject stale plans.
	InstalledVersion string
	// IncompatibleIDs declared by this optional branch; enforced only when selected.
	IncompatibleIDs []string
}
type InstallPlan struct {
	TargetMod        Version
	Targets          []PlanItem
	RequiredDeps     []PlanItem
	OptionalDeps     []PlanItem
	IncompatibleMods []string
	IncompatibleIDs  []string
	EmbeddedMods     []string
	AlreadyInstalled []string
	TotalBytes       int64
}

// SelectedOptional includes the mandatory descendants of each chosen optional mod.
func (p InstallPlan) SelectedOptional(chosen []string) []PlanItem {
	selected := map[string]bool{}
	for _, id := range chosen {
		selected[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, item := range p.OptionalDeps {
			if selected[item.Version.ProjectID] {
				continue
			}
			for _, parent := range item.RequiredBy {
				if selected[parent] {
					selected[item.Version.ProjectID] = true
					changed = true
					break
				}
			}
		}
	}
	var items []PlanItem
	for _, item := range p.OptionalDeps {
		if selected[item.Version.ProjectID] {
			items = append(items, item)
		}
	}
	return items
}

// Resolve checks the complete graph before any file is downloaded. installed maps project IDs to version IDs.
func (c *Client) Resolve(ctx context.Context, target Version, loader, minecraft, channel string, installed map[string]string) (InstallPlan, error) {
	return c.ResolveBatch(ctx, []Version{target}, loader, minecraft, channel, installed)
}

// ResolveBatch uses one graph for every selected target, so shared dependencies and conflicts agree.
func (c *Client) ResolveBatch(ctx context.Context, targets []Version, loader, minecraft, channel string, installed map[string]string) (InstallPlan, error) {
	if len(targets) == 0 {
		return InstallPlan{}, fmt.Errorf("empty installation batch")
	}
	p := InstallPlan{TargetMod: targets[0]}
	selected := map[string]string{}
	incompatible := map[string]bool{}
	targetIDs := map[string]bool{}
	var optionalQueue []Dependency
	for _, v := range targets {
		if v.ProjectID == "" || targetIDs[v.ProjectID] {
			return InstallPlan{}, fmt.Errorf("duplicate or missing target project: %s", v.ProjectID)
		}
		targetIDs[v.ProjectID] = true
	}
	var walk func(Version, bool) error
	walk = func(v Version, optional bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if v.ProjectID == "" {
			return fmt.Errorf("version %s has no project ID", v.ID)
		}
		if incompatible[v.ProjectID] {
			return fmt.Errorf("incompatible mod: %s", v.ProjectID)
		}
		if id, ok := selected[v.ProjectID]; ok {
			if id != v.ID {
				return fmt.Errorf("conflicting dependency versions: %s", v.ProjectID)
			}
			return nil
		}
		selected[v.ProjectID] = v.ID
		if !slices.Contains(v.Loaders, loader) || !slices.Contains(v.GameVersions, minecraft) {
			return fmt.Errorf("incompatible version %s", v.ID)
		}
		f, ok := v.PrimaryFile()
		if !ok || f.Hashes.SHA512 == "" {
			return fmt.Errorf("missing verified primary file for %s", v.ProjectID)
		}
		item := PlanItem{Version: v, File: f, AlreadyInstalled: installed[v.ProjectID] == v.ID, InstalledVersion: installed[v.ProjectID]}
		if item.AlreadyInstalled {
			p.AlreadyInstalled = append(p.AlreadyInstalled, v.ProjectID)
		} else {
			p.TotalBytes += f.Size
		}
		if targetIDs[v.ProjectID] {
			p.Targets = append(p.Targets, item)
		} else if optional {
			p.OptionalDeps = append(p.OptionalDeps, item)
		} else {
			p.RequiredDeps = append(p.RequiredDeps, item)
		}
		for _, d := range v.Dependencies {
			id := d.ProjectID
			if id == "" && d.VersionID != "" {
				exact, err := c.Version(ctx, d.VersionID)
				if err != nil {
					return err
				}
				id = exact.ProjectID
			}
			switch d.Type {
			case "embedded":
				p.EmbeddedMods = append(p.EmbeddedMods, id)
				continue
			case "incompatible":
				if id == "" {
					return fmt.Errorf("incompatible dependency missing project ID")
				}
				if optional {
					for i := range p.OptionalDeps {
						if p.OptionalDeps[i].Version.ProjectID == v.ProjectID {
							p.OptionalDeps[i].IncompatibleIDs = append(p.OptionalDeps[i].IncompatibleIDs, id)
							break
						}
					}
				} else {
					incompatible[id] = true
					if selected[id] != "" || installed[id] != "" {
						p.IncompatibleMods = append(p.IncompatibleMods, id)
					}
				}
				continue
			case "optional":
				optionalQueue = append(optionalQueue, d)
				continue
			case "required":
			default:
				return fmt.Errorf("unknown dependency type %q", d.Type)
			}
			if id == "" {
				return fmt.Errorf("dependency missing project ID")
			}
			if incompatible[id] {
				return fmt.Errorf("incompatible mod: %s", id)
			}
			if selected[id] != "" {
				if d.VersionID != "" && selected[id] != d.VersionID {
					return fmt.Errorf("conflicting dependency versions: %s", id)
				}
				if d.Type == "required" {
					addRequiredBy(&p, id, v.ProjectID)
				}
				continue
			}
			var dep Version
			var err error
			if d.VersionID != "" {
				dep, err = c.Version(ctx, d.VersionID)
			} else {
				dep, err = c.CompatibleVersion(ctx, id, loader, minecraft, channel)
			}
			if err != nil {
				if d.Type == "optional" {
					continue
				}
				return fmt.Errorf("required dependency %s: %w", id, err)
			}
			if dep.ProjectID != id {
				return fmt.Errorf("dependency %s returned wrong project", id)
			}
			if err = walk(dep, optional || d.Type == "optional"); err != nil {
				if d.Type == "optional" {
					return fmt.Errorf("optional dependency %s: %w", id, err)
				}
				return fmt.Errorf("required dependency %s: %w", id, err)
			}
			if d.Type == "required" {
				addRequiredBy(&p, id, v.ProjectID)
			}
		}
		return nil
	}
	for _, target := range targets {
		if err := walk(target, false); err != nil {
			return InstallPlan{}, err
		}
	}
	for len(optionalQueue) > 0 {
		d := optionalQueue[0]
		optionalQueue = optionalQueue[1:]
		id := d.ProjectID
		if id == "" && d.VersionID != "" {
			v, err := c.Version(ctx, d.VersionID)
			if err != nil {
				continue
			}
			id = v.ProjectID
		}
		if id == "" || selected[id] != "" || incompatible[id] {
			continue
		}
		var v Version
		var err error
		if d.VersionID != "" {
			v, err = c.Version(ctx, d.VersionID)
		} else {
			v, err = c.CompatibleVersion(ctx, id, loader, minecraft, channel)
		}
		if err == nil && v.ProjectID == id {
			if err = walk(v, true); err != nil {
				return InstallPlan{}, err
			}
		}
	}
	for id := range incompatible {
		p.IncompatibleIDs = append(p.IncompatibleIDs, id)
	}
	if len(p.IncompatibleMods) > 0 {
		return p, fmt.Errorf("incompatible mods: %v", p.IncompatibleMods)
	}
	return p, nil
}

func addRequiredBy(p *InstallPlan, id, parent string) {
	for _, items := range [][]PlanItem{p.Targets, p.RequiredDeps, p.OptionalDeps} {
		for i := range items {
			if items[i].Version.ProjectID == id && !slices.Contains(items[i].RequiredBy, parent) {
				items[i].RequiredBy = append(items[i].RequiredBy, parent)
			}
		}
	}
}
