// Package fabric resolves the Fabric Loader launch profile (main class +
// extra classpath entries) for a given Minecraft version, either from a
// local cache or by querying the Fabric meta API.
package fabric

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Profile holds the resolved data needed to launch Fabric: the main class
// to invoke instead of the vanilla one, and the extra classpath entries
// (Fabric loader + dependencies).
type Profile struct {
	MainClass        string
	ClasspathEntries []string
}

// Downloader fetches url into destination, optionally verifying it
// against expectedSHA1 (empty string = no verification). Implementations
// are expected to skip the fetch if destination already exists with
// size > 0, and to call wg.Done() when finished (Resolve calls wg.Add(1)
// before invoking it). See package mojang for the standard
// implementation.
type Downloader func(url, destination, expectedSHA1 string)

// ProfilePath returns where the resolved Fabric launch profile is cached
// for a given Minecraft version, so a previously launched Fabric setup
// can be detected without hitting the network again.
func ProfilePath(mcDir, version string) string {
	return filepath.Join(mcDir, "versions", version, "fabric-profile.json")
}

// ProfileExists reports whether a cached profile exists for version.
func ProfileExists(mcDir, version string) bool {
	info, err := os.Stat(ProfilePath(mcDir, version))
	return err == nil && info.Size() > 0
}

// Resolve tries to obtain the Fabric launch profile for targetVersion. It
// first checks for a cached profile on disk (written by a previous
// successful resolution); if absent, it queries the Fabric meta API. All
// network errors are handled gracefully — on any failure this returns
// ok=false so the caller can fall back to Vanilla instead of panicking or
// aborting the whole launch.
//
// Library downloads triggered here are capped via a semaphore (maxConcurrent),
// same as vanilla libraries and assets in package mojang.
func Resolve(targetVersion, mcDir, librariesPath string, wg *sync.WaitGroup, download Downloader, maxConcurrent int) (Profile, bool) {
	cachePath := ProfilePath(mcDir, targetVersion)

	// Try cached profile first (enables fully offline Fabric launches).
	if cached, err := os.ReadFile(cachePath); err == nil {
		var p Profile
		if json.Unmarshal(cached, &p) == nil && p.MainClass != "" {
			fmt.Println("   Using cached Fabric profile.")
			return p, true
		}
	}

	loaderURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", targetVersion)
	loaderResp, err := http.Get(loaderURL)
	if err != nil || loaderResp == nil {
		fmt.Println("[!] Could not reach Fabric meta servers (no internet?).")
		return Profile{}, false
	}
	loaderBody, _ := io.ReadAll(loaderResp.Body)
	loaderResp.Body.Close()

	var loaderData []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if jsonErr := json.Unmarshal(loaderBody, &loaderData); jsonErr != nil || len(loaderData) == 0 {
		fmt.Println("[!] Fabric does not have a compatible loader for this version.")
		return Profile{}, false
	}

	latestLoader := loaderData[0].Loader.Version
	fmt.Printf("-> Downloading Fabric bridge libraries v%s...\n", latestLoader)

	profileURL := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/profile/json", targetVersion, latestLoader)
	profileResp, pErr := http.Get(profileURL)
	if pErr != nil || profileResp == nil {
		fmt.Println("[!] Could not fetch Fabric profile (no internet?).")
		return Profile{}, false
	}
	profileBody, _ := io.ReadAll(profileResp.Body)
	profileResp.Body.Close()

	var rawProfile struct {
		MainClass string `json:"mainClass"`
		Libraries []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"libraries"`
	}
	if jsonErr := json.Unmarshal(profileBody, &rawProfile); jsonErr != nil || rawProfile.MainClass == "" {
		fmt.Println("[!] Fabric profile response was invalid.")
		return Profile{}, false
	}

	result := Profile{MainClass: rawProfile.MainClass}

	fabricSem := make(chan struct{}, maxConcurrent)
	for _, lib := range rawProfile.Libraries {
		parts := strings.Split(lib.Name, ":")
		if len(parts) < 3 {
			continue
		}
		group := strings.ReplaceAll(parts[0], ".", "/")
		artifact := parts[1]
		version := parts[2]
		path := fmt.Sprintf("%s/%s/%s/%s-%s.jar", group, artifact, version, artifact, version)

		baseURL := lib.URL
		if baseURL == "" {
			baseURL = "https://maven.fabricmc.net/"
		}
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}

		fullURL := baseURL + path
		fullDest := filepath.Join(librariesPath, path)

		result.ClasspathEntries = append(result.ClasspathEntries, fullDest)
		wg.Add(1)
		fabricSem <- struct{}{}
		go func(u, d string) {
			defer func() { <-fabricSem }()
			download(u, d, "")
		}(fullURL, fullDest)
	}

	// Cache the resolved profile so future launches can work offline.
	if data, mErr := json.MarshalIndent(result, "", "  "); mErr == nil {
		os.MkdirAll(filepath.Dir(cachePath), 0755)
		os.WriteFile(cachePath, data, 0644)
	}

	return result, true
}
