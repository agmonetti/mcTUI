// Package roadmap fetches the "Future Changes" panel content from a
// remote JSON file in the project's GitHub repo, so the maintainer can
// update it without requiring users to reinstall. Falls back to an
// embedded list if the remote fetch fails (offline, GitHub down, etc).
package roadmap

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const url = "https://raw.githubusercontent.com/agmonetti/mcTUI/main/roadmap.json"

// embedded is shown when the remote roadmap can't be fetched. Update this
// whenever you cut a release, so offline users still see something
// reasonably current.
var embedded = []string{
	"• Local worlds browser",
	"• Configurable memory allocation",
	"• Forge / NeoForge support (planned)",
	"• Microsoft Auth (planned)",
}

var client = &http.Client{Timeout: 2 * time.Second}

// Load fetches the current roadmap items, or returns the embedded
// fallback if the remote fetch fails for any reason (network error,
// non-200 status, invalid JSON, or an empty "changes" list).
func Load() []string {
	resp, err := client.Get(url)
	if err != nil || resp == nil {
		return embedded
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return embedded
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return embedded
	}

	var result map[string][]string
	if json.Unmarshal(body, &result) != nil {
		return embedded
	}

	if changes, ok := result["changes"]; ok && len(changes) > 0 {
		return changes
	}
	return embedded
}
