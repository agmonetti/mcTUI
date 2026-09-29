package modpacks

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"reflect"
	"strings"
	"unicode/utf8"

	"mcTUI/internal/instances"
	"mcTUI/internal/operationlock"
)

// Export writes an offline, client-side mrpack. The selected override paths are
// an explicit assertion that the caller authored those text files and reviewed
// them for private data; the path/content checks below are deliberately
// conservative, not a substitute for that review. Downloaded bytes are never
// embedded: only URLs recorded alongside hashes at installation are eligible.
// A stored URL and a matching hash prove file identity, not redistribution
// rights or that Modrinth will accept a later publication.
func (s Store) Export(ctx context.Context, in instances.Instance, output string, overrides []string) (err error) {
	if err = s.root(); err != nil {
		return err
	}
	if (in.Kind != "managed" && (in.Kind != "legacy" || in.ID != instances.LegacyID)) || !strings.EqualFold(filepath.Ext(output), ".mrpack") {
		return fmt.Errorf("export requires an instance with a pack and .mrpack destination")
	}
	if output, err = filepath.Abs(output); err != nil {
		return err
	}
	if err = exportSafeDir(filepath.Dir(output)); err != nil {
		return err
	}
	if _, e := os.Lstat(output); !os.IsNotExist(e) {
		return fmt.Errorf("export destination already exists or cannot be checked: %s: %v", output, e)
	}
	current, err := instances.Read(s.Root, in.ID)
	if err != nil {
		return err
	}
	gameDir, metadataDir, err := s.installDestination(in)
	if err != nil { return err }
	if output == gameDir || strings.HasPrefix(output, gameDir+string(filepath.Separator)) ||
		output == metadataDir || strings.HasPrefix(output, metadataDir+string(filepath.Separator)) {
		return fmt.Errorf("export destination must be outside the instance")
	}
	if err := checkPackMetadataDir(metadataDir, false); err != nil { return err }
	unlock, err := operationlock.Acquire(gameDir)
	if err != nil {
		return err
	}
	defer func() {
		if unlock != nil {
			err = errors.Join(err, unlock())
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	manifest, err := loadManifest(metadataDir)
	if err != nil {
		return err
	}
	if manifest.MinecraftVersion != current.MinecraftVersion || manifest.Loader != current.Loader || manifest.LoaderVersion != current.LoaderVersion {
		return fmt.Errorf("pack runtime no longer matches the instance")
	}
	selected := make(map[string]bool, len(overrides))
	for _, path := range overrides {
		if err = exportOverridePath(path); err != nil {
			return fmt.Errorf("unexportable override %q: %w", path, err)
		}
		key := strings.ToLower(path)
		if selected[key] {
			return fmt.Errorf("duplicate override: %s", path)
		}
		selected[key] = true
		for downloaded, base := range manifest.Files {
			if strings.EqualFold(downloaded, path) && base.Layer == "downloads" {
				return fmt.Errorf("unexportable borrowed download %s: cannot embed as override", path)
			}
		}
	}
	// Import records optional files declined by the user in Optional but does
	// not retain their download hashes or URLs in Files. Exporting only Files
	// would silently remove those choices from the resulting pack.
	for path := range manifest.Optional {
		if _, installed := manifest.Files[path]; !installed {
			return fmt.Errorf("unexportable optional download %s: not installed; source metadata unavailable", path)
		}
	}
	paths := make([]string, 0, len(manifest.Files))
	for path := range manifest.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	index := Index{FormatVersion: 1, Game: "minecraft", VersionID: manifest.IndexVersionID, Name: manifest.Name,
		Summary: manifest.Summary, Files: make([]File, 0, len(manifest.Files)),
		Dependencies: map[string]string{"minecraft": manifest.MinecraftVersion}}
	if manifest.Loader == "Fabric" {
		index.Dependencies["fabric-loader"] = manifest.LoaderVersion
	}
	for _, path := range paths {
		base := manifest.Files[path]
		if base.Layer != "downloads" {
			continue // Locally installed overrides require deliberate selection.
		}
		if err = exportDownloadPath(path); err != nil {
			return fmt.Errorf("unexportable download %s: %w", path, err)
		}
		if base.SHA1 == "" || len(base.URLs) == 0 {
			return fmt.Errorf("unexportable download %s: missing verified source SHA-1 or URL", path)
		}
		// Keep only direct Modrinth CDN sources. GitHub/GitLab can be
		// installation mirrors, but a URL alone is not permission to
		// redistribute or publish a third-party work.
		// CDN origin is a conservative filter, not a rights attestation.
		var publishURLs []string
		for _, raw := range base.URLs {
			if exportPublicationURL(raw) == nil {
				publishURLs = append(publishURLs, raw)
			}
		}
		if len(publishURLs) == 0 {
			return fmt.Errorf("unexportable download %s: no direct Modrinth CDN source eligible for export", path)
		}
		file, e := exportOpenFile(gameDir, path)
		if e != nil {
			return fmt.Errorf("unexportable download %s: %w", path, e)
		}
		h1, h512, size, e := exportDigest(ctx, file, 512<<20)
		e = errors.Join(e, file.Close())
		if e != nil {
			return fmt.Errorf("unexportable download %s: %w", path, e)
		}
		if h1 != base.SHA1 || h512 != base.SHA512 || size != base.Size {
			return fmt.Errorf("unexportable download %s: installed bytes differ from verified source", path)
		}
		entry := File{Path: path, Hashes: Hashes{SHA1: h1, SHA512: h512}, Downloads: publishURLs, FileSize: size}
		if optional, ok := manifest.Optional[path]; ok && optional {
			entry.Env = &Environment{Client: "optional", Server: "unsupported"}
		}
		index.Files = append(index.Files, entry)
	}
	for _, path := range overrides {
		if err = ctx.Err(); err != nil {
			return err
		}
		file, e := exportOpenFile(gameDir, path)
		if e != nil {
			return fmt.Errorf("unexportable override %s: %w", path, e)
		}
		if e = file.Close(); e != nil {
			return e
		}
	}
	// Build the archive entirely on the destination filesystem. The temp is
	// never visible at the requested path, even if Inspect or cancellation fails.
	tmp, err := os.CreateTemp(filepath.Dir(output), ".mctui-export-*.mrpack")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	overrideProof := make(map[string]exportOverrideProof, len(overrides))
	z := zip.NewWriter(tmp)
	for _, path := range overrides {
		if err = ctx.Err(); err != nil {
			break
		}
		var file *os.File
		file, err = exportOpenFile(gameDir, path)
		if err != nil {
			err = fmt.Errorf("unexportable override %s: %w", path, err)
			break
		}
		var entry io.Writer
		entry, err = z.Create("overrides/" + path)
		if err == nil {
			hash := sha512.New()
			guard := &exportTextGuard{out: io.MultiWriter(entry, hash)}
			var n int64
			n, err = io.Copy(guard, io.LimitReader(&exportContextReader{ctx: ctx, in: file}, 16<<20+1))
			if err == nil {
				err = guard.finish()
			}
			if err == nil && n > 16<<20 {
				err = fmt.Errorf("override exceeds 16 MiB text limit")
			}
			if err == nil {
				overrideProof[path] = exportOverrideProof{size: n, sha512: hex.EncodeToString(hash.Sum(nil))}
			}
		}
		err = errors.Join(err, file.Close())
		if err != nil {
			err = fmt.Errorf("unexportable override %s: %w", path, err)
			break
		}
	}
	if err == nil {
		var entry io.Writer
		entry, err = z.Create("modrinth.index.json")
		if err == nil {
			var data []byte
			data, err = json.Marshal(index)
			if err == nil {
				_, err = entry.Write(data)
			}
		}
	}
	err = errors.Join(err, z.Close())
	if err == nil {
		err = tmp.Sync()
	}
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return err
	}
	var inspected Archive
	inspected, err = Inspect(tmp.Name())
	if err != nil {
		return fmt.Errorf("export round-trip inspection failed: %w", err)
	}
	if !reflect.DeepEqual(inspected.Index, index) || len(inspected.Overrides) != len(overrides) {
		return fmt.Errorf("export round-trip index differs")
	}
	for _, entry := range inspected.Overrides {
		proof, ok := overrideProof[entry.Path]
		if !ok || entry.Layer != "overrides" || entry.ZipName != "overrides/"+entry.Path || proof.size != entry.Size {
			return fmt.Errorf("export round-trip override differs: %s", entry.Path)
		}
	}
	if err = exportVerifyOverrides(ctx, tmp.Name(), inspected.SHA512, overrideProof); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	release := unlock
	unlock = nil
	if err = release(); err != nil {
		return err
	}
	// Link creates the destination atomically without replacing a file created
	// after our first check. Both names are on the same filesystem.
	if err = os.Link(tmp.Name(), output); err != nil {
		return fmt.Errorf("cannot publish export without overwriting destination: %w", err)
	}
	return nil
}

type exportOverrideProof struct {
	size   int64
	sha512 string
}

func exportVerifyOverrides(ctx context.Context, path, expectedSHA512 string, proofs map[string]exportOverrideProof) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	h := sha512.New()
	_, err = io.Copy(h, io.LimitReader(&exportContextReader{ctx: ctx, in: file}, maxArchiveSize+1))
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != expectedSHA512 {
		return fmt.Errorf("mrpack changed after inspection")
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer z.Close()
	seen := make(map[string]bool, len(proofs))
	for _, entry := range z.File {
		if !strings.HasPrefix(entry.Name, "overrides/") {
			continue
		}
		rel := strings.TrimPrefix(entry.Name, "overrides/")
		proof, ok := proofs[rel]
		if !ok || seen[rel] {
			return fmt.Errorf("export round-trip unexpected override: %s", rel)
		}
		seen[rel] = true
		reader, e := entry.Open()
		if e != nil {
			return e
		}
		h := sha512.New()
		n, e := io.Copy(h, reader)
		e = errors.Join(e, reader.Close())
		if e != nil {
			return fmt.Errorf("export round-trip override %s: %w", rel, e)
		}
		if n != proof.size || hex.EncodeToString(h.Sum(nil)) != proof.sha512 {
			return fmt.Errorf("export round-trip override bytes differ: %s", rel)
		}
	}
	if len(seen) != len(proofs) {
		return fmt.Errorf("export round-trip is missing an override")
	}
	return nil
}

func exportSafeDir(dir string) error {
	for path := dir; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe export directory %s: %v", path, err)
		}
		if filepath.Dir(path) == path {
			return nil
		}
	}
}

// Restricted even when the caller explicitly selects the path. No worlds,
// logs, cache, credentials, runtime data or local executables are publishable.
func exportPrivatePath(path string) error {
	if err := ValidPath(path); err != nil {
		return err
	}
	for i, component := range strings.Split(strings.ToLower(path), "/") {
		for _, forbidden := range []string{"secret", "token", "password", "passwd", "credential", "account", "session", "auth", "private", "keystore", "id_rsa", "apikey", "api_key"} {
			if strings.Contains(component, forbidden) {
				return fmt.Errorf("sensitive path")
			}
		}
		switch component {
		case ".env", "instance.json", "pack.json", "mods.json":
			return fmt.Errorf("private instance metadata")
		case "saves", "world", "worlds", "logs", "crash-reports", "backups", "backup", "cache", "caches", "screenshots", "server-resource-packs", "servers.dat", "usercache.json", "launcher_profiles.json":
			return fmt.Errorf("private or generated instance data")
		}
		if i == 0 {
			switch component {
			case "runtime", "versions", "libraries", "assets", "downloads":
				return fmt.Errorf("private or generated instance data")
			}
		}
		if strings.HasPrefix(component, ".mctui") {
			return fmt.Errorf("private instance metadata")
		}
	}
	return nil
}

func exportDownloadPath(path string) error { return exportPrivatePath(path) }

func exportOverridePath(path string) error {
	if err := exportPrivatePath(path); err != nil {
		return err
	}
	lower := strings.ToLower(path)
	if lower == "options.txt" {
		return nil
	}
	parts := strings.Split(lower, "/")
	if len(parts) < 2 || parts[0] != "config" && parts[0] != "defaultconfigs" && parts[0] != "resourcepacks" && parts[0] != "shaderpacks" {
		return fmt.Errorf("only selected config/resource/shader text or options.txt can be embedded")
	}
	switch filepath.Ext(lower) {
	case ".txt", ".json", ".json5", ".toml", ".cfg", ".ini", ".properties", ".yaml", ".yml", ".xml", ".mcmeta", ".lang", ".fsh", ".vsh", ".glsl":
		return nil
	default:
		return fmt.Errorf("binary or executable overrides need independent redistribution permission")
	}
}

func exportPublicationURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.ForceQuery || AllowedURL(raw) != nil {
		return fmt.Errorf("URL not eligible for Modrinth publication")
	}
	// The format permits GitHub/GitLab as well, but this manifest has no
	// per-file permission attestation for those external hosts.
	if u.Host != "cdn.modrinth.com" {
		return fmt.Errorf("URL host lacks verified Modrinth CDN provenance")
	}
	return nil
}

func exportOpenFile(gameDir, rel string) (*os.File, error) {
	if err := ValidPath(rel); err != nil {
		return nil, err
	}
	path := gameDir
	target := filepath.Join(gameDir, filepath.FromSlash(rel))
	for _, component := range strings.Split(rel, "/") {
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlinked file or parent: %s", rel)
		}
		if path != target && !info.IsDir() {
			return nil, fmt.Errorf("non-directory parent: %s", rel)
		}
		if path == target && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file: %s", rel)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	known, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, known) {
		f.Close()
		return nil, fmt.Errorf("file changed while opening: %s", rel)
	}
	return f, nil
}

func exportDigest(ctx context.Context, file *os.File, max int64) (string, string, int64, error) {
	one, five := sha1.New(), sha512.New()
	var n int64
	n, err := io.Copy(io.MultiWriter(one, five), io.LimitReader(&exportContextReader{ctx: ctx, in: file}, max+1))
	if err == nil && n > max {
		err = fmt.Errorf("file exceeds export limit")
	}
	return hex.EncodeToString(one.Sum(nil)), hex.EncodeToString(five.Sum(nil)), n, err
}

type exportContextReader struct {
	ctx context.Context
	in  io.Reader
}

func (r *exportContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.in.Read(p)
}

// Writes through only after rejecting non-text and common credential markers.
// A tiny suffix detects markers spanning two streaming write boundaries.
type exportTextGuard struct {
	out     io.Writer
	tail    [32]byte
	tailLen int
	partial [4]byte
	partialLen int
}

var exportSecretMarkers = [][]byte{
	[]byte("password"), []byte("passwd"), []byte("secret"), []byte("token"),
	[]byte("private_key"), []byte("authorization"), []byte("credential"),
	[]byte("api_key"), []byte("apikey"), []byte("access_key"), []byte("bearer "),
	[]byte("-----begin"),
}

func (g *exportTextGuard) Write(p []byte) (int, error) {
	// Keep the original bytes intact: incomplete UTF-8 is retained only for
	// validation on the next write, not for deferred output.
	if g.partialLen > 0 {
		var prefix [8]byte
		n := copy(prefix[:], g.partial[:g.partialLen])
		n += copy(prefix[n:], p)
		r, size := utf8.DecodeRune(prefix[:n])
		if r == utf8.RuneError && size == 1 {
			if utf8.FullRune(prefix[:n]) {
				return 0, fmt.Errorf("override is not UTF-8 text")
			}
			g.partialLen += copy(g.partial[g.partialLen:], p)
			return g.writeValidated(p, len(p))
		}
		if r == 0 || r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return 0, fmt.Errorf("override contains binary/control data")
		}
		prefixBytes := size - g.partialLen
		g.partialLen = 0
		if prefixBytes > len(p) {
			prefixBytes = len(p)
		}
		return g.writeValidated(p, prefixBytes)
	}
	return g.writeValidated(p, 0)
}

func (g *exportTextGuard) writeValidated(p []byte, start int) (int, error) {
	for i := start; i < len(p); {
		r, size := utf8.DecodeRune(p[i:])
		if r == utf8.RuneError && size == 1 {
			if !utf8.FullRune(p[i:]) {
				g.partialLen = copy(g.partial[:], p[i:])
				break
			}
			return 0, fmt.Errorf("override is not UTF-8 text")
		}
		if r == 0 || r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return 0, fmt.Errorf("override contains binary/control data")
		}
		i += size
	}
	var bridge [64]byte
	m := copy(bridge[:], g.tail[:g.tailLen])
	m += copy(bridge[m:], p)
	for _, needle := range exportSecretMarkers {
		if exportContainsFold(bridge[:m], needle) || exportContainsFold(p, needle) {
			return 0, fmt.Errorf("override contains possible credentials; review and remove them before export")
		}
	}
	if len(p) >= len(g.tail) {
		g.tailLen = copy(g.tail[:], p[len(p)-len(g.tail):])
	} else {
		old := g.tailLen
		if old+len(p) > len(g.tail) {
			copy(g.tail[:], g.tail[old+len(p)-len(g.tail):old])
			old = len(g.tail) - len(p)
		}
		g.tailLen = old + copy(g.tail[old:], p)
	}
	return g.out.Write(p)
}

func (g *exportTextGuard) finish() error {
	if g.partialLen != 0 {
		return fmt.Errorf("override ends with incomplete UTF-8")
	}
	return nil
}

func exportContainsFold(data, needle []byte) bool {
	for len(data) >= len(needle) {
		if bytes.EqualFold(data[:len(needle)], needle) {
			return true
		}
		data = data[1:]
	}
	return false
}

