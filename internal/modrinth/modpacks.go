package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// SearchModpacks does not share mod search facets: the pack's index, not a
// search category, declares the exact Minecraft and loader runtime.
func (c *Client) SearchModpacks(ctx context.Context, query, minecraft string, offset int) (SearchResponse, error) {
	var out SearchResponse
	if offset < 0 {
		return out, fmt.Errorf("invalid modpack search offset")
	}
	facets := [][]string{{"project_type:modpack"}}
	if minecraft != "" {
		facets = append(facets, []string{"versions:" + minecraft})
	}
	filter, err := json.Marshal(facets)
	if err != nil {
		return out, err
	}
	q := url.Values{"query": {query}, "facets": {string(filter)}, "limit": {"20"}, "offset": {fmt.Sprint(offset)}}
	err = c.json(ctx, "GET", "/search?"+q.Encode(), nil, &out)
	return out, err
}

func (c *Client) PackVersions(ctx context.Context, projectID string, filters ...string) ([]Version, error) {
	if projectID == "" {
		return nil, fmt.Errorf("missing pack project ID")
	}
	q := url.Values{"include_changelog": {"false"}}
	if len(filters) > 0 && filters[0] != "" {
		q.Set("loaders", `["`+strings.ToLower(filters[0])+`"]`)
	}
	if len(filters) > 1 && filters[1] != "" {
		q.Set("game_versions", `["`+filters[1]+`"]`)
	}
	var versions []Version
	err := c.json(ctx, "GET", "/project/"+url.PathEscape(projectID)+"/version?"+q.Encode(), nil, &versions)
	return versions, err
}
