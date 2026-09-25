// Package mojang handles communication with Mojang's official metadata
// and download servers: fetching the version manifest, per-version
// metadata (libraries, asset index, Java requirement, client download
// URL), and downloading files with published SHA-1 verification.
package mojang

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const manifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

var metadataClient = &http.Client{Timeout: 20 * time.Second}

func readMetadata(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := metadataClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 8<<20 {
		return nil, fmt.Errorf("metadata %s exceeds 8 MiB", url)
	}
	return body, nil
}

// manifestEntry is one entry in the version manifest: an ID and a URL to
// fetch that version's full metadata.
type manifestEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
}

// VersionData is the subset of a version's metadata (from its per-version
// JSON) that mcTUI needs: the asset index, required Java version, client
// download info, and library list.
type VersionData struct {
	AssetIndex struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		SHA1 string `json:"sha1"`
	} `json:"assetIndex"`
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
	Downloads struct {
		Client struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"client"`
	} `json:"downloads"`
	Libraries []struct {
		Name      string `json:"name"`
		Downloads struct {
			Artifact struct {
				Path string `json:"path"`
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"artifact"`
			Classifiers map[string]struct {
				Path string `json:"path"`
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"classifiers"`
		} `json:"downloads"`
		Natives map[string]string `json:"natives"`
		Rules   []struct {
			Action string `json:"action"`
			OS     struct {
				Name string `json:"name"`
			} `json:"os"`
		} `json:"rules"`
	} `json:"libraries"`
}

// FetchReleases returns the list of "release" type version IDs from
// Mojang's manifest, or {"1.20.4"} if the manifest can't be fetched or
// parsed.
func FetchReleases() []string {
	body, err := readMetadata(context.Background(), manifestURL)
	if err != nil {
		return []string{"1.20.4"}
	}

	var manifest struct {
		Versions []manifestEntry `json:"versions"`
	}
	json.Unmarshal(body, &manifest)

	var releases []string
	for _, v := range manifest.Versions {
		if v.Type == "release" {
			releases = append(releases, v.ID)
		}
	}
	if len(releases) == 0 {
		return []string{"1.20.4"}
	}
	return releases
}

// FetchVersionData obtains metadata with a bounded, cancellable request and
// publishes the cache only after validating the response.
func FetchVersionData(ctx context.Context, targetVersion, mcDir string) (VersionData, error) {
	return fetchVersionData(ctx, manifestURL, targetVersion, mcDir)
}

func fetchVersionData(ctx context.Context, manifestURL, targetVersion, mcDir string) (VersionData, error) {
	var data VersionData
	if !filepath.IsLocal(targetVersion) || filepath.Base(targetVersion) != targetVersion || strings.ContainsAny(targetVersion, "/\\:") {
		return data, fmt.Errorf("invalid version %q", targetVersion)
	}
	body, err := readMetadata(ctx, manifestURL)
	if err != nil {
		return data, err
	}
	var manifest struct {
		Versions []manifestEntry `json:"versions"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return data, err
	}
	for _, v := range manifest.Versions {
		if v.ID != targetVersion {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(versionCachePath(mcDir, targetVersion)), 0755); err != nil {
			return data, err
		}
		stage, err := os.MkdirTemp(filepath.Dir(versionCachePath(mcDir, targetVersion)), ".mctui-version-*")
		if err != nil {
			return data, err
		}
		defer os.RemoveAll(stage)
		path := filepath.Join(stage, "version.json")
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		var wg sync.WaitGroup
		wg.Add(1)
		err = download(requestCtx, &wg, 8<<20)(v.URL, path, v.SHA1)
		cancel()
		if err != nil {
			return data, fmt.Errorf("version metadata for %s: %w", targetVersion, err)
		}
		body, err = os.ReadFile(path)
		if err != nil {
			return data, err
		}
		if err := json.Unmarshal(body, &data); err != nil {
			return data, err
		}
		if data.Downloads.Client.URL == "" || data.AssetIndex.URL == "" || data.AssetIndex.ID == "" {
			return data, fmt.Errorf("incomplete version metadata for %s", targetVersion)
		}
		if err := ctx.Err(); err != nil {
			return data, err
		}
		if err := os.Rename(path, versionCachePath(mcDir, targetVersion)); err != nil {
			return data, err
		}
		return data, nil
	}
	return data, fmt.Errorf("version %s not found in Mojang manifest", targetVersion)
}

// CachedVersionData reads a previously cached version.json (written by
// FetchVersionData), for use when offline. Returns ok=false if no cache
// exists or it can't be parsed.
func CachedVersionData(mcDir, targetVersion string) (VersionData, bool) {
	var data VersionData
	cached, err := os.ReadFile(versionCachePath(mcDir, targetVersion))
	if err != nil {
		return data, false
	}
	if json.Unmarshal(cached, &data) != nil {
		return data, false
	}
	return data, true
}

// RequiredJavaVersion reads the cached version.json for targetVersion (if
// present) and returns its javaVersion.majorVersion, or 0 if the file
// doesn't exist, can't be parsed, or doesn't specify one. Used by the UI
// to show a Java compatibility hint without needing a full
// FetchVersionData/CachedVersionData round trip.
func RequiredJavaVersion(mcDir, targetVersion string) int {
	data, ok := CachedVersionData(mcDir, targetVersion)
	if !ok {
		return 0
	}
	return data.JavaVersion.MajorVersion
}

func versionCachePath(mcDir, version string) string {
	return filepath.Join(mcDir, "versions", version, "version.json")
}

// ClientJarPath returns the path to the client jar for a given version,
// following the same layout as the official launcher.
func ClientJarPath(mcDir, version string) string {
	return filepath.Join(mcDir, "versions", version, "client.jar")
}

// ClientJarExists reports whether a non-empty client.jar exists for
// version.
func ClientJarExists(mcDir, version string) bool {
	info, err := os.Stat(ClientJarPath(mcDir, version))
	return err == nil && info.Size() > 0
}

// --- Downloads ---

// Download verifies every file against its published SHA-1. No unverifiable
// artifact is treated as installed, including a nonempty cached file.
func Download(ctx context.Context, wg *sync.WaitGroup) func(url, destination, expectedSHA1 string) error {
	return download(ctx, wg, 0)
}

func download(ctx context.Context, wg *sync.WaitGroup, maxBytes int64) func(url, destination, expectedSHA1 string) error {
	return func(url, destination, expectedSHA1 string) error {
		defer wg.Done()
		if err := ctx.Err(); err != nil {
			return err
		}
		if url == "" {
			return fmt.Errorf("missing download URL for %s", destination)
		}
		if hash, err := hex.DecodeString(expectedSHA1); err != nil || len(hash) != sha1.Size {
			return fmt.Errorf("missing or invalid SHA-1 for %s", destination)
		}
		if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			f, err := os.Open(destination)
			if err != nil {
				return err
			}
			// Mojang publishes SHA-1, not SHA-256; this checks integrity against its manifest.
			h := sha1.New() // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return err
			}
			if strings.EqualFold(fmt.Sprintf("%x", h.Sum(nil)), expectedSHA1) {
				return ctx.Err()
			}
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		r, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
		if err != nil {
			return fmt.Errorf("download %s: %w", destination, err)
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			return fmt.Errorf("download %s: HTTP %d", destination, r.StatusCode)
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "text/html") {
			return fmt.Errorf("download %s: unexpected HTML response", destination)
		}
		if maxBytes > 0 && r.ContentLength > maxBytes {
			return fmt.Errorf("download %s exceeds %d bytes", destination, maxBytes)
		}
		f, err := os.CreateTemp(filepath.Dir(destination), ".mctui-download-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		// Mojang publishes SHA-1, not SHA-256; this checks integrity against its manifest.
		h := sha1.New() // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
		reader := io.Reader(r.Body)
		if maxBytes > 0 {
			reader = io.LimitReader(r.Body, maxBytes+1)
		}
		n, err := io.Copy(io.MultiWriter(f, h), reader)
		if err == nil && maxBytes > 0 && n > maxBytes {
			err = fmt.Errorf("response exceeds %d bytes", maxBytes)
		}
		if err == nil && (n == 0 || r.ContentLength >= 0 && n != r.ContentLength) {
			err = fmt.Errorf("empty or truncated response")
		}
		if err == nil && !strings.EqualFold(expectedSHA1, fmt.Sprintf("%x", h.Sum(nil))) {
			err = fmt.Errorf("SHA-1 mismatch")
		}
		if err == nil {
			err = f.Close()
		} else {
			f.Close()
		}
		if err != nil {
			return fmt.Errorf("download %s: %w", destination, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Rename(f.Name(), destination); err != nil {
			return fmt.Errorf("publish %s: %w", destination, err)
		}
		return nil
	}
}

// osName returns the Mojang OS identifier for the current platform.
func osName() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "darwin":
		return "osx"
	case "windows":
		return "windows"
	default:
		return ""
	}
}

// ShouldIncludeLibrary applies Mojang's ordered OS rules and excludes native
// artifacts for other platforms/architectures, including from the classpath.
func ShouldIncludeLibrary(name string, rules []struct {
	Action string `json:"action"`
	OS     struct {
		Name string `json:"name"`
	} `json:"os"`
}) bool {
	if parts := strings.Split(name, ":"); len(parts) > 3 && strings.HasPrefix(parts[3], "natives-") && parts[3] != nativeClassifier(runtime.GOOS, runtime.GOARCH) {
		return false
	}
	allowed := len(rules) == 0
	for _, r := range rules {
		if r.OS.Name == "" || r.OS.Name == osName() {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}
