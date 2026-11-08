package modpacks

import (
	"archive/zip"
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// These are application budgets, not limits imposed by the mrpack format.
const (
	maxArchiveSize = int64(512 << 20)
	maxIndexSize   = int64(8 << 20)
	maxEntries     = 8192
	maxExpanded    = int64(2 << 30)
	maxOverride    = int64(256 << 20)
	maxDownloads   = int64(4 << 30)
	maxPathBytes   = 1024
	maxPathDepth   = 64
)

type Index struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary,omitempty"`
	Files         []File            `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type File struct {
	Path      string       `json:"path"`
	Hashes    Hashes       `json:"hashes"`
	Downloads []string     `json:"downloads"`
	FileSize  int64        `json:"fileSize"`
	Env       *Environment `json:"env,omitempty"`
}

type Hashes struct {
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512"`
}

type Environment struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

type Archive struct {
	Path      string
	SHA512    string
	Index     Index
	Overrides []Override
}

type Override struct {
	Path    string
	ZipName string
	Layer   string
	Size    int64
}

// ValidPath validates a relative game-directory path without cleaning it first.
// The same spelling must be safe on both Unix and Windows filesystems.
func ValidPath(p string) error {
	if p == "" || len(p) > maxPathBytes || !utf8.ValidString(p) || strings.HasPrefix(p, "/") || strings.ContainsAny(p, `\<>:"|?*`) {
		return fmt.Errorf("unsafe pack path %q", p)
	}
	parts := strings.Split(p, "/")
	if len(parts) > maxPathDepth {
		return fmt.Errorf("pack path too deep: %q", p)
	}
	for i, part := range parts {
		if part == "" || part == "." || part == ".." || len(part) > 255 || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return fmt.Errorf("unsafe pack path component in %q", p)
		}
		for _, r := range part {
			if r == 0 || unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
				return fmt.Errorf("unsafe character in pack path %q", p)
			}
		}
		base := strings.ToUpper(strings.TrimRight(strings.SplitN(part, ".", 2)[0], " "))
		digits := []rune(base)
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(digits) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsRune("123456789¹²³", digits[3]) {
			return fmt.Errorf("Windows device name in pack path %q", p)
		}
		folded := cases.Fold().String(part)
		if strings.HasPrefix(folded, ".mctui-") || folded == "mctui-backups" {
			return fmt.Errorf("protected game directory path %q", p)
		}
		if i == 0 && (folded == "instance.json" || folded == "mods.json" || folded == "pack.json" || folded == "mctui_latest.log") {
			return fmt.Errorf("protected metadata path %q", p)
		}
	}
	return nil
}

// Inspect examines the entire ZIP, verifies CRCs and records an archive digest.
// Consumers must verify SHA512 again before reopening Path to extract overrides.
func Inspect(path string) (Archive, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Archive{}, err
	}
	fd, err := os.Open(absolute)
	if err != nil {
		return Archive{}, err
	}
	defer fd.Close()
	info, err := fd.Stat()
	if err != nil {
		return Archive{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxArchiveSize {
		return Archive{}, fmt.Errorf("mrpack must be a regular ZIP of at most %d bytes", maxArchiveSize)
	}
	z, err := zip.NewReader(fd, info.Size())
	if err != nil {
		return Archive{}, fmt.Errorf("invalid mrpack ZIP: %w", err)
	}
	if len(z.File) > maxEntries {
		return Archive{}, fmt.Errorf("mrpack ZIP contains more than %d entries", maxEntries)
	}
	var indexFile *zip.File
	var overrides []Override
	archiveNames := newPathSet()
	installed := newPathSet()
	var expanded int64
	for _, entry := range z.File {
		name := entry.Name
		isDir := strings.HasSuffix(name, "/")
		cleanName := strings.TrimSuffix(name, "/")
		if err := ValidPath(cleanName); err != nil || entry.NonUTF8 || entry.Flags&1 != 0 {
			return Archive{}, fmt.Errorf("unsafe ZIP entry %q: %v (UTF-8 and no encryption required)", name, err)
		}
		if entry.Method != zip.Store && entry.Method != zip.Deflate {
			return Archive{}, fmt.Errorf("unsupported compression in ZIP entry %q", name)
		}
		if isDir && entry.UncompressedSize64 != 0 || isDir != entry.FileInfo().IsDir() || !isDir && !entry.Mode().IsRegular() {
			return Archive{}, fmt.Errorf("special ZIP entry or mismatched directory %q", name)
		}
		if err := archiveNames.add(cleanName, isDir, "zip", true); err != nil {
			return Archive{}, err
		}
		if entry.UncompressedSize64 > uint64(maxExpanded-expanded) {
			return Archive{}, fmt.Errorf("mrpack ZIP expansion exceeds %d bytes at %q", maxExpanded, name)
		}
		expanded += int64(entry.UncompressedSize64)
		if cleanName == "modrinth.index.json" {
			if isDir || entry.UncompressedSize64 > uint64(maxIndexSize) {
				return Archive{}, fmt.Errorf("invalid or oversized modrinth.index.json")
			}
			indexFile = entry
			continue
		}
		parts := strings.SplitN(cleanName, "/", 2)
		layer := parts[0]
		if layer != "overrides" && layer != "client-overrides" && layer != "server-overrides" {
			return Archive{}, fmt.Errorf("unexpected mrpack ZIP entry %q", name)
		}
		if len(parts) == 1 {
			if !isDir {
				return Archive{}, fmt.Errorf("override layer %q must be a directory", name)
			}
			continue
		}
		if !isDir && entry.UncompressedSize64 > uint64(maxOverride) {
			return Archive{}, fmt.Errorf("override %q exceeds %d bytes", name, maxOverride)
		}
		if layer == "server-overrides" {
			continue // Checked as ZIP content but never installed into a client.
		}
		if err := installed.add(parts[1], isDir, layer, true); err != nil {
			return Archive{}, err
		}
		if !isDir {
			overrides = append(overrides, Override{Path: parts[1], ZipName: name, Layer: layer, Size: int64(entry.UncompressedSize64)})
		}
	}
	if indexFile == nil {
		return Archive{}, fmt.Errorf("mrpack ZIP lacks root modrinth.index.json")
	}
	indexReader, err := indexFile.Open()
	if err != nil {
		return Archive{}, fmt.Errorf("open modrinth.index.json: %w", err)
	}
	indexBytes, err := io.ReadAll(io.LimitReader(indexReader, maxIndexSize+1))
	closeErr := indexReader.Close()
	if err != nil || closeErr != nil || len(indexBytes) > int(maxIndexSize) || !utf8.Valid(indexBytes) {
		return Archive{}, fmt.Errorf("invalid, corrupt or oversized UTF-8 modrinth.index.json: read %v, close %v", err, closeErr)
	}
	index, err := parseIndex(indexBytes)
	if err != nil {
		return Archive{}, err
	}
	for _, item := range index.Files {
		if err := installed.add(item.Path, false, "downloads", true); err != nil {
			return Archive{}, err
		}
	}
	// zip.File.Open verifies CRC only when read to EOF, including omitted server files.
	for _, entry := range z.File {
		if entry == indexFile || strings.HasSuffix(entry.Name, "/") {
			continue
		}
		r, err := entry.Open()
		if err != nil {
			return Archive{}, fmt.Errorf("open ZIP entry %q: %w", entry.Name, err)
		}
		n, readErr := io.Copy(io.Discard, io.LimitReader(r, int64(entry.UncompressedSize64)+1))
		closeErr := r.Close()
		if readErr != nil || closeErr != nil || n != int64(entry.UncompressedSize64) {
			return Archive{}, fmt.Errorf("corrupt ZIP entry %q: size %d, read %v, close %v", entry.Name, n, readErr, closeErr)
		}
	}
	if _, err := fd.Seek(0, io.SeekStart); err != nil {
		return Archive{}, err
	}
	h := sha512.New()
	if n, err := io.Copy(h, io.LimitReader(fd, maxArchiveSize+1)); err != nil || n != info.Size() {
		return Archive{}, fmt.Errorf("mrpack changed during inspection: read %d bytes, %v", n, err)
	}
	return Archive{Path: absolute, SHA512: hex.EncodeToString(h.Sum(nil)), Index: index, Overrides: overrides}, nil
}

type wireFile struct {
	Path      string       `json:"path"`
	Hashes    Hashes       `json:"hashes"`
	Downloads []string     `json:"downloads"`
	FileSize  *int64       `json:"fileSize"`
	Env       *Environment `json:"env"`
}

type wireIndex struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary"`
	Files         []wireFile        `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

func parseIndex(data []byte) (Index, error) {
	if err := checkJSONKeys(data); err != nil {
		return Index{}, fmt.Errorf("invalid modrinth.index.json: %w", err)
	}
	var raw wireIndex
	if err := json.Unmarshal(data, &raw); err != nil {
		return Index{}, fmt.Errorf("invalid modrinth.index.json: %w", err)
	}
	if raw.FormatVersion != 1 || raw.Game != "minecraft" {
		return Index{}, fmt.Errorf("unsupported mrpack formatVersion %d or game %q", raw.FormatVersion, raw.Game)
	}
	if !validText(raw.Name, 256, true) || !validText(raw.VersionID, 256, true) || !validText(raw.Summary, 4096, false) || raw.Files == nil || raw.Dependencies == nil {
		return Index{}, fmt.Errorf("invalid mrpack name, versionId, summary, files or dependencies")
	}
	for dependency, version := range raw.Dependencies {
		if dependency != "minecraft" && dependency != "fabric-loader" {
			return Index{}, fmt.Errorf("unsupported mrpack runtime dependency %q", dependency)
		}
		if !validRuntimeVersion(version) {
			return Index{}, fmt.Errorf("invalid exact version for runtime dependency %q: %q", dependency, version)
		}
	}
	if raw.Dependencies["minecraft"] == "" {
		return Index{}, fmt.Errorf("mrpack needs an exact minecraft runtime dependency")
	}
	index := Index{FormatVersion: raw.FormatVersion, Game: raw.Game, VersionID: raw.VersionID, Name: raw.Name, Summary: raw.Summary, Dependencies: raw.Dependencies, Files: make([]File, 0, len(raw.Files))}
	var total int64
	for _, f := range raw.Files {
		if err := ValidPath(f.Path); err != nil {
			return Index{}, fmt.Errorf("download path: %w", err)
		}
		if !validHex(f.Hashes.SHA1, 20) || !validHex(f.Hashes.SHA512, 64) || f.FileSize == nil || *f.FileSize < 0 || *f.FileSize > maxDownloads-total {
			return Index{}, fmt.Errorf("invalid hash, size or download budget for %q", f.Path)
		}
		total += *f.FileSize
		if len(f.Downloads) == 0 || len(f.Downloads) > 16 {
			return Index{}, fmt.Errorf("file %q requires 1 to 16 HTTPS download URLs", f.Path)
		}
		for _, address := range f.Downloads {
			if err := validDownloadURL(address); err != nil {
				return Index{}, fmt.Errorf("file %q: %w", f.Path, err)
			}
		}
		if f.Env != nil && (!validEnvironment(f.Env.Client) || !validEnvironment(f.Env.Server)) {
			return Index{}, fmt.Errorf("file %q has invalid client/server environment", f.Path)
		}
		index.Files = append(index.Files, File{Path: f.Path, Hashes: f.Hashes, Downloads: f.Downloads, FileSize: *f.FileSize, Env: f.Env})
	}
	return index, nil
}

func validRuntimeVersion(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validText(s string, max int, required bool) bool {
	if len(s) > max || required && strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}

func validHex(s string, n int) bool {
	if len(s) != n*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func validEnvironment(s string) bool {
	return s == "required" || s == "optional" || s == "unsupported"
}

func validDownloadURL(raw string) error {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, " \t\r\n\\#") {
		return fmt.Errorf("invalid download URL syntax")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Port() != "" && u.Port() != "443" {
		return fmt.Errorf("download requires an absolute HTTPS URL without credentials")
	}
	// Host authorization and every redirect are checked by the download preflight.
	return nil
}

type pathEntry struct {
	path, layer   string
	dir, explicit bool
}

type pathSet struct {
	entries map[string]pathEntry
	fold    cases.Caser
}

func newPathSet() *pathSet {
	return &pathSet{entries: make(map[string]pathEntry), fold: cases.Fold()}
}

func (s *pathSet) add(p string, dir bool, layer string, explicit bool) error {
	parts := strings.Split(p, "/")
	for i := range parts {
		component := strings.Join(parts[:i+1], "/")
		key := s.fold.String(norm.NFC.String(component))
		isDir := i < len(parts)-1 || dir
		isExplicit := i == len(parts)-1 && explicit
		prior, found := s.entries[key]
		if found {
			if prior.path != component || prior.dir != isDir || !isDir && prior.layer == layer || isDir && prior.explicit && isExplicit && prior.layer == layer {
				return fmt.Errorf("mrpack path collision: %q conflicts with %q", p, prior.path)
			}
			if isExplicit && !prior.explicit {
				prior.explicit = true
				prior.layer = layer
				s.entries[key] = prior
			}
			continue
		}
		s.entries[key] = pathEntry{path: component, layer: layer, dir: isDir, explicit: isExplicit}
	}
	return nil
}

// Reject duplicate JSON keys at every depth, including in extensible objects.
// A second spelling of the same key must not silently change runtime or paths.
func checkJSONKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := jsonValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON content")
	}
	return nil
}

func jsonValue(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	open, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if open != '{' && open != '[' {
		return fmt.Errorf("unexpected JSON delimiter %q", open)
	}
	seen := map[string]bool{}
	for d.More() {
		if open == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("duplicate or invalid JSON key %q", key)
			}
			seen[name] = true
		}
		if err := jsonValue(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
