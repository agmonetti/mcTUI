package modpacks

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"mcTUI/internal/instances"
	"mcTUI/internal/launcher"
)

// Store manages modpacks under the same data root as managed instances.
// Prepare is injectable for offline integration tests; nil prepares the real runtime.
type Store struct {
	Root    string
	HTTP    *http.Client
	Prepare func(context.Context, string, string, string, string) error
}

func (s Store) prepare(ctx context.Context, in instances.Instance) error {
	if s.Prepare != nil {
		return s.Prepare(ctx, filepath.Join(s.Root, "runtime"), in.MinecraftVersion, in.Loader, in.LoaderVersion)
	}
	_, err := launcher.PrepareWithLoaderContext(ctx, filepath.Join(s.Root, "runtime"), in.MinecraftVersion, in.Loader, in.LoaderVersion, nil)
	return err
}

func (s Store) root() error {
	if !filepath.IsAbs(s.Root) || filepath.Clean(s.Root) != s.Root {
		return fmt.Errorf("invalid modpack data root")
	}
	for p := s.Root; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinked modpack data root: %s", p)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(p) == p {
			return nil
		}
	}
}

func validInstanceName(name string) bool {
	if strings.TrimSpace(name) == "" || len(name) > 80 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func runtime(index Index) (version, loader, loaderVersion string, err error) {
	version = index.Dependencies["minecraft"]
	loaderVersion = index.Dependencies["fabric-loader"]
	if version == "" {
		return "", "", "", fmt.Errorf("pack is missing Minecraft version")
	}
	if loaderVersion != "" {
		return version, "Fabric", loaderVersion, nil
	}
	return version, "Vanilla", "", nil
}

func (s Store) Inspect(path string) (Archive, error) {
	return Inspect(path)
}

// Import publishes a managed instance only after all files and metadata are complete.
// Optional files are selected by exact index path. No selected file is silently skipped.
func (s Store) Import(ctx context.Context, archive Archive, source Source, name string, memoryMB int, optional map[string]bool, progress func(done, total int64)) (in instances.Instance, err error) {
	if err = s.root(); err != nil {
		return in, err
	}
	if !validInstanceName(name) || memoryMB <= 0 || source.Type != "local" && source.Type != "url" && source.Type != "modrinth" ||
		source.Type == "modrinth" && (source.ProjectID == "" || source.VersionID == "") {
		return in, fmt.Errorf("invalid pack instance settings or source")
	}
	current, err := Inspect(archive.Path)
	if err != nil {
		return in, err
	}
	if current.SHA512 != archive.SHA512 {
		return in, fmt.Errorf("pack archive changed after review")
	}
	archive = current
	version, loader, loaderVersion, err := runtime(archive.Index)
	if err != nil {
		return in, err
	}
	if err = os.MkdirAll(s.Root, 0700); err != nil {
		return in, err
	}
	if err = s.root(); err != nil {
		return in, err
	}
	instancesDir := filepath.Join(s.Root, "instances")
	if err = os.MkdirAll(instancesDir, 0700); err != nil {
		return in, err
	}
	if info, e := os.Lstat(instancesDir); e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return in, fmt.Errorf("unsafe instances directory: %w", e)
	}
	stage, err := os.MkdirTemp(s.Root, ".mctui-modpack-*")
	if err != nil {
		return in, err
	}
	defer func() {
		if stage != "" {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	var randomID [16]byte
	if _, err = rand.Read(randomID[:]); err != nil {
		return in, err
	}
	in = instances.Instance{SchemaVersion: 1, ID: hex.EncodeToString(randomID[:]), Kind: "managed", Name: strings.TrimSpace(name), MinecraftVersion: version, Loader: loader, LoaderVersion: loaderVersion, MemoryMB: memoryMB}
	gameDir := filepath.Join(stage, "minecraft")
	if err = os.Mkdir(gameDir, 0700); err != nil {
		return in, err
	}
	manifest := Manifest{SchemaVersion: 1, Source: source, Name: archive.Index.Name, Summary: archive.Index.Summary,
		IndexVersionID: archive.Index.VersionID, IndexSHA512: archive.SHA512,
		MinecraftVersion: version, Loader: loader, LoaderVersion: loaderVersion, Files: map[string]BaseFile{}, Optional: map[string]bool{}}
	var total int64
	for _, file := range archive.Index.Files {
		if file.Env != nil && file.Env.Client == "unsupported" {
			continue
		}
		if file.Env != nil && file.Env.Client == "optional" {
			manifest.Optional[file.Path] = optional[file.Path]
			if !optional[file.Path] {
				continue
			}
		}
		total += file.FileSize
		if total > 4<<30 {
			return in, fmt.Errorf("pack download budget exceeded")
		}
		for _, url := range file.Downloads {
			if err = AllowedURL(url); err != nil {
				return in, fmt.Errorf("unsafe URL for %s: %w", file.Path, err)
			}
		}
	}
	var done int64
	if progress != nil {
		progress(0, total)
	}
	for _, file := range archive.Index.Files {
		if err = ctx.Err(); err != nil {
			return in, err
		}
		if file.Env != nil && (file.Env.Client == "unsupported" || file.Env.Client == "optional" && !optional[file.Path]) {
			continue
		}
		maxFile := int64(512 << 20)
		if strings.HasSuffix(strings.ToLower(file.Path), ".jar") {
			maxFile = 250 << 20
		}
		if file.FileSize > maxFile {
			return in, fmt.Errorf("pack file too large: %s", file.Path)
		}
		tmp, e := Fetch(ctx, s.HTTP, file.Downloads, file.Hashes, file.FileSize, maxFile, stage)
		if e != nil {
			return in, fmt.Errorf("download %s: %w", file.Path, e)
		}
		dest, e := destination(gameDir, file.Path)
		if e == nil {
			e = os.Rename(tmp, dest)
		}
		if e != nil {
			os.Remove(tmp)
			return in, e
		}
		manifest.Files[file.Path] = BaseFile{SHA512: file.Hashes.SHA512, SHA1: file.Hashes.SHA1, Size: file.FileSize, Layer: "downloads", URLs: file.Downloads}
		done += file.FileSize
		if progress != nil {
			progress(done, total)
		}
	}
	if err = applyOverrides(ctx, archive, gameDir, manifest.Files); err != nil {
		return in, err
	}
	if err = s.prepare(ctx, in); err != nil {
		return in, err
	}
	if err = ctx.Err(); err != nil {
		return in, err
	}
	if err = saveJSON(stage, "instance.json", in); err != nil {
		return in, err
	}
	if err = saveManifest(stage, manifest); err != nil {
		return in, err
	}
	final := filepath.Join(instancesDir, in.ID)
	if _, e := os.Lstat(final); !os.IsNotExist(e) {
		return in, fmt.Errorf("instance destination exists or is unreadable: %s", final)
	}
	if err = os.Rename(stage, final); err != nil {
		return in, err
	}
	stage = ""
	return in, nil
}

func destination(gameDir, rel string) (string, error) {
	if err := ValidPath(rel); err != nil {
		return "", err
	}
	p := filepath.Join(gameDir, filepath.FromSlash(rel))
	for dir := filepath.Dir(p); dir != gameDir; dir = filepath.Dir(dir) {
		if !strings.HasPrefix(dir, gameDir+string(filepath.Separator)) {
			return "", fmt.Errorf("path outside instance")
		}
		if info, err := os.Lstat(dir); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return "", fmt.Errorf("unsafe parent directory: %s", dir)
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(p); err == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("unsafe destination: %s", p)
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return p, nil
}

func applyOverrides(ctx context.Context, archive Archive, gameDir string, files map[string]BaseFile) error {
	z, err := zip.OpenReader(archive.Path)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, layer := range []string{"overrides", "client-overrides"} {
		for _, override := range archive.Overrides {
			if override.Layer != layer {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			var entry *zip.File
			for _, f := range z.File {
				if f.Name == override.ZipName {
					entry = f
					break
				}
			}
			if entry == nil || int64(entry.UncompressedSize64) != override.Size {
				return fmt.Errorf("override changed: %s", override.Path)
			}
			path, err := destination(gameDir, override.Path)
			if err != nil {
				return err
			}
			r, err := entry.Open()
			if err != nil {
				return err
			}
			tmp, err := os.CreateTemp(filepath.Dir(path), ".pack-*")
			if err != nil {
				r.Close()
				return err
			}
			sha := sha512.New()
			one := sha1.New()
			n, cpErr := io.Copy(io.MultiWriter(tmp, sha, one), io.LimitReader(r, override.Size+1))
			cpErr = errors.Join(cpErr, r.Close())
			if cpErr == nil && n != override.Size {
				cpErr = fmt.Errorf("override size changed: %s", override.Path)
			}
			if cpErr == nil {
				cpErr = tmp.Sync()
			}
			cpErr = errors.Join(cpErr, tmp.Close())
			if cpErr == nil {
				cpErr = os.Rename(tmp.Name(), path)
			}
			os.Remove(tmp.Name())
			if cpErr != nil {
				return cpErr
			}
			files[override.Path] = BaseFile{SHA512: hex.EncodeToString(sha.Sum(nil)), SHA1: hex.EncodeToString(one.Sum(nil)), Size: n, Layer: layer}
		}
	}
	return nil
}
