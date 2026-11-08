// Package updater checks GitHub Releases for newer versions of mcTUI,
// verifies SHA-256 integrity, and applies updates directly from the TUI.
package updater

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const defaultAPIURL = "https://api.github.com/repos/agmonetti/mcTUI/releases/latest"

// Asset represents a release asset published on GitHub.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release holds the subset of GitHub's release API response that mcTUI needs.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

var client = &http.Client{Timeout: 5 * time.Second}

// Check fetches the latest release from GitHub and returns it along with
// ok=true if a newer version than currentVersion is available.
// Returns ok=false silently on any network error, parse failure, or if
// the current version is already up to date.
// "dev" builds always return false to avoid spurious update prompts during development.
func Check(currentVersion string) (Release, bool) {
	return CheckWithURL(defaultAPIURL, currentVersion)
}

// CheckWithURL allows overriding the GitHub API endpoint for testing.
func CheckWithURL(apiURL, currentVersion string) (Release, bool) {
	if currentVersion == "dev" {
		return Release{}, false
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return Release{}, false
	}
	req.Header.Set("User-Agent", "mcTUI-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil || resp == nil {
		return Release{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Release{}, false
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return Release{}, false
	}

	if release.TagName == "" {
		return Release{}, false
	}

	if !IsNewer(release.TagName, currentVersion) {
		return Release{}, false
	}

	return release, true
}

// IsNewer returns true if latestVersion has higher precedence than currentVersion.
func IsNewer(latestVersion, currentVersion string) bool {
	if currentVersion == "dev" || latestVersion == "dev" {
		return false
	}
	return CompareSemver(latestVersion, currentVersion) > 0
}

type semver struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

func parseSemver(v string) (semver, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return semver{}, false
	}

	var prerelease string
	if idx := strings.IndexByte(v, '-'); idx != -1 {
		prerelease = v[idx+1:]
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return semver{}, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}

	minor := 0
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return semver{}, false
		}
	}

	patch := 0
	if len(parts) > 2 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return semver{}, false
		}
	}

	return semver{
		major:      major,
		minor:      minor,
		patch:      patch,
		prerelease: prerelease,
	}, true
}

// CompareSemver compares two semantic versions.
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if v1 == v2.
func CompareSemver(v1, v2 string) int {
	if v1 == "dev" || v2 == "dev" || v1 == v2 {
		return 0
	}

	sv1, ok1 := parseSemver(v1)
	sv2, ok2 := parseSemver(v2)

	if !ok1 && !ok2 {
		return 0
	}
	if !ok1 {
		return -1
	}
	if !ok2 {
		return 1
	}

	if sv1.major != sv2.major {
		if sv1.major > sv2.major {
			return 1
		}
		return -1
	}
	if sv1.minor != sv2.minor {
		if sv1.minor > sv2.minor {
			return 1
		}
		return -1
	}
	if sv1.patch != sv2.patch {
		if sv1.patch > sv2.patch {
			return 1
		}
		return -1
	}

	// Major, minor, patch are equal.
	// A release without a pre-release is newer than a pre-release (e.g. 2.0.0 > 2.0.0-rc.1)
	if sv1.prerelease == "" && sv2.prerelease != "" {
		return 1
	}
	if sv1.prerelease != "" && sv2.prerelease == "" {
		return -1
	}
	if sv1.prerelease != "" && sv2.prerelease != "" {
		return strings.Compare(sv1.prerelease, sv2.prerelease)
	}

	return 0
}
