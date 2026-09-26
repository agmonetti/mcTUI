// Package mods manages local JARs only; it never executes or downloads code.
package mods

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"mcTUI/internal/operationlock"
)

const (
	maxJar      = 512 << 20
	maxEntry    = 64 << 20
	maxExpanded = 512 << 20
	maxMetadata = 1 << 20
	maxRegistry = 8 << 20
)

type Metadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
}
type Record struct {
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
	Enabled bool   `json:"enabled"`
	Origin  string `json:"origin"`
	Mtime   int64  `json:"mtime_ns"`
}
type Mod struct {
	Record
	Metadata                    Metadata
	Managed, Changed, Duplicate bool
	Problem                     string
}
type registry struct {
	SchemaVersion int      `json:"schema_version"`
	Mods          []Record `json:"mods"`
}

// Store paths are captured from the selected instance, not resolved by workers.
type Store struct{ GameDir, Registry string }

func validName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\\:<>"|?*`) || strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	stem := strings.ToUpper(strings.Split(name, ".")[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
		return false
	}
	return strings.HasSuffix(name, ".jar") || strings.HasSuffix(name, ".jar.disabled")
}

// Check all existing ancestors, including junction-like symlinks, before using a path.
func safePath(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return fmt.Errorf("unsafe path: %q", p)
	}
	for cur := p; ; cur = filepath.Dir(cur) {
		info, err := os.Lstat(cur)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink rejected: %s", cur)
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}
func (s Store) validate() error {
	if err := safePath(s.GameDir); err != nil {
		return err
	}
	if err := safePath(s.Registry); err != nil {
		return err
	}
	return safePath(filepath.Join(s.GameDir, "mods"))
}
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("size limit exceeded")
	}
	return b, nil
}
func (s Store) load() (registry, error) {
	r := registry{SchemaVersion: 1}
	info, err := os.Lstat(s.Registry)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if !info.Mode().IsRegular() {
		return r, fmt.Errorf("registry is not regular")
	}
	f, err := os.Open(s.Registry)
	if err != nil {
		return r, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return r, err
	}
	if !os.SameFile(info, opened) {
		return r, fmt.Errorf("registry changed while opening")
	}
	b, err := readLimited(f, maxRegistry)
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.SchemaVersion != 1 {
		return r, fmt.Errorf("unsupported mods registry")
	}
	seen := map[string]bool{}
	for _, v := range r.Mods {
		hash, e := hex.DecodeString(v.SHA256)
		key := strings.ToLower(v.File)
		if !validName(v.File) || e != nil || len(hash) != 32 || v.Enabled != strings.HasSuffix(v.File, ".jar") || v.Origin == "" || seen[key] {
			return r, fmt.Errorf("invalid mods registry")
		}
		seen[key] = true
	}
	return r, nil
}
func (s Store) save(r registry) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if len(b) > maxRegistry {
		return fmt.Errorf("registry size limit exceeded")
	}
	if err = safePath(s.Registry); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.Registry), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.Registry), ".mods-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), s.Registry)
}

// Inspect drains each ZIP entry to EOF (CRC verified by archive/zip). Nothing is extracted.
func Inspect(filename string) (Metadata, error) {
	var meta Metadata
	info, err := os.Lstat(filename)
	if err != nil {
		return meta, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxJar {
		return meta, fmt.Errorf("non-regular or oversized JAR")
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return meta, err
	}
	defer z.Close()
	if len(z.File) > 16384 {
		return meta, fmt.Errorf("too many ZIP entries")
	}
	var total int64
	var metadata []byte
	seen := map[string]bool{}
	for _, f := range z.File {
		n := strings.TrimSuffix(f.Name, "/")
		if n == "" || strings.ContainsAny(n, `\\:`) || path.IsAbs(n) || path.Clean(n) != n || n == ".." || strings.HasPrefix(n, "../") || len(strings.Split(n, "/")) > 64 || seen[n] {
			return meta, fmt.Errorf("unsafe ZIP path: %q", f.Name)
		}
		seen[n] = true
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return meta, fmt.Errorf("special ZIP entry")
		}
		limit := int64(maxEntry)
		if f.Name == "fabric.mod.json" {
			limit = maxMetadata
		}
		if f.UncompressedSize64 > uint64(limit) {
			return meta, fmt.Errorf("ZIP entry too large")
		}
		limit = min(limit, int64(maxExpanded)-total)
		r, e := f.Open()
		if e != nil {
			return meta, e
		}
		var size int64
		if f.Name == "fabric.mod.json" {
			metadata, e = readLimited(r, limit)
			size = int64(len(metadata))
		} else {
			size, e = io.Copy(io.Discard, io.LimitReader(r, limit+1))
		}
		e = errors.Join(e, r.Close())
		if e != nil {
			return meta, e
		}
		if size > limit {
			return meta, fmt.Errorf("ZIP entry too large")
		}
		total += size
		if total > maxExpanded {
			return meta, fmt.Errorf("ZIP expanded size exceeded")
		}
	}
	if metadata != nil {
		if err = json.Unmarshal(metadata, &meta); err != nil {
			return Metadata{}, fmt.Errorf("invalid fabric.mod.json: %w", err)
		}
		if meta.ID == "" || meta.Version == "" || (meta.Environment != "" && meta.Environment != "*" && meta.Environment != "client" && meta.Environment != "server") {
			return Metadata{}, fmt.Errorf("invalid Fabric metadata")
		}
	}
	return meta, nil
}
func snapshot(filename string) (Record, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return Record{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxJar {
		return Record{}, fmt.Errorf("non-regular or oversized JAR: %s", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return Record{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return Record{}, err
	}
	if !os.SameFile(info, opened) {
		return Record{}, fmt.Errorf("file changed")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxJar+1))
	if err != nil {
		return Record{}, err
	}
	after, err := os.Lstat(filename)
	if err != nil {
		return Record{}, err
	}
	if n > maxJar || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return Record{}, fmt.Errorf("file changed while reading")
	}
	return Record{File: filepath.Base(filename), SHA256: hex.EncodeToString(h.Sum(nil)), Enabled: strings.HasSuffix(filename, ".jar"), Mtime: info.ModTime().UnixNano(), Origin: "external"}, nil
}
func (s Store) Scan() ([]Mod, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	r, err := s.load()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.GameDir, "mods"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Mod{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jar") && !strings.HasSuffix(name, ".jar.disabled") {
			continue
		}
		m := Mod{Record: Record{File: name, Enabled: strings.HasSuffix(name, ".jar"), Origin: "external"}}
		filename := filepath.Join(s.GameDir, "mods", name)
		rec, e := snapshot(filename)
		if e == nil {
			m.Record = rec
			m.Metadata, e = Inspect(filename)
		}
		if !validName(name) {
			e = fmt.Errorf("unsafe filename")
		}
		if e != nil {
			m.Problem = e.Error()
		}
		for _, known := range r.Mods {
			if known.File == name {
				m.Managed = true
				m.Origin = known.Origin
				m.Changed = known.SHA256 != m.SHA256 || known.Mtime != m.Mtime
				break
			}
		}
		result = append(result, m)
	}
	ids := map[string]int{}
	for _, m := range result {
		if m.Metadata.ID != "" {
			ids[m.Metadata.ID]++
		}
	}
	for i := range result {
		result[i].Duplicate = result[i].Metadata.ID != "" && ids[result[i].Metadata.ID] > 1
	}
	return result, nil
}
func (s Store) acquire() (func() error, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.GameDir, 0700); err != nil {
		return nil, err
	}
	return operationlock.Acquire(s.GameDir)
}

// CheckBusy uses the same nonblocking lock as Play; it does not detect other launchers.
func (s Store) CheckBusy() error {
	release, err := s.acquire()
	if err != nil {
		return err
	}
	return release()
}
func (s Store) collision(name string) error {
	entries, err := os.ReadDir(filepath.Join(s.GameDir, "mods"))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return fmt.Errorf("destination collision: %s", name)
		}
	}
	return nil
}
func put(r *registry, old string, rec *Record) {
	out := r.Mods[:0]
	for _, v := range r.Mods {
		if v.File != old {
			out = append(out, v)
		}
	}
	r.Mods = out
	if rec != nil {
		r.Mods = append(r.Mods, *rec)
	}
}
func (s Store) Add(source string) error {
	source = filepath.Clean(source)
	if !filepath.IsAbs(source) || !validName(filepath.Base(source)) || !strings.HasSuffix(source, ".jar") {
		return fmt.Errorf("select a local .jar file")
	}
	if err := safePath(source); err != nil {
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
	before, err := snapshot(source)
	if err != nil {
		return err
	}
	dir := filepath.Join(s.GameDir, "mods")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	name := filepath.Base(source)
	if err = s.collision(name); err != nil {
		return err
	}
	if err = s.collision(name + ".disabled"); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, ".import-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(in, maxJar+1))
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	if n > maxJar {
		return fmt.Errorf("JAR too large")
	}
	rec, err := snapshot(tmp.Name())
	if err != nil {
		return err
	}
	after, err := snapshot(source)
	if err != nil {
		return err
	}
	if rec.SHA256 != before.SHA256 || after.SHA256 != before.SHA256 || after.Mtime != before.Mtime {
		return fmt.Errorf("source changed")
	}
	if _, err = Inspect(tmp.Name()); err != nil {
		return err
	}
	// Reserve exclusively: unlike Rename on Unix, this never overwrites an external file.
	dest := filepath.Join(dir, name)
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	staged, err := os.Open(tmp.Name())
	if err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	_, err = io.Copy(out, staged)
	err = errors.Join(err, staged.Close(), out.Sync(), out.Close())
	if err != nil {
		cleanup := os.Remove(dest)
		return errors.Join(err, cleanup)
	}
	rec, err = snapshot(dest)
	if err != nil {
		return err
	}
	rec.Origin = "local"
	put(&r, name, &rec)
	if err = s.save(r); err != nil {
		return fmt.Errorf("file added at %s but registry failed; refresh: %w", dest, err)
	}
	return nil
}

// mutate always compares the UI snapshot and managed baseline under the game lock.
func (s Store) mutate(expected Mod, remove, confirmed bool) error {
	if remove && !confirmed {
		return fmt.Errorf("explicit removal confirmation required")
	}
	if !validName(expected.File) {
		return fmt.Errorf("unsafe filename")
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
	filename := filepath.Join(s.GameDir, "mods", expected.File)
	now, err := snapshot(filename)
	if err != nil {
		return err
	}
	if now.SHA256 != expected.SHA256 || now.Mtime != expected.Mtime || expected.Changed {
		return fmt.Errorf("external change detected; file preserved")
	}
	for _, known := range r.Mods {
		if known.File == now.File && (known.SHA256 != now.SHA256 || known.Mtime != now.Mtime) {
			return fmt.Errorf("managed file changed externally; file preserved")
		}
	}
	if remove {
		if err = os.Remove(filename); err != nil {
			return err
		}
		put(&r, expected.File, nil)
	} else {
		name := strings.TrimSuffix(expected.File, ".disabled")
		if now.Enabled {
			name += ".disabled"
		}
		if err = s.collision(name); err != nil {
			return err
		}
		// Reserve destination before rename to prevent overwriting pre-existing files.
		dest := filepath.Join(s.GameDir, "mods", name)
		reservation, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		if e = reservation.Close(); e != nil {
			os.Remove(dest)
			return e
		}
		if err = os.Rename(filename, dest); err != nil {
			return errors.Join(err, os.Remove(dest))
		}
		now.File = name
		now.Enabled = !now.Enabled
		now.Origin = expected.Origin
		if now.Origin == "" {
			now.Origin = "external"
		}
		put(&r, expected.File, &now)
	}
	if err = s.save(r); err != nil {
		return fmt.Errorf("file operation completed in %s but registry failed; refresh: %w", filepath.Dir(filename), err)
	}
	return nil
}
func (s Store) Toggle(expected Mod) error                 { return s.mutate(expected, false, false) }
func (s Store) Remove(expected Mod, confirmed bool) error { return s.mutate(expected, true, confirmed) }
