// Package updater checks GitHub Releases for a newer version of mcTUI
// and returns the release info so the UI can display a notification.
// It never downloads or installs anything automatically — it only
// informs the user that a newer version is available.
package updater

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

const apiURL = "https://api.github.com/repos/agmonetti/mcTUI/releases/latest"

// Release holds the subset of GitHub's release API response that mcTUI
// needs: the version tag and the URL to the releases page.
type Release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

var client = &http.Client{Timeout: 2 * time.Second}

// Check fetches the latest release from GitHub and returns it along with
// ok=true if a newer version than currentVersion is available.
// Returns ok=false silently on any network error, parse failure, or if
// the current version is already up to date.
// "dev" builds (local compilations without -ldflags) always return false
// to avoid spurious update prompts during development.
func Check(currentVersion string) (Release, bool) {
	if currentVersion == "dev" {
		return Release{}, false
	}

	resp, err := client.Get(apiURL)
	if err != nil || resp == nil {
		return Release{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return Release{}, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Release{}, false
	}

	var release Release
	if json.Unmarshal(body, &release) != nil {
		return Release{}, false
	}

	if release.TagName == "" {
		return Release{}, false
	}

	// Normalize both tags before comparing (some repos use "v1.3.0",
	// some use "1.3.0" — strip leading "v" so both formats work).
	current := strings.TrimPrefix(currentVersion, "v")
	latest := strings.TrimPrefix(release.TagName, "v")

	if current == latest {
		return Release{}, false
	}

	return release, true
}
