package mojang

import (
	"archive/zip"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

func nativeClassifier(goos, arch string) string {
	if goos == "darwin" {
		goos = "macos"
	}
	suffix := ""
	switch arch {
	case "amd64":
	case "386":
		suffix = "-x86"
	case "arm64":
		suffix = "-arm64"
	default:
		return ""
	}
	return "natives-" + goos + suffix
}

// NativeLibDir is platform-specific; the old unverified natives directory is
// left untouched and is never used as evidence of a complete extraction.
func NativeLibDir(mcDir, version string) string {
	return filepath.Join(mcDir, "versions", version, "natives-"+runtime.GOOS+"-"+runtime.GOARCH)
}

// DownloadAndExtractNatives revalidates archives even offline and extracts into
// staging before publishing. A nonempty destination is not proof of integrity.
// Publication is per-file, not an atomic multi-file transaction; any error
// blocks launch and a retry re-extracts the whole set.
func DownloadAndExtractNatives(data VersionData, mcDir, version string, wg *sync.WaitGroup, download func(url, destination, expectedSHA1 string) error) (string, error) {
	if osName() == "" || nativeClassifier(runtime.GOOS, runtime.GOARCH) == "" {
		return "", fmt.Errorf("unsupported native platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if !filepath.IsLocal(version) || strings.ContainsAny(version, "/\\:") || version == "." {
		return "", fmt.Errorf("invalid native version %q", version)
	}
	destDir, err := nativePath(mcDir, "versions/"+version+"/natives-"+runtime.GOOS+"-"+runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(destDir), ".mctui-natives-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	seen := make(map[string]bool)
	for _, lib := range data.Libraries {
		if !ShouldIncludeLibrary(lib.Name, lib.Rules) {
			continue
		}
		info := lib.Downloads.Artifact
		if key := lib.Natives[osName()]; key != "" {
			// Old ${arch} classifiers distinguish x86 bitness, not ARM.
			if runtime.GOARCH != "amd64" && runtime.GOARCH != "386" {
				return "", fmt.Errorf("legacy native %s has no verified %s classifier", lib.Name, runtime.GOARCH)
			}
			info = lib.Downloads.Classifiers[strings.ReplaceAll(key, "${arch}", strconv.Itoa(strconv.IntSize))]
		} else if !strings.HasSuffix(lib.Name, ":"+nativeClassifier(runtime.GOOS, runtime.GOARCH)) {
			continue
		}
		hash, hashErr := hex.DecodeString(info.SHA1)
		if info.URL == "" || hashErr != nil || len(hash) != 20 {
			return "", fmt.Errorf("missing or invalid native metadata for %s", lib.Name)
		}
		// Validate before joining, so traversal cannot be cleaned away.
		if !fs.ValidPath(info.Path) || strings.ContainsAny(info.Path, "\\:") || info.Path == "." {
			return "", fmt.Errorf("invalid native library path %q", info.Path)
		}
		jarPath, err := nativePath(mcDir, "libraries/"+info.Path)
		if err != nil {
			return "", err
		}
		// ponytail: serial native extraction avoids colliding writers; parallelize
		// downloads separately only if this small set becomes a measured bottleneck.
		wg.Add(1)
		if err := download(info.URL, jarPath, info.SHA1); err != nil {
			return "", err
		}
		if err := extractJar(jarPath, stage, seen); err != nil {
			return "", fmt.Errorf("extract native %s: %w", lib.Name, err)
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) == 0 {
		return "", err // Metadata with no native libraries needs no java.library.path.
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	for _, entry := range entries {
		dest, err := nativePath(mcDir, "versions/"+version+"/"+filepath.Base(destDir)+"/"+entry.Name())
		if err != nil {
			return "", err
		}
		if err := os.Rename(filepath.Join(stage, entry.Name()), dest); err != nil {
			return "", fmt.Errorf("publish native %s: %w", dest, err)
		}
	}
	return destDir, nil
}

// nativePath rejects linked ancestors and nonportable paths within the runtime.
// Cross-process mutation locking is separate work; these checks do not claim
// protection against an adversary concurrently replacing directories.
func nativePath(root, relative string) (string, error) {
	if !filepath.IsAbs(root) || !fs.ValidPath(relative) || !filepath.IsLocal(relative) || strings.ContainsAny(relative, "\\:") {
		return "", fmt.Errorf("invalid native path %q", relative)
	}
	path := root
	for _, part := range append([]string{""}, strings.Split(relative, "/")...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("linked native path %s", path)
		}
	}
	return path, nil
}

// extractJar reads native files to EOF (including ZIP CRC verification), with
// bounded expansion, and refuses links, traversal and flattened-name collisions.
func extractJar(jarPath, destDir string, seen map[string]bool) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()
	const maxEntries = 4096
	const maxBytes = 256 << 20 // Per native archive, not a generic modpack limit.
	if len(r.File) > maxEntries {
		return fmt.Errorf("native archive exceeds %d entries", maxEntries)
	}
	var expanded uint64
	count := 0
	for _, f := range r.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(name) || !filepath.IsLocal(name) || strings.ContainsAny(name, "\\:") || name == "." {
			return fmt.Errorf("invalid native archive path %q", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return fmt.Errorf("non-regular native entry %q", f.Name)
		}
		if f.FileInfo().IsDir() || strings.HasPrefix(name, "META-INF/") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".so" && ext != ".dll" && ext != ".dylib" && ext != ".jnilib" {
			continue
		}
		if f.UncompressedSize64 > maxBytes-expanded {
			return fmt.Errorf("native archive exceeds %d expanded bytes", maxBytes)
		}
		expanded += f.UncompressedSize64
		base := filepath.Base(name)
		key := strings.ToLower(base)
		if seen[key] {
			return fmt.Errorf("duplicate native filename %q", base)
		}
		seen[key] = true
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(destDir, base), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			in.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, int64(f.UncompressedSize64)+1))
		in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if uint64(n) != f.UncompressedSize64 {
			return fmt.Errorf("incorrect native size for %q", name)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("native archive contains no native libraries")
	}
	return nil
}
