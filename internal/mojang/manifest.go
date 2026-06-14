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
	"sync"
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
		ID  string `json:"id"`
		URL string `json:"url"`
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
		Downloads struct {
			Artifact struct {
				Path string `json:"path"`
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"artifact"`
		} `json:"downloads"`
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

// Download fetches url into destination. If expectedSHA1 is non-empty,
// the downloaded content is verified against it; on mismatch, the
// partial/corrupt file is removed so a future run will retry instead of
// treating it as already-installed.
//
// If destination already exists with size > 0, Download skips the fetch
// entirely — hash verification only applies to freshly downloaded
// content, to avoid re-hashing potentially large files (assets,
// client.jar) on every launch.
//
// Download calls wg.Done() when finished; callers must call wg.Add(1)
// before invoking it (matching the fabric.Downloader contract).
func Download(wg *sync.WaitGroup) func(url, destination, expectedSHA1 string) {
	return func(url, destination, expectedSHA1 string) {
		defer wg.Done()
		if url == "" {
			return
		}
		os.MkdirAll(filepath.Dir(destination), 0755)
		if info, err := os.Stat(destination); err == nil && info.Size() > 0 {
			return
		}
		r, err := http.Get(url)
		if err != nil || r == nil {
			return
		}
		defer r.Body.Close()

		f, err := os.Create(destination)
		if err != nil {
			return
		}

		if expectedSHA1 == "" {
			io.Copy(f, r.Body)
			f.Close()
			return
		}

		h := sha1.New()
		_, copyErr := io.Copy(io.MultiWriter(f, h), r.Body)
		f.Close()

		if copyErr != nil {
			os.Remove(destination)
			return
		}

		if fmt.Sprintf("%x", h.Sum(nil)) != expectedSHA1 {
			fmt.Printf("   [!] Checksum mismatch for %s, removing.\n", filepath.Base(destination))
			os.Remove(destination)
		}
	}
}
