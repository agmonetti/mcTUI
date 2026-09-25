// Package mojang handles communication with Mojang's official metadata
// and download servers: fetching the version manifest, per-version
// metadata (libraries, asset index, Java requirement, client download
// URL), and downloading files with optional SHA1 verification.
package mojang

import (
	"crypto/sha1"
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

// manifestEntry is one entry in the version manifest: an ID and a URL to
// fetch that version's full metadata.
type manifestEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
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
	resp, err := http.Get(manifestURL)
	if err != nil || resp == nil {
		return []string{"1.20.4"}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

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

// FetchVersionData fetches and caches the per-version metadata for
// targetVersion. On success, the raw JSON is also written to
// <mcDir>/versions/<targetVersion>/version.json for later offline use by
// CachedVersionData and RequiredJavaVersion.
//
// Returns ok=false if the manifest or version JSON can't be fetched or
// parsed (e.g. no internet) — callers should fall back to
// CachedVersionData in that case.
func FetchVersionData(targetVersion, mcDir string) (VersionData, bool) {
	var data VersionData

	resp, err := http.Get(manifestURL)
	if err != nil || resp == nil {
		fmt.Println("[!] Could not reach Mojang servers (no internet?).")
		return data, false
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return data, false
	}

	var manifest struct {
		Versions []manifestEntry `json:"versions"`
	}
	if json.Unmarshal(body, &manifest) != nil {
		return data, false
	}

	var specificURL string
	for _, v := range manifest.Versions {
		if v.ID == targetVersion {
			specificURL = v.URL
			break
		}
	}
	if specificURL == "" {
		fmt.Println("[!] Version", targetVersion, "not found in Mojang manifest.")
		return data, false
	}

	respVersion, vErr := http.Get(specificURL)
	if vErr != nil || respVersion == nil {
		fmt.Println("[!] Could not fetch version metadata (no internet?).")
		return data, false
	}
	bodyVersion, _ := io.ReadAll(respVersion.Body)
	respVersion.Body.Close()

	if json.Unmarshal(bodyVersion, &data) != nil {
		return data, false
	}

	// Cache the raw version data locally so a future offline launch can
	// still resolve the asset index ID and library list if needed.
	cachePath := versionCachePath(mcDir, targetVersion)
	os.MkdirAll(filepath.Dir(cachePath), 0755)
	os.WriteFile(cachePath, bodyVersion, 0644)

	return data, true
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

// Download verifies cached files with a known SHA-1 and publishes downloads
// only after a complete, successful transfer. Callers must Add before calling;
// Done is called even on failure.
func Download(wg *sync.WaitGroup) func(url, destination, expectedSHA1 string) error {
	return func(url, destination, expectedSHA1 string) error {
		defer wg.Done()
		if url == "" {
			return fmt.Errorf("missing download URL for %s", destination)
		}
		if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			if expectedSHA1 == "" {
				return nil
			}
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
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		r, err := (&http.Client{Timeout: 2 * time.Minute}).Get(url)
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
		f, err := os.CreateTemp(filepath.Dir(destination), ".mctui-download-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		// Mojang publishes SHA-1, not SHA-256; this checks integrity against its manifest.
		h := sha1.New() // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
		n, err := io.Copy(io.MultiWriter(f, h), r.Body)
		if err == nil && (n == 0 || r.ContentLength >= 0 && n != r.ContentLength) {
			err = fmt.Errorf("empty or truncated response")
		}
		if err == nil && expectedSHA1 != "" && !strings.EqualFold(expectedSHA1, fmt.Sprintf("%x", h.Sum(nil))) {
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
