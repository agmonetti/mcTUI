// Package packs manages instance-local resource packs and shader packs without
// changing Minecraft's selected packs or options.txt.
package packs

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"mcTUI/internal/operationlock"
)

// Kind identifies the Minecraft pack directory.
type Kind string

const (
	Resource Kind = "resourcepack"
	Shader   Kind = "shader"

	maxArchive  int64 = 512 << 20
	maxEntry    int64 = 64 << 20
	maxExpanded int64 = 512 << 20
	maxMetadata int64 = 1 << 20
	maxEntries        = 16384
)

type Store struct{ GameDir string }

// Pack describes a local pack. Format is the legacy integer pack_format;
// FormatRange preserves the exact declared compatibility interval, including
// modern major.minor versions. Zero Format with a nonempty FormatRange means
// a versioned pack with no legacy integer format, not an unknown version.
// An invalid or unsafe entry is included with Problem set, never followed.
type Pack struct {
	File             string
	Name             string
	Description      string
	Format           int
	FormatRange      string
	SupportedFormats []int
	Directory        bool
	Problem          string
}

func packDir(kind Kind) (string, error) {
	switch kind {
	case Resource:
		return "resourcepacks", nil
	case Shader:
		return "shaderpacks", nil
	default:
		return "", fmt.Errorf("unknown pack kind %q", kind)
	}
}

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
			return nil
		}
	}
}

func (s Store) directory(kind Kind) (string, error) {
	name, err := packDir(kind)
	if err != nil {
		return "", err
	}
	if err = safePath(s.GameDir); err != nil {
		return "", err
	}
	info, err := os.Lstat(s.GameDir)
	dir := filepath.Join(s.GameDir, name)
	if os.IsNotExist(err) {
		if err = safePath(dir); err != nil {
			return "", err
		}
		return dir, nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("game directory is not a directory: %s", s.GameDir)
	}
	if err = safePath(dir); err != nil {
		return "", err
	}
	if info, err = os.Lstat(dir); err == nil && !info.IsDir() {
		return "", fmt.Errorf("pack directory is not a directory: %s", dir)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return dir, nil
}

func validComponent(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.TrimSpace(name) != name || strings.ContainsAny(name, `/\\:<>"|?*`) {
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
	return true
}

func validFile(name string) bool {
	return validComponent(name) && !strings.HasPrefix(name, ".")
}

func limited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("pack metadata too large")
	}
	return b, nil
}

type packVersion struct {
	major, minor int
	pair         bool
}

func parseVersion(raw json.RawMessage) (packVersion, error) {
	var v packVersion
	var major int64
	if err := json.Unmarshal(raw, &major); err == nil {
		if major < 1 || major > 1<<31-1 {
			return v, fmt.Errorf("invalid pack format number")
		}
		v.major = int(major)
		return v, nil
	}
	var pair []int64
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 || pair[0] < 1 || pair[0] > 1<<31-1 || pair[1] < 0 || pair[1] > 1<<31-1 {
		return v, fmt.Errorf("invalid pack format version")
	}
	return packVersion{major: int(pair[0]), minor: int(pair[1]), pair: true}, nil
}

func (v packVersion) String() string {
	if v.pair {
		return fmt.Sprintf("%d.%d", v.major, v.minor)
	}
	return fmt.Sprint(v.major)
}

func parseSupportedFormats(raw json.RawMessage) (string, []int, error) {
	var major int64
	if json.Unmarshal(raw, &major) == nil && major >= 1 && major <= 1<<31-1 {
		return fmt.Sprint(major), []int{int(major), int(major)}, nil
	}

	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		if len(list) == 2 {
			minV, errMin := parseVersion(list[0])
			maxV, errMax := parseVersion(list[1])
			if errMin == nil && errMax == nil && (maxV.major > minV.major || (maxV.major == minV.major && (!maxV.pair || maxV.minor >= minV.minor))) {
				rangeStr := fmt.Sprintf("%d–%d", minV.major, maxV.major)
				if minV.pair || maxV.pair {
					rangeStr = minV.String() + "–" + maxV.String()
				}
				return rangeStr, []int{minV.major, maxV.major}, nil
			}
		} else if len(list) == 1 {
			v, e := parseVersion(list[0])
			if e == nil {
				return v.String(), []int{v.major, v.major}, nil
			}
		} else {
			var ints []int
			if json.Unmarshal(raw, &ints) == nil && len(ints) > 0 {
				slices.Sort(ints)
				if ints[0] >= 1 {
					return fmt.Sprintf("%d–%d", ints[0], ints[len(ints)-1]), []int{ints[0], ints[len(ints)-1]}, nil
				}
			}
		}
	}

	var obj struct {
		MinInclusive json.RawMessage `json:"min_inclusive"`
		MaxInclusive json.RawMessage `json:"max_inclusive"`
	}
	if json.Unmarshal(raw, &obj) == nil && (len(obj.MinInclusive) > 0 || len(obj.MaxInclusive) > 0) {
		minV := packVersion{major: 1}
		maxV := packVersion{major: 1<<31 - 1}
		if len(obj.MinInclusive) > 0 {
			if v, err := parseVersion(obj.MinInclusive); err == nil {
				minV = v
			}
		}
		if len(obj.MaxInclusive) > 0 {
			if v, err := parseVersion(obj.MaxInclusive); err == nil {
				maxV = v
			}
		}
		if maxV.major > minV.major || (maxV.major == minV.major && (!maxV.pair || maxV.minor >= minV.minor)) {
			rangeStr := fmt.Sprintf("%d–%d", minV.major, maxV.major)
			if minV.pair || maxV.pair {
				rangeStr = minV.String() + "–" + maxV.String()
			}
			return rangeStr, []int{minV.major, maxV.major}, nil
		}
	}

	return "", nil, fmt.Errorf("invalid supported_formats")
}

func parseDescription(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var obj struct {
		Text     string `json:"text"`
		Fallback string `json:"fallback"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		if obj.Fallback != "" {
			return obj.Fallback
		}
		if obj.Text != "" {
			return obj.Text
		}
	}
	var list []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &list) == nil {
		var parts []string
		for _, item := range list {
			if item.Text != "" {
				parts = append(parts, item.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "")
		}
	}
	return ""
}

func parseMetadata(data []byte) (Pack, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return Pack{}, fmt.Errorf("invalid pack.mcmeta: %w", err)
	}
	var p struct {
		PackFormat       json.RawMessage `json:"pack_format"`
		SupportedFormats json.RawMessage `json:"supported_formats"`
		MinFormat        json.RawMessage `json:"min_format"`
		MaxFormat        json.RawMessage `json:"max_format"`
		Description      json.RawMessage `json:"description"`
	}
	if len(obj["pack"]) == 0 || string(obj["pack"]) == "null" || json.Unmarshal(obj["pack"], &p) != nil {
		return Pack{}, fmt.Errorf("invalid pack.mcmeta: missing pack object")
	}
	result := Pack{}
	if len(p.PackFormat) != 0 {
		v, err := parseVersion(p.PackFormat)
		if err != nil || v.pair {
			return Pack{}, fmt.Errorf("invalid pack_format")
		}
		result.Format = v.major
		result.FormatRange = v.String()
	}
	if len(p.SupportedFormats) != 0 {
		rangeStr, formats, err := parseSupportedFormats(p.SupportedFormats)
		if err == nil {
			result.SupportedFormats = formats
			result.FormatRange = rangeStr
		} else if result.FormatRange == "" {
			return Pack{}, fmt.Errorf("invalid supported_formats")
		}
	}
	if len(p.MinFormat) != 0 || len(p.MaxFormat) != 0 {
		if len(p.MinFormat) == 0 || len(p.MaxFormat) == 0 {
			return Pack{}, fmt.Errorf("incomplete pack format range")
		}
		minFormat, minErr := parseVersion(p.MinFormat)
		maxFormat, maxErr := parseVersion(p.MaxFormat)
		if minErr != nil || maxErr != nil || maxFormat.major < minFormat.major ||
			(maxFormat.major == minFormat.major && maxFormat.pair && maxFormat.minor < minFormat.minor) {
			return Pack{}, fmt.Errorf("invalid pack format range")
		}
		result.FormatRange = minFormat.String() + "–" + maxFormat.String()
		if !minFormat.pair && !maxFormat.pair {
			result.SupportedFormats = []int{minFormat.major, maxFormat.major}
		} else {
			result.SupportedFormats = nil
		}
	}
	if result.FormatRange == "" {
		return Pack{}, fmt.Errorf("pack.mcmeta has no format")
	}
	if len(p.Description) != 0 {
		result.Description = parseDescription(p.Description)
	}
	return result, nil
}

// pathSet rejects duplicate paths, case-folded aliases, and files used as parents.
// Explicit directory entries may follow their implicit parent.
type pathSet struct {
	files    map[string]bool
	dirs     map[string]bool
	explicit map[string]bool
	names    map[string]string
}

func newPathSet() pathSet {
	return pathSet{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]string{}}
}

func (s pathSet) add(name string, directory bool) error {
	parts := strings.Split(name, "/")
	for _, component := range parts {
		if !validComponent(component) {
			return fmt.Errorf("unsafe pack path: %q", name)
		}
	}
	for i := 1; i <= len(parts); i++ {
		key := strings.ToLower(strings.Join(parts[:i], "/"))
		spelling := strings.Join(parts[:i], "/")
		if previous, found := s.names[key]; found && previous != spelling {
			return fmt.Errorf("case-conflicting pack path: %q", name)
		}
		s.names[key] = spelling
		if i < len(parts) {
			if s.files[key] {
				return fmt.Errorf("pack path conflicts with a file: %q", name)
			}
			s.dirs[key] = true
		} else if directory {
			if s.files[key] || s.explicit[key] {
				return fmt.Errorf("duplicate pack path: %q", name)
			}
			s.dirs[key], s.explicit[key] = true, true
		} else {
			if s.files[key] || s.dirs[key] {
				return fmt.Errorf("duplicate or conflicting pack path: %q", name)
			}
			s.files[key] = true
		}
	}
	return nil
}

func inspectZIP(filename string, kind Kind) (Pack, error) {
	var result Pack
	info, err := os.Lstat(filename)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArchive {
		return result, fmt.Errorf("pack is not a regular ZIP under %d bytes", maxArchive)
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return result, err
	}
	defer z.Close()
	if len(z.File) > maxEntries {
		return result, fmt.Errorf("too many pack entries")
	}
	seen := newPathSet()
	var metadata []byte
	var total int64
	var assets, shaders bool
	for _, f := range z.File {
		if f.NonUTF8 || f.Name == "" || strings.ContainsAny(f.Name, `\:`) || path.IsAbs(f.Name) || strings.Contains(f.Name, "//") {
			return result, fmt.Errorf("unsafe pack path: %q", f.Name)
		}
		name := strings.TrimSuffix(f.Name, "/")
		mode := f.Mode()
		directory := strings.HasSuffix(f.Name, "/")
		if (directory && !f.FileInfo().IsDir()) || (!directory && !mode.IsRegular()) || mode&os.ModeSymlink != 0 {
			return result, fmt.Errorf("special pack entry: %q", f.Name)
		}
		if len(strings.Split(name, "/")) > 64 {
			return result, fmt.Errorf("pack path too deep: %q", name)
		}
		if err := seen.add(name, directory); err != nil {
			return result, err
		}
		if strings.HasPrefix(strings.ToLower(name), "assets/") && !directory {
			assets = true
		}
		if strings.HasPrefix(strings.ToLower(name), "shaders/") && !directory {
			shaders = true
		}
		limit := maxEntry
		if name == "pack.mcmeta" {
			limit = maxMetadata
		}
		if f.UncompressedSize64 > uint64(limit) || f.UncompressedSize64 > uint64(maxExpanded-total) {
			return result, fmt.Errorf("pack ZIP entry too large: %q", name)
		}
		if f.UncompressedSize64 > 1<<20 && f.UncompressedSize64 > f.CompressedSize64*200 {
			return result, fmt.Errorf("pack ZIP expansion ratio too high: %q", name)
		}
		r, err := f.Open()
		if err != nil {
			return result, err
		}
		var size int64
		if name == "pack.mcmeta" {
			metadata, err = limited(r, limit)
			size = int64(len(metadata))
		} else {
			size, err = io.Copy(io.Discard, io.LimitReader(r, limit+1))
		}
		err = errors.Join(err, r.Close())
		if err != nil {
			return result, fmt.Errorf("invalid ZIP entry %q: %w", name, err)
		}
		if size > limit || size > maxExpanded-total {
			return result, fmt.Errorf("pack ZIP expansion limit exceeded")
		}
		total += size
	}
	if kind == Resource {
		if metadata == nil || !assets {
			return result, fmt.Errorf("resource pack requires pack.mcmeta and assets/")
		}
		return parseMetadata(metadata)
	}
	if !shaders {
		return result, fmt.Errorf("shader pack requires shaders/ with files")
	}
	return result, nil
}

func inspectFolder(folder string, kind Kind) (Pack, error) {
	var result Pack
	seen := newPathSet()
	var metadata []byte
	var total int64
	var assets, shaders bool
	entries := 0
	err := filepath.WalkDir(folder, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == folder {
			return nil
		}
		rel, err := filepath.Rel(folder, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		entries++
		if entries > maxEntries || len(strings.Split(name, "/")) > 64 {
			return fmt.Errorf("pack folder contains too many or too deeply nested entries")
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			return fmt.Errorf("special pack file: %q", name)
		}
		if err = seen.add(name, entry.IsDir()); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		limit := maxEntry
		if name == "pack.mcmeta" {
			limit = maxMetadata
		}
		if info.Size() > limit || info.Size() > maxExpanded-total {
			return fmt.Errorf("pack file too large: %q", name)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			f.Close()
			return fmt.Errorf("pack file changed: %q", name)
		}
		var size int64
		if name == "pack.mcmeta" {
			metadata, err = limited(f, limit)
			size = int64(len(metadata))
		} else {
			size, err = io.Copy(io.Discard, io.LimitReader(f, limit+1))
		}
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		if size > limit || size > maxExpanded-total {
			return fmt.Errorf("pack folder expansion limit exceeded")
		}
		total += size
		if strings.HasPrefix(strings.ToLower(name), "assets/") {
			assets = true
		}
		if strings.HasPrefix(strings.ToLower(name), "shaders/") {
			shaders = true
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if kind == Resource {
		if metadata == nil || !assets {
			return result, fmt.Errorf("resource pack requires pack.mcmeta and assets/")
		}
		return parseMetadata(metadata)
	}
	if !shaders {
		return result, fmt.Errorf("shader pack requires shaders/ with files")
	}
	return result, nil
}

func inspect(p string, kind Kind) (Pack, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return Pack{}, err
	}
	if info.IsDir() {
		result, err := inspectFolder(p, kind)
		result.Directory = true
		return result, err
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(p), ".zip") {
		return Pack{}, fmt.Errorf("pack must be a ZIP or unpacked folder")
	}
	return inspectZIP(p, kind)
}

// List reads only the selected instance. Unsafe or malformed entries appear
// with Problem set so the UI can report why Minecraft cannot use them.
func (s Store) List(kind Kind) ([]Pack, error) {
	dir, err := s.directory(kind)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Pack{}, nil
	}
	if err != nil {
		return nil, err
	}
	packs := make([]Pack, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".mctui-pack-") {
			continue // unpublished temporary files are not packs
		}
		pack := Pack{File: name, Name: name, Directory: entry.IsDir()}
		if !pack.Directory && strings.EqualFold(filepath.Ext(name), ".zip") {
			pack.Name = name[:len(name)-4]
		}
		if !validFile(name) {
			pack.Problem = "unsafe pack name"
		} else {
			metadata, err := inspect(filepath.Join(dir, name), kind)
			if err != nil {
				pack.Problem = err.Error()
			} else {
				pack.Description = metadata.Description
				pack.Format = metadata.Format
				pack.FormatRange = metadata.FormatRange
				pack.SupportedFormats = metadata.SupportedFormats
			}
		}
		packs = append(packs, pack)
	}
	slices.SortFunc(packs, func(a, b Pack) int { return strings.Compare(strings.ToLower(a.File), strings.ToLower(b.File)) })
	return packs, nil
}

func (s Store) lockedDir(kind Kind) (string, func() error, error) {
	dir, err := s.directory(kind)
	if err != nil {
		return "", nil, err
	}
	if err = os.Mkdir(s.GameDir, 0700); err != nil && !os.IsExist(err) {
		return "", nil, err
	}
	if err = safePath(s.GameDir); err != nil {
		return "", nil, err
	}
	release, err := operationlock.Acquire(s.GameDir)
	if err != nil {
		return "", nil, err
	}
	if err = os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		release()
		return "", nil, err
	}
	if err = safePath(dir); err != nil {
		release()
		return "", nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		release()
		return "", nil, fmt.Errorf("invalid pack directory: %s", dir)
	}
	return dir, release, nil
}

// publishZIP copies into a private file in the instance and atomically creates
// the destination via a hard link. No rename can replace an existing pack.
func publishZIP(source, destination, dir string, kind Kind) error {
	f, err := os.CreateTemp(dir, ".mctui-pack-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		f.Close()
		return err
	}
	info, statErr := os.Lstat(source)
	opened, openedErr := in.Stat()
	if statErr != nil || openedErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, opened) || info.Size() > maxArchive {
		in.Close()
		f.Close()
		return fmt.Errorf("source changed or is not a regular ZIP")
	}
	_, err = io.Copy(f, io.LimitReader(in, maxArchive+1))
	err = errors.Join(err, in.Close())
	if err == nil {
		info, err = f.Stat()
		if err == nil && info.Size() > maxArchive {
			err = fmt.Errorf("pack ZIP too large")
		}
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if _, err = inspectZIP(f.Name(), kind); err != nil {
		return err
	}
	if err = os.Link(f.Name(), destination); err != nil {
		return fmt.Errorf("cannot publish pack (already exists or invalid destination): %w", err)
	}
	return nil
}

func copyFolder(source, target string) error {
	return filepath.WalkDir(source, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == source {
			return nil
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in source pack: %s", p)
		}
		if entry.IsDir() {
			return os.Mkdir(destination, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("special file in source pack: %s", p)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		original, originalErr := os.Lstat(p)
		opened, openedErr := in.Stat()
		if originalErr != nil || openedErr != nil || !original.Mode().IsRegular() || !os.SameFile(original, opened) || opened.Size() > maxEntry {
			in.Close()
			return fmt.Errorf("source pack changed: %s", p)
		}
		out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			in.Close()
			return err
		}
		n, err := io.Copy(out, io.LimitReader(in, maxEntry+1))
		err = errors.Join(err, in.Close(), out.Close())
		if n > maxEntry {
			return fmt.Errorf("source pack file too large")
		}
		return err
	})
}

func (s Store) add(kind Kind, source, filename string, zipOnly bool) (err error) {
	if !validFile(filename) || (zipOnly && !strings.EqualFold(filepath.Ext(filename), ".zip")) {
		return fmt.Errorf("unsafe pack filename: %q", filename)
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	if err = safePath(absolute); err != nil {
		return err
	}
	dir, release, err := s.lockedDir(kind)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), filename) {
			return fmt.Errorf("pack already exists: %s", entry.Name())
		}
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if zipOnly && !info.Mode().IsRegular() {
		return fmt.Errorf("downloaded pack must be a regular ZIP")
	}
	if info.IsDir() {
		if _, err = inspectFolder(absolute, kind); err != nil {
			return err
		}
		target := filepath.Join(dir, filename)
		if err = os.Mkdir(target, 0700); err != nil {
			return err
		}
		// Remove only the directory created by this operation if copying fails.
		if err = copyFolder(absolute, target); err != nil {
			return errors.Join(err, os.RemoveAll(target))
		}
		if _, err = inspectFolder(target, kind); err != nil {
			return errors.Join(err, os.RemoveAll(target))
		}
		return nil
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(filename), ".zip") {
		return fmt.Errorf("pack must be a ZIP or unpacked folder")
	}
	return publishZIP(absolute, filepath.Join(dir, filename), dir, kind)
}

// Add imports a local ZIP or unpacked pack into the selected instance.
func (s Store) Add(kind Kind, source string) error {
	return s.add(kind, source, filepath.Base(source), false)
}

// Install publishes a downloaded, SHA-512 verified staged ZIP. The caller owns
// and removes source (even on failure); this method never overwrites a pack.
func (s Store) Install(kind Kind, source, filename string) error {
	return s.add(kind, source, filename, true)
}

// Remove removes only the requested local pack. UI must obtain confirmation.
func (s Store) Remove(kind Kind, file string) (err error) {
	if !validFile(file) {
		return fmt.Errorf("unsafe pack filename: %q", file)
	}
	dir, release, err := s.lockedDir(kind)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	target := filepath.Join(dir, file)
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		// Malformed packs may still be removed. Refuse recursive removal if
		// the tree contains symlinks or special files.
		err = filepath.WalkDir(target, func(p string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
				return fmt.Errorf("unsafe file in unpacked pack: %s", p)
			}
			return nil
		})
		if err != nil {
			return err
		}
		current, err := os.Lstat(target)
		if err != nil || !os.SameFile(info, current) {
			return fmt.Errorf("pack changed before removal: %s", file)
		}
		return os.RemoveAll(target)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsafe pack entry: %s", file)
	}
	return os.Remove(target)
}
