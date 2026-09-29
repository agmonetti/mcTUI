package modrinth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func (c *Client) Search(ctx context.Context, query, loader, minecraft string, offset int, sort ...string) (SearchResponse, error) {
	var out SearchResponse
	if offset < 0 || loader == "" || minecraft == "" {
		return out, fmt.Errorf("invalid search filters")
	}
	index := "relevance"
	if len(sort) > 0 && sort[0] != "" {
		switch sort[0] {
		case "relevance", "downloads", "follows", "newest", "updated":
			index = sort[0]
		}
	}
	facets, _ := json.Marshal([][]string{{"project_type:mod"}, {"categories:" + strings.ToLower(loader)}, {"versions:" + minecraft}})
	q := url.Values{"query": {query}, "facets": {string(facets)}, "index": {index}, "limit": {"20"}, "offset": {fmt.Sprint(offset)}}
	err := c.json(ctx, "GET", "/search?"+q.Encode(), nil, &out)
	return out, err
}

// SearchProjects searches packs using Modrinth's project type, category, and
// exact Minecraft version facets. Versions must still be checked when selecting
// a downloadable project release.
func (c *Client) SearchProjects(ctx context.Context, query, projectType, category, minecraft string, offset int, sort ...string) (SearchResponse, error) {
	var out SearchResponse
	if offset < 0 || minecraft == "" ||
		!((projectType == "resourcepack" && category == "minecraft") ||
			(projectType == "shader" && (category == "iris" || category == "optifine"))) {
		return out, fmt.Errorf("invalid search filters")
	}
	index := "relevance"
	if len(sort) > 0 && sort[0] != "" {
		switch sort[0] {
		case "relevance", "downloads", "follows", "newest", "updated":
			index = sort[0]
		}
	}
	facets, _ := json.Marshal([][]string{{"project_type:" + projectType}, {"categories:" + category}, {"versions:" + minecraft}})
	q := url.Values{"query": {query}, "facets": {string(facets)}, "index": {index}, "limit": {"20"}, "offset": {fmt.Sprint(offset)}}
	err := c.json(ctx, "GET", "/search?"+q.Encode(), nil, &out)
	return out, err
}
func (c *Client) Project(ctx context.Context, id string) (Project, error) {
	var p Project
	if id == "" {
		return p, fmt.Errorf("missing project ID")
	}
	err := c.json(ctx, "GET", "/project/"+url.PathEscape(id), nil, &p)
	return p, err
}
