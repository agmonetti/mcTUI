package mods

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"mcTUI/internal/modrinth"
)

// Identify links local JARs only when Modrinth confirms their current SHA-512.
// Network I/O happens outside the instance lock; every candidate is rechecked before saving.
func (s Store) Identify(ctx context.Context, client interface {
	Identify(context.Context, []string) (map[string]modrinth.Version, error)
}) error {
	items, err := s.Scan()
	if err != nil {
		return err
	}
	candidates := map[string]Mod{}
	hashes := []string{}
	for _, item := range items {
		if item.Problem != "" || item.Changed || item.ProjectID != "" || item.Hashes.SHA512 == "" {
			continue
		}
		candidates[item.File] = item
		hashes = append(hashes, item.Hashes.SHA512)
	}
	if len(hashes) == 0 {
		return nil
	}
	versions, err := client.Identify(ctx, hashes)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	release, err := s.acquire()
	if err != nil {
		return err
	}
	defer release()
	r, err := s.load()
	if err != nil {
		return err
	}
	changed := false
	for name, candidate := range candidates {
		v, ok := versions[candidate.Hashes.SHA512]
		if !ok {
			continue
		}
		if v.ID == "" || v.ProjectID == "" || !slices.Contains([]string{"release", "beta", "alpha"}, v.Channel) || !slices.ContainsFunc(v.Files, func(f modrinth.File) bool {
			return strings.EqualFold(f.Hashes.SHA512, candidate.Hashes.SHA512)
		}) {
			return fmt.Errorf("invalid Modrinth identification for %s", name)
		}
		current, e := snapshot(filepath.Join(s.GameDir, "mods", name))
		if e != nil || current.SHA256 != candidate.SHA256 || current.Hashes.SHA512 != candidate.Hashes.SHA512 || current.Mtime != candidate.Mtime {
			return fmt.Errorf("mod %s changed during identification; metadata preserved", name)
		}
		index := -1
		for i, known := range r.Mods {
			if known.File == name {
				index = i
				if known.ProjectID != "" || known.SHA256 != candidate.SHA256 || known.Mtime != candidate.Mtime {
					return fmt.Errorf("mod %s registry changed during identification", name)
				}
				break
			}
		}
		if candidate.Managed && index < 0 || !candidate.Managed && index >= 0 {
			return fmt.Errorf("mod %s registry changed during identification", name)
		}
		if index >= 0 {
			current = r.Mods[index]
		}
		current.ProjectID, current.VersionID, current.Channel = v.ProjectID, v.ID, v.Channel
		current.Hashes.SHA512 = candidate.Hashes.SHA512
		put(&r, name, &current)
		changed = true
	}
	if changed {
		return s.save(r)
	}
	return nil
}
