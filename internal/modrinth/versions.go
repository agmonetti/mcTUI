package modrinth

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

func (c *Client) Versions(ctx context.Context, id, loader, minecraft string) ([]Version, error) {
	if id == "" || loader == "" || minecraft == "" {
		return nil, fmt.Errorf("missing version filters")
	}
	q := url.Values{"loaders": {`["` + strings.ToLower(loader) + `"]`}, "game_versions": {`["` + minecraft + `"]`}}
	var versions []Version
	err := c.json(ctx, "GET", "/project/"+url.PathEscape(id)+"/version?"+q.Encode(), nil, &versions)
	return versions, err
}
func Compatible(versions []Version, loader, minecraft, channel string) (Version, error) {
	matches, err := compatibleVersions(versions, loader, minecraft, channel)
	if err != nil {
		return Version{}, err
	}
	return matches[0], nil
}

func compatibleVersions(versions []Version, loader, minecraft, channel string) ([]Version, error) {
	allowed := map[string]int{"release": 0, "beta": 1, "alpha": 2}
	max, ok := allowed[channel]
	if !ok {
		return nil, fmt.Errorf("invalid channel %q", channel)
	}
	matches := make([]Version, 0, len(versions))
	for _, v := range versions {
		rank, ok := allowed[v.Channel]
		if ok && rank <= max && slices.Contains(v.Loaders, strings.ToLower(loader)) && slices.Contains(v.GameVersions, minecraft) {
			if file, valid := v.PrimaryFile(); valid && file.Hashes.SHA512 != "" {
				matches = append(matches, v)
			}
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no compatible %s version for %s %s", channel, loader, minecraft)
	}
	slices.SortFunc(matches, func(a, b Version) int { return b.Published.Compare(a.Published) })
	return matches, nil
}
func (c *Client) CompatibleVersion(ctx context.Context, id, loader, minecraft, channel string) (Version, error) {
	versions, err := c.Versions(ctx, id, loader, minecraft)
	if err != nil {
		return Version{}, err
	}
	return Compatible(versions, loader, minecraft, channel)
}
func (c *Client) Version(ctx context.Context, id string) (Version, error) {
	var v Version
	if id == "" {
		return v, fmt.Errorf("missing version ID")
	}
	err := c.json(ctx, "GET", "/version/"+url.PathEscape(id), nil, &v)
	return v, err
}
