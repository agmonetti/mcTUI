// Package updater checks GitHub Releases for newer versions of mcTUI,
// verifies SHA-256 integrity, and applies updates directly from the TUI.
package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
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

// ErrRateLimited reports that the anonymous GitHub request was rate-limited
// and the optional authenticated gh fallback could not provide metadata.
var ErrRateLimited = errors.New("github rate limit")

// CheckResult carries the outcome of a release check. Err is only surfaced
// for conditions callers may want to show quietly, such as a rate limit whose
// gh fallback also failed.
type CheckResult struct {
	Release   Release
	Available bool
	Err       error
}

// ghLatestReleaseJSON fetches the release through the authenticated gh CLI.
// It is a variable so tests can supply a fixture without invoking gh.
var ghLatestReleaseJSON = runGHLatestRelease

// ghRun executes gh without a shell and returns stdout only. Stderr is
// deliberately discarded so credentials or diagnostics never reach the UI.
var ghRun = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

// ghAPIPath converts the public releases endpoint into the path expected by
// "gh api". Only the public GitHub API is accepted.
func ghAPIPath(apiURL string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" || u.Hostname() != "api.github.com" || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("gh fallback requires the public GitHub API")
	}
	path := strings.TrimPrefix(u.Path, "/")
	if !strings.HasPrefix(path, "repos/") || !strings.HasSuffix(path, "/releases/latest") {
		return "", errors.New("gh fallback cannot derive a releases endpoint")
	}
	return path, nil
}

// runGHLatestRelease asks the authenticated gh CLI for the full release JSON.
// Only the repository path is passed, and no credential is read or logged.
func runGHLatestRelease(apiURL string) ([]byte, error) {
	path, err := ghAPIPath(apiURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	body, err := ghRun(ctx, "api", path)
	if err != nil {
		return nil, fmt.Errorf("gh api: %w", err)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("gh api: empty response")
	}
	return body, nil
}

// fetchReleaseJSON queries the public API first. A 403 or 429 response is
// treated as a rate limit and the authenticated gh CLI is tried as an
// optional fallback. Any other failure is returned unchanged.
func fetchReleaseJSON(apiURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcTUI-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return io.ReadAll(resp.Body)
	}
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil, fmt.Errorf("github releases: status %d", resp.StatusCode)
	}

	body, ghErr := ghLatestReleaseJSON(apiURL)
	if ghErr == nil {
		return body, nil
	}
	return nil, fmt.Errorf("%w: status %d; gh fallback failed: %v", ErrRateLimited, resp.StatusCode, ghErr)
}

// Check fetches the latest release from GitHub and returns it along with
// ok=true if a newer version than currentVersion is available.
// Returns ok=false silently on any network error, parse failure, or if
// the current version is already up to date.
// "dev" builds always return false to avoid spurious update prompts during development.
func Check(currentVersion string) (Release, bool) {
	return CheckWithURL(defaultAPIURL, currentVersion)
}

// CheckStatus is Check with error details for the caller. Ordinary failures
// remain silent through the (Release, bool) wrapper; ErrRateLimited is exposed
// so the UI can show a quiet notice instead of claiming the app is up to date.
func CheckStatus(currentVersion string) CheckResult {
	return CheckWithURLStatus(defaultAPIURL, currentVersion)
}

// CheckWithURL allows overriding the GitHub API endpoint for testing.
func CheckWithURL(apiURL, currentVersion string) (Release, bool) {
	result := CheckWithURLStatus(apiURL, currentVersion)
	return result.Release, result.Available
}

// CheckWithURLStatus is CheckWithURL with error details.
func CheckWithURLStatus(apiURL, currentVersion string) CheckResult {
	if currentVersion == "dev" {
		return CheckResult{}
	}

	body, err := fetchReleaseJSON(apiURL)
	if err != nil {
		return CheckResult{Err: err}
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return CheckResult{Err: err}
	}

	if release.TagName == "" {
		return CheckResult{}
	}

	if !IsNewer(release.TagName, currentVersion) {
		return CheckResult{}
	}

	return CheckResult{Release: release, Available: true}
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
